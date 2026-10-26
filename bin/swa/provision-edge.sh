#!/usr/bin/env bash
# provision-edge.sh — Create a Conjur Cloud Edge instance and start it locally via Docker.
#
# The Edge installer token is obtained programmatically via the Conjur Cloud API.
# No UI access required.
#
# Edge names must use underscores, not hyphens. A hyphen causes HTTP 422.
#
# Usage:
#   bin/swa/provision-edge.sh [--name <edge_name>] [--port <host_port>] [--restart]
#
# Options:
#   --name      Edge instance name — underscores only (default: sm_edge_swa_e2e)
#   --port      Host port mapping to container 8443 (default: 443)
#   --restart   Restart an existing container without reprovisioning
#
# Environment:
#   CONJURRC_FILE   Path to conjurrc (default: ~/.conjurrc-swa)
#   NETRC_FILE      Path to netrc   (default: ~/.netrc-swa)
#   EDGE_IMAGE      Docker image    (default: ${REGISTRY_URL:-registry.tld}/cyberark/edge:latest)
#   EDGE_DATA_DIR   Bind-mount dir  (default: /tmp/swa-e2e/edge)

set -euo pipefail


main() {
  setup_env "$@"
  validate_edge_name
  read_conjur_credentials
  if [[ "$RESTART_ONLY" == "true" ]]; then
    restart_container
    exit 0
  fi
  create_edge_in_conjur
  fetch_installer_token
  prepare_data_dir
  remove_stale_container
  start_edge_container
  extract_tls_cert
  write_edge_env
  print_summary
}

announce() { echo ""; echo "=== ${*} ==="; echo ""; }

setup_env() {
  CONJURRC_FILE="${CONJURRC_FILE:-${HOME}/.conjurrc-swa}"
  NETRC_FILE="${NETRC_FILE:-${HOME}/.netrc-swa}"
  EDGE_NAME="sm_edge_swa_e2e"
  EDGE_PORT=443
  RESTART_ONLY=false
  # Use REGISTRY_URL when set (exported by ci-run.sh from INFRAPOOL_REGISTRY_URL on
  # infrapool agents); fall back to the internal mirror default for local dev.
  EDGE_IMAGE="${EDGE_IMAGE:-${REGISTRY_URL:-registry.tld}/cyberark/edge:latest}"
  EDGE_DATA_DIR="${EDGE_DATA_DIR:-/tmp/swa-e2e/edge}"

  while [[ $# -gt 0 ]]; do
    case $1 in
      --name)    EDGE_NAME="$2";    shift 2 ;;
      --port)    EDGE_PORT="$2";    shift 2 ;;
      --restart) RESTART_ONLY=true; shift ;;
      *) echo "Unknown flag: $1"; exit 1 ;;
    esac
  done

  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  source "${SCRIPT_DIR}/setup-lib.sh"
}

validate_edge_name() {
  # Underscores only in edge name.
  if echo "$EDGE_NAME" | grep -q '-'; then
    echo "ERROR: Edge name '${EDGE_NAME}' contains hyphens. Use underscores only (Conjur API constraint)." >&2
    exit 1
  fi
}

read_conjur_credentials() {
  # ── Read Conjur URL ────────────────────────────────────────────────────────

  # CI path: accept URL directly from env (exported by ci-run.sh from INFRAPOOL_*).
  if [[ -n "${CONJUR_APPLIANCE_URL:-}" ]]; then
    API_BASE="${CONJUR_APPLIANCE_URL%/}"
    [[ "${API_BASE}" != */api ]] && API_BASE="${API_BASE}/api"
  elif [[ -f "$CONJURRC_FILE" ]]; then
    API_BASE=$(appliance_url_from_conjurrc "$CONJURRC_FILE")
    [[ -z "$API_BASE" ]] && { echo "ERROR: appliance_url not found in ${CONJURRC_FILE}" >&2; exit 1; }
  else
    echo "ERROR: Set CONJUR_APPLIANCE_URL or provide ${CONJURRC_FILE}." >&2; exit 1
  fi

  # ── Read Conjur token ──────────────────────────────────────────────────────

  # CI path: conjToken from getConjurCloudTenant.tokens() is already the base64-encoded
  # Conjur access token — use it directly without additional encoding.
  if [[ -n "${CONJUR_AUTHN_TOKEN:-}" ]]; then
    CONJUR_TOKEN="${CONJUR_AUTHN_TOKEN}"
  else
    CONJUR_TOKEN=$(oidc_token_b64 "$NETRC_FILE") || {
      echo "ERROR: no OIDC token in ${NETRC_FILE}. Run: CONJURRC=${CONJURRC_FILE} conjur login" >&2; exit 1
    }
  fi

  AUTH_HEADER="Authorization: Token token=\"${CONJUR_TOKEN}\""
}

