#!/bin/sh

set -eu

fail() {
	printf 'benchmarksgame-runtime-controller-test: %s\n' "$1" >&2
	exit 1
}

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-native-benchmark-controller.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

program=$test_root/program.sh
cat > "$program" <<'EOF'
#!/bin/sh
printf '%s-output\n' "$2"
EOF
chmod 0755 "$program"

fake_runexec=$test_root/runexec
cat > "$fake_runexec" <<'EOF'
#!/bin/sh
set -eu
if test "${1:-}" = --version; then
	printf 'runexec 3.35-test\n'
	exit 0
fi
output=
while test "$#" -gt 0; do
	case "$1" in
	--quiet) shift ;;
	--memlimit | --walltimelimit | --cores | --read-only-dir | --overlay-dir | --output)
		option=$1
		value=$2
		shift 2
		if test "$option" = --output; then output=$value; fi
		;;
	--) shift; break ;;
	*) exit 64 ;;
	esac
done
test -n "$output"
candidate=$3
case "$candidate" in
typerb-native) number=1 ;;
typerb-go) number=2 ;;
c) number=3 ;;
cpp) number=4 ;;
go) number=5 ;;
rust) number=6 ;;
java) number=7 ;;
*) exit 64 ;;
esac
payload=$output.payload
set +e
"$@" > "$payload" 2>&1
actual_status=$?
set -e
exit_key=returnvalue
exit_value=$actual_status
if test "${FAKE_FAIL_CANDIDATE:-}" = "$candidate"; then exit_value=9; fi
if test "${FAKE_SIGNAL_CANDIDATE:-}" = "$candidate"; then
	exit_key=exitsignal
	exit_value=15
fi
if test "${FAKE_INVALID_CANDIDATE:-}" = "$candidate"; then exit_value=invalid; fi
{
	printf '%s\n\n\n' "$*"
	printf '%s\n\n\n' '--------------------------------------------------------------------------------'
	cat "$payload"
} > "$output"
printf 'starttime=2026-08-30T00:00:00+00:00\n'
printf '%s=%s\n' "$exit_key" "$exit_value"
printf 'walltime=0.%ss\n' "$number"
printf 'cputime=0.0%ss\n' "$number"
printf 'memory=100%sB\n' "$number"
EOF
chmod 0755 "$fake_runexec"

cache_control=$test_root/cache-control
cat > "$cache_control" <<'EOF'
#!/bin/sh
set -eu
test "$#" -eq 1
printf 'cache_control=test\n' > "$1"
EOF
chmod 0755 "$cache_control"

expected=$test_root/expected.txt
printf 'fannkuch-redux-output\n' > "$expected"
catalog=$test_root/catalog.tsv
printf 'case\tcandidate\tcommand\targ1\targ2\targ3\targ4\texpected\n' > "$catalog"
for candidate in typerb-native typerb-go c cpp go rust java; do
	printf 'fannkuch-redux\t%s\t/bin/sh\t%s\t%s\tfannkuch-redux\t-\t%s\n' \
		"$candidate" "$program" "$candidate" "$expected" >> "$catalog"
done

/bin/sh "$script_directory/runtime-controller.sh" \
	test "$fake_runexec" "$catalog" fannkuch-redux one-core 0 "$cache_control" \
	"$test_root/pass-workspace" "$test_root/pass-evidence" \
	> "$test_root/pass.stdout" 2> "$test_root/pass.stderr"
test "$(cat "$test_root/pass.stdout")" = \
	"benchmarksgame-runtime-controller: fannkuch-redux one-core passed"
test ! -s "$test_root/pass.stderr" || fail "passing controller wrote stderr"
test "$(wc -l < "$test_root/pass-evidence/raw.tsv" | tr -d ' ')" -eq 92 || fail "raw row count differs"
test "$(wc -l < "$test_root/pass-evidence/medians.tsv" | tr -d ' ')" -eq 8 || fail "median row count differs"
test "$(find "$test_root/pass-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 91 ||
	fail "observation count differs"

