#!/bin/bash
set -eo pipefail

mkdir -p output/integration

echo "Running integration tests..."

go test -v -timeout 30m -p=1 -tags integration \
  ./pkg/conjurprovider/... \
  | go-junit-report -set-exit-code > output/integration/junit.xml
