#!/usr/bin/env bash
# setup-swa.sh — Provision SWA (Secure Workload Access) resources and configure Conjur
# Cloud authenticators for the SWA E2E test harness.
#
# What this script does:
#   1. Generates PKI: RSA-2048 JWT key pair (for SWA Server auth), EC P-256 x509pop
#      CA and agent certificate (for SWA Server host attestation).
#   2. Builds JWKS from the RSA-2048 public key using Python stdlib (no third-party libs).
#   3. Provisions SWA resources via the Conjur Cloud management API:
#      trust domain, server group (x509pop), server component (JWT auth), node group.
#   4. Generates the initial SWA Server JWT.
#   5. Writes bootstrapConfig.yaml (SWA Server) and agent/config.yaml (SWA Agent).
#   6. Loads the authn-jwt/swa-e2e-jwt policy into Conjur (via CLI), sets all required
#      variables, and enables the authenticator.
#   7. Creates and seeds the test secret at data/swa-e2e/test-secret.
#   8. Registers the current user's workload SPIFFE host in Conjur and grants it access.
#
# Usage:
#   bin/swa/setup-swa.sh
#
# Required environment:
#   CONJUR_APPLIANCE_URL   Bare Conjur Cloud tenant URL (no trailing slash, no /api)
#   CONJUR_ACCOUNT         Conjur account name (default: conjur)
#
# Optional environment:
#   CONJURRC_FILE          Path to conjurrc for the tenant (default: ~/.conjurrc-swa)
#   NETRC_FILE             Path to netrc holding the OIDC session token (default: ~/.netrc-swa)
#   SWA_WORKDIR            Local working directory for PKI, configs, logs (default: /tmp/swa-e2e)
#   SWA_BIN_DIR            Directory containing swa-server and swa-agent binaries (default: /tmp/swa-bins)
#   TRUST_DOMAIN           SPIFFE trust domain name (default: swa-e2e.conjur.cloud)
#   SERVER_GROUP           SWA server group name (default: swa-e2e-server-group)
#   SERVER_NAME            SWA server component name (default: swa-e2e-server)
#   NODE_GROUP             SWA node group name — must match the agent cert CN (default: swa-e2e-server-agent)
#   JWT_SERVICE_ID         authn-jwt service ID (default: swa-e2e-jwt)
#   TEST_SECRET_VALUE      Value written to the test Conjur variable (default: swa-e2e-secret-value)
#   WORKLOAD_USER          Unix username of the test runner (default: current $USER)
#
# Prerequisite:
#   Log in first:  CONJURRC=~/.conjurrc-swa conjur login
#
# IMPORTANT — controlPlane.url gotcha:
#   bootstrapConfig.yaml must contain the bare tenant URL with NO /api suffix.
#   The SWA Server prepends /api internally. Supplying /api causes 404 on every
#   auth call. This script strips it automatically.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/setup-lib.sh"

# ── Helpers ────────────────────────────────────────────────────────────────


main() {
  load_config
  resolve_api_base
  load_auth_token
  prepare_workdir
  step_1_generate_pki
  step_2_build_jwks
  step_3_provision_trust_domain
  step_4_provision_server_group
  step_5_provision_server_component
  step_6_provision_node_group
  step_7_generate_jwt
  step_8_write_bootstrap_config
  step_9_write_agent_config
  configure_authn
  step_11_create_test_secret
  step_12_register_workload_host
  step_13_update_cert_ca
  print_summary
}

announce() { echo ""; echo "=== ${*} ==="; echo ""; }

# HTTP helper — writes body to /tmp/swa_http_out.json, returns status code.
http_call() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-sk -o /tmp/swa_http_out.json -w "%{http_code}"
    -X "$method"
    --config "${_AUTH_CONF}"
    -H "Accept: application/x.secretsmgr.v2+json"
    -H "Content-Type: application/json")
  [[ -n "$body" ]] && args+=(--data-binary "$body")
  curl "${args[@]}" "${API_BASE}${path}"
}

# Policy load helper — uses the REST API directly to avoid CLI OIDC token expiry issues.
# Usage: load_policy <branch> <policy_file>
load_policy() {
  local branch="$1" file="$2"
  local encoded_branch http
  encoded_branch=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$branch")
  http=$(curl -sk -o /tmp/swa_policy_out.json -w "%{http_code}" \
    -X PATCH \
    --config "${_AUTH_CONF}" \
    -H "Content-Type: application/x-yaml" \
    --data-binary "@${file}" \
    "${API_BASE}/policies/${CONJUR_ACCOUNT}/policy/${encoded_branch}")
  if [[ "$http" == "200" || "$http" == "201" || "$http" == "204" ]]; then
    echo "  Policy loaded (HTTP ${http})."
  else
    echo "  ERROR: policy load into '${branch}' returned HTTP ${http}" >&2
    cat /tmp/swa_policy_out.json >&2
    exit 1
  fi
}

