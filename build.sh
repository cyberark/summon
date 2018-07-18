#!/bin/bash

# Platforms to build: https://golang.org/doc/install/source#environment

DEFAULT_PLATFORMS=(
  'darwin:amd64'     # MacOS
  'freebsd:amd64'
  'linux:amd64'
  # 'linux:arm'
  # 'linux:arm64'
  'netbsd:amd64'
  'openbsd:amd64'
  'solaris:amd64'
  # 'windows:386'
  'windows:amd64'
)
PLATFORMS="${1:-${DEFAULT_PLATFORMS[@]}}"  # override this with a positional argument, like 'linux:amd64'

OUTPUT_DIR='output'

echo "Creating summon binaries in $OUTPUT_DIR/"
docker-compose build --pull summon-builder

for platform in ${PLATFORMS}; do
  GOOS=${platform%%:*}
  GOARCH=${platform#*:}

  echo "-----"
  echo "GOOS=$GOOS, GOARCH=$GOARCH"
  echo "....."

  docker-compose run --rm \
    -e GOOS=$GOOS -e GOARCH=$GOARCH \
    summon-builder \
    build -v -o $OUTPUT_DIR/summon-$GOOS-$GOARCH
done
