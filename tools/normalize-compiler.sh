#!/bin/sh

set -eu

if [ "$#" -ne 4 ]; then
	echo "usage: normalize-compiler.sh CC B1_ASSEMBLY B2_ASSEMBLY WORKSPACE" >&2
	exit 64
fi

cc=$1
b1_assembly=$2
b2_assembly=$3
workspace=$4

mkdir -p "$workspace/b1" "$workspace/b2"

"$cc" "$b1_assembly" -Wl,-dead_strip -Wl,-no_uuid -o "$workspace/b1/compiler"
"$cc" "$b2_assembly" -Wl,-dead_strip -Wl,-no_uuid -o "$workspace/b2/compiler"

cmp "$workspace/b1/compiler" "$workspace/b2/compiler"

if /usr/bin/otool -l "$workspace/b1/compiler" | grep -q LC_UUID; then
	echo "normalized compiler still contains LC_UUID" >&2
	exit 1
fi

hash=$(shasum -a 256 "$workspace/b1/compiler" | cut -d ' ' -f 1)
size=$(stat -f '%z' "$workspace/b1/compiler")

echo "normalized-sha256,$hash"
echo "normalized-size,$size"
echo "normalization-policy,no-uuid-same-basename"
