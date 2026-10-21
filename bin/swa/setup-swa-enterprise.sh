#!/usr/bin/env bash
# setup-swa-enterprise.sh — Start and configure a local Conjur Enterprise appliance
# for SWA E2E self-hosted scenarios (S8: JWT-SVID, S9: X.509-SVID).
#
# Provisions the conjur-leader Docker container with:
#   - authn-jwt/swa-e2e-ent-jwt  (inline JWKS fetched from the SWA trust domain)
#   - authn-cert/swa-e2e-ent-cert (SWA Server firefly CA chain)
#   - data/swa-e2e/summon-test host with sub and san-uri annotations
#   - data/swa-e2e/test-secret variable (same value as the Cloud secret)
#
# Usage:
#   bin/swa/setup-swa-enterprise.sh
#   source /tmp/swa-e2e/enterprise-env.sh
#   bin/swa/run-swa-scenarios.sh
#
# Prerequisite: SWA services must be running (bin/swa/start-swa-services.sh --detach).
# The SWA Server TLS chain is needed for the authn-cert CA cert (step 5).
#
# Required environment:
#   CONJUR_APPLIANCE_URL           Conjur Cloud tenant URL (used to fetch SWA JWKS)
#
# Optional environment:
#   CONJURRC_FILE                  Path to conjurrc for Conjur Cloud (default: ~/.conjurrc-swa)
#   NETRC_FILE                     Path to netrc for Conjur Cloud (default: ~/.netrc-swa)
#   SWA_WORKDIR                    Local working directory (default: /tmp/swa-e2e)
#   SWA_SERVER_ADDR                SWA Server address for CA cert fetch (default: localhost:8081)
#   TRUST_DOMAIN                   SWA trust domain name (default: swa-e2e.conjur.cloud)
#   NODE_GROUP                     SWA node group / agent cert CN (default: swa-e2e-server-agent)
#   WORKLOAD_USER                  Unix user to register (default: $USER)
#   CONJUR_ACCOUNT                 Cloud account (default: conjur)
#   CONJUR_ADMIN_PASSWORD          Enterprise appliance admin password (required — no default; must be set in env)
#   ENT_JWT_SERVICE_ID             authn-jwt service ID on enterprise (default: swa-e2e-ent-jwt)
#   ENT_CERT_SERVICE_ID            authn-cert service ID on enterprise (default: swa-e2e-ent-cert)
#   ENTERPRISE_APPLIANCE_PORT      Host port mapped to conjur-leader:443 (default: 8443)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/setup-lib.sh"

# ── Utility ───────────────────────────────────────────────────────────────────


main() {
  load_env
  setup_cloud_connection
  step_start_appliance
  step_configure_master
  step_fetch_jwks
  step_fetch_x509_bundle
  step_load_authn_jwt
  step_load_authn_cert
  step_load_workload
  step_write_env_file
  print_summary
}

announce() { echo ""; echo "=== ${*} ==="; echo ""; }

# ── Enterprise REST helpers (exec inside conjur-leader container) ─────────────

_ent_b64_token() {
  docker compose --profile swa-enterprise exec -T conjur-leader \
    curl -sfk \
    --data "${ENT_ADMIN_API_KEY}" \
    "https://localhost/authn/conjur/admin/authenticate" \
    | base64 -w 0
}

_ent_policy_load() {
  local branch="$1" policy="$2"
  local tok status
  tok="$(_ent_b64_token)"
  status="$(printf '%s' "$policy" \
    | docker exec -i "$(docker compose --profile swa-enterprise ps -q conjur-leader)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${tok}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "https://localhost/policies/conjur/policy/${branch}")"
  [[ "$status" == "201" || "$status" == "200" ]] \
    || { echo "ERROR: policy load on '${branch}' returned HTTP ${status}" >&2; exit 1; }
}

_ent_variable_set() {
  local var_id="$1" value="$2"
  local tok status
  tok="$(_ent_b64_token)"
  status="$(printf '%s' "$value" \
    | docker exec -i "$(docker compose --profile swa-enterprise ps -q conjur-leader)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${tok}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "https://localhost/secrets/conjur/variable/${var_id}")"
  [[ "$status" == "201" || "$status" == "200" || "$status" == "204" ]] \
    || { echo "ERROR: variable set '${var_id}' returned HTTP ${status}" >&2; exit 1; }
  echo "  Set ${var_id} (${status})."
}

