#!/bin/sh

set -eu

BENCH_EXEC_VERSION=3.35
MEMORY_LIMIT=4GB
WARMUP_ROUNDS=2
RETAINED_ROUNDS=11
TOTAL_CANDIDATES=7
CATALOG_HEADER='case	candidate	command	arg1	arg2	arg3	arg4	expected'
SUMMARY_HEADER='phase	round	retained_index	order	case	lane	candidate	verdict	returnvalue	exitsignal	terminationreason	walltime_seconds	cputime_seconds	memory_bytes'
CANDIDATES='typerb-native typerb-go c cpp go rust java'

usage() {
	cat >&2 <<'EOF'
usage: runtime-controller.sh test|formal RUNEXEC CATALOG CASE
       one-core|four-core CORES CACHE_CONTROL WORKSPACE EVIDENCE
EOF
	exit 64
}

fail() {
	printf 'benchmarksgame-runtime-controller: %s\n' "$1" >&2
	exit 1
}

measurement_value() {
	key=$1
	path=$2
	awk -F= -v wanted="$key" '
		$1 == wanted {
			count += 1
			value = substr($0, length($1) + 2)
		}
		END {
			if (count > 1) exit 1
			if (count == 1) print value
		}
	' "$path"
}

is_decimal() {
	awk -v value="$1" 'BEGIN { exit !(value ~ /^[0-9]+([.][0-9]+)?$/) }'
}

is_unsigned() {
	case "$1" in
	'' | *[!0-9]*) return 1 ;;
	*) return 0 ;;
	esac
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

core_is_allowed() {
	awk -v wanted="$1" -v list="$2" 'BEGIN {
		count = split(list, groups, ",")
		for (group_index = 1; group_index <= count; group_index += 1) {
			parts = split(groups[group_index], bounds, "-")
			first = bounds[1] + 0
			last = parts == 1 ? first : bounds[2] + 0
			if (wanted >= first && wanted <= last) exit 0
		}
		exit 1
	}'
}

test "$#" -eq 9 || usage
mode=$1
runexec=$2
catalog=$3
case_name=$4
lane=$5
cores=$6
cache_control=$7
workspace=$8
evidence=$9

case "$mode" in
test | formal) ;;
*) usage ;;
esac
case "$case_name" in
fannkuch-redux)
	walltime_limit=300s
	formal_input=12
	formal_expected_sha256=4265a65135c506a68d90d6474003fb9030b7ee244a06c046bd89b3932a28ce20
	;;
n-body)
	walltime_limit=300s
	formal_input=50000000
	formal_expected_sha256=3e6c9ef9d26cfe312a4cd8e1b81b3f671b88fbce84de543e8c23c206a942504d
	;;
spectral-norm)
	walltime_limit=600s
	formal_input=5500
	formal_expected_sha256=f9d5b5e3eb7657cf1bbba4cc856651864df9cd9fd9a6be9b9bc5fcbb67150deb
	;;
*) usage ;;
esac
case "$lane" in
one-core) required_core_count=1 ;;
four-core) required_core_count=4 ;;
*) usage ;;
esac

test -x "$runexec" || fail "runexec is not executable"
test -f "$catalog" || fail "catalog does not exist"
test -x "$cache_control" || fail "cache-control command is not executable"
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"
case "$cores" in
'' | *[!0-9,]*) fail "cores must be a comma-separated list of CPU identifiers" ;;
esac

core_count=$(awk -F, '{ print NF }' <<EOF
$cores
EOF
)
test "$core_count" -eq "$required_core_count" || fail "$lane requires $required_core_count CPU identifiers"
unique_core_count=$(awk -F, '{ for (i = 1; i <= NF; i += 1) seen[$i] = 1 } END { for (value in seen) count += 1; print count + 0 }' <<EOF
$cores
EOF
)
test "$unique_core_count" -eq "$core_count" || fail "CPU identifiers must be unique"

if test "$mode" = formal; then
	test "$(uname -s)" = Linux || fail "formal mode requires Linux"
	case "$(uname -m)" in
	aarch64 | arm64) ;;
	*) fail "formal mode requires Linux arm64" ;;
	esac
	runexec_version=$("$runexec" --version)
	test "$runexec_version" = "runexec $BENCH_EXEC_VERSION" ||
		fail "formal mode requires runexec $BENCH_EXEC_VERSION: $runexec_version"
	allowed_cores=$(awk -F: '$1 == "Cpus_allowed_list" { gsub(/[[:space:]]/, "", $2); print $2 }' /proc/self/status)
	test -n "$allowed_cores" || fail "cannot determine the allowed CPU set"
	old_ifs=$IFS
	IFS=,
	for core in $cores; do
		core_is_allowed "$core" "$allowed_cores" || fail "CPU $core is outside the allowed CPU set $allowed_cores"
	done
	IFS=$old_ifs
else
	runexec_version=$("$runexec" --version 2>/dev/null || printf 'test-double\n')
	allowed_cores=test-mode
fi

