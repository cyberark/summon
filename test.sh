#!/bin/bash

docker build -t cauldron/build .

projectpath="/goroot/src/github.com/conjurinc/cauldron"

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
cauldron/build \
bash -c "xargs -L1 go get <Godeps && go build ./... && go test -v ./... | go-junit-report tee junit.xml"
