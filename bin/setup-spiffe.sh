#!/bin/bash
set -euo pipefail
# Starts the SPIRE stack and configures Conjur for SPIFFE Workload API integration tests.
# Source from bin/test.sh when TEST_SPIFFE=true (exports SPIFFE_ENDPOINT_SOCKET and
# CONJUR_AUTHN_CERT_SERVICE_ID).

cd "$(dirname "${BASH_SOURCE[0]}")/.."

announce() {
  echo "
  ================================
   ${1}
  ================================
  "
}

# Admin socket path written by the SPIRE server (see ci/spiffe/server.conf
# socket_path). Passed to all spire-server CLI commands via -socketPath and
# shared with the SPIRE OIDC discovery provider via the spire-server-socket volume.
readonly SPIRE_SERVER_SOCKET="/run/spire-server/private/api.sock"

export CONJUR_ADMIN_PASSWORD="${CONJUR_ADMIN_PASSWORD:-MySecretP@ss1}"

# ── Conjur REST API helpers ───────────────────────────────────────────────────
# The conjur CLI is not present in the Conjur server container.
# These helpers use conjurctl + curl (both available inside the container)
# to perform the same operations, following the pattern in bin/functions.sh.
# Requires CONJUR_ADMIN_API_KEY exported by initEnvironment() in functions.sh.

# Authenticate as admin; return a base64-encoded token for the Authorization header.
function _conjur_b64_token() {
  local token
  token="$(exec_on conjur curl -sf \
    --data "${CONJUR_ADMIN_API_KEY}" \
    "http://localhost/authn/cucumber/admin/authenticate")"
  printf '%s' "$token" | base64 -w 0
}

# Load a Conjur policy into the given branch.
# Usage: conjur_policy_load <branch> <policy_yaml>
function conjur_policy_load() {
  local branch="$1" policy="$2"
  local encoded_token status
  encoded_token="$(_conjur_b64_token)"
  status="$(printf '%s' "$policy" \
    | docker exec -i "$(docker compose ps -q conjur)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${encoded_token}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "http://localhost/policies/cucumber/policy/${branch}")"
  [[ "$status" == "201" || "$status" == "200" ]] \
    || { echo "ERROR: policy load on branch '${branch}' returned HTTP ${status}"; exit 1; }
}

# Set a Conjur variable value.
# Usage: conjur_variable_set <variable_id> <value>
function conjur_variable_set() {
  local var_id="$1" value="$2"
  local encoded_token status
  encoded_token="$(_conjur_b64_token)"
  status="$(printf '%s' "$value" \
    | docker exec -i "$(docker compose ps -q conjur)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${encoded_token}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "http://localhost/secrets/cucumber/variable/${var_id}")"
  [[ "$status" == "201" || "$status" == "200" || "$status" == "204" ]] \
    || { echo "ERROR: variable set '${var_id}' returned HTTP ${status}"; exit 1; }
}

# Enable a Conjur authenticator.
# Usage: conjur_authenticator_enable <authenticator_type/service_id>
# e.g.:  conjur_authenticator_enable "authn-jwt/spiffe-jwt"
function conjur_authenticator_enable() {
  local authn_id="$1"
  local encoded_token status
  encoded_token="$(_conjur_b64_token)"
  status="$(exec_on conjur curl -sk -o /dev/null -w "%{http_code}" -X PATCH \
    -H "Authorization: Token token=\"${encoded_token}\"" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data "enabled=true" \
    "http://localhost/${authn_id}/cucumber")"
  [[ "$status" == "200" || "$status" == "204" || "$status" == "201" ]] \
    || { echo "ERROR: authenticator enable '${authn_id}' returned HTTP ${status}"; exit 1; }
}

# ── Conjur Enterprise Appliance REST API helpers ─────────────────────────────
# These target the conjur-leader Enterprise Appliance (HTTPS) and require
# CONJUR_CERT_AUTHN_API_KEY to be exported by start_conjur_leader_for_spiffe.
# All calls run inside the conjur-leader container via docker compose exec.

# Authenticate as admin against the Enterprise Appliance; return a base64-encoded token.
function _conjur_appliance_b64_token() {
  local token
  token="$(docker compose --profile spiffe exec -T conjur-leader \
    curl -sfk \
    --data "${CONJUR_CERT_AUTHN_API_KEY}" \
    "https://localhost/authn/conjur/admin/authenticate")"
  printf '%s' "$token" | base64 -w 0
}

