#!/usr/bin/env bash
# run-swa-scenarios.sh — Execute SWA E2E scenarios end-to-end with summon.
#
# Each scenario ends with a real secret retrieval from Conjur Cloud or a
# verified error response. Scenario files are auto-discovered: drop a
# bin/swa/sNN-*.sh file and it runs in the next invocation.
#
# Usage:
#   bin/swa/run-swa-scenarios.sh
#
# Required environment:
#   CONJUR_APPLIANCE_URL          Bare Conjur Cloud tenant URL
#   CONJUR_ACCOUNT                Conjur account (default: conjur)
#   CONJUR_AUTHN_JWT_SERVICE_ID   authn-jwt service ID (default: swa-e2e-jwt)
#   SPIFFE_ENDPOINT_SOCKET        unix:// path to SWA Agent socket
#
# Optional environment:
#   SWA_WORKDIR     Working directory (default: /tmp/swa-e2e)
#   TEST_VAR_PATH   Conjur variable to fetch (default: data/swa-e2e/test-secret)
#   SUMMON_BINARY   Path to summon binary (default: summon, found on PATH)

set -uo pipefail

# ---------------------------------------------------------------------------


main() {
  setup_env
  print_banner
  check_prerequisites
  run_scenarios
  print_results
}

setup_env() {
  # ── Configuration ─────────────────────────────────────────────────────────

  CONJUR_APPLIANCE_URL="${CONJUR_APPLIANCE_URL:?ERROR: CONJUR_APPLIANCE_URL is required}"
  CONJUR_ACCOUNT="${CONJUR_ACCOUNT:-conjur}"
  CONJUR_AUTHN_JWT_SERVICE_ID="${CONJUR_AUTHN_JWT_SERVICE_ID:-swa-e2e-jwt}"
  CONJUR_AUTHN_CERT_SERVICE_ID="${CONJUR_AUTHN_CERT_SERVICE_ID:-swa-e2e-cert}"
  export -n CONJUR_AUTHN_CERT_SERVICE_ID
  SPIFFE_ENDPOINT_SOCKET="${SPIFFE_ENDPOINT_SOCKET:?ERROR: SPIFFE_ENDPOINT_SOCKET is required (run bin/swa/start-swa-services.sh first)}"
  SWA_EDGE_URL="${SWA_EDGE_URL:-}"
  SWA_EDGE_CA_CERT="${SWA_EDGE_CA_CERT:-}"

  CONJUR_ENTERPRISE_URL="${CONJUR_ENTERPRISE_URL:-}"
  CONJUR_ENTERPRISE_ACCOUNT="${CONJUR_ENTERPRISE_ACCOUNT:-conjur}"
  CONJUR_ENTERPRISE_JWT_SERVICE_ID="${CONJUR_ENTERPRISE_JWT_SERVICE_ID:-${CONJUR_AUTHN_JWT_SERVICE_ID}}"
  CONJUR_ENTERPRISE_CERT_SERVICE_ID="${CONJUR_ENTERPRISE_CERT_SERVICE_ID:-${CONJUR_AUTHN_CERT_SERVICE_ID}}"
  CONJUR_ENTERPRISE_CA_CERT="${CONJUR_ENTERPRISE_CA_CERT:-}"
  CONJUR_ENTERPRISE_JWT_HOST_ID="${CONJUR_ENTERPRISE_JWT_HOST_ID:-}"
  CONJUR_ENTERPRISE_CERT_HOST_ID="${CONJUR_ENTERPRISE_CERT_HOST_ID:-}"
  CONJUR_ENTERPRISE_IGNORE_SSL="${CONJUR_ENTERPRISE_IGNORE_SSL:-false}"
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  TEST_VAR_PATH="${TEST_VAR_PATH:-data/swa-e2e/test-secret}"
  SUMMON_BINARY="${SUMMON_BINARY:-summon}"

  # Force the built-in Conjur provider for all scenarios in this script.
  export SUMMON_PROVIDER=conjur

  # ── Test-secret value (used to verify retrieval and check for leakage) ─────

  SECRET_VALUE_FILE="${SWA_WORKDIR}/test-secret-value.txt"
  if [[ -f "$SECRET_VALUE_FILE" ]]; then
    EXPECTED_SECRET="$(cat "$SECRET_VALUE_FILE")"
  else
    EXPECTED_SECRET="${TEST_SECRET_VALUE:-swa-e2e-secret-value}"
  fi

  # ── Load shared helpers ────────────────────────────────────────────────────

  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  SWA_DIR="${SCRIPT_DIR}"

  # shellcheck source=bin/swa/lib.sh
  source "${SWA_DIR}/lib.sh"

}

