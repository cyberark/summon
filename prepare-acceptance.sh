#!/bin/bash -e

GLOB='./output/summon_*_linux_amd64'

for file in $GLOB ; do
  cp $file summon
done