# Set a Conjur variable value via the REST API.
set_variable() {
  local var_id="$1" value="$2"
  local encoded_id http
  encoded_id=$(urlencode "$var_id")
  http=$(printf '%s' "$value" | curl -sk -o /dev/null -w "%{http_code}" \
    -X POST \
    --config "${_AUTH_CONF}" \
    -H "Content-Type: text/plain" \
    --data-binary @- \
    "${API_BASE}/secrets/${CONJUR_ACCOUNT}/variable/${encoded_id}")
  [[ "$http" == "201" || "$http" == "204" ]] || echo "  WARNING: set ${var_id} returned HTTP ${http}"
  echo "  Set ${var_id} (${http})."
}

# ── Configuration ──────────────────────────────────────────────────────────

load_config() {
  CONJURRC_FILE="${CONJURRC_FILE:-${HOME}/.conjurrc-swa}"
  NETRC_FILE="${NETRC_FILE:-${HOME}/.netrc-swa}"
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  SWA_BIN_DIR="${SWA_BIN_DIR:-/tmp/swa-bins}"
  TRUST_DOMAIN="${TRUST_DOMAIN:-swa-e2e.conjur.cloud}"
  SERVER_GROUP="${SERVER_GROUP:-swa-e2e-server-group}"
  SERVER_NAME="${SERVER_NAME:-swa-e2e-server}"
  # NODE_GROUP is also the x509pop agent cert CN. The SWA Server uses the agent's cert
  # CN as the node group identifier when minting workload JWT SVIDs. Both must match.
  NODE_GROUP="${NODE_GROUP:-swa-e2e-server-agent}"
  JWT_SERVICE_ID="${JWT_SERVICE_ID:-swa-e2e-jwt}"
  TEST_SECRET_VALUE="${TEST_SECRET_VALUE:-swa-e2e-secret-value}"
  CONJUR_ACCOUNT="${CONJUR_ACCOUNT:-conjur}"

  # JWT signing fields used by the SWA Server to authenticate to Conjur.
  SERVER_JWT_SUB="${SERVER_JWT_SUB:-${SERVER_NAME}}"
  SERVER_JWT_ISS="${SERVER_JWT_ISS:-https://swa-e2e.local}"
  SERVER_JWT_AUD="${SERVER_JWT_AUD:-conjur}"
}

# ── Resolve Conjur tenant URL ──────────────────────────────────────────────

resolve_api_base() {
  if [[ -n "${CONJUR_APPLIANCE_URL:-}" ]]; then
    API_BASE="${CONJUR_APPLIANCE_URL%/}"
    # Normalize: Conjur REST calls require /api; append it when the caller supplied the bare URL.
    [[ "${API_BASE}" != */api ]] && API_BASE="${API_BASE}/api"
  elif [[ -f "$CONJURRC_FILE" ]]; then
    API_BASE=$(appliance_url_from_conjurrc "$CONJURRC_FILE")
    [[ -z "$API_BASE" ]] && { echo "ERROR: appliance_url not found in ${CONJURRC_FILE}" >&2; exit 1; }
  else
    echo "ERROR: Set CONJUR_APPLIANCE_URL or provide ${CONJURRC_FILE}." >&2
    exit 1
  fi

  echo "Tenant URL: ${API_BASE}"
}

# ── Load OIDC session token ────────────────────────────────────────────────

load_auth_token() {
  # CI path: accept a Conjur session token injected by getConjurCloudTenant.tokens().
  # The conjToken returned by the pipeline library is already a base64-encoded Conjur
  # access token (the raw response from the /authenticate endpoint), ready to embed in
  # Authorization: Token token="<value>" without additional encoding.
  local _ci_token="${CONJUR_AUTHN_TOKEN:-}"
  if [[ -n "${_ci_token}" ]]; then
    # Reject whitespace-only values — they produce a syntactically valid but
    # semantically empty Authorization header, causing confusing 401 errors.
    if [[ -z "${_ci_token//[[:space:]]}" ]]; then
      echo "ERROR: CONJUR_AUTHN_TOKEN is set but contains only whitespace." >&2
      exit 1
    fi
    CONJUR_TOKEN="${_ci_token}"
  else
    CONJUR_TOKEN=$(oidc_token_b64 "$NETRC_FILE") || {
      echo "ERROR: no OIDC token found in ${NETRC_FILE}." >&2
      echo "  Set CONJUR_AUTHN_TOKEN (CI) or run: CONJURRC=${CONJURRC_FILE} conjur login" >&2
      exit 1
    }
  fi

  # Write the Authorization header to a temp config file so the token never appears
  # on the command line (visible in ps aux for the duration of each HTTP call).
  _AUTH_CONF=$(mktemp)
  printf 'header = "Authorization: Token token=\\"%s\\""\n' "${CONJUR_TOKEN}" > "${_AUTH_CONF}"
  trap 'rm -f "${_AUTH_CONF}"' EXIT
}

# ── Prepare working directory ──────────────────────────────────────────────

prepare_workdir() {
  mkdir -p "${SWA_WORKDIR}/agent"
}