print_banner() {
  echo ""
  echo "========================================="
  echo " SWA E2E Scenario Runner"
  echo "========================================="
  echo ""

  SUMMON_VERSION=$("${SUMMON_BINARY}" --version 2>&1 | head -1 || echo "unknown")
  CONJUR_VERSION=$(curl -sk "${CONJUR_APPLIANCE_URL%/api}/api/info" 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('server',{}).get('version','unknown'))" 2>/dev/null \
    || echo "unknown")
  echo "  summon version : ${SUMMON_VERSION}"
  echo "  Conjur version : ${CONJUR_VERSION}"
  echo "  OS             : $(uname -srm)"
  echo "  Date (UTC)     : $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "  Conjur tenant  : ${CONJUR_APPLIANCE_URL}"
  echo "  JWT service ID : ${CONJUR_AUTHN_JWT_SERVICE_ID}"
  echo "  Cert service ID: ${CONJUR_AUTHN_CERT_SERVICE_ID}"
  echo "  Socket         : ${SPIFFE_ENDPOINT_SOCKET}"
  [[ -n "$SWA_EDGE_URL" ]] && echo "  Edge URL       : ${SWA_EDGE_URL}"
  [[ -n "$CONJUR_ENTERPRISE_URL" ]] && echo "  Enterprise URL : ${CONJUR_ENTERPRISE_URL}"
  echo ""
}

check_prerequisites() {
  SOCKET_PATH="${SPIFFE_ENDPOINT_SOCKET#unix://}"
  if [[ ! -S "$SOCKET_PATH" ]]; then
    echo "ERROR: SWA Agent socket not found at ${SOCKET_PATH}." >&2
    echo "  Run: bin/swa/start-swa-services.sh" >&2
    exit 1
  fi

  if ! command -v "${SUMMON_BINARY}" &>/dev/null; then
    echo "ERROR: summon not found at '${SUMMON_BINARY}'." >&2
    echo "  Build from source: go build -o /usr/local/bin/summon ./cmd/summon" >&2
    exit 1
  fi
}

run_scenarios() {
  # ── Run scenarios ──────────────────────────────────────────────────────────

  shopt -s nullglob
  for _scenario_file in "${SWA_DIR}"/s[0-9][0-9]-*.sh; do
    # shellcheck source=/dev/null
    source "$_scenario_file"
  done
  shopt -u nullglob
}

print_results() {
  # ── Results table ──────────────────────────────────────────────────────────

  echo ""
  echo "========================================="
  echo " Results"
  echo "========================================="
  printf "%-5s %-62s %s\n" "---" "----------------------------------------------------------" "------------------------------------"
  for entry in "${RESULTS[@]}"; do
    IFS='|' read -r status label detail <<< "$entry"
    printf "%-5s %-62s %s\n" "[${status}]" "$label" "$detail"
  done
  printf "%-5s %-62s %s\n" "---" "----------------------------------------------------------" "------------------------------------"
  echo ""
  echo "  Passed: ${PASS}   Failed: ${FAIL}   Skipped: ${SKIP}"
  echo ""

  if (( FAIL > 0 )); then
    echo "FAIL: ${FAIL} scenario(s) did not pass." >&2
    exit 1
  fi
  echo "All scenarios passed."
  exit 0
}

main "$@"
