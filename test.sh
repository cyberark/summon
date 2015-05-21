#!/bin/bash

docker build -t cauldron/build .

projectpath="/goroot/src/github.com/conjurinc/cauldron"

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
-e "CAULDRON_PROVIDER=provider" \
cauldron/build \
bash -c "xargs -L1 go get <Godeps && go install ./... && go test -v ./... | go-junit-report tee junit.xml"