restart_container() {
  # ── Restart only path ──────────────────────────────────────────────────────

  echo "==> Restarting Edge container '${EDGE_NAME}'..."
  docker restart "${EDGE_NAME}"
  echo "  Waiting for Edge readiness..."
  deadline=$(($(date +%s) + 30))
  while (( $(date +%s) < deadline )); do
    code=$(curl -sk -o /dev/null -w "%{http_code}" "https://localhost:${EDGE_PORT}/health" 2>/dev/null || echo 0)
    [[ "$code" == "200" ]] && break
    sleep 2
  done
  echo "  Edge ready."
}

create_edge_in_conjur() {
  # ── Create Edge in Conjur Cloud ────────────────────────────────────────────

  announce "Creating Edge '${EDGE_NAME}' in Conjur Cloud"
  HTTP_CODE=$(curl -sk -o /tmp/swa-edge-create.json -w "%{http_code}" \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -d "{\"edge_name\":\"${EDGE_NAME}\"}" \
    "${API_BASE}/edge/conjur")

  case "$HTTP_CODE" in
    201) echo "  Edge created (201)." ;;
    409) echo "  Edge already exists (409) — continuing." ;;
    *)
      echo "ERROR: POST /api/edge/conjur returned HTTP ${HTTP_CODE}" >&2
      cat /tmp/swa-edge-create.json >&2; exit 1 ;;
  esac
}

fetch_installer_token() {
  # ── Fetch installer token ──────────────────────────────────────────────────

  announce "Fetching Edge installer token"
  INSTALLER_TOKEN=$(curl -sk \
    -H "${AUTH_HEADER}" \
    "${API_BASE}/edge/edge-creds/conjur/${EDGE_NAME}" 2>/dev/null)

  [[ -z "$INSTALLER_TOKEN" ]] && { echo "ERROR: empty token from edge-creds endpoint." >&2; exit 1; }
  HOST_ID=$(echo "$INSTALLER_TOKEN" | base64 -d 2>/dev/null | cut -d: -f1)
  echo "  Edge host ID: ${HOST_ID}"
}

prepare_data_dir() {
  # ── Prepare data directory ─────────────────────────────────────────────────

  # Pre-create the subdirectories that the Edge container creates on startup.
  # The Edge container runs as a non-root user whose UID does not match the
  # Jenkins agent user.  Without world-write on the bind-mount subdirectories
  # the container panics immediately with "permission denied" on its log file.
  # 0777 is required here; chown-to-UID is not an option because the Edge
  # container UID is managed by the SWA team and may change across versions.
  mkdir -p "${EDGE_DATA_DIR}" \
           "${EDGE_DATA_DIR}/logs" \
           "${EDGE_DATA_DIR}/data" \
           "${EDGE_DATA_DIR}/certs"
  chmod -R 0777 "${EDGE_DATA_DIR}"
  echo "  Data directory: ${EDGE_DATA_DIR}"
}

remove_stale_container() {
  # ── Remove any stale container ─────────────────────────────────────────────

  if docker inspect "${EDGE_NAME}" &>/dev/null; then
    echo "  Removing existing container '${EDGE_NAME}'..."
    docker rm -f "${EDGE_NAME}"
  fi
}

