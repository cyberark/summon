#!/bin/bash -e

GLOB='summon-*-amd64'

echo "==> Packaging..."

rm -rf output/dist && mkdir -p output/dist

pushd output

for binary_name in $GLOB; do
  pushd dist

  cp ../$binary_name summon && \
  tar -cvzf $binary_name.tar.gz summon && \
  rm -f summon

  popd
done

popd

shasum_binary=shasum
shasum_args='-a256'
if ! hash $shasum_binary 2>/dev/null; then
  shasum_binary=sha256sum  # on alpine linux
  shasum_args=''
fi

# # Make the checksums
echo "==> Checksumming..."
pushd output/dist
$shasum_binary $shasum_args * > SHA256SUMS.txt
popd
