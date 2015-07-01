#!/usr/bin/env bash

set -e

if [[ ! $(type -t unzip) ]]; then
  echo "unzip is not installed, please install to continue"
  exit 1
fi

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
  if   [[ $(type -t wget) ]]; then wget -q -c -O "$2" "$1" >/dev/null
  elif [[ $(type -t curl) ]]; then curl -sSL -C -o "$2" "$1"
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

LATEST_VERSION=$(get_latest_version 'https://raw.githubusercontent.com/conjurinc/summon/master/version.go')
BASEURL="https://github.com/conjurinc/summon/releases/download/"
URL=${BASEURL}"v${LATEST_VERSION}/summon_v${LATEST_VERSION}_${DISTRO}_amd64.zip"


ZIP_PATH="${tmp_dir}/summon.zip"
do_download ${URL} ${ZIP_PATH}

echo "Installing summon v${LATEST_VERSION} into /usr/local/bin"

sudo unzip -q -o ${ZIP_PATH} -d /usr/local/bin

echo "Success!"
echo "Run summon -h for usage"