_ent_role_grant() {
  local role_id="$1" member_id="$2"
  local tok status role_kind role_path_enc member_enc
  tok="$(_ent_b64_token)"
  # role_id format: "{kind}/{identifier}" — e.g. "group/conjur/authn-jwt/foo/authenticatable"
  # The Conjur API path is /roles/{account}/{kind}/{url-encoded-identifier}.
  role_kind="${role_id%%/*}"
  role_path_enc=$(urlencode "${role_id#*/}")
  member_enc=$(urlencode "conjur:host:${member_id}")
  status="$(docker compose --profile swa-enterprise exec -T conjur-leader \
    curl -sk -o /dev/null -w "%{http_code}" -X POST \
    -H "Authorization: Token token=\"${tok}\"" \
    "https://localhost/roles/conjur/${role_kind}/${role_path_enc}?members&member=${member_enc}")"
  [[ "$status" == "204" || "$status" == "422" ]] \
    || echo "  WARNING: role grant returned HTTP ${status}" >&2
}

_ent_authn_enable() {
  local authn_id="$1"
  local tok status
  tok="$(_ent_b64_token)"
  status="$(docker compose --profile swa-enterprise exec -T conjur-leader \
    curl -sk -o /dev/null -w "%{http_code}" -X PATCH \
    -H "Authorization: Token token=\"${tok}\"" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data "enabled=true" \
    "https://localhost/${authn_id}/conjur")"
  [[ "$status" == "200" || "$status" == "204" || "$status" == "201" ]] \
    || { echo "ERROR: authenticator enable '${authn_id}' returned HTTP ${status}" >&2; exit 1; }
}

# ── Setup functions ───────────────────────────────────────────────────────────

load_env() {
  CONJURRC_FILE="${CONJURRC_FILE:-${HOME}/.conjurrc-swa}"
  NETRC_FILE="${NETRC_FILE:-${HOME}/.netrc-swa}"
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  SWA_SERVER_ADDR="${SWA_SERVER_ADDR:-localhost:8081}"
  TRUST_DOMAIN="${TRUST_DOMAIN:-swa-e2e.conjur.cloud}"
  NODE_GROUP="${NODE_GROUP:-swa-e2e-server-agent}"
  WORKLOAD_USER="${WORKLOAD_USER:-${USER}}"
  CONJUR_ACCOUNT="${CONJUR_ACCOUNT:-conjur}"
  CONJUR_ADMIN_PASSWORD="${CONJUR_ADMIN_PASSWORD:?CONJUR_ADMIN_PASSWORD must be set (e.g. export CONJUR_ADMIN_PASSWORD=<your-password>)}"
  ENT_JWT_SERVICE_ID="${ENT_JWT_SERVICE_ID:-swa-e2e-ent-jwt}"
  ENT_CERT_SERVICE_ID="${ENT_CERT_SERVICE_ID:-swa-e2e-ent-cert}"
  ENTERPRISE_APPLIANCE_PORT="${ENTERPRISE_APPLIANCE_PORT:-8443}"

  WORKLOAD_SPIFFE_ID="spiffe://${TRUST_DOMAIN}/${NODE_GROUP}/workload/${WORKLOAD_USER}"
  ENT_HOST_ID="host/data/swa-e2e/summon-test"

  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
}

setup_cloud_connection() {
  # ── Read Conjur Cloud tenant URL ──────────────────────────────────────────────

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

  echo "Cloud tenant : ${API_BASE}"
  echo "Workload ID  : ${WORKLOAD_SPIFFE_ID}"
  echo ""

  # ── Read Conjur Cloud token (for JWKS fetch) ──────────────────────────────────

  # CI path: conjToken from getConjurCloudTenant.tokens() is already the base64-encoded
  # Conjur access token — use it directly without additional encoding.
  if [[ -n "${CONJUR_AUTHN_TOKEN:-}" ]]; then
    CONJUR_CLOUD_TOKEN="${CONJUR_AUTHN_TOKEN}"
  else
    CONJUR_CLOUD_TOKEN=$(oidc_token_b64 "$NETRC_FILE") || {
      echo "ERROR: no OIDC token found in ${NETRC_FILE}." >&2
      echo "  Set CONJUR_AUTHN_TOKEN (CI) or run: CONJURRC=${CONJURRC_FILE} conjur login" >&2
      exit 1
    }
  fi

  # Write the Authorization header to a temp config file so the token never appears
  # on the command line (visible in ps aux for the duration of each HTTP call).
  _AUTH_CONF=$(mktemp)
  printf 'header = "Authorization: Token token=\\"%s\\""\n' "${CONJUR_CLOUD_TOKEN}" > "${_AUTH_CONF}"
  trap 'rm -f "${_AUTH_CONF}"' EXIT
}

