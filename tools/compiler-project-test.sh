#!/bin/sh

set -eu
script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_directory/compiler-project.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/native-compiler-project.XXXXXX")
trap 'rm -r "$test_root"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

make_project() {
	mkdir -p "$1/src"
	printf 'def main()\n\treturn\nend\n' > "$1/src/compiler.trb"
	printf '{}\n' > "$1/trbconfig.jsonc"
}

current=$test_root/current
historical=$test_root/historical
spaced=$test_root/'source with spaces'
make_project "$current/compiler"
make_project "$historical/compiler/gate4"
make_project "$spaced/compiler"
test "$(native_compiler_project_directory "$current")" = "$current/compiler"
test "$(native_compiler_project_directory "$historical")" = "$historical/compiler/gate4"
test "$(native_compiler_project_directory "$spaced")" = "$spaced/compiler"
test "$(cd "$test_root" && native_compiler_project_directory current)" = current/compiler

# Nested modules retain the same project root, independently of a frozen flat
# checkout. A controller must not select a subdirectory as another project.
mkdir -p "$current/compiler/src/checking" "$spaced/compiler/src/mir/analysis"
printf 'def value(): Integer\nreturn 1\nend\n' > "$current/compiler/src/checking/value.trb"
printf 'def value(): Integer\nreturn 2\nend\n' > "$spaced/compiler/src/mir/analysis/value.trb"
test "$(native_compiler_project_directory "$current")" = "$current/compiler"
test "$(native_compiler_project_directory "$spaced")" = "$spaced/compiler"
test "$(native_compiler_project_directory "$historical")" = "$historical/compiler/gate4"

assert_rejected() {
	if native_compiler_project_directory "$1" > "$test_root/output" 2> "$test_root/error"; then
		printf '%s\n' 'accepted an invalid project layout' >&2
		exit 1
	fi
	test ! -s "$test_root/output"
	test -s "$test_root/error"
}

assert_rejected "$test_root/missing"
mkdir -p "$test_root/empty"
assert_rejected "$test_root/empty"
make_project "$test_root/missing-entry/compiler"
rm "$test_root/missing-entry/compiler/src/compiler.trb"
assert_rejected "$test_root/missing-entry"
make_project "$test_root/missing-config/compiler"
rm "$test_root/missing-config/compiler/trbconfig.jsonc"
assert_rejected "$test_root/missing-config"
make_project "$test_root/duplicate/compiler"
make_project "$test_root/duplicate/compiler/gate4"
assert_rejected "$test_root/duplicate"
make_project "$test_root/broken-legacy/compiler"
ln -s missing "$test_root/broken-legacy/compiler/gate4"
assert_rejected "$test_root/broken-legacy"

printf '%s\n' 'compiler project layout tests passed'

mkdir -p "$current/recovery/src/driver" "$spaced/recovery/src/driver" "$historical/src"
for project_root in "$current" "$spaced"; do
	printf '{}\n' > "$project_root/recovery/trbconfig.jsonc"
	printf 'def main()\nend\n' > "$project_root/recovery/src/driver/main.trb"
done
printf '{}\n' > "$historical/trbconfig.reference.jsonc"
printf 'def main()\nend\n' > "$historical/src/recovery_driver.trb"
test "$(native_recovery_project_config "$current")" = "$current/recovery/trbconfig.jsonc"
test "$(native_recovery_project_config "$spaced")" = "$spaced/recovery/trbconfig.jsonc"
test "$(native_recovery_project_config "$historical")" = "$historical/trbconfig.reference.jsonc"
printf '{}\n' > "$current/trbconfig.reference.jsonc"
if native_recovery_project_config "$current" > "$test_root/output" 2> "$test_root/error"; then
	exit 1
fi
rm "$current/trbconfig.reference.jsonc" "$current/recovery/src/driver/main.trb"
if native_recovery_project_config "$current" > "$test_root/output" 2> "$test_root/error"; then
	exit 1
fi

make_fixture() {
	mkdir -p "$1/configured-project"
	printf '{}\n' > "$1/configured-project/trbconfig.jsonc"
}

make_fixture "$current/corpus/configured-project"
make_fixture "$historical/corpus/gate6k"
make_fixture "$spaced/corpus/configured-project"
test "$(native_configured_fixture_directory "$current")" = "$current/corpus/configured-project/configured-project"
test "$(native_configured_fixture_directory "$historical")" = "$historical/corpus/gate6k/configured-project"
test "$(native_configured_fixture_directory "$spaced")" = "$spaced/corpus/configured-project/configured-project"
test "$(cd "$test_root" && native_configured_fixture_directory historical)" = historical/corpus/gate6k/configured-project

reject_fixture() {
	if native_configured_fixture_directory "$1" > "$test_root/output" 2> "$test_root/error"; then
		printf '%s\n' 'accepted an invalid configured fixture layout' >&2
		exit 1
	fi
	test ! -s "$test_root/output"
	test -s "$test_root/error"
}

reject_fixture "$test_root/missing"
reject_fixture "$test_root/empty"
make_fixture "$test_root/missing-fixture-config/corpus/configured-project"
rm "$test_root/missing-fixture-config/corpus/configured-project/configured-project/trbconfig.jsonc"
reject_fixture "$test_root/missing-fixture-config"
make_fixture "$test_root/duplicate-fixture/corpus/configured-project"
make_fixture "$test_root/duplicate-fixture/corpus/gate6k"
reject_fixture "$test_root/duplicate-fixture"
mkdir -p "$test_root/broken-current-fixture/corpus"
ln -s missing "$test_root/broken-current-fixture/corpus/configured-project"
make_fixture "$test_root/broken-current-fixture/corpus/gate6k"
reject_fixture "$test_root/broken-current-fixture"
make_fixture "$test_root/broken-historical-fixture/corpus/configured-project"
ln -s missing "$test_root/broken-historical-fixture/corpus/gate6k"
reject_fixture "$test_root/broken-historical-fixture"
printf '%s\n' 'configured fixture layout tests passed'
