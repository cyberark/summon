#!/bin/bash -e

./test.sh
./build.sh

cp ./pkg/linux-amd64/summon .
pushd acceptance
make
popd

sudo chmod -R 777 pkg/
./package.sh