# Load a Conjur policy on the Enterprise Appliance.
# Usage: conjur_appliance_policy_load <branch> <policy_yaml>
function conjur_appliance_policy_load() {
  local branch="$1" policy="$2"
  local encoded_token status
  encoded_token="$(_conjur_appliance_b64_token)"
  status="$(printf '%s' "$policy" \
    | docker exec -i "$(docker compose --profile spiffe ps -q conjur-leader)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${encoded_token}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "https://localhost/policies/conjur/policy/${branch}")"
  [[ "$status" == "201" || "$status" == "200" ]] \
    || { echo "ERROR: appliance policy load on branch '${branch}' returned HTTP ${status}"; exit 1; }
}

# Set a Conjur variable value on the Enterprise Appliance.
# Usage: conjur_appliance_variable_set <variable_id> <value>
function conjur_appliance_variable_set() {
  local var_id="$1" value="$2"
  local encoded_token status
  encoded_token="$(_conjur_appliance_b64_token)"
  status="$(printf '%s' "$value" \
    | docker exec -i "$(docker compose --profile spiffe ps -q conjur-leader)" \
      curl -sk -o /dev/null -w "%{http_code}" -X POST \
      -H "Authorization: Token token=\"${encoded_token}\"" \
      -H "Content-Type: text/plain" \
      --data-binary "@-" \
      "https://localhost/secrets/conjur/variable/${var_id}")"
  [[ "$status" == "201" || "$status" == "200" || "$status" == "204" ]] \
    || { echo "ERROR: appliance variable set '${var_id}' returned HTTP ${status}"; exit 1; }
}

# Enable a Conjur authenticator on the Enterprise Appliance.
# Usage: conjur_appliance_authenticator_enable <authenticator_type/service_id>
function conjur_appliance_authenticator_enable() {
  local authn_id="$1"
  local encoded_token status
  encoded_token="$(_conjur_appliance_b64_token)"
  status="$(docker compose --profile spiffe exec -T conjur-leader \
    curl -sk -o /dev/null -w "%{http_code}" -X PATCH \
    -H "Authorization: Token token=\"${encoded_token}\"" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data "enabled=true" \
    "https://localhost/${authn_id}/conjur")"
  [[ "$status" == "200" || "$status" == "204" || "$status" == "201" ]] \
    || { echo "ERROR: appliance authenticator enable '${authn_id}' returned HTTP ${status}"; exit 1; }
}

# ── Conjur Enterprise Appliance startup ───────────────────────────────────────

function start_conjur_leader_for_spiffe() {
  # If setup-cert-auth.sh already started and configured conjur-leader
  # (when TEST_CERT=true and TEST_SPIFFE=true both run), reuse its credentials.
  if [[ -n "${CONJUR_CERT_AUTHN_API_KEY:-}" ]]; then
    announce "Conjur Enterprise appliance already running (reusing cert-auth setup)."
    return 0
  fi

  announce "Pulling Conjur Enterprise appliance image for SPIFFE cert tests..."
  docker compose --profile spiffe pull conjur-leader

  announce "Starting Conjur Enterprise appliance..."
  docker compose --profile spiffe up --no-deps -d conjur-leader

  local timeout=60 elapsed=0
  until docker compose --profile spiffe exec -T conjur-leader true 2>/dev/null; do
    if (( elapsed >= timeout )); then
      echo "ERROR: timed out waiting for conjur-leader container to be exec-able"
      exit 1
    fi
    sleep 2
    (( elapsed += 2 ))
  done

  announce "Configuring Conjur Enterprise master..."
  timeout=120; elapsed=0
  until docker compose --profile spiffe exec -T conjur-leader evoke configure master \
    --hostname=conjur-leader-1.mycompany.local \
    --master-altnames="conjur-leader.mycompany.local,conjur-leader-1.mycompany.local" \
    --accept-eula \
    --admin-password="${CONJUR_ADMIN_PASSWORD}" \
    conjur; do
    if (( elapsed >= timeout )); then
      echo "ERROR: timed out configuring Conjur Enterprise master"
      exit 1
    fi
    sleep 5
    (( elapsed += 5 ))
  done

  announce "Waiting for Enterprise Appliance TLS certificate..."
  timeout=120; elapsed=0
  until docker compose --profile spiffe exec -T conjur-leader test -f /opt/conjur/etc/ssl/conjur.pem; do
    if (( elapsed >= timeout )); then
      echo "ERROR: timed out waiting for Enterprise Appliance TLS certificate"
      exit 1
    fi
    sleep 2
    (( elapsed += 2 ))
  done

  CONJUR_CERT_AUTHN_API_KEY="$(docker compose --profile spiffe exec -T conjur-leader \
    curl -sfk --user "admin:${CONJUR_ADMIN_PASSWORD}" \
    https://localhost/authn/conjur/login | tr -d '\r\n')"
  if [[ -z "${CONJUR_CERT_AUTHN_API_KEY}" || "${CONJUR_CERT_AUTHN_API_KEY}" == *"<"* ]]; then
    echo "ERROR: failed to retrieve admin API key from Enterprise Appliance"
    exit 1
  fi
  export CONJUR_CERT_AUTHN_API_KEY

  CONJUR_CERT_SSL_CERTIFICATE="$(docker compose --profile spiffe exec -T conjur-leader \
    cat /opt/conjur/etc/ssl/conjur.pem)"
  export CONJUR_CERT_SSL_CERTIFICATE
  export CONJUR_CERT_APPLIANCE_URL="https://conjur-leader-1.mycompany.local"
  announce "Enterprise Appliance ready at ${CONJUR_CERT_APPLIANCE_URL}"
}

