#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cache="$repository_root/.trb/bootstrap"
seed_release=bootstrap-seed-2026-09-29-module-cycles
output="$repository_root/bin"
crypto=${TRBN_CRYPTO_LIB_DIR:-$output/crypto}
export TRBN_CRYPTO_LIB_DIR="$crypto"
fail() { printf 'trbn: %s\n' "$1" >&2; exit 1; }
sha256_files() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}
source_hashes() {
	python3 tools/compiler_sources.py --hash "$1" > "$stage/source-hashes"
	LC_ALL=C sort "$stage/source-hashes"
}
sha256() { sha256_files "$1" | cut -d ' ' -f 1; }
verify() { test "$(sha256 "$1")" = "$2" || fail "checksum mismatch: $1"; }
# --plan reports whether a build would reuse everything (cached), rebuild only
# the CLI (cli) or rebuild the core and CLI (core), without building anything.
mode=build
case "$#:${1:-}" in
	0:) ;;
	1:--plan) mode=plan ;;
	*) fail 'usage: tools/build-native.sh [--plan]' ;;
esac
case "$(uname -s)/$(uname -m)" in
	Darwin/arm64)
		profile=darwin-arm64-v0
		asset=type-rb-native-bootstrap-darwin-arm64
		seed_digest=54337b2b4e3aebc1a81260ad3be071008b88b6e30685d182db81a138f11ac216
		;;
	Linux/aarch64)
		profile=linux-arm64-v0
		asset=type-rb-native-bootstrap-linux-arm64
		seed_digest=3627fd8c574a668173ac6bb25eb41371c0f6cf0fa9da1016eebfc08b078d4060
		command -v ld.lld >/dev/null 2>&1 || fail 'Linux builds require lld'
		;;
	*) fail 'checkout bootstrap currently supports Darwin arm64 and Linux arm64' ;;
esac
cc=${TRBN_CC:-/usr/bin/cc}
test -x "$cc" || fail "C toolchain is missing: $cc"
mkdir -p "$cache" "$output"
lock="$cache/build.lock"
lock_wait=0
while ! mkdir "$lock" 2>/dev/null; do
	lock_wait=$((lock_wait + 1))
	test "$lock_wait" -le 600 || fail "another build holds $lock; inspect its owner before removing a stale lock"
	sleep 1
