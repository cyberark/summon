#!/bin/bash

docker build -t cauldron/build .

docker run --rm \
-v "$(pwd)":/usr/src/cauldron \
-w /usr/src/cauldron \
cauldron/build \
gpm && gpm local name github.com/conjur/cauldron; go test ./...