tab=$(printf '\t')
actual_header=$(sed -n '1p' "$catalog")
expected_header=$(printf "$CATALOG_HEADER")
test "$actual_header" = "$expected_header" || fail "catalog header differs"
awk -F '\t' 'NR > 1 && NF != 8 { exit 1 }' "$catalog" || fail "catalog rows must have eight fields"
catalog_rows=$(awk -F '\t' -v wanted="$case_name" 'NR > 1 && $1 == wanted { count += 1 } END { print count + 0 }' "$catalog")
test "$catalog_rows" -eq "$TOTAL_CANDIDATES" || fail "$case_name must have seven candidates"

mkdir -p "$workspace" "$evidence/correctness" "$evidence/observations"
case_catalog=$workspace/catalog.tsv
awk -F '\t' -v wanted="$case_name" 'NR > 1 && $1 == wanted { print }' "$catalog" > "$case_catalog"

candidate_position=1
for expected_candidate in $CANDIDATES; do
	row=$(sed -n "${candidate_position}p" "$case_catalog")
	IFS="$tab" read -r row_case actual_candidate command arg1 arg2 arg3 arg4 expected <<EOF
$row
EOF
	test "$actual_candidate" = "$expected_candidate" ||
		fail "candidate $candidate_position must be $expected_candidate"
	if test "$mode" = formal; then
		test -f "$expected" || fail "$actual_candidate formal expected output is missing"
		test "$(sha256 "$expected")" = "$formal_expected_sha256" ||
			fail "$actual_candidate formal expected output SHA-256 differs"
		if test "$actual_candidate" = java; then
			test "$arg1" = -cp && test "$arg4" = "$formal_input" ||
				fail "Java formal invocation differs"
			test "$arg2" != - && test "$arg3" != - || fail "Java classpath or class is missing"
		else
			test "$arg1" = "$formal_input" || fail "$actual_candidate formal input differs"
			test "$arg2" = - && test "$arg3" = - && test "$arg4" = - ||
				fail "$actual_candidate formal invocation has extra arguments"
		fi
	fi
	candidate_position=$((candidate_position + 1))
done

{
	printf 'mode=%s\n' "$mode"
	printf 'case=%s\n' "$case_name"
	printf 'lane=%s\n' "$lane"
	printf 'cores=%s\n' "$cores"
	printf 'memory_limit=%s\n' "$MEMORY_LIMIT"
	printf 'walltime_limit=%s\n' "$walltime_limit"
	printf 'warmup_rounds=%s\n' "$WARMUP_ROUNDS"
	printf 'retained_rounds=%s\n' "$RETAINED_ROUNDS"
	printf 'runexec=%s\n' "$runexec"
	printf 'runexec_version=%s\n' "$runexec_version"
	printf 'allowed_cores=%s\n' "$allowed_cores"
	printf 'cache_control=%s\n' "$cache_control"
	printf 'network_access=disabled-by-default-container-policy\n'
	printf 'root_filesystem=read-only\n'
	printf 'home_filesystem=isolated-overlay\n'
	uname -a | sed 's/^/uname=/'
	if test -r /proc/cpuinfo; then
		awk -F: '$1 ~ /model name|Model|CPU implementer|CPU part/ { gsub(/^[[:space:]]+/, "", $2); print "cpu_" NR "=" $2 }' /proc/cpuinfo
	fi
} > "$evidence/environment.txt"

printf 'case\tcandidate\tstatus\tstderr_empty\tstdout_exact\n' > "$evidence/correctness/summary.tsv"
correctness_failures=0
while IFS="$tab" read -r row_case candidate command arg1 arg2 arg3 arg4 expected; do
	test "$row_case" = "$case_name" || fail "filtered catalog contains another case"
	test -x "$command" || fail "$candidate command is not executable: $command"
	test -f "$expected" || fail "$candidate expected output is missing: $expected"
	correctness_directory=$evidence/correctness/$candidate
	mkdir -p "$correctness_directory"
	set -- "$command"
	for argument in "$arg1" "$arg2" "$arg3" "$arg4"; do
		if test "$argument" != -; then
			set -- "$@" "$argument"
		fi
	done
	set +e
	"$@" > "$correctness_directory/stdout" 2> "$correctness_directory/stderr"
	correctness_status=$?
	set -e
	stderr_empty=true
	stdout_exact=true
	if test -s "$correctness_directory/stderr"; then
		stderr_empty=false
		correctness_failures=$((correctness_failures + 1))
	fi
	if ! cmp "$expected" "$correctness_directory/stdout" >/dev/null; then
		stdout_exact=false
		correctness_failures=$((correctness_failures + 1))
	fi
	if test "$correctness_status" -ne 0; then
		correctness_failures=$((correctness_failures + 1))
	fi
	printf '%s\t%s\t%s\t%s\t%s\n' \
		"$case_name" "$candidate" "$correctness_status" "$stderr_empty" "$stdout_exact" \
		>> "$evidence/correctness/summary.tsv"
