#!/bin/bash

APP=$1

for GOOS in darwin linux windows; do
  for GOARCH in amd64; do
    GOOS=$GOOS GOARCH=$GOARCH go build -v -o pkg/$GOOS-$GOARCH/$APP
  done
done
