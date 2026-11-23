#!/bin/sh

set -eu

fail() {
	printf 'benchmarksgame-build-controller-test: %s\n' "$1" >&2
	exit 1
}

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-native-build-controller.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

fake_compiler=$test_root/fake-compiler
cat > "$fake_compiler" <<'EOF'
#!/bin/sh
set -eu
candidate=$(basename "$0")
command=${1:-}
shift || true
case "$command" in
check)
	if test "${FAKE_BAD_CHECK_CANDIDATE:-}" = "$candidate"; then
		printf 'unexpected\n'
	elif test "$candidate" = native-compiler; then
		printf 'ok\n'
	else
		printf 'checked 1 file(s) for mode go\n'
	fi
	exit 0
	;;
build) ;;
*) exit 64 ;;
esac
output=
while test "$#" -gt 0; do
	case "$1" in
	--output | --outfile)
		output=$2
		shift 2
		;;
	--qbe | --cc | --target | --config)
		shift 2
		;;
	--compile) shift ;;
	*) shift ;;
	esac
done
test -n "$output"
mkdir -p "$(dirname -- "$output")"
if test "${FAKE_BAD_PROGRAM_CANDIDATE:-}" = "$candidate"; then
	program_output=incorrect
else
	program_output=correct
fi
cat > "$output" <<PROGRAM
#!/bin/sh
if test "$program_output" = incorrect; then
	printf 'incorrect\\n'
else
	printf '228\\nPfannkuchen(7) = 16\\n'
fi
PROGRAM
if test "${FAKE_VARY_ARTIFACT_CANDIDATE:-}" = "$candidate"; then
	counter_directory=${FAKE_VARIANT_COUNTER_DIR:?}
	mkdir -p "$counter_directory"
	counter_path=$counter_directory/$candidate
	variant=0
	if test -f "$counter_path"; then variant=$(cat "$counter_path"); fi
	variant=$((variant + 1))
	printf '%s\n' "$variant" > "$counter_path"
	printf '# variant %s\n' "$variant" >> "$output"
fi
chmod 0755 "$output"
if test "$candidate" = trb; then printf 'executable -> %s\n' "$output"; fi
EOF
chmod 0755 "$fake_compiler"
cp "$fake_compiler" "$test_root/native-compiler"
cp "$fake_compiler" "$test_root/trb"
chmod 0755 "$test_root/native-compiler" "$test_root/trb"

for tool in qbe cc; do
	cat > "$test_root/$tool" <<'EOF'
#!/bin/sh
exit 0
EOF
	chmod 0755 "$test_root/$tool"
done

mkdir -p "$test_root/go-root"
cat > "$test_root/go" <<EOF
#!/bin/sh
set -eu
if test "\${1:-}" = env && test "\${2:-}" = GOROOT; then
	printf '%s\n' '$test_root/go-root'
	exit 0
fi
if test "\${1:-}" = version; then
	printf 'go version go-test linux/arm64\n'
	exit 0
fi
exit 64
EOF
chmod 0755 "$test_root/go"

fake_runexec=$test_root/runexec
cat > "$fake_runexec" <<'EOF'
#!/bin/sh
set -eu
if test "${1:-}" = --version; then
	printf 'runexec 3.35-test\n'
	exit 0
fi
if test -n "${FAKE_RUNEXEC_ARGS_LOG:-}"; then
	printf '%s\n' "$*" >> "$FAKE_RUNEXEC_ARGS_LOG"
fi
output=
while test "$#" -gt 0; do
	case "$1" in
	--quiet) shift ;;
	--memlimit | --walltimelimit | --cores | --read-only-dir | --overlay-dir | --full-access-dir | --output)
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
case "$(basename "$1")" in
native-compiler) candidate=typerb-native; number=1 ;;
trb) candidate=typerb-go; number=2 ;;
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
printf 'starttime=2026-08-31T00:00:00+00:00\n'
printf '%s=%s\n' "$exit_key" "$exit_value"
printf 'walltime=0.%ss\n' "$number"
printf 'cputime=0.0%ss\n' "$number"
printf 'memory=100%sB\n' "$number"
EOF
chmod 0755 "$fake_runexec"