# ── Conjur Enterprise workload identity for cert tests ────────────────────────

# Creates the SPIFFE test host and secret on the Enterprise Appliance.
# Reuses the secret path and value set by configure_conjur_workload on OSS Conjur
# so TestAuthnSpiffeCertIntegration can use the same CONJUR_SPIFFE_TEST_SECRET_ID
# and CONJUR_SPIFFE_TEST_SECRET_VALUE env vars.
function configure_conjur_cert_workload_enterprise() {
  announce "Configuring Conjur Enterprise workload identity for SPIFFE cert tests..."

  # SPIFFE X.509-SVID certs have an empty Subject CN (per the SPIFFE spec).
  # authn-cert uses URL-based mode: the host ID goes in the URL and the
  # Enterprise Appliance validates the cert's URI SAN against the
  # authn-cert/<service-id>/san-uri annotation on the host.
  local workload_policy
  workload_policy="$(cat <<EOF
- !policy
  id: data
  body:
  - !policy
    id: spiffe-test
    body:
    - !host
      id: summon-test
      annotations:
        authn-cert/${SPIFFE_CERT_SERVICE_ID}/san-uri: spiffe://example.org/workload/summon-test
    - !variable spiffe-secret
    - !permit
      role: !host summon-test
      privilege: [ read, execute ]
      resource: !variable spiffe-secret
EOF
)"
  conjur_appliance_policy_load "root" "$workload_policy"

  # Set the same secret value as OSS Conjur so the test env var is consistent.
  conjur_appliance_variable_set "data/spiffe-test/spiffe-secret" \
    "${CONJUR_SPIFFE_TEST_SECRET_VALUE}"

  announce "Enterprise Appliance workload identity configured."
}

# ── SPIRE server ──────────────────────────────────────────────────────────────

function start_spire_server() {
  announce "Starting SPIRE server..."
  docker compose --profile spiffe up --no-deps -d spire-server
  if ! docker compose --profile spiffe up --no-deps --wait spire-server; then
    echo "ERROR: SPIRE server failed to start. Container logs:"
    docker compose --profile spiffe logs spire-server || true
    exit 1
  fi
  announce "SPIRE server ready."
}

# ── SPIRE agent ───────────────────────────────────────────────────────────────

function start_spire_agent() {
  announce "Generating SPIRE join token..."
  local join_token
  join_token="$(docker compose --profile spiffe exec -T spire-server \
    /opt/spire/bin/spire-server token generate \
    -socketPath "${SPIRE_SERVER_SOCKET}" \
    -spiffeID "spiffe://example.org/agent/summon-test" \
    -ttl 300 2>&1 | sed -nE 's/.*Token: ([^ ]+).*/\1/p')"

  if [[ -z "${join_token}" ]]; then
    echo "ERROR: failed to generate SPIRE join token"
    exit 1
  fi

  export SPIRE_JOIN_TOKEN="${join_token}"
  announce "Starting SPIRE agent..."
  docker compose --profile spiffe up --no-deps -d spire-agent
  docker compose --profile spiffe up --no-deps --wait spire-agent
  announce "SPIRE agent ready."
}

