#!/usr/bin/env bash

set -euo pipefail

output_dir="${1:-dist/site}"
version="${2:-}"
site_mode="${3:-interactive}"
docs_only=false
case "$site_mode" in
	interactive) ;;
	docs-only) docs_only=true ;;
	*) printf 'unknown site mode: %s\n' "$site_mode" >&2; exit 2 ;;
esac
if [[ -z "$version" ]]; then
	version="$(go run ./cmd/trb version)"
fi

go run ./cmd/trb-site --out "$output_dir" --docs docs --version "$version" --docs-only="$docs_only"
node tools/site/highlight.mjs "$output_dir" syntaxes/typerb.tmLanguage.json
cp llms.txt "$output_dir/llms.txt"
if [[ "$site_mode" == interactive ]]; then
	GOOS=js GOARCH=wasm go build \
		-o "$output_dir/assets/trb.wasm" \
		./cmd/trb-wasm
	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$output_dir/assets/wasm_exec.js"
fi

printf 'built TypeRB website %s in %s\n' "$version" "$output_dir"