cat > "$test_root/process.trace" <<'EOF'
100  execve("/direct", ["/direct"], 0xffff) = 0
101  execve("/missing", ["/missing"], 0xffff) = -1 ENOENT (No such file or directory)
102  execve("/split", ["/split"], 0xffff <unfinished ...>
102  <... execve resumed>) = 0
103  execve("/split-missing", ["/split-missing"], 0xffff <unfinished ...>
103  <... execve resumed>) = -1 ENOENT (No such file or directory)
104  execve("/unfinished", ["/unfinished"], 0xffff <unfinished ...>
EOF
cat > "$test_root/process-executables.expected" <<'EOF'
/direct
/split
EOF
awk -f "$script_directory/trace-executables.awk" "$test_root/process.trace" \
	> "$test_root/process-executables.actual"
cmp "$test_root/process-executables.expected" "$test_root/process-executables.actual" ||
	fail "process trace executable extraction differs"

cache_control=$test_root/cache-control
cat > "$cache_control" <<'EOF'
#!/bin/sh
set -eu
test "$#" -eq 1
printf 'cache_control=test\n' > "$1"
EOF
chmod 0755 "$cache_control"

PATH="$test_root:$PATH" \
	FAKE_RUNEXEC_ARGS_LOG="$test_root/runexec-args.log" \
	FAKE_VARY_ARTIFACT_CANDIDATE=trb \
	FAKE_VARIANT_COUNTER_DIR="$test_root/variant-counters" \
	/bin/sh "$script_directory/build-controller.sh" \
	test "$fake_runexec" fannkuch-redux \
	"$test_root/native-compiler" "$test_root/trb" "$test_root/qbe" "$test_root/cc" "$test_root/go" \
	linux-arm64-v0 0,1,2,3 "$cache_control" \
	"$test_root/pass-workspace" "$test_root/pass-evidence" \
	> "$test_root/pass.stdout" 2> "$test_root/pass.stderr"
test "$(cat "$test_root/pass.stdout")" = "benchmarksgame-build-controller: fannkuch-redux passed"
test ! -s "$test_root/pass.stderr" || fail "passing controller wrote stderr"
test "$(wc -l < "$test_root/pass-evidence/raw.tsv" | tr -d ' ')" -eq 27 || fail "raw row count differs"
test "$(wc -l < "$test_root/pass-evidence/medians.tsv" | tr -d ' ')" -eq 3 || fail "median row count differs"
test "$(find "$test_root/pass-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 26 ||
	fail "observation count differs"
test "$(wc -l < "$test_root/pass-evidence/artifacts.tsv" | tr -d ' ')" -eq 5 || fail "artifact inventory differs"
test "$(wc -l < "$test_root/pass-evidence/distribution/dependencies.tsv" | tr -d ' ')" -eq 3 ||
	fail "dependency inventory differs"
test "$(wc -l < "$test_root/pass-evidence/distribution/process-executables.tsv" | tr -d ' ')" -eq 6 ||
	fail "process executable inventory differs"
test "$(grep -F -- '--overlay-dir /home' "$test_root/runexec-args.log" | wc -l | tr -d ' ')" -eq 26 ||
	fail "home overlay policy differs"
if grep -F -- '--overlay-dir /tmp' "$test_root/runexec-args.log" >/dev/null; then
	fail "temporary directory must use BenchExec default hidden mode"
fi
grep '^tmp_filesystem=benchexec-default-hidden$' "$test_root/pass-evidence/environment.txt" >/dev/null ||
	fail "temporary directory evidence differs"
if grep -F -- '--overlay-dir /tmp' \
	"$script_directory/../../.github/workflows/benchmarksgame-build-formal.yml" >/dev/null; then
	fail "host probe must use BenchExec default hidden temporary directory"
fi

awk -F '\t' '
	NR == 2 && !($1 == "warmup" && $2 == 1 && $4 == 1 && $6 == "typerb-native") { exit 1 }
	NR == 4 && !($2 == 2 && $4 == 1 && $6 == "typerb-go") { exit 1 }
	NR == 26 && !($2 == 13 && $4 == 1 && $6 == "typerb-native") { exit 1 }
' "$test_root/pass-evidence/raw.tsv" || fail "rotation order differs"
awk -F '\t' '
	$2 == "typerb-native" {
		found += 1
		if (!($3 == 11 && $4 == 11 && $5 == 0.1 && $6 == 0.01 && $7 == 1001 && $11 == 1 && $12 == "true" && $13 != "" && $14 == "pass")) exit 1
	}
	END { exit !(found == 1) }
' "$test_root/pass-evidence/medians.tsv" || fail "median values differ"
awk -F '\t' '
	$2 == "typerb-go" {
		found += 1
		if (!($3 == 11 && $4 == 11 && $11 == 11 && $12 == "false" && $13 == "" && $14 == "pass")) exit 1
	}
	END { exit !(found == 1) }
' "$test_root/pass-evidence/medians.tsv" || fail "artifact variant summary differs"

set +e
PATH="$test_root:$PATH" \
	FAKE_FAIL_CANDIDATE=typerb-go \
	FAKE_INVALID_CANDIDATE=typerb-native \
	/bin/sh "$script_directory/build-controller.sh" \
	test "$fake_runexec" fannkuch-redux \
	"$test_root/native-compiler" "$test_root/trb" "$test_root/qbe" "$test_root/cc" "$test_root/go" \
	linux-arm64-v0 0,1,2,3 "$cache_control" \
	"$test_root/fail-workspace" "$test_root/fail-evidence" \
	> "$test_root/fail.stdout" 2> "$test_root/fail.stderr"
failure_status=$?
set -e
test "$failure_status" -ne 0 || fail "measured failures did not fail the controller"
test "$(wc -l < "$test_root/fail-evidence/raw.tsv" | tr -d ' ')" -eq 27 || fail "failure truncated raw rows"
test "$(awk -F '\t' '$1 == "retained" && $6 == "typerb-go" && $7 == "return-9" { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/raw.tsv")" -eq 11 ||
	fail "retained Go failure count differs"
