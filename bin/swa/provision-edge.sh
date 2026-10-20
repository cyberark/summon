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
#   EDGE_IMAGE      Docker image    (default: localhost/cyberark/edge:1.9.1)
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
  print_summary
}

announce() { echo ""; echo "=== ${*} ==="; echo ""; }

setup_env() {
  CONJURRC_FILE="${CONJURRC_FILE:-${HOME}/.conjurrc-swa}"
  NETRC_FILE="${NETRC_FILE:-${HOME}/.netrc-swa}"
  EDGE_NAME="sm_edge_swa_e2e"
  EDGE_PORT=443
  RESTART_ONLY=false
  EDGE_IMAGE="${EDGE_IMAGE:-localhost/cyberark/edge:1.9.1}"
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
  # ── Read Conjur URL and token ──────────────────────────────────────────────

  API_BASE=$(appliance_url_from_conjurrc "$CONJURRC_FILE")
  [[ -z "$API_BASE" ]] && { echo "ERROR: appliance_url not found in ${CONJURRC_FILE}" >&2; exit 1; }

  CONJUR_TOKEN=$(oidc_token_b64 "$NETRC_FILE") || {
    echo "ERROR: no OIDC token in ${NETRC_FILE}. Run: CONJURRC=${CONJURRC_FILE} conjur login" >&2; exit 1
  }

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

  mkdir -p "${EDGE_DATA_DIR}"
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

  echo "  Waiting for Edge to initialise and replicate (up to 45s)..."
  deadline=$(($(date +%s) + 45))
  started=false
  while (( $(date +%s) < deadline )); do
    if docker logs "${EDGE_NAME}" 2>&1 | grep -qiE "Replicated|Starting server|replicated"; then
      started=true; break
    fi
    sleep 2
  done

  if [[ "$started" != "true" ]]; then
    echo "  WARNING: Edge may not have replicated yet — check: docker logs ${EDGE_NAME}"
  fi
}

extract_tls_cert() {
  # ── Extract self-signed TLS cert ───────────────────────────────────────────

  echo ""
  echo "=== Edge TLS certificate ==="
  EDGE_CERT=$(echo | openssl s_client \
    -connect "localhost:${EDGE_PORT}" \
    -servername localhost 2>/dev/null \
    | openssl x509 2>/dev/null)

  CERT_PATH="${EDGE_DATA_DIR}/edge-tls.crt"
  if [[ -n "$EDGE_CERT" ]]; then
    echo "$EDGE_CERT" > "$CERT_PATH"
    CN=$(echo "$EDGE_CERT" | openssl x509 -noout -subject 2>/dev/null | sed 's/.*CN=//')
    echo "  CN: ${CN}"
    echo "  Written to: ${CERT_PATH}"
  else
    echo "  WARNING: could not fetch TLS cert yet — Edge may still be starting. Re-run after a few seconds."
  fi
}

print_summary() {
  # ── Summary ───────────────────────────────────────────────────────────────

  echo ""
  echo "=== Edge ready ==="
  echo ""
  echo "  SWA_EDGE_URL=https://localhost:${EDGE_PORT}/api"
  if [[ -f "$CERT_PATH" ]]; then
    echo "  SWA_EDGE_CA_CERT=$(cat "$CERT_PATH")"
  fi
  echo "  SWA_EDGE_CONTAINER=${EDGE_NAME}"
  echo ""
  echo "  To export for use with run-swa-scenarios.sh:"
  echo "    export SWA_EDGE_URL=https://localhost:${EDGE_PORT}/api"
  echo "    export SWA_EDGE_CONTAINER=${EDGE_NAME}"
  if [[ -f "$CERT_PATH" ]]; then
    echo "    export SWA_EDGE_CA_CERT=\$(cat ${CERT_PATH})"
  fi
}

main "$@"
