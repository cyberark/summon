#!/bin/bash

rm -rf pkg

docker build -t cauldron/build .

docker run --rm \
-v "$(pwd)":/usr/src/cauldron \
-w /usr/src/cauldron \
cauldron/build
