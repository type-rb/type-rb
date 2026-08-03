#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
generated_dir=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-selfhost.XXXXXX")
trap 'rm -rf "$generated_dir"' EXIT HUP INT TERM

cd "$repository_root"
go run ./cmd/trb build \
  --config selfhost/trbconfig.jsonc \
  --out-dir "$generated_dir"
diff -ru selfhost/generated "$generated_dir"
go test ./selfhost/generated
