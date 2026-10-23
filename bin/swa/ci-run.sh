#!/usr/bin/env bash
# ci-run.sh — CI entry point for the SWA E2E test suite.
#
# Orchestrates the full SWA test run in Jenkins:
#   1. Downloads SWA Server + Agent from Artifactory via `jf rt download`
#      (jf CLI pre-configured by conjEntArtifactory.jfCliLoginInfrapool())
#   2. Provisions Conjur Cloud resources via setup-swa.sh
#   3. Starts SWA services (server + agent) in the background
#   4. Provisions a Conjur Cloud Edge node for X.509-SVID scenarios (S2, S9)
#   4b. Starts a self-hosted Conjur Enterprise container for S8 and S9
#   5. Runs all nine E2E scenarios via run-swa-scenarios.sh
#   6. Exits non-zero when any scenario failed
#   7. Tears down SWA services, Enterprise container, and temp files on exit
#
# Required environment (injected by the Jenkinsfile stage):
#   CONJUR_APPLIANCE_URL      Bare Conjur Cloud tenant URL (no /api suffix)
#   CONJUR_AUTHN_TOKEN        Raw Conjur session token from getConjurCloudTenant.tokens()
#
# The Jenkinsfile 'Authenticate' stage calls conjEntArtifactory.jfCliLoginInfrapool()
# before this script runs, so `jf` CLI is already configured on the agent. No
# SWA-specific Artifactory credential is needed.
#
# Optional environment:
#   SWA_VERSION             SWA binary version to download (default: 1.1.4)
#   SWA_BIN_DIR             Directory to place downloaded binaries (default: /tmp/swa-ci-bins)
#   SWA_WORKDIR             Working directory for SWA configs and sockets (default: /tmp/swa-e2e)
#   SUMMON_BINARY           Path to summon binary (default: summon, found on PATH)
#   REGISTRY_URL            Docker registry for Conjur images (default: INFRAPOOL_REGISTRY_URL or registry.tld)
#   CONJUR_ADMIN_PASSWORD   Admin password set on the ephemeral Enterprise container (default: MySecretP@ss1)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ---------------------------------------------------------------------------

main() {
  # Register teardown before setup_env so any resource allocated during setup
  # is cleaned up if the script exits unexpectedly.
  trap teardown EXIT
  setup_env
  download_swa_binaries
  provision_conjur_cloud
  start_swa_services
  update_cloud_ca_cert
  provision_edge
  provision_enterprise
  run_scenarios
  dump_container_logs
  exit "${SCENARIO_EXIT}"
}

announce() {
  printf '\n================================\n %s\n================================\n\n' "$1"
}

# ---------------------------------------------------------------------------

setup_env() {
  SWA_VERSION="${SWA_VERSION:-1.1.4}"
  SWA_BIN_DIR="${SWA_BIN_DIR:-/tmp/swa-ci-bins}"
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  SCENARIO_EXIT=0

  # Resolve the summon binary. The 'Build Release Artifacts' stage produces a
  # Linux binary at dist/goreleaser/summon-linux_linux_amd64_v1/summon relative
  # to the repo root. Fall back to 'summon' on PATH for local development.
  if [[ -z "${SUMMON_BINARY:-}" ]]; then
    GORELEASER_BIN="${REPO_ROOT}/dist/goreleaser/summon-linux_linux_amd64_v1/summon"
    SUMMON_BINARY="$([[ -x "${GORELEASER_BIN}" ]] && echo "${GORELEASER_BIN}" || echo "summon")"
  fi
  export SUMMON_BINARY

  # Remap INFRAPOOL_* vars forwarded via SendEnv=INFRAPOOL_* to the plain names
  # that setup-swa.sh and run-swa-scenarios.sh expect.
  export CONJUR_APPLIANCE_URL="${CONJUR_APPLIANCE_URL:-${INFRAPOOL_CONJUR_APPLIANCE_URL:-}}"
  export CONJUR_AUTHN_TOKEN="${CONJUR_AUTHN_TOKEN:-${INFRAPOOL_CONJUR_AUTHN_TOKEN:-}}"

  # Registry for Conjur Enterprise appliance image; infrapool agents set INFRAPOOL_REGISTRY_URL.
  export REGISTRY_URL="${REGISTRY_URL:-${INFRAPOOL_REGISTRY_URL:-registry.tld}}"

  # Admin password for the ephemeral self-hosted Enterprise container (S8/S9).
  # This container is torn down at the end of every CI run; the value only needs
  # to satisfy Conjur's password policy for the duration of the run.
  export CONJUR_ADMIN_PASSWORD="${CONJUR_ADMIN_PASSWORD:-MySecretP@ss1}"

  # Artifactory repo path for SWA customer components.
  # jf CLI must already be configured on this agent (done by the Jenkinsfile
  # 'Authenticate' stage via conjEntArtifactory.jfCliLoginInfrapool()).
  SWA_REPO="swa-customer-components-dist-dev-local"
}

