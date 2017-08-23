#!/usr/bin/env bash

DISTRO=`uname | tr "[:upper:]" "[:lower:]"`

# Get the version from the command line
if [ -z $VERSION ]; then
    VERSION=$(git describe --abbrev=0 --tags)
fi

cp ./output/summon_$VERSION_$DISTRO_amd64 summon
