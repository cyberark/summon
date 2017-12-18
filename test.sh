#!/bin/bash

echo "Running unit tests"

docker-compose build --pull summon-builder

docker-compose run --rm --entrypoint bash summon-builder \
  -c 'go test -v ./... | tee junit.output && cat junit.output | go-junit-report --set-exit-code=true > output/junit.xml'