teardown() {
  announce "Teardown"
  # Stop SWA services started with --detach
  if [[ -f "${SWA_WORKDIR:-}/server/swa-server.pid" ]]; then
    kill "$(cat "${SWA_WORKDIR:-}/server/swa-server.pid")" 2>/dev/null || true
  fi
  if [[ -f "${SWA_WORKDIR:-}/agent/swa-agent.pid" ]]; then
    kill "$(cat "${SWA_WORKDIR:-}/agent/swa-agent.pid")" 2>/dev/null || true
  fi
  # start-swa-services.sh handles its own tmux sessions; kill them if still running
  tmux kill-session -t swa-server 2>/dev/null || true
  tmux kill-session -t swa-agent  2>/dev/null || true
  # Tear down the self-hosted Enterprise container and its volumes.
  # cd to REPO_ROOT first so docker compose finds docker-compose.yml regardless
  # of where the EXIT trap fires.
  (cd "${REPO_ROOT}" && docker compose --profile swa-enterprise down -v 2>/dev/null) || true
  rm -rf "${SWA_BIN_DIR:-}" 2>/dev/null || true
  echo "Teardown complete."
}

download_swa_binaries() {
  announce "Downloading SWA binaries (v${SWA_VERSION})"
  mkdir -p "${SWA_BIN_DIR}"
  for component in swa-server swa-agent; do
    local archive="${component}_${SWA_VERSION}_linux_amd64.tar.gz"
    local remote="${SWA_REPO}/v${SWA_VERSION}/binaries/${archive}"
    echo "Downloading ${archive} via jf rt download ..."
    jf rt download --flat=true "${remote}" "${SWA_BIN_DIR}/"
    tar -xzf "${SWA_BIN_DIR}/${archive}" -C "${SWA_BIN_DIR}"
    chmod +x "${SWA_BIN_DIR}/${component}"
    echo "  ${component}: $("${SWA_BIN_DIR}/${component}" version 2>/dev/null || echo 'ok')"
  done
  export SWA_BIN_DIR
}

provision_conjur_cloud() {
  announce "Provisioning Conjur Cloud resources"
  # setup-swa.sh reads CONJUR_APPLIANCE_URL and CONJUR_AUTHN_TOKEN directly.
  # It creates the SWA server group, JWT authenticator, test secret, and Edge policy.
  "${SCRIPT_DIR}/setup-swa.sh"
}

start_swa_services() {
  announce "Starting SWA services"
  "${SCRIPT_DIR}/start-swa-services.sh" --detach

  # Source the env file written by start-swa-services.sh so that
  # SPIFFE_ENDPOINT_SOCKET (and related vars) are available to all subsequent
  # steps and to run-swa-scenarios.sh.
  local swa_env="${SWA_WORKDIR}/env.sh"
  if [[ -f "${swa_env}" ]]; then
    # shellcheck source=/dev/null
    source "${swa_env}"
    echo "Sourced ${swa_env}"
  fi
}

