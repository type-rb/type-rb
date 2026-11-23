#!/bin/sh
# Check the CLI and core together with the explicitly selected reference tool.
set -eu
reference=${1:?usage: check-native-cli.sh /path/to/trb}
repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/native-cli-check.XXXXXX")
trap 'rm -rf "$stage"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$stage/src"
python3 "$repository_root/tools/compiler_sources.py" "$stage/src" \
	"$repository_root/compiler/src" "$repository_root/compiler/cli"
# The source-level CLI main is an empty native entry marker. The reference
# checks the CLI as a library beside the core's standalone main marker.
sed '/^def main()/,/^end$/d' "$repository_root/compiler/cli/main.trb" > "$stage/src/main.trb"
cat > "$stage/trbconfig.jsonc" <<'JSON'
{"name":"native-cli-check","mode":"go","sourceDir":"src","go":{"module":"example.com/native-cli-check"}}
JSON
"$reference" check --config "$stage/trbconfig.jsonc"
