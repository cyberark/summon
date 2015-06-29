#!/usr/bin/env bash

set -e

ARCH=`uname -m`

if [ "${ARCH}" != "x86_64" ]; then
  echo "summon only works on 64-bit systems"
  echo "exiting installer"
  exit 1
fi

DISTRO=`uname | tr "[:upper:]" "[:lower:]"`

if [ "${DISTRO}" != "linux" ] && [ "${DISTRO}" != "darwin"  ]; then
  echo "This installer only supports Linux and OSX"
  echo "exiting installer"
  exit 1
fi

LATEST_VERSION=`curl -sSL https://raw.githubusercontent.com/conjurinc/summon/master/version.go | grep -o -e "\d.\d.\d"`
BASEURL="https://github.com/conjurinc/summon/releases/download/"
URL=${BASEURL}"v${LATEST_VERSION}/summon_v${LATEST_VERSION}_${DISTRO}_amd64.zip"

echo "Installing summon v${LATEST_VERSION} into /usr/local/bin"

ZIP_PATH="/tmp/summon.zip"
curl -sSL $URL -o ${ZIP_PATH}

unzip -q -o ${ZIP_PATH} -d /usr/local/bin

echo "Success!"
echo "Run summon -h for usage"
