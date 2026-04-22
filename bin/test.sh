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
  runTests
}

function prepareAWS() {
  if [[ "${TEST_AWS}" == "true" ]]; then
    if [[ -z "${CONJUR_AWS_ACCOUNT_ID:-}" ]] || [[ -z "${CONJUR_AWS_ROLE_NAME:-}" ]]; then
      echo "ERROR: CONJUR_AWS_ACCOUNT_ID and CONJUR_AWS_ROLE_NAME must be set when TEST_AWS=true."
      echo "  These are expected to be provided by summon via secrets.yml (ci/aws/account and ci/aws/instance-read-jenkins-executor-username)."
      exit 1
    fi
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
    -e TEST_AZURE \
    -e AZURE_SUBSCRIPTION_ID \
    -e AZURE_RESOURCE_GROUP \
    -e AZURE_TENANT_ID \
    -e CONJUR_AWS_ACCOUNT_ID \
    -e CONJUR_AWS_ROLE_NAME \
    "$service" || failed
}

main