# ── Step 1: Generate PKI ───────────────────────────────────────────────────

step_1_generate_pki() {
  announce "Step 1: Generating PKI"

  JWT_KEY="${SWA_WORKDIR}/server-jwt.key"
  JWT_PUB="${SWA_WORKDIR}/server-jwt.pub"
  X509_CA_KEY="${SWA_WORKDIR}/x509pop-ca.key"
  X509_CA_CERT="${SWA_WORKDIR}/x509pop-ca.crt"
  X509_AGENT_KEY="${SWA_WORKDIR}/x509pop-agent.key"
  X509_AGENT_CERT="${SWA_WORKDIR}/x509pop-agent.crt"

  if [[ ! -f "$JWT_KEY" ]]; then
    openssl genrsa -out "$JWT_KEY" 2048 2>/dev/null
    openssl rsa -in "$JWT_KEY" -pubout -out "$JWT_PUB" 2>/dev/null
    echo "  RSA-2048 JWT key pair generated."
  else
    echo "  RSA-2048 JWT key pair exists — reusing."
  fi

  if [[ ! -f "$X509_CA_KEY" ]]; then
    openssl ecparam -name prime256v1 -genkey -noout -out "$X509_CA_KEY" 2>/dev/null
    openssl req -new -x509 -key "$X509_CA_KEY" \
      -out "$X509_CA_CERT" -days 365 \
      -subj "/CN=swa-e2e-x509pop-ca" 2>/dev/null
    echo "  EC P-256 x509pop CA generated."
  else
    echo "  EC P-256 x509pop CA exists — reusing."
  fi

  if [[ ! -f "$X509_AGENT_KEY" ]]; then
    openssl ecparam -name prime256v1 -genkey -noout -out "$X509_AGENT_KEY" 2>/dev/null
    CSR=$(mktemp)
    # The CN MUST match NODE_GROUP — the SWA Server uses the agent cert CN as the
    # node group identifier when routing workload JWT SVID requests.
    openssl req -new -key "$X509_AGENT_KEY" -out "$CSR" \
      -subj "/CN=${NODE_GROUP}" 2>/dev/null
    # The x509pop attestor requires digitalSignature key usage for challenge signing.
    EXT=$(mktemp)
    cat > "$EXT" <<'EXTEOF'
[req_ext]
keyUsage = critical, digitalSignature
extendedKeyUsage = clientAuth
EXTEOF
    openssl x509 -req -in "$CSR" -CA "$X509_CA_CERT" -CAkey "$X509_CA_KEY" \
      -CAcreateserial -out "$X509_AGENT_CERT" -days 365 -sha256 \
      -extfile "$EXT" -extensions req_ext 2>/dev/null
    rm -f "$CSR" "$EXT"
    echo "  EC P-256 agent cert generated (with digitalSignature usage)."
  else
    echo "  EC P-256 agent cert exists — reusing."
  fi
}

# ── Step 2: Build JWKS (Python stdlib only — no third-party libs) ──────────

step_2_build_jwks() {
  announce "Step 2: Building JWKS from RSA-2048 public key"

  RSA_JWKS=$(python3 "${SCRIPT_DIR}/build-jwks.py" "${JWT_PUB}") \
    || { echo "ERROR: Failed to build JWKS from ${JWT_PUB}." >&2; exit 1; }

  echo "  JWKS built (key ID: swa-e2e-key-1)"
}

# ── Step 3: Provision SWA trust domain ────────────────────────────────────

step_3_provision_trust_domain() {
  announce "Step 3: Provisioning SWA trust domain '${TRUST_DOMAIN}'"

  TD_CODE=$(http_call POST "/swa/trust-domains" \
    "{\"name\":\"${TRUST_DOMAIN}\",\"jwt\":{\"signing_key_type\":\"RSA_4096\",\"signature_algorithm\":\"RS512\"},\"x509\":{}}")

  case "$TD_CODE" in
    201) echo "  Trust domain created (201)." ;;
    409)
      echo "  Already exists (409) — patching to RSA signing..."
      PATCH_CODE=$(http_call PATCH "/swa/trust-domains/${TRUST_DOMAIN}" \
        '{"jwt":{"signing_key_type":"RSA_4096","signature_algorithm":"RS512"}}')
      echo "  PATCH returned HTTP ${PATCH_CODE}."
      ;;
    *)
      echo "ERROR: POST /swa/trust-domains returned HTTP ${TD_CODE}" >&2
      cat /tmp/swa_http_out.json >&2; exit 1 ;;
  esac
}

# ── Step 4: Provision SWA server group ────────────────────────────────────