step_start_appliance() {
  # ── Step 1: Start conjur-leader ────────────────────────────────────────────────

  announce "Step 1: Starting Conjur Enterprise appliance"

  CONTAINER_ID=$(docker compose --profile swa-enterprise ps -q conjur-leader 2>/dev/null || echo "")
  if [[ -n "$CONTAINER_ID" ]]; then
    # Container exists — check if already configured.
    EXISTING_KEY=$(docker compose --profile swa-enterprise exec -T conjur-leader \
      curl -sfk --user "admin:${CONJUR_ADMIN_PASSWORD}" \
      https://localhost/authn/conjur/login 2>/dev/null | tr -d '\r\n' || echo "")
    if [[ -n "$EXISTING_KEY" && "$EXISTING_KEY" != *"<"* && "$EXISTING_KEY" != *"error"* ]]; then
      echo "  Appliance already configured — reusing."
      ENT_ADMIN_API_KEY="$EXISTING_KEY"
      ALREADY_CONFIGURED=true
    else
      ALREADY_CONFIGURED=false
    fi
  else
    docker compose --profile swa-enterprise pull conjur-leader 2>&1 | tail -5
    docker compose --profile swa-enterprise up --no-deps -d conjur-leader
    ALREADY_CONFIGURED=false
  fi

  # Wait for the container to be exec-able.
  TIMEOUT=60; ELAPSED=0
  until docker compose --profile swa-enterprise exec -T conjur-leader true 2>/dev/null; do
    if (( ELAPSED >= TIMEOUT )); then
      echo "ERROR: timed out waiting for conjur-leader container" >&2; exit 1
    fi
    sleep 2; (( ELAPSED += 2 ))
  done
}

step_configure_master() {
  [[ "${ALREADY_CONFIGURED:-false}" == "true" ]] && return

  # ── Step 2: Configure master ───────────────────────────────────────────────────

  announce "Step 2: Configuring Conjur Enterprise master"

  # evoke configure master initializes postgres internally; retry until it succeeds
  # (the appliance needs a moment after container start before accepting the command).
  TIMEOUT=300; ELAPSED=0
  until docker compose --profile swa-enterprise exec -T conjur-leader \
      evoke configure master \
      --hostname=conjur-leader-1.mycompany.local \
      --accept-eula \
      --admin-password="${CONJUR_ADMIN_PASSWORD}" \
      conjur; do
    if (( ELAPSED >= TIMEOUT )); then
      echo "ERROR: timed out configuring Conjur Enterprise master" >&2; exit 1
    fi
    sleep 5; (( ELAPSED += 5 ))
  done

  # Wait for TLS cert to be generated.
  TIMEOUT=120; ELAPSED=0
  until docker compose --profile swa-enterprise exec -T conjur-leader \
      test -f /opt/conjur/etc/ssl/conjur.pem; do
    if (( ELAPSED >= TIMEOUT )); then
      echo "ERROR: timed out waiting for TLS certificate" >&2; exit 1
    fi
    sleep 2; (( ELAPSED += 2 ))
  done

  ENT_ADMIN_API_KEY="$(docker compose --profile swa-enterprise exec -T conjur-leader \
    curl -sfk --user "admin:${CONJUR_ADMIN_PASSWORD}" \
    https://localhost/authn/conjur/login | tr -d '\r\n')"
  [[ -z "${ENT_ADMIN_API_KEY}" || "${ENT_ADMIN_API_KEY}" == *"<"* ]] && {
    echo "ERROR: failed to retrieve admin API key" >&2; exit 1
  }
  echo "  Enterprise appliance ready."
}

step_fetch_jwks() {
  # ── Step 3: Fetch SWA JWKS from Conjur Cloud trust domain ─────────────────────

  announce "Step 3: Fetching SWA JWKS from trust domain '${TRUST_DOMAIN}'"

  JWKS_URL="${API_BASE}/swa/trust-domains/${TRUST_DOMAIN}/.well-known/jwks"
  SWA_JWKS=$(curl -sk \
    --config "${_AUTH_CONF}" \
    -H "Accept: application/json" \
    "${JWKS_URL}" 2>/dev/null) || {
      echo "ERROR: failed to fetch JWKS from ${JWKS_URL}" >&2; exit 1
    }

  [[ -z "$SWA_JWKS" || "$SWA_JWKS" == "null" || "$SWA_JWKS" == *'"error"'* ]] && {
    echo "ERROR: JWKS response is empty or an error — is the SWA trust domain provisioned?" >&2
    echo "  Run bin/swa/setup-swa.sh first." >&2
    exit 1
  }

  PUBLIC_KEYS="{\"type\":\"jwks\",\"value\":${SWA_JWKS}}"
  echo "  JWKS fetched from ${JWKS_URL}"
}

step_fetch_x509_bundle() {
  # ── Step 4: Fetch SWA Agent X.509 bundle (CA cert for authn-cert) ────────────

  announce "Step 4: Fetching SWA X.509 CA bundle from SPIFFE Workload API"

  AGENT_SOCKET="${AGENT_SOCKET:-/tmp/swa-e2e-agent.sock}"
  SWA_BIN_DIR="${SWA_BIN_DIR:-/tmp/swa-bins}"

  if [[ ! -S "${AGENT_SOCKET}" ]]; then
    echo "ERROR: SWA Agent socket not found at ${AGENT_SOCKET}." >&2
    echo "  Run bin/swa/start-swa-services.sh --detach first." >&2
    exit 1
  fi

  _BUNDLE_DIR=$(mktemp -d)
  # Retry: the agent socket may be open while node attestation still completes.
  TIMEOUT=60; ELAPSED=0
  until "${SWA_BIN_DIR}/swa-agent" api fetch x509 \
        -s "${AGENT_SOCKET}" \
        --write "${_BUNDLE_DIR}" \
        --silent; do
    if (( ELAPSED >= TIMEOUT )); then
      echo "ERROR: swa-agent api fetch x509 failed after ${TIMEOUT}s." >&2
      rm -rf "${_BUNDLE_DIR}"
      exit 1
    fi
    echo "  Agent not ready yet; retrying in 5s..." >&2
    sleep 5; (( ELAPSED += 5 ))
    rm -rf "${_BUNDLE_DIR}"; _BUNDLE_DIR=$(mktemp -d)
  done
  # Diagnose what files swa-agent actually wrote.
  echo "  Files written by swa-agent:" >&2
  ls -1 "${_BUNDLE_DIR}/" >&2

  _FULL_BUNDLE=""
  _SVID=""
  # Accept both .pem (swa-agent ≥ v1.2 / SPIRE with --pem) and .der (older/default).
  for ext in pem der; do
    [[ -z "${_FULL_BUNDLE}" && -f "${_BUNDLE_DIR}/bundle.0.${ext}" ]] && _FULL_BUNDLE="${_BUNDLE_DIR}/bundle.0.${ext}"
    [[ -z "${_SVID}" && -f "${_BUNDLE_DIR}/svid.0.${ext}" ]] && _SVID="${_BUNDLE_DIR}/svid.0.${ext}"
  done

  if [[ -z "${_FULL_BUNDLE}" ]]; then
    echo "ERROR: empty X.509 bundle from SWA Agent — is the Agent running?" >&2
    rm -rf "${_BUNDLE_DIR}"; exit 1
  fi

  # Convert DER to PEM in-place if needed.
  if [[ "${_FULL_BUNDLE}" == *.der ]]; then
    # SPIFFE bundles are always X.509 DER, never PKCS12.
    openssl x509 -inform DER -in "${_FULL_BUNDLE}" -out "${_BUNDLE_DIR}/bundle.0.pem" 2>/dev/null || true
    [[ -f "${_BUNDLE_DIR}/bundle.0.pem" ]] && _FULL_BUNDLE="${_BUNDLE_DIR}/bundle.0.pem"
  fi
  if [[ -n "${_SVID}" && "${_SVID}" == *.der ]]; then
    openssl x509 -inform DER -in "${_SVID}" -out "${_BUNDLE_DIR}/svid.0.pem" 2>/dev/null || true
    [[ -f "${_BUNDLE_DIR}/svid.0.pem" ]] && _SVID="${_BUNDLE_DIR}/svid.0.pem"
  fi

  _BUNDLE_CERTS=$(grep -c 'BEGIN CERTIFICATE' "${_FULL_BUNDLE}" 2>/dev/null || echo "0")
  echo "  X.509 trust bundle fetched (${_BUNDLE_CERTS} cert(s)) from ${_FULL_BUNDLE}."

  # Conjur Enterprise authn-cert requires exactly ONE root certificate in ca-cert.
  # Identify the signing CA by matching the SVID's issuer DN against cert subjects in
  # the bundle — more reliable than openssl verify which requires self-signed CAs.
  echo "  Finding the signing CA for the current SVID..."
  FIREFLY_CA=""
  if [[ -n "${_SVID}" && -f "${_SVID}" ]]; then
    FIREFLY_CA=$(python3 "$(dirname "$0")/find-signing-ca.py" \
      "${_FULL_BUNDLE}" "${_SVID}" "${SWA_SERVER_ADDR}")
  else
    echo "  WARNING: SVID file not found at ${_SVID:-<unset>} — cannot match signing CA." >&2
  fi

  # Final fallback: use the full bundle. Conjur Enterprise will reject it if it
  # contains more than one root CA (ROOT_CA_COUNT error). Better than no value.
  if [[ -z "${FIREFLY_CA}" ]]; then
    echo "  WARNING: could not identify signing CA; using full bundle (may fail if >1 root)." >&2
    FIREFLY_CA="$(cat "${_FULL_BUNDLE}")"
  fi
  rm -rf "${_BUNDLE_DIR}"
}

step_load_authn_jwt() {
  # ── Step 5: Load authn-jwt policy ─────────────────────────────────────────────

  announce "Step 5: Loading authn-jwt/${ENT_JWT_SERVICE_ID} policy"

  _ent_policy_load "root" "$(cat <<'EOF'
- !policy
  id: conjur/authn-jwt
EOF
)" 2>/dev/null || true

  _ent_policy_load "conjur/authn-jwt" "$(cat <<EOF
- !policy
  id: ${ENT_JWT_SERVICE_ID}
  body:
  - !webservice
  - !variable public-keys
  - !variable issuer
  - !variable audience
  # token-app-property is intentionally absent. The SPIFFE sub claim contains
  # "://" which Conjur Enterprise cannot use as a host identity (CONJ00098E).
  # Host identity is provided explicitly via CONJUR_AUTHN_JWT_HOST_ID in the URL.
  - !webservice status
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
EOF
)"

  echo "  Setting authn-jwt variables..."
  _ent_variable_set "conjur/authn-jwt/${ENT_JWT_SERVICE_ID}/public-keys" "${PUBLIC_KEYS}"
  _ent_variable_set "conjur/authn-jwt/${ENT_JWT_SERVICE_ID}/issuer" "${API_BASE}/swa/trust-domains/${TRUST_DOMAIN}"
  _ent_variable_set "conjur/authn-jwt/${ENT_JWT_SERVICE_ID}/audience" "conjur"

  _ent_authn_enable "authn-jwt/${ENT_JWT_SERVICE_ID}"
  echo "  authn-jwt/${ENT_JWT_SERVICE_ID} enabled."
}