test "$(awk -F '\t' '$1 == "retained" && $6 == "typerb-native" && $7 == "exit-invalid" { count += 1 } END { print count + 0 }' "$test_root/fail-evidence/raw.tsv")" -eq 11 ||
	fail "retained Native malformed-exit count differs"
test "$(find "$test_root/fail-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 26 ||
	fail "failure stopped later observations"

set +e
PATH="$test_root:$PATH" \
	FAKE_BAD_PROGRAM_CANDIDATE=native-compiler \
	/bin/sh "$script_directory/build-controller.sh" \
	test "$fake_runexec" fannkuch-redux \
	"$test_root/native-compiler" "$test_root/trb" "$test_root/qbe" "$test_root/cc" "$test_root/go" \
	linux-arm64-v0 0,1,2,3 "$cache_control" \
	"$test_root/incorrect-workspace" "$test_root/incorrect-evidence" \
	> "$test_root/incorrect.stdout" 2> "$test_root/incorrect.stderr"
incorrect_status=$?
set -e
test "$incorrect_status" -ne 0 || fail "incorrect preflight program reached timing"
test ! -e "$test_root/incorrect-evidence/raw.tsv" || fail "timing data exists after preflight failure"
test "$(find "$test_root/incorrect-evidence/observations" -name runexec.stdout -type f | wc -l | tr -d ' ')" -eq 0 ||
	fail "runexec started after preflight failure"

set +e
PATH="$test_root:$PATH" \
	FAKE_BAD_CHECK_CANDIDATE=native-compiler \
	/bin/sh "$script_directory/build-controller.sh" \
	test "$fake_runexec" fannkuch-redux \
	"$test_root/native-compiler" "$test_root/trb" "$test_root/qbe" "$test_root/cc" "$test_root/go" \
	linux-arm64-v0 0,1,2,3 "$cache_control" \
	"$test_root/check-workspace" "$test_root/check-evidence" \
	> "$test_root/check.stdout" 2> "$test_root/check.stderr"
check_status=$?
set -e
test "$check_status" -ne 0 || fail "unexpected check output reached build preflight"
grep 'Native preflight check stdout differs' "$test_root/check.stderr" >/dev/null ||
	fail "unexpected check output diagnostic differs"
test ! -e "$test_root/check-evidence/raw.tsv" || fail "timing data exists after check-output failure"

printf 'benchmarksgame-build-controller-test: passed\n'