step_4_provision_server_group() {
  announce "Step 4: Provisioning SWA server group '${SERVER_GROUP}'"

  SG_BODY=$(jq -n \
    --arg name "${SERVER_GROUP}" \
    --rawfile pem "${X509_CA_CERT}" \
    '{"name":$name,"description":"SWA E2E test harness server group","attestation":{"x509pop":{"ca_certificates":$pem}}}')
  SG_CODE=$(http_call POST "/swa/trust-domains/${TRUST_DOMAIN}/server-groups" "$SG_BODY")
  case "$SG_CODE" in
    201) echo "  Server group created (201)." ;;
    409)
      echo "  Already exists (409) — patching CA cert with current key material..."
      PATCH_SG_BODY=$(jq -n --rawfile pem "${X509_CA_CERT}" \
        '{"attestation":{"x509pop":{"ca_certificates":$pem}}}')
      PATCH_SG_CODE=$(http_call PATCH "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}" "$PATCH_SG_BODY")
      echo "  PATCH returned HTTP ${PATCH_SG_CODE}."
      ;;
    *)
      echo "ERROR: POST /swa/trust-domains/${TRUST_DOMAIN}/server-groups returned HTTP ${SG_CODE}" >&2
      cat /tmp/swa_http_out.json >&2; exit 1 ;;
  esac
}

# ── Step 5: Provision SWA server component ────────────────────────────────

step_5_provision_server_component() {
  announce "Step 5: Provisioning SWA server component '${SERVER_NAME}'"

  SRV_BODY=$(jq -n \
    --arg name "${SERVER_NAME}" \
    --arg sub "${SERVER_JWT_SUB}" \
    --arg iss "${SERVER_JWT_ISS}" \
    --arg aud "${SERVER_JWT_AUD}" \
    --argjson jwks "${RSA_JWKS}" \
    '{"name":$name,"authentication":{"type":"JWT","data":{"sub":$sub,"issuer":$iss,"audience":$aud,"public_keys":{"type":"jwks","value":$jwks}}}}')

  SRV_CODE=$(http_call POST \
    "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/components" \
    "$SRV_BODY")

  case "$SRV_CODE" in
    201)
      echo "  Server component created (201)."
      AUTHN_ID_B64=$(jq -r '.authn_id // empty' /tmp/swa_http_out.json 2>/dev/null || echo "")
      AUTHN_URL=$(printf '%s==' "${AUTHN_ID_B64}" | base64 -d 2>/dev/null | tr -d '\r\n' || echo "")
      [[ -n "$AUTHN_URL" ]] && echo "  authn_url: ${AUTHN_URL}"
      ;;
    409)
      # Conjur does not support resource deletion, so delete+reprovision burns a UUID each time.
      # Only reprovision when the registered key differs from the current local key.
      http_call GET \
        "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/components/${SERVER_NAME}"
      REGISTERED_N=$(jq -r '.authentication.data.public_keys.value.keys[0].n // empty' \
        /tmp/swa_http_out.json 2>/dev/null || echo "")
      LOCAL_N=$(python3 -c "import json; d=json.loads('${RSA_JWKS}'); print(d['keys'][0]['n'])" 2>/dev/null || echo "")

      if [[ -n "$REGISTERED_N" && "$REGISTERED_N" == "$LOCAL_N" ]]; then
        echo "  Already exists with matching key — reusing existing component."
        AUTHN_ID_B64=$(jq -r '.authn_id // empty' /tmp/swa_http_out.json 2>/dev/null || echo "")
        AUTHN_URL=$(printf '%s==' "${AUTHN_ID_B64}" | base64 -d 2>/dev/null | tr -d '\r\n' || echo "")
        echo "  authn_url: ${AUTHN_URL}"
      else
        echo "  Key mismatch — deleting and re-provisioning with current key material..."
        [[ -n "$REGISTERED_N" ]] && echo "  registered n[:20]: ${REGISTERED_N:0:20}  local n[:20]: ${LOCAL_N:0:20}"
        DEL_CODE=$(http_call DELETE \
          "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/components/${SERVER_NAME}")
        echo "  DELETE returned HTTP ${DEL_CODE}."
        # Retry POST with exponential backoff to handle eventual-consistency delay.
        SRV_CODE="409"
        for _wait in 3 6 12; do
          echo "  Waiting ${_wait}s for delete to propagate..."
          sleep "${_wait}"
          SRV_CODE=$(http_call POST \
            "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/components" \
            "$SRV_BODY")
          [[ "$SRV_CODE" != "409" ]] && break
        done
        echo "  Re-provision returned HTTP ${SRV_CODE}."
        if [[ "$SRV_CODE" == "201" ]]; then
          AUTHN_ID_B64=$(jq -r '.authn_id // empty' /tmp/swa_http_out.json 2>/dev/null || echo "")
          AUTHN_URL=$(printf '%s==' "${AUTHN_ID_B64}" | base64 -d 2>/dev/null | tr -d '\r\n' || echo "")
          echo "  authn_url: ${AUTHN_URL}"
        else
          echo "ERROR: re-provision returned HTTP ${SRV_CODE} after retries." >&2
          cat /tmp/swa_http_out.json >&2; exit 1
        fi
      fi
      ;;
    *)
      echo "ERROR: POST components returned HTTP ${SRV_CODE}" >&2
      cat /tmp/swa_http_out.json >&2; exit 1 ;;
  esac

  # Write provision-server.json so refresh-swa-jwt.sh and config generation can recover key fields.
  jq -n \
    --arg authn_id "${AUTHN_ID_B64}" \
    --arg sub "${SERVER_JWT_SUB}" \
    --arg iss "${SERVER_JWT_ISS}" \
    --arg aud "${SERVER_JWT_AUD}" \
    '{authn_id:$authn_id,authentication:{data:{sub:$sub,issuer:$iss,audience:$aud}}}' \
    > "${SWA_WORKDIR}/provision-server.json"
  echo "  Written: ${SWA_WORKDIR}/provision-server.json"
}

