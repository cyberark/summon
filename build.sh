#!/bin/bash

APP="summon"
WORKDIR="/go/src/github.com/conjurinc/${APP}"

rm -rf pkg

docker run --rm \
-v "$PWD":$WORKDIR \
-e "GOPATH=$WORKDIR/Godeps/_workspace:/go" \
-w $WORKDIR \
golang:1.5.3 \
./compile.sh
