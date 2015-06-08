#!/bin/bash

docker build -t summon/build .

projectpath="/goroot/src/github.com/conjurinc/summon"

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
summon/build \
bash -ceo pipefail "xargs -L1 go get <Godeps && \
go build ./... && \
go test -v ./... | tee test.tmp \
&& cat test.tmp | go-junit-report > junit.xml && rm test.tmp"