# ── Step 6: Provision SWA node group ──────────────────────────────────────

step_6_provision_node_group() {
  announce "Step 6: Provisioning SWA node group '${NODE_GROUP}'"

  # workload_registration_policies is a list of CEL expressions evaluated against
  # workload attributes. "true" means all workloads on this agent may receive SVIDs.
  NG_CODE=$(http_call POST \
    "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/node-groups" \
    "{\"name\":\"${NODE_GROUP}\",\"workload_type\":\"unix\",\"description\":\"SWA E2E unix workloads\"}")
  case "$NG_CODE" in
    201) echo "  Node group created (201)." ;;
    409) echo "  Already exists (409) — continuing." ;;
    *)
      echo "ERROR: POST node-groups returned HTTP ${NG_CODE}" >&2
      cat /tmp/swa_http_out.json >&2; exit 1 ;;
  esac

  # Enable JWT SVID generation for this node group via a PATCH.
  # workload_registration_policies must be CEL expressions (string array).
  # "true" permits all workloads; more specific expressions can restrict access.
  NG_PATCH_CODE=$(http_call PATCH \
    "/swa/trust-domains/${TRUST_DOMAIN}/server-groups/${SERVER_GROUP}/node-groups/${NODE_GROUP}" \
    '{"workload_configuration":{"workload_registration_policies":["true"]}}')
  case "$NG_PATCH_CODE" in
    200) echo "  Node group JWT policy set (200)." ;;
    *)
      echo "  WARNING: PATCH node-group returned HTTP ${NG_PATCH_CODE}; JWT SVIDs may not work." >&2 ;;
  esac
}

# ── Step 7: Generate initial SWA Server JWT ────────────────────────────────

step_7_generate_jwt() {
  announce "Step 7: Generating initial SWA Server JWT"
  "$(dirname "$0")/refresh-swa-jwt.sh"
}

# ── Step 8: Write SWA Server bootstrapConfig.yaml ─────────────────────────

step_8_write_bootstrap_config() {
  announce "Step 8: Writing SWA Server bootstrapConfig.yaml"

  # BARE_URL: strip /api suffix — the SWA Server adds it internally.
  BARE_URL="${API_BASE%/api}"

  cat > "${SWA_WORKDIR}/bootstrapConfig.yaml" <<EOF
# SWA Server bootstrap configuration — generated by bin/swa/setup-swa.sh.
# IMPORTANT: controlPlane.url must NOT have a /api suffix.
trustDomain:
  name: "${TRUST_DOMAIN}"
controlPlane:
  url: "${BARE_URL}"
  syncInterval: 5m
  useVCP: false
  auth:
    type: jwt
    authnID: ${AUTHN_ID_B64}
    tokenPath: "${SWA_WORKDIR}/server.jwt"
server:
  apiAddr: "0.0.0.0:8081"
  webAddr: "127.0.0.1:8080"
logging:
  level: debug
  format: text
  source: false
telemetry:
  logging:
    level: info
    format: text
    source: false
EOF
  echo "  Written: ${SWA_WORKDIR}/bootstrapConfig.yaml"
}

# ── Step 9: Write SWA Agent config.yaml ───────────────────────────────────

step_9_write_agent_config() {
  announce "Step 9: Writing SWA Agent config.yaml"

  cat > "${SWA_WORKDIR}/agent/config.yaml" <<EOF
# SWA Agent configuration — generated by bin/swa/setup-swa.sh.
trustDomain:
  name: "${TRUST_DOMAIN}"
bootstrap:
  insecure: true
servers:
  - addr: "127.0.0.1:8081"
agent:
  socketPath: "/tmp/swa-e2e-agent.sock"
  key:
    type: ECP256
    ttl: 1h
  nodeAttestor:
    type: x509pop
    config:
      key: "../x509pop-agent.key"
      cert: "../x509pop-agent.crt"
workload:
  key:
    type: ECP256
    ttl: 1h
logging:
  level: debug
  format: text
telemetry:
  logging:
    level: info
EOF
  echo "  Written: ${SWA_WORKDIR}/agent/config.yaml"
}

# ── Step 10: Configure Conjur authenticators ──────────────────────────────

configure_authn() {
  configure_authn_jwt
  configure_authn_cert
}

