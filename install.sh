#!/usr/bin/env bash

set -e
set -o pipefail

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

if [ ! -z "$TMPDIR" ]; then
  tmp="/tmp"
else
  tmp=$TMPDIR
fi
# secure-ish temp dir creation without having mktemp available (DDoS-able but not exploitable)
tmp_dir="$tmp/install.sh.$$"
(umask 077 && mkdir $tmp_dir) || exit 1

# do_download URL DIR
do_download() {
  echo "Downloading $1"
  if [[ $(type -t wget) ]]; then
    wget -q -O "$2" "$1" >/dev/null
  elif [[ $(type -t curl) ]]; then
    curl --fail -sSL -o "$2" "$1" &>/dev/null || true
  else
    error "Could not find wget or curl"
    return 1
  fi
}

# get_latest_version URL
get_latest_version() {
  versionloc=${tmp_dir}/summon.version
  versionfile=$(do_download "${1}" "${versionloc}")
  if [ -f "${versionloc}" ]; then
    local version=$(cat ${versionloc} | grep -o -e "[[:digit:]].[[:digit:]]*.[[:digit:]]*")
    echo "${version}"
  fi
}

LATEST_VERSION=$(get_latest_version 'https://raw.githubusercontent.com/cyberark/summon/master/pkg/summon/version.go')
# TODO: This should be removed after we publish the newest version (v0.6.9+)
if [[ -z "$LATEST_VERSION" ]]; then
  echo "Trying the old version endpoint..."
  LATEST_VERSION=$(get_latest_version 'https://raw.githubusercontent.com/cyberark/summon/master/version.go')
fi

echo "Using version number: v$LATEST_VERSION"

BASEURL="https://github.com/cyberark/summon/releases/download/"
URL=${BASEURL}"v${LATEST_VERSION}/summon-${DISTRO}-amd64.tar.gz"

ZIP_PATH="${tmp_dir}/summon.tar.gz"
do_download ${URL} ${ZIP_PATH}

echo "Installing summon v${LATEST_VERSION} into /usr/local/bin"

if sudo -h >/dev/null 2>&1; then
  sudo tar -C /usr/local/bin -zxvf ${ZIP_PATH} >/dev/null
else
  tar -C /usr/local/bin -zxvf ${ZIP_PATH} >/dev/null
fi

if [ -d "/etc/bash_completion.d" ]; then
  do_download "https://raw.githubusercontent.com/cyberark/summon/master/script/complete_summon" "/etc/bash_completion.d/complete_summon"
fi

echo "Success!"
echo "Run summon -h for usage"