done < "$case_catalog"
test "$correctness_failures" -eq 0 || fail "correctness failed; timing was not started"

summary=$evidence/raw.tsv
printf "$SUMMARY_HEADER\n" > "$summary"
total_rounds=$((WARMUP_ROUNDS + RETAINED_ROUNDS))
measurement_failures=0
round=1
while test "$round" -le "$total_rounds"; do
	if test "$round" -le "$WARMUP_ROUNDS"; then
		phase=warmup
		retained_index=0
	else
		phase=retained
		retained_index=$((round - WARMUP_ROUNDS))
	fi
	order=1
	while test "$order" -le "$TOTAL_CANDIDATES"; do
		candidate_position=$(((round + order - 2) % TOTAL_CANDIDATES + 1))
		row=$(sed -n "${candidate_position}p" "$case_catalog")
		IFS="$tab" read -r row_case candidate command arg1 arg2 arg3 arg4 expected <<EOF
$row
EOF
		observation=$evidence/observations/round-$round-order-$order-$candidate
		mkdir -p "$observation"
		"$cache_control" "$observation/cache.txt" \
			> "$observation/cache.stdout" 2> "$observation/cache.stderr" ||
			fail "cache control failed for round $round order $order"

		set -- "$command"
		for argument in "$arg1" "$arg2" "$arg3" "$arg4"; do
			if test "$argument" != -; then
				set -- "$@" "$argument"
			fi
		done
		set +e
		"$runexec" \
			--quiet \
			--memlimit "$MEMORY_LIMIT" \
			--walltimelimit "$walltime_limit" \
			--cores "$cores" \
			--read-only-dir / \
			--overlay-dir /home \
			--output "$observation/run.log" \
			-- "$@" \
			> "$observation/runexec.stdout" \
			2> "$observation/runexec.stderr"
		runexec_status=$?
		set -e
		test "$runexec_status" -eq 0 || fail "runexec infrastructure failed for round $round order $order"
		test -f "$observation/run.log" || fail "runexec did not produce a run log"

		log_separator=$(sed -n '4p' "$observation/run.log")
		test "$log_separator" = '--------------------------------------------------------------------------------' ||
			fail "runexec log header differs for round $round order $order"
		sed '1,6d' "$observation/run.log" > "$observation/payload.stdout"
		returnvalue=$(measurement_value returnvalue "$observation/runexec.stdout") ||
			fail "runexec repeated returnvalue"
		exitsignal=$(measurement_value exitsignal "$observation/runexec.stdout") ||
			fail "runexec repeated exitsignal"
		terminationreason=$(measurement_value terminationreason "$observation/runexec.stdout") ||
			fail "runexec repeated terminationreason"
		walltime=$(measurement_value walltime "$observation/runexec.stdout") ||
			fail "runexec repeated walltime"
		cputime=$(measurement_value cputime "$observation/runexec.stdout") ||
			fail "runexec repeated cputime"
		memory=$(measurement_value memory "$observation/runexec.stdout") ||
			fail "runexec repeated memory"
		walltime=${walltime%s}
		cputime=${cputime%s}
		memory=${memory%B}

		verdict=pass
		if test -z "$walltime" || test -z "$cputime" || test -z "$memory"; then
			verdict=measurement-missing
		elif ! is_decimal "$walltime" || ! is_decimal "$cputime" || ! is_unsigned "$memory"; then
			verdict=measurement-invalid
		elif test -n "$returnvalue" && ! is_unsigned "$returnvalue"; then
			verdict=exit-invalid
		elif test -n "$exitsignal" && ! is_unsigned "$exitsignal"; then
			verdict=exit-invalid
		elif test -n "$returnvalue" && test -n "$exitsignal"; then
			verdict=exit-ambiguous
		elif test -z "$returnvalue" && test -z "$exitsignal"; then
			verdict=exit-missing
		elif test -n "$terminationreason"; then
			verdict=terminated-$terminationreason
		elif test -n "$exitsignal"; then
			verdict=signal-$exitsignal
		elif test "$returnvalue" -ne 0; then
			verdict=return-$returnvalue
		elif ! cmp "$expected" "$observation/payload.stdout" >/dev/null; then
			verdict=output-diff
		fi
		if test "$verdict" != pass; then
			measurement_failures=$((measurement_failures + 1))
		fi
		printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
			"$phase" "$round" "$retained_index" "$order" "$case_name" "$lane" "$candidate" \
			"$verdict" "$returnvalue" "$exitsignal" "$terminationreason" "$walltime" "$cputime" "$memory" \
			>> "$summary"
		order=$((order + 1))
	done
	round=$((round + 1))
done

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
awk -f "$script_directory/summarize.awk" "$summary" > "$evidence/medians.tsv" ||
	fail "measurement summary could not be generated"

if test "$measurement_failures" -ne 0; then
	fail "$measurement_failures measured runs failed; raw evidence is retained"
fi
printf 'benchmarksgame-runtime-controller: %s %s passed\n' "$case_name" "$lane"
