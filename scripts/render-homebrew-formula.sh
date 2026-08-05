#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 VERSION CHECKSUMS OUTPUT" >&2
  exit 2
fi

version=${1#v}
checksums=$2
output=$3

case "$version" in
  ""|*[!0-9A-Za-z.-]*)
    echo "invalid version: $version" >&2
    exit 2
    ;;
esac
if [ ! -f "$checksums" ]; then
  echo "checksums file not found: $checksums" >&2
  exit 2
fi

checksum_for() {
  archive=$1
  checksum=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$checksums")
  case "$checksum" in
    ""|*[!0-9a-f]*)
      echo "missing or invalid SHA256 for $archive" >&2
      exit 2
      ;;
  esac
  if [ "${#checksum}" -ne 64 ]; then
    echo "missing or invalid SHA256 for $archive" >&2
    exit 2
  fi
  printf '%s\n' "$checksum"
}

darwin_arm64=$(checksum_for "trb_${version}_darwin_arm64.tar.gz")
darwin_amd64=$(checksum_for "trb_${version}_darwin_amd64.tar.gz")
linux_arm64=$(checksum_for "trb_${version}_linux_arm64.tar.gz")
linux_amd64=$(checksum_for "trb_${version}_linux_amd64.tar.gz")

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
sed \
  -e "s/@VERSION@/$version/g" \
  -e "s/@DARWIN_ARM64_SHA256@/$darwin_arm64/g" \
  -e "s/@DARWIN_AMD64_SHA256@/$darwin_amd64/g" \
  -e "s/@LINUX_ARM64_SHA256@/$linux_arm64/g" \
  -e "s/@LINUX_AMD64_SHA256@/$linux_amd64/g" \
  "$repository_root/packaging/homebrew/trb.rb.in" > "$output"
