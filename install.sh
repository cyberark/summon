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

if test "x$TMPDIR" = "x"; then
  tmp="/tmp"
else
  tmp=$TMPDIR
fi
# secure-ish temp dir creation without having mktemp available (DDoS-able but not expliotable)
tmp_dir="$tmp/install.sh.$$"
(umask 077 && mkdir $tmp_dir) || exit 1

# do_download URL DIR
function do_download(){
  echo "Downloading $1"
  if   [[ $(type -t wget) ]]; then wget -q -O "$2" "$1" >/dev/null
  elif [[ $(type -t curl) ]]; then curl -sSL -o "$2" "$1"
  else
    error "Could not find wget or curl"
    return 1
  fi
}

# get_latest_version URL
get_latest_version() {
  versionloc=${tmp_dir}/summon.version
  versionfile=$(do_download ${1} ${versionloc})
  local version=$(cat ${versionloc} | grep -o -e "[[:digit:]].[[:digit:]]*.[[:digit:]]*")
  echo "${version}"
}

LATEST_VERSION=$(get_latest_version 'https://raw.githubusercontent.com/cyberark/summon/master/version.go')
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