# ── Workload registration ─────────────────────────────────────────────────────

# Register the test workload. The test container runs as root (UID 0); both
# authn-cert (X.509-SVID) and authn-jwt (JWT-SVID) use the same SPIFFE ID.
function register_workload() {
  announce "Registering test workload with SPIRE..."
  local spiffe_id="spiffe://example.org/workload/summon-test"
  docker compose --profile spiffe exec -T spire-server \
    /opt/spire/bin/spire-server entry create \
    -socketPath "${SPIRE_SERVER_SOCKET}" \
    -spiffeID "${spiffe_id}" \
    -parentID "spiffe://example.org/agent/summon-test" \
    -selector "unix:uid:0"

  echo "Registered workload: ${spiffe_id}"
  export CONJUR_SPIFFE_ID="${spiffe_id}"
}

# ── Conjur policy — authn-jwt SPIFFE (JWT-SVID) ───────────────────────────────

readonly SPIFFE_JWT_SERVICE_ID="spiffe-jwt"
readonly SPIFFE_JWT_OIDC_SERVICE_ID="spiffe-jwt-oidc"

function configure_conjur_jwt_spiffe() {
  announce "Configuring Conjur authn-jwt for SPIFFE JWT-SVID..."

  # Use the OIDC Discovery Provider URL as the JWT issuer to match the value
  # that SPIRE emits in the "iss" claim when jwt_issuer is set in server.conf.
  # Both this authenticator (static public-keys) and spiffe-jwt-oidc (jwks-uri)
  # must use the same issuer, because SPIRE signs all JWTs with a single issuer.
  local issuer="http://spire-oidc:8085"

  # Create authn-jwt branch if it does not already exist (idempotent).
  local parent_policy
  parent_policy="$(cat <<'EOF'
- !policy
  id: conjur/authn-jwt
EOF
)"
  conjur_policy_load "root" "$parent_policy" || true

  local service_policy
  service_policy="$(cat <<EOF
- !policy
  id: ${SPIFFE_JWT_SERVICE_ID}
  body:
  - !webservice
  - !variable public-keys
  - !variable issuer
  - !variable audience
  # token-app-property is intentionally absent. The SPIFFE JWT sub claim
  # (e.g. spiffe://example.org/workload/summon-test) contains "://" which
  # Conjur treats as invalid even for annotation-based identity resolution,
  # throwing CONJ00098E. Host identity is provided explicitly in the URL path
  # via CONJUR_AUTHN_JWT_HOST_ID (URL-encoded: host%2Fdata%2F...).
  # identity-path is also absent for the same reason.
  - !webservice status
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
  - !grant
    role: !group authenticatable
    member: !host /data/spiffe-test/summon-test
EOF
)"
  conjur_policy_load "conjur/authn-jwt" "$service_policy"

  # Fetch JWKS for the public-keys variable.
  # Primary: the SPIRE OIDC Discovery Provider (already confirmed reachable from
  # the conjur container by start_spire_oidc). It serves a standard JWKS object.
  # Fallback: spire-server bundle show with a short retry loop for the case where
  # JWT authorities are still being generated at startup.
  local jwks
  jwks="$(exec_on conjur curl -sf "http://spire-oidc:8085/keys" || echo "")"
  if [[ -n "${jwks}" ]]; then
    jwks="{\"type\":\"jwks\",\"value\":${jwks}}"
  else
    # OIDC endpoint unavailable — extract JWT authorities from the SPIRE bundle.
    # Retry for up to 30s; SPIRE may not have generated JWT keys yet at startup.
    local elapsed=0
    while (( elapsed < 30 )); do
      local raw
      raw="$(docker compose --profile spiffe exec -T spire-server \
        /opt/spire/bin/spire-server bundle show \
        -socketPath "${SPIRE_SERVER_SOCKET}" \
        -format spiffe 2>/dev/null || echo "")"
      if [[ -n "$raw" ]]; then
        local keys
        keys="$(echo "$raw" \
          | python3 -c "import json,sys; b=json.load(sys.stdin); print(json.dumps(b.get('jwtAuthorities', b.get('jwt_authorities', []))))" \
          2>/dev/null || echo "[]")"
        if [[ "$keys" != "[]" && -n "$keys" ]]; then
          jwks="{\"type\":\"jwks\",\"value\":{\"keys\":${keys}}}"
          break
        fi
      fi
      sleep 2
      (( elapsed += 2 ))
    done
  fi

  if [[ -z "${jwks}" ]]; then
    echo "ERROR: failed to obtain SPIRE JWKS — OIDC endpoint and bundle show both returned empty"
    exit 1
  fi

  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_SERVICE_ID}/public-keys" "${jwks}"
  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_SERVICE_ID}/issuer" "${issuer}"
  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_SERVICE_ID}/audience" "conjur"
  # token-app-property is not set; host identity comes from the URL path.

  conjur_authenticator_enable "authn-jwt/${SPIFFE_JWT_SERVICE_ID}"

  export CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID="${SPIFFE_JWT_SERVICE_ID}"
  announce "authn-jwt SPIFFE authenticator configured."
}

