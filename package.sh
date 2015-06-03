#!/bin/bash
set -e

# Get the version from the command line
if [ -z $VERSION ]; then
    echo "Please set a version in the VERSION env var."
    exit 1
fi

app="cauldron"

# Zip and copy to the dist dir
echo "==> Packaging..."
rm -rf ./pkg/dist
mkdir -p ./pkg/dist

for PLATFORM in $(find ./pkg -mindepth 1 -maxdepth 1 -type d); do
    OSARCH=$(basename ${PLATFORM})

    if [ $OSARCH = "dist" ]; then
        continue
    fi

    echo "--> ${OSARCH}"
    pushd $PLATFORM >/dev/null 2>&1
    zip ../dist/${app}_${VERSION}_${OSARCH}.zip ./*
    popd >/dev/null 2>&1
done

# Make the checksums
echo "==> Checksumming..."
pushd ./pkg/dist >/dev/null 2>&1
shasum -a256 * > ./${app}_${VERSION}_SHA256SUMS
popd >/dev/null 2>&1