step_load_authn_cert() {
  # ── Step 6: Load authn-cert policy ────────────────────────────────────────────

  announce "Step 6: Loading authn-cert/${ENT_CERT_SERVICE_ID} policy"

  _ent_policy_load "root" "$(cat <<'EOF'
- !policy
  id: conjur/authn-cert
EOF
)" 2>/dev/null || true

  _ent_policy_load "conjur/authn-cert" "$(cat <<EOF
- !policy
  id: ${ENT_CERT_SERVICE_ID}
  body:
  - !webservice
  - !webservice status
  - !variable ca-cert
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
EOF
)"

  echo "  Setting authn-cert ca-cert variable..."
  _ent_variable_set "conjur/authn-cert/${ENT_CERT_SERVICE_ID}/ca-cert" "${FIREFLY_CA}"

  _ent_authn_enable "authn-cert/${ENT_CERT_SERVICE_ID}"
  echo "  authn-cert/${ENT_CERT_SERVICE_ID} enabled."
}

step_load_workload() {
  # ── Step 7: Load workload host and test secret ─────────────────────────────────

  announce "Step 7: Loading workload host and test secret"

  _ent_policy_load "root" "$(cat <<EOF
- !policy
  id: data
  body:
  - !policy
    id: swa-e2e
    body:
    - !host
      id: summon-test
      annotations:
        authn-jwt/${ENT_JWT_SERVICE_ID}/sub: "${WORKLOAD_SPIFFE_ID}"
        authn-cert/${ENT_CERT_SERVICE_ID}/san-uri: "${WORKLOAD_SPIFFE_ID}"
    - !variable test-secret
    - !permit
      role: !host summon-test
      privilege: [ read, execute ]
      resource: !variable test-secret
EOF
)"

  echo "  Granting host to authn-jwt authenticatable group..."
  _ent_role_grant "group/conjur/authn-jwt/${ENT_JWT_SERVICE_ID}/authenticatable" "data/swa-e2e/summon-test"

  echo "  Granting host to authn-cert authenticatable group..."
  _ent_role_grant "group/conjur/authn-cert/${ENT_CERT_SERVICE_ID}/authenticatable" "data/swa-e2e/summon-test"

  # Set test secret to same value as the Cloud secret.
  TEST_SECRET_VALUE="$(cat "${SWA_WORKDIR}/test-secret-value.txt" 2>/dev/null || echo "swa-e2e-secret-value")"
  _ent_variable_set "data/swa-e2e/test-secret" "${TEST_SECRET_VALUE}"
}