configure_authn_jwt() {
  announce "Step 10: Configuring Conjur authn-jwt/${JWT_SERVICE_ID}"

  # The JWKS is served by the SWA trust domain endpoint (public, no auth required).
  # Conjur Cloud can call itself since the URL is on the same tenant.
  JWKS_URI="${API_BASE}/swa/trust-domains/${TRUST_DOMAIN}/.well-known/jwks"
  ISSUER="${API_BASE}/swa/trust-domains/${TRUST_DOMAIN}"

  cat > /tmp/swa-authn-jwt-policy.yml <<YAML
- !policy
  id: ${JWT_SERVICE_ID}
  body:
  - !webservice

  - !group consumers

  - !permit
    role: !group consumers
    privilege: [ read, authenticate ]
    resource: !webservice

  - !variable jwks-uri
  - !variable issuer
  - !variable token-app-property
  - !variable identity-path
  - !variable token-ttl
YAML

  echo "  Loading policy into conjur/authn-jwt..."
  load_policy "conjur/authn-jwt" /tmp/swa-authn-jwt-policy.yml

  echo ""
  echo "  Setting variables..."

  set_variable "conjur/authn-jwt/${JWT_SERVICE_ID}/jwks-uri"            "$JWKS_URI"
  set_variable "conjur/authn-jwt/${JWT_SERVICE_ID}/issuer"              "$ISSUER"
  set_variable "conjur/authn-jwt/${JWT_SERVICE_ID}/token-app-property"  "sub"
  set_variable "conjur/authn-jwt/${JWT_SERVICE_ID}/identity-path"       "data/swa-e2e"
  set_variable "conjur/authn-jwt/${JWT_SERVICE_ID}/token-ttl"           "PT10M"

  echo ""
  echo "  Enabling authenticator..."
  ENABLE_CODE=$(http_call PATCH "/authenticators/jwt/${JWT_SERVICE_ID}" \
    '{"enabled":true}')
  if [[ "$ENABLE_CODE" == "200" || "$ENABLE_CODE" == "204" ]]; then
    echo "  Authenticator enabled (${ENABLE_CODE})."
  else
    echo "  WARNING: enable returned HTTP ${ENABLE_CODE}"
    cat /tmp/swa_http_out.json
  fi
}

# ── Step 10b: Configure authn-cert for SPIFFE X.509-SVID authentication ───

