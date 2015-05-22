#!/bin/bash

docker build -t cauldron/build .

projectpath="/goroot/src/github.com/conjurinc/cauldron"

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
cauldron/build \
bash -ceo pipefail "xargs -L1 go get <Godeps && \
go build ./... && \
go test -v ./... | tee test.out \
&& cat test.out | go-junit-report | tee junit.xml"