step_write_env_file() {
  # ── Step 8: Write enterprise-env.sh ───────────────────────────────────────────

  announce "Step 8: Writing ${SWA_WORKDIR}/enterprise-env.sh"

  ENTERPRISE_URL="https://localhost:${ENTERPRISE_APPLIANCE_PORT}"

  # Extract the self-signed appliance cert to use as the trusted CA.
  # The cert is signed by its own key (no separate CA), so we use it directly.
  ENT_CA_CERT="$(docker compose --profile swa-enterprise exec -T conjur-leader \
    cat /opt/conjur/etc/ssl/conjur.pem 2>/dev/null | tr -d '\r')"
  if [[ -z "${ENT_CA_CERT}" ]]; then
    echo "ERROR: could not extract Enterprise CA cert from container" >&2; exit 1
  fi
  ENT_CA_FILE="${SWA_WORKDIR}/enterprise-ca.pem"
  printf '%s\n' "${ENT_CA_CERT}" > "${ENT_CA_FILE}"
  echo "  Enterprise CA cert written to ${ENT_CA_FILE}"

  cat > "${SWA_WORKDIR}/enterprise-env.sh" <<EOF
# Generated by bin/swa/setup-swa-enterprise.sh — source before running bin/swa/run-swa-scenarios.sh.
export CONJUR_ENTERPRISE_URL="${ENTERPRISE_URL}"
export CONJUR_ENTERPRISE_ACCOUNT="conjur"
export CONJUR_ENTERPRISE_JWT_SERVICE_ID="${ENT_JWT_SERVICE_ID}"
export CONJUR_ENTERPRISE_CERT_SERVICE_ID="${ENT_CERT_SERVICE_ID}"
export CONJUR_ENTERPRISE_JWT_HOST_ID="${ENT_HOST_ID}"
export CONJUR_ENTERPRISE_CERT_HOST_ID="${ENT_HOST_ID}"
export CONJUR_ENTERPRISE_IGNORE_SSL="false"
export CONJUR_ENTERPRISE_CA_CERT="\$(cat ${ENT_CA_FILE})"
# Conjur Enterprise 5.x generates certs with DER-negative serials; allow them.
export CONJUR_ENTERPRISE_GODEBUG="x509negativeserial=1"
EOF
  echo "  Written: ${SWA_WORKDIR}/enterprise-env.sh"
}

print_summary() {
  # ── Summary ────────────────────────────────────────────────────────────────────

  cat <<SUMMARY

=== Enterprise setup complete ===

  Enterprise URL   : ${ENTERPRISE_URL}
  JWT service      : ${ENT_JWT_SERVICE_ID}
  Cert service     : ${ENT_CERT_SERVICE_ID}
  Workload host    : data/swa-e2e/summon-test
  Test secret      : data/swa-e2e/test-secret

Next step — source the env file and run the full scenario suite:

  source ${SWA_WORKDIR}/enterprise-env.sh
  bin/swa/run-swa-scenarios.sh

SUMMARY
}

# ── Entrypoint ────────────────────────────────────────────────────────────────

main "$@"