# ── Conjur policy — authn-jwt SPIFFE via OIDC jwks-uri (JWT-SVID, dynamic keys) ─

function configure_conjur_jwt_spiffe_oidc() {
  announce "Configuring Conjur authn-jwt for SPIFFE JWT-SVID (jwks-uri / OIDC)..."

  # Create the authn-jwt/spiffe-jwt-oidc service: same host identity as
  # spiffe-jwt but keys are fetched dynamically from the SPIRE OIDC provider.
  local service_policy
  service_policy="$(cat <<EOF
- !policy
  id: ${SPIFFE_JWT_OIDC_SERVICE_ID}
  body:
  - !webservice
  - !variable jwks-uri
  - !variable issuer
  - !variable audience
  # token-app-property is intentionally absent — see configure_conjur_jwt_spiffe.
  # Host identity is provided explicitly in the URL path via CONJUR_AUTHN_JWT_HOST_ID.
  # identity-path is also absent for the same reason.
  - !webservice status
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
  - !grant
    role: !group authenticatable
    member: !host /data/spiffe-test/summon-test
EOF
)"
  conjur_policy_load "conjur/authn-jwt" "$service_policy"

  # The SPIRE OIDC Discovery Provider serves JWKS at http://spire-oidc:8085/keys.
  # Conjur fetches keys from this URL dynamically, enabling automatic key rotation.
  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_OIDC_SERVICE_ID}/jwks-uri" "http://spire-oidc:8085/keys"
  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_OIDC_SERVICE_ID}/issuer" "http://spire-oidc:8085"
  conjur_variable_set "conjur/authn-jwt/${SPIFFE_JWT_OIDC_SERVICE_ID}/audience" "conjur"
  # token-app-property is not set; host identity comes from the URL path.

  conjur_authenticator_enable "authn-jwt/${SPIFFE_JWT_OIDC_SERVICE_ID}"

  export CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID="${SPIFFE_JWT_OIDC_SERVICE_ID}"
  announce "authn-jwt SPIFFE OIDC authenticator configured."
}

# ── Conjur policy — authn-cert SPIFFE (X.509-SVID) ───────────────────────────

readonly SPIFFE_CERT_SERVICE_ID="spiffe-cert"

function configure_conjur_cert_spiffe() {
  announce "Configuring Conjur Enterprise authn-cert for SPIFFE X.509-SVID..."

  # Extract the SPIRE trust bundle CA (PEM) for the authn-cert CA configuration.
  local ca_cert
  ca_cert="$(docker compose --profile spiffe exec -T spire-server \
    /opt/spire/bin/spire-server bundle show \
    -socketPath "${SPIRE_SERVER_SOCKET}" \
    -format pem)"
  if [[ -z "${ca_cert}" ]]; then
    echo "ERROR: failed to retrieve SPIRE trust bundle"
    exit 1
  fi

  # Create the conjur/authn-cert branch on the Enterprise Appliance if needed.
  local parent_policy
  parent_policy="$(cat <<'EOF'
- !policy
  id: conjur/authn-cert
EOF
)"
  conjur_appliance_policy_load "root" "$parent_policy" || true

  # Load the authn-cert service policy.
  local service_policy
  service_policy="$(cat <<EOF
- !policy
  id: ${SPIFFE_CERT_SERVICE_ID}
  body:
  - !webservice
  - !webservice status
  - !variable ca-cert
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
  - !grant
    role: !group authenticatable
    member: !host /data/spiffe-test/summon-test
