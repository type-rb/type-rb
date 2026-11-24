#!/bin/sh
# Verify the tools used to execute and type-check reference source output.
set -eu
compiler=$(CDPATH= cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test "$(go env GOVERSION)" = "go$(cat "$root/.go-version")"
test "$(ruby -e 'print RUBY_VERSION')" = "$(cat "$root/.ruby-version")"
test "$(node --version)" = "v$(cat "$root/.node-version")"
test "$(bun --version)" = "$(cat "$root/.bun-version")"
test "$(tsc --version)" = "Version $(cat "$root/.typescript-version")"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
mkdir src
cat > src/main.trb <<'TRB'
def main()
	puts("source toolchains")
	return
end
TRB
for mode in go ruby; do
    test "$("$compiler" run --mode "$mode" src/main.trb)" = 'source toolchains'
done
for runtime in node bun; do
    test "$("$compiler" run --mode typescript --runtime "$runtime" src/main.trb)" = 'source toolchains'
done

cat > trbconfig.jsonc <<'JSON'
{"name":"source-check","mode":"typescript","sourceDir":"src","outDir":"out"}
JSON
"$compiler" build --config trbconfig.jsonc
tsc --noEmit --strict --target esnext --module esnext --moduleResolution bundler --allowImportingTsExtensions out/main.ts
