#!/bin/bash
set -e

# Get the version from the command line
if [ -z $VERSION ]; then
    echo "Please set a version in the VERSION env var."
    exit 1
fi

# Make sure we have a bintray API key
if [ -z $BINTRAY_API_KEY ]; then
    echo "Please set your bintray API key in the BINTRAY_API_KEY env var."
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

echo "==> Uploading..."
for ARCHIVE in ./pkg/dist/*; do
    ARCHIVE_NAME=$(basename ${ARCHIVE})

    echo Uploading: $ARCHIVE_NAME
    curl \
        -T ${ARCHIVE} \
        -u conjur:${BINTRAY_API_KEY} \
        "https://api.bintray.com/content/conjur/cauldron/cauldron/${VERSION}/${ARCHIVE_NAME}"
done
