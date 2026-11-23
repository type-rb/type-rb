#!/bin/sh

set -eu

fail() {
	printf 'bootstrap-seed-arguments-test: %s\n' "$1" >&2
	exit 1
}

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
bootstrap_tool=$script_directory/bootstrap-seed.sh
task_tmp=$(printenv TMPDIR 2>/dev/null || printf /tmp)
test_root=$(mktemp -d "$task_tmp/type-rb-native-bootstrap-arguments.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

run_case() {
	label=$1
	expected_status=$2
	shift 2
	stdout=$test_root/$label.stdout
	stderr=$test_root/$label.stderr
	set +e
	/bin/sh "$bootstrap_tool" \
		"$@" \
		--input "$test_root/missing-input" \
		--qbe "$test_root/missing-qbe" \
		--cc "$test_root/missing-cc" \
		--runner-image ubuntu-24.04 \
		--workspace "$test_root/$label-workspace" \
		--output "$test_root/$label-output" \
		--evidence "$test_root/$label-evidence" \
		--metadata "$test_root/$label-metadata.json" \
		--asset-name "$label-asset" \
		> "$stdout" 2> "$stderr"
	status=$?
	set -e
	test "$status" -eq "$expected_status" ||
		fail "$label returned status $status, expected $expected_status"
	test ! -s "$stdout" || fail "$label wrote stdout"
}

require_usage_rejection() {
	label=$1
	shift
	run_case "$label" 64 "$@"
	grep -F 'usage: bootstrap-seed.sh' "$test_root/$label.stderr" >/dev/null ||
		fail "$label did not print usage"
}

require_argument_acceptance() {
	label=$1
	shift
	run_case "$label" 1 "$@"
	printf 'bootstrap-seed: input does not exist\n' > "$test_root/$label.expected"
	cmp "$test_root/$label.expected" "$test_root/$label.stderr" >/dev/null ||
		fail "$label did not reach input validation"
}

require_usage_rejection amd64-initial-ordinary \
	--profile linux-amd64-v0 --mode initial --input-role ordinary
require_usage_rejection amd64-initial-transition \
	--profile linux-amd64-v0 --mode initial --input-role transition
require_usage_rejection amd64-previous-ordinary \
	--profile linux-amd64-v0 --mode previous --input-role ordinary
require_usage_rejection amd64-previous-default \
	--profile linux-amd64-v0 --mode previous
require_argument_acceptance amd64-previous-transition \
	--profile linux-amd64-v0 --mode previous --input-role transition
require_argument_acceptance darwin-initial-ordinary \
	--profile darwin-arm64-v0 --mode initial --input-role ordinary
require_argument_acceptance linux-arm64-initial-ordinary \
	--profile linux-arm64-v0 --mode initial --input-role ordinary

require_argument_acceptance diagnostic-previous-transition \
	--profile linux-arm64-v0 --mode previous --input-role transition --measurement-policy diagnostic
require_argument_acceptance explicit-bootstrap \
	--profile darwin-arm64-v0 --mode initial --measurement-policy bootstrap
require_usage_rejection unknown-measurement-policy \
	--profile linux-arm64-v0 --mode previous --input-role transition --measurement-policy unknown
require_usage_rejection empty-measurement-policy \
	--profile linux-arm64-v0 --mode previous --input-role transition --measurement-policy ''
require_usage_rejection duplicate-measurement-policy \
	--profile linux-arm64-v0 --mode previous --input-role transition --measurement-policy bootstrap --measurement-policy diagnostic
require_usage_rejection diagnostic-initial \
	--profile linux-arm64-v0 --mode initial --measurement-policy diagnostic
require_usage_rejection diagnostic-ordinary \
	--profile linux-arm64-v0 --mode previous --input-role ordinary --measurement-policy diagnostic
require_usage_rejection diagnostic-default-role \
	--profile linux-arm64-v0 --mode previous --measurement-policy diagnostic

grep -F 'linux-amd64-v0 requires --mode previous --input-role transition' \
	"$test_root/amd64-previous-default.stderr" >/dev/null ||
	fail "amd64 usage did not describe its transition-only contract"

printf 'bootstrap-seed-arguments-test: passed\n'
