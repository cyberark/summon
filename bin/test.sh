#!/bin/bash -e

export CONJUR_ACCOUNT=cucumber

export REGISTRY_URL="${REGISTRY_URL:-${INFRAPOOL_REGISTRY_URL:-docker.io}}"

source ./bin/functions.sh

function finish {
  echo 'Removing environment'
  echo '-----'
  if docker compose ps -q conjur 2>/dev/null | grep -q .; then
    mkdir -p output/integration
    docker compose logs conjur > output/integration/conjur-logs.txt 2>&1 || true
  fi
  if [[ "${TEST_CERT:-false}" == "true" ]]; then
    docker compose --profile cert logs conjur-leader >> output/integration/conjur-leader-logs.txt 2>&1 || true
    docker compose --profile cert down -v --remove-orphans || true
  fi
  docker compose down -v
}
trap finish EXIT

function failed {
  echo 'TESTS FAILED'
  echo '-----'
  echo 'Conjur logs:'
  docker compose logs conjur || true
  if [[ "${TEST_CERT:-false}" == "true" ]]; then
    echo 'Conjur Enterprise logs:'
    docker compose --profile cert logs conjur-leader || true
  fi
  echo '-----'
  exit 1
}

function main() {
  startConjur
  initEnvironment
  prepareAWS
  prepareGCP
  prepareCertIfNeeded
  runTests
}

function prepareCertIfNeeded() {
  if [[ "${INFRAPOOL_TEST_CERT}" == "true" || "${TEST_CERT}" == "true" ]]; then
    export TEST_CERT=true
    # shellcheck source=bin/setup-cert-auth.sh
    source ./bin/setup-cert-auth.sh
  fi
}

function prepareAWS() {
  if [[ "${INFRAPOOL_TEST_AWS}" == "true" ]]; then
    if [[ -z "${CONJUR_AWS_ACCOUNT_ID:-}" ]] || [[ -z "${CONJUR_AWS_ROLE_NAME:-}" ]]; then
      echo "ERROR: CONJUR_AWS_ACCOUNT_ID and CONJUR_AWS_ROLE_NAME must be set when TEST_AWS=true (provided by summon via secrets.yml)."
      exit 1
    fi
  fi
}

function prepareGCP() {
  if [[ "${INFRAPOOL_TEST_GCP}" == "true" ]]; then
    GCP_PROJECT_ID=""
    GCP_ID_TOKEN=""
    if [[ -f "gcp/project-id" ]]; then
      read -r GCP_PROJECT_ID < "gcp/project-id"
    fi
    if [[ -f "gcp/token" ]]; then
      read -r GCP_ID_TOKEN < "gcp/token"
    fi
    if [[ -z "$GCP_PROJECT_ID" || -z "$GCP_ID_TOKEN" ]]; then
      echo "GCP_PROJECT_ID and GCP_ID_TOKEN must be set to run GCP tests"
      failed
    fi
    export GCP_PROJECT_ID
    export GCP_ID_TOKEN
  fi
}

function runTests() {
  local service=test

  local arch="${TARGETARCH:-amd64_v1}"
  local binarypath="dist/goreleaser/summon-linux_linux_${arch}/summon"

  # Guard: fail fast with a clear message if the pre-built binary is missing.
  # Build it first with: ./build --snapshot
  if [[ ! -f "${binarypath}" ]]; then
    echo "ERROR: Pre-built summon binary not found at ${binarypath}"
    echo "Build it first with: ./build --snapshot"
    exit 1
  fi

  # Map Jenkinsfile infra env vars to test-level env vars
  export TEST_AWS="${INFRAPOOL_TEST_AWS:-}"
  export TEST_AZURE="${INFRAPOOL_TEST_AZURE:-}"
  export TEST_GCP="${INFRAPOOL_TEST_GCP:-}"
  export TEST_CERT="${TEST_CERT:-${INFRAPOOL_TEST_CERT:-false}}"

  mkdir -p output/integration

  if [[ "$TEST_CERT" == "true" ]]; then
    if [[ -z "${CERT_TMPDIR:-}" || ! -d "${CERT_TMPDIR}" ]]; then
      echo "ERROR: TEST_CERT=true but CERT_TMPDIR is missing or not a directory (PKI setup failed?)"
      exit 1
    fi
  fi

  # Mount generated client certs into the summon test container only (not conjur-leader).
  CERT_EXTRA_ARGS=()
  if [[ "$TEST_CERT" == "true" ]]; then
    CERT_EXTRA_ARGS+=("-v" "$CERT_TMPDIR:/certs:ro")
  fi

  docker compose build --pull \
    --build-arg TARGETARCH="${TARGETARCH:-amd64_v1}" \
    --build-arg BINARYPATH="${binarypath}" \
    $service

  docker compose run --rm \
    "${CERT_EXTRA_ARGS[@]}" \
    -e CONJUR_ACCOUNT \
    -e CONJUR_APPLIANCE_URL \
    -e CONJUR_AUTHN_LOGIN \
    -e CONJUR_AUTHN_API_KEY \
    -e CONJUR_ADMIN_LOGIN \
    -e CONJUR_ADMIN_API_KEY \
    -e TEST_AWS \
    -e TEST_GCP \
    -e GCP_PROJECT_ID \
    -e GCP_ID_TOKEN \
    -e TEST_AZURE \
    -e AZURE_SUBSCRIPTION_ID \
    -e AZURE_RESOURCE_GROUP \
    -e AZURE_TENANT_ID \
    -e CONJUR_AWS_ACCOUNT_ID \
    -e CONJUR_AWS_ROLE_NAME \
    -e TEST_CERT \
    -e CONJUR_CERT_APPLIANCE_URL \
    -e CONJUR_CERT_AUTHN_LOGIN \
    -e CONJUR_CERT_AUTHN_API_KEY \
    -e CONJUR_CERT_SSL_CERTIFICATE \
    -e CONJUR_AUTHN_CERT_FILE \
    -e CONJUR_AUTHN_CERT_KEY_FILE \
    -e TEST_CERT_CA_CERT \
    -e CONJUR_SSL_CERTIFICATE \
    "$service" || failed
}

main
