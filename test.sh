#!/bin/bash

app="summon"

docker build -t summon/build .

projectpath="/goroot/src/github.com/conjurinc/summon"

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
-e "GOPATH=/goroot/src/github.com/conjurinc/${app}/Godeps/_workspace:/goroot" \
summon/build \
bash -ceo pipefail "export PATH=/goroot/src/github.com/conjurinc/${app}/Godeps/_workspace/bin:\$PATH && \
go get -u github.com/jstemmer/go-junit-report && \
go test -v ./... | tee test.tmp \
&& cat test.tmp | go-junit-report > junit.xml && rm test.tmp"
