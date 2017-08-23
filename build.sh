#!/bin/bash

# Get the version from the command line
if [ -z $VERSION ]; then
    VERSION=$(git describe --abbrev=0 --tags)
fi

# Platforms to build: https://golang.org/doc/install/source#environment
PLATFORMS=(
  'darwin:amd64'     # MacOS
  # 'dragonfly:amd64'  # Dragonfly https://www.dragonflybsd.org/
  'freebsd:amd64'
  # 'linux:386'
  'linux:amd64'
  # 'linux:arm'
  # 'linux:arm64'
  'netbsd:amd64'
  'openbsd:amd64'
  'solaris:amd64'
  # 'windows:386'
  'windows:amd64'
)
OUTPUT_DIR='output'

echo "Creating summon binaries in $OUTPUT_DIR/"
docker-compose build --pull summon-builder

for platform in "${PLATFORMS[@]}"; do
  GOOS=${platform%%:*}
  GOARCH=${platform#*:}

  echo "-----"
  echo "GOOS=$GOOS, GOARCH=$GOARCH"
  echo "....."

  docker-compose run --rm \
    -e GOOS=$GOOS -e GOARCH=$GOARCH \
    summon-builder \
    build -v -o $OUTPUT_DIR/summon_$VERSION_$GOOS_$GOARCH
done