awk -F '\t' '
	NR == 2 && !($1 == "warmup" && $2 == 1 && $4 == 1 && $7 == "typerb-native") { exit 1 }
	NR == 9 && !($2 == 2 && $4 == 1 && $7 == "typerb-go") { exit 1 }
	NR == 86 && !($2 == 13 && $4 == 1 && $7 == "rust") { exit 1 }
' "$test_root/pass-evidence/raw.tsv" || fail "rotation order differs"
awk -F '\t' '
	$3 == "typerb-native" {
		found += 1
		if (!($4 == 11 && $5 == 11 && $6 == 0.1 && $7 == 0.01 && $8 == 1001 && $9 == "pass")) exit 1
	}
	END { exit !(found == 1) }
' "$test_root/pass-evidence/medians.tsv" || fail "median values differ"

set +e
FAKE_FAIL_CANDIDATE=rust \
	FAKE_SIGNAL_CANDIDATE=go \
	FAKE_INVALID_CANDIDATE=cpp \
	/bin/sh "$script_directory/runtime-controller.sh" \
	test "$fake_runexec" "$catalog" fannkuch-redux four-core 0,1,2,3 "$cache_control" \
	"$test_root/fail-workspace" "$test_root/fail-evidence" \
	> "$test_root/fail.stdout" 2> "$test_root/fail.stderr"
failure_status=$?
set -e
test "$failure_status" -ne 0 || fail "measured failure did not fail the controller"
test "$(wc -l < "$test_root/fail-evidence/raw.tsv" | tr -d ' ')" -eq 92 || fail "failure truncated raw rows"
test "$(awk -F '\t' '$1 == "retained" && $7 == "rust" && $8 == "return-9" { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/raw.tsv")" -eq 11 ||
	fail "retained failure count differs"
test "$(awk -F '\t' '$1 == "retained" && $7 == "go" && $8 == "signal-15" && $10 == 15 { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/raw.tsv")" -eq 11 ||
	fail "retained signal count differs"
test "$(awk -F '\t' '$1 == "retained" && $7 == "cpp" && $8 == "exit-invalid" { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/raw.tsv")" -eq 11 ||
	fail "retained malformed-exit count differs"
test "$(awk -F '\t' '$3 == "rust" && $9 == "incomplete" { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/medians.tsv")" -eq 1 ||
	fail "failed candidate received a passing median"
test "$(find "$test_root/fail-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 91 ||
	fail "failure stopped later observations"

incorrect_program=$test_root/incorrect-program.sh
cat > "$incorrect_program" <<'EOF'
#!/bin/sh
if test "$1" = rust; then printf 'unexpected diagnostic\n' >&2; fi
printf '%s-output\n' "$2"
EOF
chmod 0755 "$incorrect_program"
incorrect_catalog=$test_root/incorrect-catalog.tsv
printf 'case\tcandidate\tcommand\targ1\targ2\targ3\targ4\texpected\n' > "$incorrect_catalog"
for candidate in typerb-native typerb-go c cpp go rust java; do
	printf 'fannkuch-redux\t%s\t/bin/sh\t%s\t%s\tfannkuch-redux\t-\t%s\n' \
		"$candidate" "$incorrect_program" "$candidate" "$expected" >> "$incorrect_catalog"
done
set +e
/bin/sh "$script_directory/runtime-controller.sh" \
	test "$fake_runexec" "$incorrect_catalog" fannkuch-redux one-core 0 "$cache_control" \
	"$test_root/incorrect-workspace" "$test_root/incorrect-evidence" \
	> "$test_root/incorrect.stdout" 2> "$test_root/incorrect.stderr"
incorrect_status=$?
set -e
test "$incorrect_status" -ne 0 || fail "incorrect program reached timing"
test ! -e "$test_root/incorrect-evidence/raw.tsv" || fail "timing data exists after correctness failure"
test "$(find "$test_root/incorrect-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 0 ||
	fail "runexec started after correctness failure"
test "$(awk -F '\t' '$2 == "rust" && $4 == "false" { count += 1 } END { print count + 0 }' "$test_root/incorrect-evidence/correctness/summary.tsv")" -eq 1 ||
	fail "correctness diagnostic was not preserved"

printf 'benchmarksgame-runtime-controller-test: passed\n'
