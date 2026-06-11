#!/bin/bash
set -eo pipefail

mkdir -p output/integration

echo "Running integration tests..."

test_run_args=()
if [[ "${TEST_SAAS}" == "true" ]]; then
  # SaaS stage targets an ephemeral tenant; run only SaaS JWT tests here.
  test_run_args=(-run '^TestAuthnJWTSaaSIntegration$')
fi

# ${arr[@]+"${arr[@]}"} avoids set -u failure on empty arrays in older bash.
go test -v -timeout 30m -p=1 -tags integration \
  ${test_run_args[@]+"${test_run_args[@]}"} \
  ./pkg/conjurprovider/... \
  | go-junit-report -set-exit-code > output/integration/junit.xml
