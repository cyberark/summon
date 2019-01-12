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
  if [[ $(command -v wget) ]]; then
    wget -q -O "$2" "$1" >/dev/null
  elif [[ $(command -v curl) ]]; then
    curl --fail -sSL -o "$2" "$1" &>/dev/null || true
  else
    error "Could not find wget or curl"
    return 1
  fi
}

# get_latest_version
get_latest_version() {
  local latest;
  if [[ $(command -v wget) ]]; then
    latest=$(wget -q -O - "https://api.github.com/repos/cyberark/summon/releases/latest")
  elif [[ $(command -v curl) ]]; then
    latest=$(curl --silent "https://api.github.com/repos/cyberark/summon/releases/latest")
  else
    error "Could not find curl"
    return 1
  fi
  
  echo "$latest" | # Get latest release from GitHub api
    grep '"tag_name":' |                                            # Get tag line
    sed -E 's/.*"([^"]+)".*/\1/'                                    # Pluck JSON value
}

LATEST_VERSION=$(get_latest_version)

echo "Using version number: v$LATEST_VERSION"

BASEURL="https://github.com/cyberark/summon/releases/download/"
URL=${BASEURL}"${LATEST_VERSION}/summon-${DISTRO}-amd64.tar.gz"

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
