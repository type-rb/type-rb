#!/bin/sh

set -eu

BENCH_EXEC_VERSION=3.35
MEMORY_LIMIT=4GB
WALLTIME_LIMIT=300s
WARMUP_ROUNDS=2
RETAINED_ROUNDS=11
TOTAL_CANDIDATES=2
SUMMARY_HEADER='phase	round	retained_index	order	case	candidate	verdict	returnvalue	exitsignal	terminationreason	walltime_seconds	cputime_seconds	memory_bytes	artifact_bytes	artifact_sha256	program_status	program_stderr_empty	program_stdout_exact'
CANDIDATES='typerb-native typerb-go'

usage() {
	cat >&2 <<'EOF'
usage: build-controller.sh test|formal RUNEXEC CASE NATIVE_COMPILER
       REFERENCE_TRB QBE CC GO TARGET CORES CACHE_CONTROL WORKSPACE EVIDENCE
EOF
	exit 64
}

fail() {
	printf 'benchmarksgame-build-controller: %s\n' "$1" >&2
	exit 1
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

file_size() {
	wc -c < "$1" | tr -d ' '
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

resolve_executable() {
	value=$1
	case "$value" in
	/*) ;;
	*) value=$(command -v "$value" 2>/dev/null || true) ;;
	esac
	test -n "$value" || return 1
	if command -v readlink >/dev/null 2>&1; then
		resolved=$(readlink -f "$value" 2>/dev/null || true)
		if test -n "$resolved"; then value=$resolved; fi
	fi
	printf '%s\n' "$value"
}

directory_apparent_size() {
	path=$1
	if du -sb --apparent-size "$path" >/dev/null 2>&1; then
		du -sb --apparent-size "$path" | awk '{ print $1 }'
	else
		du -sk "$path" | awk '{ print $1 * 1024 }'
	fi
}

record_file() {
	scope=$1
	component=$2
	path=$3
	resolved=$(resolve_executable "$path") || fail "cannot resolve distribution component: $path"
	test -f "$resolved" || fail "distribution component is not a file: $resolved"
	printf '%s\t%s\t%s\t%s\t%s\n' \
		"$scope" "$component" "$resolved" "$(file_size "$resolved")" "$(sha256 "$resolved")" \
		>> "$evidence/distribution/files.tsv"
}

record_dependencies() {
	label=$1
	path=$2
	scope=$3
	resolved=$(resolve_executable "$path") || fail "cannot resolve dependency component: $path"
	output=$evidence/distribution/dependencies/$label.txt
	if test "$mode" = formal; then
		ldd "$resolved" > "$output" 2>&1 || true
	else
		printf 'not-collected-in-test-mode\n' > "$output"
	fi
	printf '%s\t%s\t%s\t%s\n' "$scope" "$label" "$resolved" "dependencies/$label.txt" \
		>> "$evidence/distribution/dependencies.tsv"
}

clean_output() {
	output=$1
	rm -f "$output"
	find "$(dirname -- "$output")" -maxdepth 1 -type f -name '*.trbn.*' -exec rm -f {} +
	find "$(dirname -- "$output")" -maxdepth 1 -type d -name '*.trbn.*' -exec rm -rf {} +
}

build_direct() {
	candidate=$1
	output=$2
	stdout_path=$3
	stderr_path=$4
	clean_output "$output"
	if test "$candidate" = typerb-native; then
		"$native_compiler" build "$source" \
			--output "$output" \
			--qbe "$qbe" \
			--cc "$cc" \
			--target "$target" \
			> "$stdout_path" 2> "$stderr_path"
	else
		(
			cd "$case_root"
			"$reference_trb" build --compile --config "$config" --outfile "$output"
		) > "$stdout_path" 2> "$stderr_path"
	fi
}

run_program_check() {
	program=$1
	stdout_path=$2
	stderr_path=$3
	set +e
	"$program" "$correctness_input" > "$stdout_path" 2> "$stderr_path"
	program_status=$?
	set -e
	program_stderr_empty=true
	program_stdout_exact=true
	if test -s "$stderr_path"; then program_stderr_empty=false; fi
	if ! cmp "$expected" "$stdout_path" >/dev/null; then program_stdout_exact=false; fi
}

trace_build() {
	candidate=$1
	output=$2
	trace=$3
	stdout_path=$4
	stderr_path=$5
	clean_output "$output"
	if test "$mode" = formal; then
		strace=$(command -v strace)
		test -x "$strace" || fail "strace is required in formal mode"
		if test "$candidate" = typerb-native; then
			"$strace" -f -e trace=process -o "$trace" \
				"$native_compiler" build "$source" \
				--output "$output" --qbe "$qbe" --cc "$cc" --target "$target" \
				> "$stdout_path" 2> "$stderr_path"
		else
			(
				cd "$case_root"
				"$strace" -f -e trace=process -o "$trace" \
					"$reference_trb" build --compile --config "$config" --outfile "$output"
			) > "$stdout_path" 2> "$stderr_path"
		fi
	else
		build_direct "$candidate" "$output" "$stdout_path" "$stderr_path"
		printf 'test-mode direct build: %s\n' "$candidate" > "$trace"
	fi
}

test "$#" -eq 13 || usage
mode=$1
runexec=$2
case_name=$3
native_compiler=$4
reference_trb=$5
qbe=$6
cc=$7
go_tool=$8
target=$9
shift 9
cores=$1
cache_control=$2
workspace=$3
evidence=$4

case "$mode" in
test | formal) ;;
*) usage ;;
esac
case "$case_name" in
fannkuch-redux)
	correctness_input=7
	expected_name=7
	;;
n-body)
	correctness_input=1000
	expected_name=1000
	;;
spectral-norm)
	correctness_input=100
	expected_name=100
	;;
*) usage ;;
esac

for executable in "$runexec" "$native_compiler" "$reference_trb" "$qbe" "$cc" "$go_tool" "$cache_control"; do
	test -x "$executable" || fail "required tool is not executable: $executable"
done
test "$target" = linux-arm64-v0 || usage
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"
case "$cores" in
'' | *[!0-9,]*) fail "cores must be a comma-separated list of CPU identifiers" ;;
esac
core_count=$(awk -F, '{ print NF }' <<EOF
$cores
EOF
)
test "$core_count" -eq 4 || fail "build measurement requires four CPU identifiers"
unique_core_count=$(awk -F, '{ for (i = 1; i <= NF; i += 1) seen[$i] = 1 } END { for (value in seen) count += 1; print count + 0 }' <<EOF
$cores
EOF
)
test "$unique_core_count" -eq 4 || fail "CPU identifiers must be unique"

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
	test "$(command -v go)" = "$go_tool" || fail "Go command does not match PATH"
else
	runexec_version=$("$runexec" --version 2>/dev/null || printf 'test-double\n')
	allowed_cores=test-mode
fi

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/../.." && pwd)
case_root=$repository_root/benchmarks/benchmarksgame/$case_name
source=$case_root/src/main.trb
config=$case_root/trbconfig.jsonc
expected=$case_root/expected/$expected_name.txt
test -f "$source" || fail "case source is missing"
test -f "$config" || fail "case config is missing"
test -f "$expected" || fail "case expected output is missing"

mkdir -p \
	"$workspace/artifacts/typerb-native" \
	"$workspace/artifacts/typerb-go" \
	"$workspace/go-cache" \
	"$workspace/go-module-cache" \
	"$evidence/preflight/typerb-native" \
	"$evidence/preflight/typerb-go" \
	"$evidence/observations" \
	"$evidence/artifacts/typerb-native" \
	"$evidence/artifacts/typerb-go" \
	"$evidence/distribution/dependencies" \
	"$evidence/distribution/process-traces"

export GOCACHE=$workspace/go-cache
export GOMODCACHE=$workspace/go-module-cache
export TMPDIR=/tmp

{
	printf 'mode=%s\n' "$mode"
	printf 'case=%s\n' "$case_name"
	printf 'target=%s\n' "$target"
	printf 'cores=%s\n' "$cores"
	printf 'memory_limit=%s\n' "$MEMORY_LIMIT"
	printf 'walltime_limit=%s\n' "$WALLTIME_LIMIT"
	printf 'warmup_rounds=%s\n' "$WARMUP_ROUNDS"
	printf 'retained_rounds=%s\n' "$RETAINED_ROUNDS"
	printf 'runexec=%s\n' "$runexec"
	printf 'runexec_version=%s\n' "$runexec_version"
	printf 'allowed_cores=%s\n' "$allowed_cores"
	printf 'cache_control=%s\n' "$cache_control"
	printf 'go_cache=%s\n' "$GOCACHE"
	printf 'go_module_cache=%s\n' "$GOMODCACHE"
	printf 'output_policy=deleted-before-every-observation\n'
	printf 'root_filesystem=read-only\n'
	printf 'home_filesystem=isolated-overlay\n'
	printf 'tmp_filesystem=benchexec-default-hidden\n'
	printf 'writable_workspace=%s\n' "$workspace"
	printf 'source_sha256=%s\n' "$(sha256 "$source")"
	printf 'expected_sha256=%s\n' "$(sha256 "$expected")"
	uname -a | sed 's/^/uname=/'
} > "$evidence/environment.txt"

"$native_compiler" check "$source" \
	> "$evidence/preflight/typerb-native/check.stdout" \
	2> "$evidence/preflight/typerb-native/check.stderr"
(
	cd "$case_root"
	"$reference_trb" check --config "$config"
) > "$evidence/preflight/typerb-go/check.stdout" \
	2> "$evidence/preflight/typerb-go/check.stderr"
test "$(cat "$evidence/preflight/typerb-native/check.stdout")" = ok ||
	fail "Native preflight check stdout differs"
test ! -s "$evidence/preflight/typerb-native/check.stderr" || fail "Native preflight check wrote stderr"
test "$(cat "$evidence/preflight/typerb-go/check.stdout")" = 'checked 1 file(s) for mode go' ||
	fail "Go preflight check stdout differs"
test ! -s "$evidence/preflight/typerb-go/check.stderr" || fail "Go preflight check wrote stderr"

printf 'candidate\tbuild_status\tbuild_stderr_empty\tprogram_status\tprogram_stderr_empty\tprogram_stdout_exact\n' \
	> "$evidence/preflight/summary.tsv"
for candidate in $CANDIDATES; do
	preflight=$evidence/preflight/$candidate
	program=$workspace/artifacts/$candidate/program
	set +e
	trace_build "$candidate" "$program" \
		"$evidence/distribution/process-traces/$candidate.trace" \
		"$preflight/build.stdout" "$preflight/build.stderr"
	build_status=$?
	set -e
	build_stderr_empty=true
	if test -s "$preflight/build.stderr"; then build_stderr_empty=false; fi
	program_status=
	program_stderr_empty=false
	program_stdout_exact=false
	if test "$build_status" -eq 0 && test -x "$program"; then
		run_program_check "$program" "$preflight/program.stdout" "$preflight/program.stderr"
	fi
	printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$candidate" "$build_status" "$build_stderr_empty" "$program_status" \
		"$program_stderr_empty" "$program_stdout_exact" >> "$evidence/preflight/summary.tsv"
	test "$build_status" -eq 0 || fail "$candidate preflight build failed"
	test "$build_stderr_empty" = true || fail "$candidate preflight build wrote stderr"
	test "$program_status" -eq 0 || fail "$candidate preflight program failed"
	test "$program_stderr_empty" = true || fail "$candidate preflight program wrote stderr"
	test "$program_stdout_exact" = true || fail "$candidate preflight program output differs"
done

# The traced correctness preflight is not one of the registered warmups. Reset
# the explicit Go caches so only the two scheduled warmups establish the cache
# state used by retained observations.
rm -rf "$GOCACHE" "$GOMODCACHE"
mkdir -p "$GOCACHE" "$GOMODCACHE"

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
		candidate=$(printf '%s\n' $CANDIDATES | sed -n "${candidate_position}p")
		observation=$evidence/observations/round-$round-order-$order-$candidate
		mkdir -p "$observation"
		"$cache_control" "$observation/cache.txt" \
			> "$observation/cache.stdout" 2> "$observation/cache.stderr" ||
			fail "cache control failed for round $round order $order"
		program=$workspace/artifacts/$candidate/program
		clean_output "$program"

		set +e
		if test "$candidate" = typerb-native; then
			(
				cd "$case_root"
				"$runexec" \
					--quiet \
					--memlimit "$MEMORY_LIMIT" \
					--walltimelimit "$WALLTIME_LIMIT" \
					--cores "$cores" \
					--read-only-dir / \
					--overlay-dir /home \
					--full-access-dir "$workspace" \
					--output "$observation/run.log" \
					-- "$native_compiler" build "$source" \
						--output "$program" --qbe "$qbe" --cc "$cc" --target "$target"
			) > "$observation/runexec.stdout" 2> "$observation/runexec.stderr"
		else
			(
				cd "$case_root"
				"$runexec" \
					--quiet \
					--memlimit "$MEMORY_LIMIT" \
					--walltimelimit "$WALLTIME_LIMIT" \
					--cores "$cores" \
					--read-only-dir / \
					--overlay-dir /home \
					--full-access-dir "$workspace" \
					--output "$observation/run.log" \
					-- "$reference_trb" build --compile --config "$config" --outfile "$program"
			) > "$observation/runexec.stdout" 2> "$observation/runexec.stderr"
		fi
		runexec_status=$?
		set -e
		test "$runexec_status" -eq 0 || fail "runexec infrastructure failed for round $round order $order"
		test -f "$observation/run.log" || fail "runexec did not produce a run log"
		log_separator=$(sed -n '4p' "$observation/run.log")
		test "$log_separator" = '--------------------------------------------------------------------------------' ||
			fail "runexec log header differs for round $round order $order"

		returnvalue=$(measurement_value returnvalue "$observation/runexec.stdout") || fail "runexec repeated returnvalue"
		exitsignal=$(measurement_value exitsignal "$observation/runexec.stdout") || fail "runexec repeated exitsignal"
		terminationreason=$(measurement_value terminationreason "$observation/runexec.stdout") || fail "runexec repeated terminationreason"
		walltime=$(measurement_value walltime "$observation/runexec.stdout") || fail "runexec repeated walltime"
		cputime=$(measurement_value cputime "$observation/runexec.stdout") || fail "runexec repeated cputime"
		memory=$(measurement_value memory "$observation/runexec.stdout") || fail "runexec repeated memory"
		walltime=${walltime%s}
		cputime=${cputime%s}
		memory=${memory%B}

		verdict=pass
		artifact_bytes=
		artifact_sha256=
		program_status=
		program_stderr_empty=false
		program_stdout_exact=false
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
		elif test ! -x "$program"; then
			verdict=artifact-missing
		else
			artifact_bytes=$(file_size "$program")
			artifact_sha256=$(sha256 "$program")
			run_program_check "$program" "$observation/program.stdout" "$observation/program.stderr"
			if test "$program_status" -ne 0; then
				verdict=program-return-$program_status
			elif test "$program_stderr_empty" != true; then
				verdict=program-stderr
			elif test "$program_stdout_exact" != true; then
				verdict=program-output-diff
			fi
		fi
		if test "$verdict" != pass; then measurement_failures=$((measurement_failures + 1)); fi
		printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
			"$phase" "$round" "$retained_index" "$order" "$case_name" "$candidate" "$verdict" \
			"$returnvalue" "$exitsignal" "$terminationreason" "$walltime" "$cputime" "$memory" \
			"$artifact_bytes" "$artifact_sha256" "$program_status" "$program_stderr_empty" "$program_stdout_exact" \
			>> "$summary"
		order=$((order + 1))
	done
	round=$((round + 1))
done

awk -f "$script_directory/summarize.awk" "$summary" > "$evidence/medians.tsv" ||
	fail "measurement summary could not be generated"

printf 'candidate\tkind\tbytes\tsha256\n' > "$evidence/artifacts.tsv"
printf 'scope\tlabel\tpath\tevidence\n' > "$evidence/distribution/dependencies.tsv"
for candidate in $CANDIDATES; do
	program=$workspace/artifacts/$candidate/program
	test -x "$program" || continue
	raw=$evidence/artifacts/$candidate/program.raw
	stripped=$evidence/artifacts/$candidate/program.stripped
	cp "$program" "$raw"
	if test "$mode" = formal; then
		strip --strip-all -o "$stripped" "$program"
	else
		cp "$program" "$stripped"
	fi
	printf '%s\traw\t%s\t%s\n' "$candidate" "$(file_size "$raw")" "$(sha256 "$raw")" >> "$evidence/artifacts.tsv"
	printf '%s\tstripped\t%s\t%s\n' "$candidate" "$(file_size "$stripped")" "$(sha256 "$stripped")" >> "$evidence/artifacts.tsv"
	record_dependencies "$candidate-program" "$program" deploy-artifact
done

printf 'scope\tcomponent\tpath\tbytes\tsha256\n' > "$evidence/distribution/files.tsv"
record_file native-controlled native-compiler "$native_compiler"
record_file native-controlled qbe "$qbe"
record_file go-controlled reference-trb "$reference_trb"
record_file go-controlled go-command "$go_tool"
record_file native-host-prerequisite cc "$cc"

if test "$mode" = formal; then
	printf 'candidate\tpath\n' > "$evidence/distribution/process-executables.tsv"
	for candidate in $CANDIDATES; do
		trace=$evidence/distribution/process-traces/$candidate.trace
		awk -f "$script_directory/trace-executables.awk" "$trace" |
			awk -v candidate="$candidate" '{ print candidate "\t" $0 }'
	done | LC_ALL=C sort -u >> "$evidence/distribution/process-executables.tsv"
	observed_index=0
	tab=$(printf '\t')
	sed '1d' "$evidence/distribution/process-executables.tsv" |
	while IFS="$tab" read -r observed_candidate executable; do
		test -n "$executable" || continue
		resolved=$(resolve_executable "$executable" 2>/dev/null || true)
		if test -n "$resolved" && test -f "$resolved"; then
			observed_index=$((observed_index + 1))
			observed_label=$(printf 'observed-%03d' "$observed_index")
			observed_scope=observed-$observed_candidate-build-closure
			printf '%s\t%s\t%s\t%s\t%s\n' \
				"$observed_scope" executable "$resolved" "$(file_size "$resolved")" "$(sha256 "$resolved")" \
				>> "$evidence/distribution/files.tsv"
			record_dependencies "$observed_label" "$resolved" "$observed_scope"
		fi
	done
	go_root=$($go_tool env GOROOT)
	test -d "$go_root" || fail "Go root is not a directory"
	go_root_bytes=$(directory_apparent_size "$go_root")
	go_root_files=$(find "$go_root" -type f | wc -l | tr -d ' ')
	printf 'scope\tcomponent\tpath\tapparent_bytes\tfiles\n' > "$evidence/distribution/directories.tsv"
	printf 'go-controlled\tgo-root\t%s\t%s\t%s\n' "$go_root" "$go_root_bytes" "$go_root_files" \
		>> "$evidence/distribution/directories.tsv"
	for label_path in \
		"native-controlled:native-compiler:$native_compiler" \
		"native-controlled:qbe:$qbe" \
		"go-controlled:reference-trb:$reference_trb" \
		"go-controlled:go:$go_tool" \
		"native-host-prerequisite:cc:$cc"; do
		dependency_scope=${label_path%%:*}
		label_and_path=${label_path#*:}
		label=${label_and_path%%:*}
		path=${label_and_path#*:}
		record_dependencies "$label" "$path" "$dependency_scope"
	done
	for candidate in $CANDIDATES; do
		trace=$evidence/distribution/process-traces/$candidate.trace
		if grep -E 'execve\("[^"]*/(sh|bash|zsh)"' "$trace" >/dev/null; then
			fail "$candidate preflight build launched an unregistered shell"
		fi
	done
	strip_root=$workspace/distribution-stripped
	mkdir -p "$strip_root"
	strip --strip-all -o "$strip_root/native-compiler" "$native_compiler"
	strip --strip-all -o "$strip_root/qbe" "$qbe"
	strip --strip-all -o "$strip_root/reference-trb" "$reference_trb"
	native_raw_total=$(( $(file_size "$native_compiler") + $(file_size "$qbe") ))
	native_stripped_total=$(( $(file_size "$strip_root/native-compiler") + $(file_size "$strip_root/qbe") ))
	go_raw_total=$(( $(file_size "$reference_trb") + go_root_bytes ))
	go_stripped_total=$(( $(file_size "$strip_root/reference-trb") + go_root_bytes ))
	{
		printf 'scope\tkind\tbytes\n'
		printf 'native-controlled\traw\t%s\n' "$native_raw_total"
		printf 'native-controlled\tstripped\t%s\n' "$native_stripped_total"
		printf 'go-controlled\traw\t%s\n' "$go_raw_total"
		printf 'go-controlled\tstripped-compiler\t%s\n' "$go_stripped_total"
	} > "$evidence/distribution/payload-totals.tsv"
	{
		printf 'native-controlled payload includes the Native compiler and QBE.\n'
		printf 'Native host prerequisites are inventoried but excluded from the controlled payload total.\n'
		printf 'Go-controlled payload includes reference trb and the complete pinned GOROOT tree.\n'
		printf 'Deploy artifacts and dynamic dependencies are recorded separately from build payloads.\n'
	} > "$evidence/distribution/boundary.txt"
else
	printf 'scope\tcomponent\tpath\tapparent_bytes\tfiles\n' > "$evidence/distribution/directories.tsv"
	printf 'test-mode\tgo-root\t%s\t0\t0\n' "$($go_tool env GOROOT)" >> "$evidence/distribution/directories.tsv"
	printf 'scope\tkind\tbytes\n' > "$evidence/distribution/payload-totals.tsv"
	printf 'test-mode inventory only\n' > "$evidence/distribution/boundary.txt"
	printf 'candidate\tpath\n' > "$evidence/distribution/process-executables.tsv"
	printf 'test-mode\t%s\n' "$native_compiler" "$reference_trb" "$qbe" "$cc" "$go_tool" |
		LC_ALL=C sort -u >> "$evidence/distribution/process-executables.tsv"
fi

if test "$measurement_failures" -ne 0; then
	fail "$measurement_failures measured builds failed; raw evidence is retained"
fi
if awk -F '\t' 'NR > 1 && $14 != "pass" { exit 1 }' "$evidence/medians.tsv"; then :; else
	fail "retained artifacts or medians are incomplete"
fi
printf 'benchmarksgame-build-controller: %s passed\n' "$case_name"
