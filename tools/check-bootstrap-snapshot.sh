#!/bin/sh
# Check the canonical compiler source against the recovery snapshot subset.
set -eu

reference=${1:?usage: check-bootstrap-snapshot.sh /path/to/pinned/trb [snapshot.json]}
output=${2:-}
if [ "$#" -gt 2 ]; then
	printf 'usage: check-bootstrap-snapshot.sh /path/to/pinned/trb [snapshot.json]\n' >&2
	exit 2
fi

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/native-bootstrap-snapshot.XXXXXX")
trap 'rm -rf "$stage"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$stage/compiler"
python3 "$repository_root/tools/compiler_sources.py" "$stage/compiler" \
	"$repository_root/compiler/src"

"$reference" compiler bootstrap-snapshot --snapshot-version 4 \
	"$stage/compiler/compiler.trb" > "$stage/snapshot.json"
test -s "$stage/snapshot.json"
python3 "$repository_root/tools/bootstrap_snapshot_size.py" "$stage/snapshot.json"
if [ -n "$output" ]; then
	cp "$stage/snapshot.json" "$output"
fi
