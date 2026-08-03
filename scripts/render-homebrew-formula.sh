#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 VERSION SHA256 OUTPUT" >&2
  exit 2
fi

version=${1#v}
checksum=$2
output=$3

case "$version" in
  ""|*[!0-9A-Za-z.-]*)
    echo "invalid version: $version" >&2
    exit 2
    ;;
esac
case "$checksum" in
  ""|*[!0-9a-f]*)
    echo "SHA256 must be 64 lowercase hexadecimal characters" >&2
    exit 2
    ;;
esac
if [ "${#checksum}" -ne 64 ]; then
  echo "SHA256 must be 64 lowercase hexadecimal characters" >&2
  exit 2
fi

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
sed \
  -e "s/@VERSION@/$version/g" \
  -e "s/@SHA256@/$checksum/g" \
  "$repository_root/packaging/homebrew/trb.rb.in" > "$output"
