#!/bin/bash

APP="summon"
WORKDIR="/go/src/github.com/conjurinc/${APP}"

docker run --rm \
-v "$PWD":$WORKDIR \
-e "GOPATH=$WORKDIR/Godeps/_workspace:/go" \
-w $WORKDIR \
golang:1.5.3 \
bash -ceo pipefail "export PATH=$WORKDIR/Godeps/_workspace/bin:\$PATH && \
go get -u github.com/jstemmer/go-junit-report && \
go test -v ./... | tee test.tmp \
&& cat test.tmp | go-junit-report > junit.xml && rm test.tmp"
