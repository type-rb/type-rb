#!/usr/bin/env bash

set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
classifier="$root_dir/scripts/pages-full-tests-required.sh"

assert_classification() {
	local expected="$1"
	local paths="$2"
	local actual
	actual="$(printf '%s' "$paths" | "$classifier")"
	if [[ "$actual" != "$expected" ]]; then
		printf 'expected %s, got %s for paths:\n%s\n' "$expected" "$actual" "$paths" >&2
		exit 1
	fi
}

assert_classification false $'docs/getting-started.md\ninternal/site/assets/landing.html\ntools/site/highlight.mjs\n'
assert_classification false $'internal/playground/assets/app.js\nsyntaxes/typerb.tmLanguage.json\n'
assert_classification true $'docs/getting-started.md\ninternal/cli/cli.go\n'
assert_classification true $'.github/workflows/pages.yml\n'
assert_classification true ''