update_cloud_ca_cert() {
  announce "Updating Cloud authn-cert CA cert"

  local server_addr="${SWA_SERVER_ADDR:-localhost:8081}"
  local cert_service_id="${CONJUR_AUTHN_CERT_SERVICE_ID:-swa-e2e-cert}"
  local account="${CONJUR_ACCOUNT:-conjur}"
  local ca_var_id="conjur/authn-cert/${cert_service_id}/ca-cert"
  local api_base="${CONJUR_APPLIANCE_URL%/}"
  [[ "${api_base}" != */api ]] && api_base="${api_base}/api"

  # Fetch the current SVID and trust bundle from the SPIFFE Workload API.
  local svid_dir
  svid_dir=$(mktemp -d)
  local firefly_ca=""

  if "${SWA_BIN_DIR}/swa-agent" api fetch x509 \
      -s "${SPIFFE_ENDPOINT_SOCKET#unix://}" \
      --write "${svid_dir}" --silent 2>/dev/null; then

    local bundle_file="${svid_dir}/bundle.0.pem"
    local svid_file="${svid_dir}/svid.0.pem"

    if [[ -f "${bundle_file}" && -f "${svid_file}" ]]; then
      firefly_ca=$(python3 "${SCRIPT_DIR}/find-signing-ca.py" \
        "${bundle_file}" "${svid_file}" "${server_addr}")
    fi
  fi
  rm -rf "${svid_dir}"

  if [[ -n "${firefly_ca}" ]]; then
    local auth_conf
    auth_conf=$(mktemp)
    # shellcheck disable=SC2064 — early expansion intentional: captures the specific tempfile path.
    trap "rm -f '${auth_conf}'" RETURN
    printf 'header = "Authorization: Token token=\\"%s\\""\n' "${CONJUR_AUTHN_TOKEN}" > "${auth_conf}"
    local ca_var_enc
    ca_var_enc=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "${ca_var_id}")
    local http_code
    http_code=$(printf '%s' "${firefly_ca}" | curl -sk -o /dev/null -w "%{http_code}" \
      -X POST \
      --config "${auth_conf}" \
      -H "Content-Type: text/plain" \
      --data-binary @- \
      "${api_base}/secrets/${account}/variable/${ca_var_enc}")
    if [[ "${http_code}" =~ ^2 ]]; then
      echo "  Set ${ca_var_id} (HTTP ${http_code})."
    else
      echo "  WARNING: failed to set ${ca_var_id} (HTTP ${http_code}); cert-auth scenarios (S2/S9) may fail." >&2
    fi
  else
    echo "  WARNING: could not identify signing CA; S2 (authn-cert via Edge) may fail." >&2
  fi
}

provision_edge() {
  announce "Provisioning Conjur Cloud Edge"

  # provision-edge.sh writes SWA_EDGE_URL and SWA_EDGE_CA_CERT to
  # ${SWA_WORKDIR}/edge-env.sh for sourcing below.
  # Edge name must use underscores, not hyphens.
  "${SCRIPT_DIR}/provision-edge.sh"

  # Source Edge environment so run-swa-scenarios.sh picks up SWA_EDGE_URL and
  # SWA_EDGE_CA_CERT for the s02 (X.509-SVID via Edge) scenario.
  local edge_env="${SWA_WORKDIR}/edge-env.sh"
  if [[ -f "${edge_env}" ]]; then
    # shellcheck source=/dev/null
    source "${edge_env}"
    echo "Edge URL: ${SWA_EDGE_URL:-<not set>}"
  else
    echo "WARNING: ${edge_env} not found; s02 (X.509-SVID) scenario will skip." >&2
  fi
}

provision_enterprise() {
  announce "Provisioning self-hosted Conjur Enterprise"

  # setup-swa-enterprise.sh starts a conjur-leader container via
  # docker compose --profile swa-enterprise, configures authn-jwt and authn-cert,
  # and writes ${SWA_WORKDIR}/enterprise-env.sh with CONJUR_ENTERPRISE_* vars.
  # REGISTRY_URL and CONJUR_ADMIN_PASSWORD are exported in setup_env().
  "${SCRIPT_DIR}/setup-swa-enterprise.sh"

  local enterprise_env="${SWA_WORKDIR}/enterprise-env.sh"
  if [[ -f "${enterprise_env}" ]]; then
    # shellcheck source=/dev/null
    source "${enterprise_env}"
    echo "Enterprise URL: ${CONJUR_ENTERPRISE_URL:-<not set>}"
  else
    echo "WARNING: ${enterprise_env} not found; S8/S9 scenarios will skip." >&2
  fi
}

run_scenarios() {
  announce "Running SWA E2E scenarios"

  set +e
  "${SCRIPT_DIR}/run-swa-scenarios.sh"
  SCENARIO_EXIT=$?
  set -e
}

dump_container_logs() {
  announce "Enterprise container logs (last 40 lines)"
  docker compose --profile swa-enterprise logs conjur-leader --tail=40 2>/dev/null || true

  announce "Edge container logs (last 40 lines)"
  local edge_container="${SWA_EDGE_CONTAINER:-sm_edge_swa_e2e}"
  docker logs "${edge_container}" --tail=40 2>&1 || true
}

# ---------------------------------------------------------------------------

main "$@"