configure_authn_cert() {
  CERT_SERVICE_ID="${CERT_SERVICE_ID:-swa-e2e-cert}"

  announce "Step 10b: Configuring Conjur authn-cert/${CERT_SERVICE_ID}"

  # Create the authn-cert policy: webservice, consumers group, permit, ca-cert
  # variable. This is required before step 13 can set the CA cert value and
  # before the Edge can validate SVIDs.
  cat > /tmp/swa-authn-cert-policy.yml <<YAML
- !policy
  id: ${CERT_SERVICE_ID}
  body:
  - !webservice

  - !group consumers

  - !permit
    role: !group consumers
    privilege: [ read, authenticate ]
    resource: !webservice

  - !variable ca-cert
YAML

  echo "  Loading policy into conjur/authn-cert..."
  load_policy "conjur/authn-cert" /tmp/swa-authn-cert-policy.yml

  echo ""
  echo "  Enabling authenticator..."
  # Conjur Cloud uses the enterprise-compatible URL format for authn-cert:
  #   PATCH /{authn-cert}/{service_id}/{account}  (form body: enabled=true)
  # The /authenticators/{type}/{service_id} path used by authn-jwt does not
  # support authn-cert on Conjur Cloud (returns 404 for all type variants).
  _URL="${API_BASE}/authn-cert/${CERT_SERVICE_ID}/${CONJUR_ACCOUNT}"
  _ENABLE_CODE=$(curl -sk -o /tmp/swa_http_out.json -w "%{http_code}" \
    -X PATCH \
    --config "${_AUTH_CONF}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data "enabled=true" \
    "${_URL}")
  if [[ "$_ENABLE_CODE" == "200" || "$_ENABLE_CODE" == "204" || "$_ENABLE_CODE" == "201" ]]; then
    echo "  Authenticator enabled (HTTP ${_ENABLE_CODE})."
  else
    echo "  WARNING: authn-cert enable returned HTTP ${_ENABLE_CODE}." >&2
    cat /tmp/swa_http_out.json >&2
  fi
}

# ── Step 11: Create test secret ────────────────────────────────────────────

step_11_create_test_secret() {
  announce "Step 11: Creating test secret data/swa-e2e/test-secret"

  TEST_VAR_POLICY=$(cat <<YAML
- !policy
  id: swa-e2e
  body:
  - !variable test-secret
YAML
)

  echo "${TEST_VAR_POLICY}" > /tmp/swa-test-var-policy.yml
  echo "  Loading policy into data branch..."
  # Conjur Cloud: user-defined resources live under the 'data' branch, not 'root'.
  load_policy "data" /tmp/swa-test-var-policy.yml

  set_variable "data/swa-e2e/test-secret" "${TEST_SECRET_VALUE}"

  echo "${TEST_SECRET_VALUE}" > "${SWA_WORKDIR}/test-secret-value.txt"
  echo "  Expected value written to ${SWA_WORKDIR}/test-secret-value.txt"
}

# ── Step 12: Register workload Conjur host ─────────────────────────────────

step_12_register_workload_host() {
  announce "Step 12: Registering workload Conjur host for current user"

  # The SWA Agent assigns each workload a SPIFFE ID using this template:
  #   spiffe://{{ .trustdomain }}/{{ .nodegroup }}/workload/{{ .unix.user }}
  # where nodegroup = NODE_GROUP (= the agent cert CN).
  # We register the current user's SPIFFE ID so the E2E test runner can authenticate.
  WORKLOAD_USER="${WORKLOAD_USER:-${USER}}"
  WORKLOAD_SPIFFE_ID="spiffe://${TRUST_DOMAIN}/${NODE_GROUP}/workload/${WORKLOAD_USER}"
  CERT_SERVICE_ID="${CERT_SERVICE_ID:-swa-e2e-cert}"
  echo "  Workload SPIFFE ID: ${WORKLOAD_SPIFFE_ID}"
  echo "  (override with WORKLOAD_USER=<user> if the test runner differs)"

  # Include both authn-jwt and authn-cert annotations so the workload can
  # authenticate via either authenticator.
  WORKLOAD_HOST_POLICY=$(cat <<YAML
- !policy
  id: swa-e2e
  body:
  - !host
    id: "${WORKLOAD_SPIFFE_ID}"
    annotations:
      authn-jwt/${JWT_SERVICE_ID}/sub: "${WORKLOAD_SPIFFE_ID}"
      authn-cert/${CERT_SERVICE_ID}/san-uri: "${WORKLOAD_SPIFFE_ID}"

  - !permit
    role: !host "${WORKLOAD_SPIFFE_ID}"
    privilege: [ read, execute ]
    resource: !variable test-secret
YAML
)

  echo "  Loading workload host policy..."
  echo "${WORKLOAD_HOST_POLICY}" > /tmp/swa-workload-host-policy.yml
  load_policy "data" /tmp/swa-workload-host-policy.yml

  WORKLOAD_HOST_ID="conjur:host:data/swa-e2e/${WORKLOAD_SPIFFE_ID}"
  WORKLOAD_HOST_ID_ENC=$(urlencode "${WORKLOAD_HOST_ID}")

  # Grant the workload host access to the authn-jwt consumers group via REST API.
  # Policy YAML cross-branch !grant is not supported; the REST /roles endpoint is used.
  echo "  Granting host to authn-jwt consumers group..."
  GRANT_CODE=$(http_call POST \
    "/roles/conjur/group/conjur%2Fauthn-jwt%2F${JWT_SERVICE_ID}%2Fconsumers?members&member=${WORKLOAD_HOST_ID_ENC}")
  case "$GRANT_CODE" in
    204) echo "  Granted (204)." ;;
    422) echo "  Already a member (422) — continuing." ;;
    *)
      echo "  WARNING: grant to authn-jwt returned HTTP ${GRANT_CODE}." >&2
      cat /tmp/swa_http_out.json >&2 ;;
  esac

  # Grant the workload host access to the authn-cert consumers group so it can
  # authenticate via X.509-SVID (S2 scenario via Edge).
  echo "  Granting host to authn-cert consumers group..."
  GRANT_CERT_CODE=$(http_call POST \
    "/roles/conjur/group/conjur%2Fauthn-cert%2F${CERT_SERVICE_ID}%2Fconsumers?members&member=${WORKLOAD_HOST_ID_ENC}")
  case "$GRANT_CERT_CODE" in
    204) echo "  Granted (204)." ;;
    422) echo "  Already a member (422) — continuing." ;;
    *)
      echo "  WARNING: grant to authn-cert returned HTTP ${GRANT_CERT_CODE}." >&2
      cat /tmp/swa_http_out.json >&2 ;;
  esac
}

# ── Step 13: Update authn-cert CA cert from the running SWA Agent ─────────