start_edge_container() {
  # ── Start Edge Docker container ───────────────────────────────────────────

  announce "Starting Edge Docker container"
  docker run -d \
    --name "${EDGE_NAME}" \
    -e "CONJUR_CLOUD_URL=${API_BASE}" \
    -e "EDGE_INITIAL_CREDS=${INSTALLER_TOKEN}" \
    -e "COMMON_NAME=localhost" \
    -e "SAN=localhost" \
    -p "${EDGE_PORT}:8443" \
    -p 444:8444 \
    --mount "type=bind,source=${EDGE_DATA_DIR},target=/opt/edge/data" \
    --restart unless-stopped \
    "${EDGE_IMAGE}"

  # Wait for Edge TLS port to accept connections — more reliable than log scanning
  # because log messages vary across Edge versions.
  echo "  Waiting for Edge TLS port ${EDGE_PORT} (up to 180s)..."
  deadline=$(($(date +%s) + 180))
  started=false
  while (( $(date +%s) < deadline )); do
    if openssl s_client -connect "localhost:${EDGE_PORT}" -servername localhost \
        </dev/null 2>/dev/null | grep -q 'BEGIN CERTIFICATE'; then
      started=true; break
    fi
    sleep 3
  done

  if [[ "$started" != "true" ]]; then
    echo "  WARNING: Edge TLS port not responding after 180s." >&2
    echo "  Last 40 lines of Edge container logs:" >&2
    docker logs --tail 40 "${EDGE_NAME}" 2>&1 | sed 's/^/    /' >&2 || true
  fi
}

extract_tls_cert() {
  # ── Extract self-signed TLS cert ───────────────────────────────────────────

  echo ""
  echo "=== Edge TLS certificate ==="

  CERT_PATH="${EDGE_DATA_DIR}/edge-tls.crt"

  # By this point start_edge_container() has already confirmed the TLS port is
  # up. Try a few times in case the cert isn't fully ready yet.
  EDGE_CERT=""
  for _attempt in 1 2 3 4 5; do
    EDGE_CERT=$(echo | openssl s_client \
      -connect "localhost:${EDGE_PORT}" \
      -servername localhost 2>/dev/null \
      | openssl x509 2>/dev/null) || true
    [[ -n "$EDGE_CERT" ]] && break
    sleep 3
  done

  if [[ -n "$EDGE_CERT" ]]; then
    echo "$EDGE_CERT" > "$CERT_PATH"
    CN=$(echo "$EDGE_CERT" | openssl x509 -noout -subject 2>/dev/null | sed 's/.*CN=//') || true
    echo "  CN: ${CN}"
    echo "  Written to: ${CERT_PATH}"
  else
    echo "  WARNING: could not fetch TLS cert — Edge TLS port not responding."
  fi
}

write_edge_env() {
  # ── Write edge-env.sh for ci-run.sh to source ─────────────────────────────

  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  EDGE_ENV="${SWA_WORKDIR}/edge-env.sh"
  mkdir -p "${SWA_WORKDIR}"
  {
    echo "export SWA_EDGE_URL=\"https://localhost:${EDGE_PORT}/api\""
    echo "export SWA_EDGE_CONTAINER=\"${EDGE_NAME}\""
    if [[ -f "${CERT_PATH:-}" ]]; then
      # Inline the cert so the sourcing script doesn't need the file path.
      echo "export SWA_EDGE_CA_CERT=\"$(cat "${CERT_PATH}")\""
    fi
  } > "${EDGE_ENV}"
  echo "  Written: ${EDGE_ENV}"
}

print_summary() {
  # ── Summary ───────────────────────────────────────────────────────────────

  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  EDGE_ENV="${SWA_WORKDIR}/edge-env.sh"

  echo ""
  echo "=== Edge ready ==="
  echo ""
  echo "  SWA_EDGE_URL=https://localhost:${EDGE_PORT}/api"
  if [[ -f "${CERT_PATH:-}" ]]; then
    echo "  SWA_EDGE_CA_CERT=$(cat "${CERT_PATH}")"
  fi
  echo "  SWA_EDGE_CONTAINER=${EDGE_NAME}"
  echo ""
  echo "  To export for use with run-swa-scenarios.sh:"
  echo "    source ${EDGE_ENV}"
}

main "$@"
