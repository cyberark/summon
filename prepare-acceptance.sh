#!/usr/bin/env bash

set -e

for file in ./output/summon_*_linux_amd64 ; do
  cp $file summon
done