done
printf '%s\n' "$$" > "$lock/pid"
stage=
cleanup() {
	if test -n "$stage"; then rm -rf "$stage"; fi
	rm -f "$lock/pid"
	rmdir "$lock"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
stage=$(mktemp -d "$cache/build.XXXXXX")
# Hash each source set in one process. Paths, additions and deletions remain
# part of the manifest; timestamps and Git revision do not determine freshness.
(
	cd "$repository_root"
	set -- tools/build-native.sh tools/qbe_toolchain.py tools/qbe/backend.json tools/qbe/sqrt.patch tools/qbe/arm64-immediates.patch tools/qbe/arm64-memory.patch tools/qbe/gcm.patch tools/qbe/fma.patch "$cc"
	if test -n "${TRBN_QBE:-}"; then set -- "$@" "$TRBN_QBE"; fi
	if test -n "${TRBN_BOOTSTRAP_SEED:-}"; then set -- "$@" "$TRBN_BOOTSTRAP_SEED"; fi
	sha256_files "$@"
	source_hashes compiler/src
	printf '%s\n' "$profile" "$cc" "${TRBN_QBE:-bundled}" "${TRBN_BOOTSTRAP_SEED:-published}"
	"$cc" --version
) > "$stage/core-inputs"
(
	cat "$stage/core-inputs"
	cd "$repository_root"
	sha256_files tools/compiler_sources.py
	if test -f "$crypto/libcrypto.a" && test -f "$crypto/crypto-identity.o"; then
		(cd "$crypto" && sha256_files libcrypto.a crypto-identity.o)
	else
		printf '%s\n' 'missing crypto archives'
	fi
	source_hashes compiler/cli
) > "$stage/inputs"
# One decision serves the build and --plan, so plan checks exercise the same
# content keys and published-state checks. The lock keeps the cache stable.
decision=core
if test -x "$output/trbn" && test -x "$output/qbe" && test -f "$cache/inputs" &&
	cmp -s "$stage/inputs" "$cache/inputs"; then
	decision=cached
elif test -x "$cache/core/compiler" && test -f "$cache/core/inputs" &&
	cmp -s "$stage/core-inputs" "$cache/core/inputs"; then
	decision=cli
fi
if test "$mode" = plan; then
	printf '%s\n' "$decision"
	exit 0
fi
for archive in libcrypto.a crypto-identity.o; do
	test -f "$crypto/$archive" || fail 'REPL requires pinned crypto archives: install bin/crypto or set TRBN_CRYPTO_LIB_DIR'
done
if test "$decision" = cached; then
	printf '%s\n' "$output/trbn"
	exit 0
fi
printf '%s\n' 'trbn: bootstrapping the native compiler' >&2
qbe=${TRBN_QBE:-$cache/qbe-1.3/qbe}
if test -n "${TRBN_QBE:-}"; then
	python3 "$repository_root/tools/qbe_toolchain.py" --check "$qbe"
else
	qbe=$(CC="$cc" python3 "$repository_root/tools/qbe_toolchain.py" "$cache/qbe-1.3")
fi
seed=${TRBN_BOOTSTRAP_SEED:-$cache/$seed_release/$asset}
if test ! -f "$seed"; then
	test -z "${TRBN_BOOTSTRAP_SEED:-}" || fail "bootstrap seed is missing: $seed"
	curl --fail --location --retry 3 "https://github.com/type-rb/type-rb-native/releases/download/$seed_release/$asset" -o "$stage/seed"
	verify "$stage/seed" "$seed_digest"
	mkdir -p "$cache/$seed_release"
	mv "$stage/seed" "$seed"
fi
verify "$seed" "$seed_digest"
chmod 0755 "$seed"
mkdir -p "$stage/first" "$stage/runtime" "$stage/core" "$stage/verify" "$stage/source"
core="$cache/core/compiler"
if test "$decision" = cli; then
	printf '%s\n' 'trbn: reusing the verified core compiler' >&2
else
	source="$repository_root/compiler/src/compiler.trb"
	"$seed" build "$source" --output "$stage/first/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
	"$stage/first/compiler" build "$source" --output "$stage/runtime/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
	"$stage/runtime/compiler" build "$source" --output "$stage/core/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
	"$stage/core/compiler" build "$source" --output "$stage/verify/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
	cmp "$stage/core/compiler" "$stage/verify/compiler" || fail 'compiler fixed point differs'
	core="$stage/core/compiler"
fi
# The core and CLI share one import root in this derived source tree. The
# canonical core project remains independently checkable by the reference tool.
python3 "$repository_root/tools/compiler_sources.py" "$stage/source" \
	"$repository_root/compiler/src" "$repository_root/compiler/cli"
"$core" build "$stage/source/main.trb" --output "$stage/trbn" --qbe "$qbe" --cc "$cc" --target "$profile"
"$stage/trbn" --version >&2
"$stage/trbn" --internal-driver build "$stage/source/main.trb" --output "$stage/verify/trbn" --qbe "$qbe" --cc "$cc"
cmp "$stage/trbn" "$stage/verify/trbn" || fail 'CLI fixed point differs'
cp "$qbe" "$stage/qbe"
# Publish the verified core only after the CLI fixed point also succeeds.
if test "$core" = "$stage/core/compiler"; then
	mkdir -p "$cache/core"
	rm -f "$cache/core/inputs"
	mv "$stage/core/compiler" "$cache/core/compiler"
	mv "$stage/core-inputs" "$cache/core/inputs"
fi
revision=$(git -C "$repository_root" rev-parse HEAD 2>/dev/null || printf 'source-archive')
printf '%s\n' "profile=$profile" "revision=$revision" "inputs_sha256=$(sha256 "$stage/inputs")" "compiler_sha256=$(sha256 "$stage/trbn")" "qbe_sha256=$(sha256 "$stage/qbe")" > "$stage/build-info.txt"
python3 "$repository_root/tools/qbe_toolchain.py" --identity > "$stage/qbe-backend.json"
rm -f "$cache/inputs"
mv "$stage/qbe-backend.json" "$output/qbe-backend.json"
mv "$stage/trbn" "$output/trbn"
mv "$stage/qbe" "$output/qbe"
mv "$stage/build-info.txt" "$output/build-info.txt"
mv "$stage/inputs" "$cache/inputs"
printf '%s\n' "$output/trbn"