EOF
)"
  conjur_appliance_policy_load "conjur/authn-cert" "$service_policy"

  # Store the SPIRE CA as the webservice's CA configuration variable.
  conjur_appliance_variable_set "conjur/authn-cert/${SPIFFE_CERT_SERVICE_ID}/ca-cert" "${ca_cert}"

  conjur_appliance_authenticator_enable "authn-cert/${SPIFFE_CERT_SERVICE_ID}"

  export CONJUR_AUTHN_CERT_HOST_ID="host/data/spiffe-test/summon-test"
  export CONJUR_AUTHN_CERT_SERVICE_ID="${SPIFFE_CERT_SERVICE_ID}"
  announce "authn-cert SPIFFE authenticator configured on Enterprise Appliance."
}

# ── Conjur test workload identity and secret ──────────────────────────────────

function configure_conjur_workload() {
  announce "Configuring Conjur test workload identity and secret..."

  local workload_policy
  workload_policy="$(cat <<EOF
- !policy
  id: data/spiffe-test
  body:
  - !host
    id: summon-test
    annotations:
      authn-jwt/${SPIFFE_JWT_SERVICE_ID}/sub: spiffe://example.org/workload/summon-test
      authn-jwt/${SPIFFE_JWT_OIDC_SERVICE_ID}/sub: spiffe://example.org/workload/summon-test
  - !variable spiffe-secret
  - !permit
    role: !host summon-test
    privilege: [ read, execute ]
    resource: !variable spiffe-secret
EOF
)"
  conjur_policy_load "root" "$workload_policy"

  local secret_value="spiffe-integration-test-secret-$(date +%s)"
  conjur_variable_set "data/spiffe-test/spiffe-secret" "${secret_value}"

  export CONJUR_SPIFFE_TEST_SECRET_ID="data/spiffe-test/spiffe-secret"
  export CONJUR_SPIFFE_TEST_SECRET_VALUE="${secret_value}"
  # CONJUR_AUTHN_JWT_HOST_ID tells conjur-api-go to embed the Conjur host
  # identity in the authn-jwt URL path:
  #   POST /authn-jwt/<svc>/<account>/host%2Fdata%2Fspiffe-test%2Fsummon-test/authenticate
  # Without this, Conjur derives the identity from the JWT sub claim
  # (spiffe://example.org/workload/summon-test) and looks up
  # host/spiffe://example.org/workload/summon-test which does not exist.
  export CONJUR_AUTHN_JWT_HOST_ID="host/data/spiffe-test/summon-test"
  announce "Conjur workload identity and secret configured."
}

# ── SPIFFE_ENDPOINT_SOCKET ────────────────────────────────────────────────────

function start_spire_oidc() {
  announce "Starting SPIRE OIDC Discovery Provider..."
  docker compose --profile spiffe up --no-deps -d spire-oidc
  docker compose --profile spiffe up --no-deps --wait spire-oidc

  # Wait until Conjur can reach the JWKS endpoint. Conjur and spire-oidc are on
  # the 'conjur' network so the http://spire-oidc:8085/keys URL is reachable.
  announce "Waiting for SPIRE OIDC JWKS endpoint..."
  local timeout=60 elapsed=0
  until exec_on conjur curl -sf "http://spire-oidc:8085/keys" > /dev/null 2>&1; do
    if (( elapsed >= timeout )); then
      echo "ERROR: timed out waiting for SPIRE OIDC JWKS endpoint"
      docker compose --profile spiffe logs spire-oidc || true
      exit 1
    fi
    sleep 2
    (( elapsed += 2 ))
  done
  announce "SPIRE OIDC Discovery Provider ready."
}

# ── SPIFFE_ENDPOINT_SOCKET ────────────────────────────────────────────────────

function export_spiffe_socket() {
  # The SPIRE agent socket is mounted at /run/spire-agent/api.sock inside the
  # test container (see docker-compose.yml spiffe-sockets volume).
  export SPIFFE_ENDPOINT_SOCKET="unix:///run/spire-agent/api.sock"
  announce "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
}

# ── Main ──────────────────────────────────────────────────────────────────────

start_spire_server
start_spire_agent
register_workload
start_spire_oidc
configure_conjur_workload
configure_conjur_jwt_spiffe
configure_conjur_jwt_spiffe_oidc
start_conjur_leader_for_spiffe
configure_conjur_cert_workload_enterprise
configure_conjur_cert_spiffe
export_spiffe_socket
