#!/bin/bash -e

export CONJUR_ACCOUNT=cucumber

export REGISTRY_URL=${INFRAPOOL_REGISTRY_URL:-"docker.io"}

source ./bin/functions.sh

function finish {
  echo 'Removing environment'
  echo '-----'
  docker compose down -v
}
trap finish EXIT

function failed {
  echo 'TESTS FAILED'
  echo '-----'
  echo 'Conjur logs:'
  docker compose logs conjur || true
  echo '-----'
  exit 1
}

function main() {
  startConjur
  initEnvironment
  prepareAWS
  prepareGCP
  runTests
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

  mkdir -p output/integration

  docker compose build --pull \
    --build-arg TARGETARCH="${TARGETARCH:-amd64_v1}" \
    --build-arg BINARYPATH="${binarypath}" \
    $service

  docker compose run --rm \
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
    "$service" || failed
}

main