step_13_update_cert_ca() {
  # The SWA Agent exposes the full x509 trust bundle via the SPIFFE Workload API.
  # authn-cert requires exactly ONE root certificate in ca-cert. We find the root
  # that signed the current SVID rather than storing the full multi-root bundle,
  # which would be rejected by Conjur with ROOT_CA_COUNT.
  # The bundle rotates when the SWA Server restarts; re-run setup-swa.sh after
  # each service restart.

  CERT_SERVICE_ID="${CERT_SERVICE_ID:-swa-e2e-cert}"
  AGENT_SOCKET="${AGENT_SOCKET:-/tmp/swa-e2e-agent.sock}"

  announce "Step 13: Updating authn-cert/${CERT_SERVICE_ID}/ca-cert from SWA Agent bundle"

  if [[ ! -S "${AGENT_SOCKET}" ]]; then
    echo "  WARNING: Agent socket not found at ${AGENT_SOCKET}; skipping ca-cert update."
    echo "  Start SWA services first, then re-run setup-swa.sh to set the cert auth CA."
    return
  fi

  # Fetch the x509 trust bundle and current SVID from the SWA Agent.
  BUNDLE_DIR=$(mktemp -d)
  if "${SWA_BIN_DIR}/swa-agent" api fetch x509 \
      -s "${AGENT_SOCKET}" \
      --write "${BUNDLE_DIR}" \
      --silent 2>/dev/null; then
    BUNDLE_FILE="${BUNDLE_DIR}/bundle.0.pem"
    SVID_FILE="${BUNDLE_DIR}/svid.0.pem"
    if [[ -f "${BUNDLE_FILE}" && -s "${BUNDLE_FILE}" ]]; then
      # Find the root CA that signed the current SVID. authn-cert requires
      # exactly one root certificate; storing the full bundle fails validation.
      SIGNING_CA=""
      if [[ -f "${SVID_FILE}" ]]; then
        echo "  Finding the signing CA for the current SVID..."
        SIGNING_CA=$(python3 - "${BUNDLE_FILE}" "${SVID_FILE}" <<'PYEOF'
import subprocess, re, tempfile, os, sys
bundle_path, svid_path = sys.argv[1], sys.argv[2]
with open(bundle_path, 'r') as f:
    bundle = f.read()
certs = re.findall(r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', bundle, re.DOTALL)
for cert in certs:
    with tempfile.NamedTemporaryFile(mode='w', suffix='.pem', delete=False) as f:
        f.write(cert + '\n')
        cafile = f.name
    result = subprocess.run(['openssl', 'verify', '-partial_chain', '-CAfile', cafile, svid_path],
                            capture_output=True, text=True)
    os.unlink(cafile)
    if 'OK' in result.stdout:
        print(cert + '\n', end='', flush=True)
        break
PYEOF
        )
      fi

      # Fall back to the full bundle if the signing CA could not be identified.
      # This will fail Conjur's ROOT_CA_COUNT check if the bundle has >1 root,
      # but is better than an empty value; investigate why SVID verification failed.
      if [[ -z "${SIGNING_CA}" ]]; then
        echo "  WARNING: could not identify signing CA; using full bundle (may fail if >1 root)." >&2
        SIGNING_CA="$(cat "${BUNDLE_FILE}")"
      fi

      CA_CERT_TMP=$(mktemp)
      printf '%s' "${SIGNING_CA}" > "${CA_CERT_TMP}"
      CA_CERT_VAR_ID="conjur/authn-cert/${CERT_SERVICE_ID}/ca-cert"
      CA_HTTP=$(curl -sk -o /dev/null -w "%{http_code}" \
        -X POST \
        --config "${_AUTH_CONF}" \
        -H "Content-Type: text/plain" \
        --data-binary "@${CA_CERT_TMP}" \
        "${API_BASE}/secrets/${CONJUR_ACCOUNT}/variable/$(urlencode "${CA_CERT_VAR_ID}")")
      rm -f "${CA_CERT_TMP}"
      echo "  Set ${CA_CERT_VAR_ID} (HTTP ${CA_HTTP})."
    else
      echo "  WARNING: bundle file not found after fetch; skipping ca-cert update."
    fi
  else
    echo "  WARNING: swa-agent fetch failed; skipping ca-cert update."
    echo "  Re-run setup-swa.sh after SWA services are running to set the cert auth CA."
  fi
  rm -rf "${BUNDLE_DIR}"
}

# ── Summary ────────────────────────────────────────────────────────────────

print_summary() {
  cat <<SUMMARY

=== Setup complete ===

  SWA_WORKDIR            : ${SWA_WORKDIR}
  SWA_BIN_DIR            : ${SWA_BIN_DIR}
  TRUST_DOMAIN           : ${TRUST_DOMAIN}
  JWT_SERVICE_ID         : ${JWT_SERVICE_ID}

  Conjur tenant          : ${API_BASE}
  authn-jwt policy       : conjur/authn-jwt/${JWT_SERVICE_ID}  [loaded + enabled]
  JWKS URI               : ${JWKS_URI}
  Test variable          : data/swa-e2e/test-secret           [set]
  Workload host          : data/swa-e2e/${WORKLOAD_SPIFFE_ID}  [registered]

Next steps:
  1. Start SWA services:    bin/swa/start-swa-services.sh --detach
  2. Run E2E scenarios:     bin/swa/run-swa-scenarios.sh

Required for run-swa-scenarios.sh:
  export CONJUR_APPLIANCE_URL="${API_BASE}"
  export CONJUR_ACCOUNT="${CONJUR_ACCOUNT}"
  export CONJUR_AUTHN_JWT_SERVICE_ID="${JWT_SERVICE_ID}"
  export SWA_WORKDIR="${SWA_WORKDIR}"
  # Set after running start-swa-services.sh:
  export SPIFFE_ENDPOINT_SOCKET=unix:///tmp/swa-e2e-agent.sock

SUMMARY
}

# ── Main ───────────────────────────────────────────────────────────────────

main "$@"
