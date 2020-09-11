#!/usr/bin/env bash

set -e
set -o pipefail

error() {
  echo "ERROR: $@" 1>&2
  echo "Exiting installer" 1>&2
  exit 1
}

ARCH=`uname -m`

if [ "${ARCH}" != "x86_64" ]; then
  error "summon only works on 64-bit systems"
fi

DISTRO=`uname | tr "[:upper:]" "[:lower:]"`

if [ "${DISTRO}" != "linux" ] && [ "${DISTRO}" != "darwin"  ]; then
  error "This installer only supports Linux and OSX"
fi

tmp="/tmp"
if [ ! -z "$TMPDIR" ]; then
  tmp=$TMPDIR
fi

# secure-ish temp dir creation without having mktemp available (DDoS-able but not exploitable)
tmp_dir="$tmp/install.sh.$$"
(umask 077 && mkdir $tmp_dir) || exit 1

# do_download URL DIR
do_download() {
  echo "Downloading $1"
  if [[ $(command -v wget) ]]; then
    wget -q -O "$2" "$1" >/dev/null
  elif [[ $(command -v curl) ]]; then
    curl --fail -sSL -o "$2" "$1" &>/dev/null || true
  else
    error "Could not find wget or curl"
  fi
}

# get_latest_version
get_latest_version() {
  local LATEST_VERSION_URL="https://api.github.com/repos/cyberark/summon/releases/latest"
  local latest_payload

  if [[ $(command -v wget) ]]; then
    latest_payload=$(wget -q -O - "$LATEST_VERSION_URL")
  elif [[ $(command -v curl) ]]; then
    latest_payload=$(curl --fail -sSL "$LATEST_VERSION_URL")
  else
    error "Could not find wget or curl"
  fi

  echo "$latest_payload" | # Get latest release from GitHub api
    grep '"tag_name":' | # Get tag line
    sed -E 's/.*"([^"]+)".*/\1/' # Pluck JSON value
}

LATEST_VERSION=$(get_latest_version)

echo "Using version number: v$LATEST_VERSION"

BASEURL="https://github.com/cyberark/summon/releases/download/"
URL=${BASEURL}"${LATEST_VERSION}/summon-${DISTRO}-amd64.tar.gz"

ZIP_PATH="${tmp_dir}/summon.tar.gz"
do_download ${URL} ${ZIP_PATH}

echo "Installing summon v${LATEST_VERSION} into /usr/local/bin"

if sudo -h >/dev/null 2>&1; then
  sudo tar -C /usr/local/bin -o -zxvf ${ZIP_PATH} >/dev/null
else
  tar -C /usr/local/bin -o -zxvf ${ZIP_PATH} >/dev/null
fi

if [ -d "/etc/bash_completion.d" ]; then
  do_download "https://raw.githubusercontent.com/cyberark/summon/master/script/complete_summon" "/etc/bash_completion.d/complete_summon"
fi

echo "Success!"
echo "Run summon -h for usage"
