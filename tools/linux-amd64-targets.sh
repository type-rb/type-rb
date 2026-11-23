#!/bin/sh

set -eu

PRE_IMPLEMENTATION_REVISION=266c996668a4c3e0ad6eb833ca646b73ca7e56e1
TYPE_RB_REVISION=cc8c734f73f5149464838825cdbb2165058ed26d
TYPE_RB_VERSION=0.4.12-dev
ROOT_QBE_SIZE=658639
ROOT_QBE_SHA256=62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a
PROFILE=linux-amd64-v0
RUNNER_IMAGE=ubuntu-24.04
APPLICATION_BUILD_ELAPSED_REPETITIONS=8
APPLICATION_RUNTIME_ELAPSED_REPETITIONS=32

usage() {
	cat >&2 <<'EOF'
usage: linux-amd64-targets.sh CANDIDATE_ROOT ROOT_QBE QBE CC
       REFERENCE_TRB GO WORKSPACE EVIDENCE OUTPUT_COMPILER [SEED_SOURCE_ROOT [LOOP_SOURCE_ROOT [BOOLEAN_SOURCE_ROOT [RECORD_SOURCE_ROOT [HASH_SOURCE_ROOT [ITERATION_SOURCE_ROOT [NAMES_SOURCE_ROOT [CYCLES_SOURCE_ROOT]]]]]]]]
EOF
	exit 64
}

fail() {
	printf 'linux-amd64-targets: %s\n' "$1" >&2
	exit 1
}

sha256() {
	sha256sum "$1" | awk '{print $1}'
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

require_empty_file() {
	test ! -s "$1" || fail "$2"
}

require_no_intermediates() {
	if find "$1" \( -name '*.trbn.*' -o -name '*.native-target-external.*' \) -print |
		grep . > /dev/null 2>&1; then
		fail "Native build left an intermediate below $1"
	fi
}

require_clean_revision() {
	root=$1
	label=$2
	test -n "$(git -C "$root" rev-parse HEAD)" || fail "$label has no revision"
	test -z "$(git -C "$root" status --porcelain)" || fail "$label worktree is not clean"
}

require_forbidden_processes_absent() {
	trace=$1
	label=$2
	if grep -E 'execve\("[^"]*/(go|trb|sh|bash|dash|zsh)"|compiler-recovery' "$trace" > /dev/null; then
		fail "$label launched Go, reference trb, a shell, or a recovery generator"
	fi
}

require_tool_observed() {
	trace=$1
	pattern=$2
	label=$3
	grep -E "$pattern" "$trace" > /dev/null || fail "$label was not observed"
}

write_successful_exec_inventory() {
	trace=$1
	inventory=$2
	awk '
		function executable_path(line, path) {
			path = line
			sub(/^.*execve\("/, "", path)
			sub(/".*$/, "", path)
			return path
		}
		{
			pid = $1
			if (index($0, "execve(\"") != 0) {
				path = executable_path($0)
				if ($0 ~ /<unfinished \.\.\.>$/) {
					pending[pid] = path
				} else if ($0 ~ /= 0$/) {
					print path
				}
				next
			}
			if ($0 ~ /<\.\.\. execve resumed>/) {
				if ($0 ~ /= 0$/ && pid in pending) {
					print pending[pid]
				}
				delete pending[pid]
			}
		}
	' "$trace" > "$inventory"
}

require_exact_tool_observed() {
	inventory=$1
	tool=$2
	label=$3
	grep -Fx "$tool" "$inventory" > /dev/null ||
		fail "$label was not observed at $tool"
}

require_exact_assembler_observed() {
	inventory=$1
	label=$2
	if grep -Fx "$assembler_command" "$inventory" > /dev/null; then
		return
	fi
	if test -n "$architecture_assembler_command" &&
		grep -Fx "$architecture_assembler_command" "$inventory" > /dev/null; then
		return
	fi
	fail "$label assembler was not observed at a registered path"
}

require_closed_ordinary_trace() {
	trace=$1
	compiler=$2
	label=$3
	inventory=$trace.executables
	write_successful_exec_inventory "$trace" "$inventory"
	test -s "$inventory" || fail "$label process inventory is empty"
	while IFS= read -r executable; do
		case "$executable" in
		"$compiler" | "$qbe" | "$cc" | "$cc1_command" | "$collect2_command" | \
			"$assembler_command" | "$architecture_assembler_command" | "$ld_lld_command") ;;
		*) fail "$label launched an unregistered executable: $executable" ;;
		esac
	done < "$inventory"
	require_exact_tool_observed "$inventory" "$qbe" "$label QBE"
	require_exact_tool_observed "$inventory" "$cc" "$label CC"
	require_exact_assembler_observed "$inventory" "$label"
	require_exact_tool_observed "$inventory" "$ld_lld_command" "$label LLD"
	require_forbidden_processes_absent "$trace" "$label"
	if grep -F -- '--source-content' "$trace" > /dev/null; then
		fail "$label used the hidden source-content adapter"
	fi
}

require_successful_command() {
	stdout=$1
	stderr=$2
	label=$3
	shift 3
	"$@" > "$stdout" 2> "$stderr" || fail "$label failed"
	require_empty_file "$stdout" "$label wrote stdout"
	require_empty_file "$stderr" "$label wrote stderr"
}

require_go_build() {
	stdout=$1
	stderr=$2
	expected=$3
	output=$4
	label=$5
	shift 5
	"$@" > "$stdout" 2> "$stderr" || fail "$label failed"
	printf 'executable -> %s\n' "$output" > "$expected"
	cmp "$expected" "$stdout" > /dev/null || fail "$label stdout differs"
	require_empty_file "$stderr" "$label wrote stderr"
	test -x "$output" || fail "$label did not publish an executable"
}

test "$#" -eq 9 || test "$#" -eq 10 || test "$#" -eq 11 || test "$#" -eq 12 || test "$#" -eq 13 || test "$#" -eq 14 || test "$#" -eq 15 || test "$#" -eq 16 || test "$#" -eq 17 || usage

candidate_root=$1
root_qbe=$2
qbe=$3
cc=$4
reference_trb=$5
go_command=$6
workspace=$7
evidence=$8
output_compiler=$9
seed_source_root=${10:-$candidate_root}
loop_source_root=${11:-}
boolean_source_root=${12:-}
record_source_root=${13:-}
hash_source_root=${14:-}
iteration_source_root=${15:-}
names_source_root=${16:-}
cycles_source_root=${17:-}

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
verifier_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
. "$script_directory/compiler-project.sh"
. "$script_directory/compiler-cost.sh"
compiler_cost_mode > /dev/null || exit 64
external_recipe=$verifier_root/tools/external-qbe-build.sh
measurement_controller=$verifier_root/tools/measure-command.py

test "$(uname -s)" = Linux || fail "Linux is required"
case "$(uname -m)" in
x86_64 | amd64) ;;
*) fail "Linux amd64 is required" ;;
esac

for command_name in as awk cmp cp cut file find git grep jq ld.lld ldd nm objdump python3 readelf sed sha256sum strace strings strip; do
	command -v "$command_name" > /dev/null 2>&1 || fail "$command_name is required"
done
python_command=$(command -v python3)
test -x "$python_command" || fail "resolved Python command is not executable"

verify_binary_format() {
record_native_elf() {
	label=$1
	executable=$2
	directory=$evidence/elf/$label
	mkdir -p "$directory"
	file "$executable" > "$directory/file.txt"
	readelf -W -h "$executable" > "$directory/header.txt"
	readelf -W -l "$executable" > "$directory/segments.txt"
	readelf -W -S "$executable" > "$directory/sections.txt"
	readelf -W -d "$executable" > "$directory/dependencies.txt"
	readelf -W -n "$executable" > "$directory/notes.txt"
	readelf -W -s "$executable" > "$directory/symbols.txt"
	nm -D -u "$executable" > "$directory/undefined-symbols.txt"
	ldd "$executable" > "$directory/ldd.txt"
	strings -a "$executable" > "$directory/strings.txt"

	grep -Eq 'Class:[[:space:]]+ELF64' "$directory/header.txt" || fail "$label is not ELF64"
	grep -Eq 'Machine:[[:space:]]+Advanced Micro Devices X86-64' "$directory/header.txt" ||
		fail "$label is not x86-64"
	grep -Eq 'Type:[[:space:]]+DYN' "$directory/header.txt" || fail "$label is not a PIE executable"
	grep -Eq 'INTERP' "$directory/segments.txt" || fail "$label has no ELF interpreter"
	grep -Eq 'GNU_RELRO' "$directory/segments.txt" || fail "$label has no GNU_RELRO segment"
	stack_count=$(awk '$1 == "GNU_STACK" {count += 1} END {print count + 0}' "$directory/segments.txt")
	test "$stack_count" -eq 1 || fail "$label does not have exactly one GNU_STACK segment"
	if awk '$1 == "GNU_STACK" && $(NF - 1) ~ /E/ {found = 1} END {exit !found}' \
		"$directory/segments.txt"; then
		fail "$label requests an executable stack"
	fi
	if grep -Eq '\.go\.buildinfo|\.note\.go\.buildid' "$directory/sections.txt"; then
		fail "$label contains a Go build-information section"
	fi
	set +e
	"$go_command" version -m "$executable" \
		> "$directory/go-version-m.stdout" \
		2> "$directory/go-version-m.stderr"
	go_metadata_status=$?
	set -e
	printf '%s\n' "$go_metadata_status" > "$directory/go-version-m.status"
	test "$go_metadata_status" -ne 0 || fail "$label unexpectedly contains Go build metadata"
}

record_native_elf compiler "$output_compiler"
record_native_elf portable-entry "$native_application"
grep -Eq '=d sqrt ' "$evidence/applications/portable-entry-first.ssa" ||
	fail "Native portable-entry QBE does not contain the sqrt operation"
objdump -d "$native_application" > "$evidence/elf/portable-entry/disassembly.txt"
grep -Eq '[[:space:]]sqrtsd[[:space:]]' "$evidence/elf/portable-entry/disassembly.txt" ||
	fail "Native portable-entry executable does not contain scalar square root"

mkdir -p "$evidence/go-elf"
file "$go_application" > "$evidence/go-elf/file.txt"
readelf -W -h "$go_application" > "$evidence/go-elf/header.txt"
readelf -W -l "$go_application" > "$evidence/go-elf/segments.txt"
readelf -W -S "$go_application" > "$evidence/go-elf/sections.txt"
readelf -W -n "$go_application" > "$evidence/go-elf/notes.txt"
set +e
readelf -W -d "$go_application" > "$evidence/go-elf/dependencies.txt" 2> "$evidence/go-elf/dependencies.stderr"
go_dynamic_status=$?
ldd "$go_application" > "$evidence/go-elf/ldd.txt" 2> "$evidence/go-elf/ldd.stderr"
go_ldd_status=$?
set -e
printf '%s\n' "$go_dynamic_status" > "$evidence/go-elf/dependencies.status"
printf '%s\n' "$go_ldd_status" > "$evidence/go-elf/ldd.status"
"$go_command" version -m "$go_application" \
	> "$evidence/go-elf/go-version-m.stdout" \
	2> "$evidence/go-elf/go-version-m.stderr" || fail "optimized Go metadata probe failed"

}

run_formal_evidence() {
measurements=$evidence/measurements.csv
measurement_logs=$evidence/measurement-logs
mkdir -p "$measurement_logs"
printf 'stage,candidate,iteration,elapsed_seconds_per_repetition,elapsed_status,peak_rss_bytes,rss_status,observer_status\n' \
	> "$measurements"

record_input_identity() {
	input=$1
	output=$2
	if test -f "$input"; then
		{
			printf 'path=%s\n' "$input"
			printf 'exists=true\n'
			printf 'executable=%s\n' "$(test -x "$input" && printf true || printf false)"
			printf 'size=%s\n' "$(file_size "$input")"
			printf 'sha256=%s\n' "$(sha256 "$input")"
		} > "$output"
	else
		{
			printf 'path=%s\n' "$input"
			printf 'exists=false\n'
		} > "$output"
	fi
}

measure_command() {
	stage=$1
	candidate=$2
	iteration=$3
	artifact=$4
	probe_expected=$5
	probe_argument=$6
	shift 6
	measurement_input=$1
	measurement_repetitions=1
	case "$stage" in
	application-build) measurement_repetitions=$APPLICATION_BUILD_ELAPSED_REPETITIONS ;;
	application-runtime) measurement_repetitions=$APPLICATION_RUNTIME_ELAPSED_REPETITIONS ;;
	esac
	log_prefix=$measurement_logs/$stage-$candidate-$iteration
	if test "$artifact" != -; then
		rm -f "$artifact"
	fi
	set +e
	"$python_command" "$measurement_controller" \
		"$log_prefix.elapsed.json" \
		"$log_prefix.elapsed.stdout" \
		"$log_prefix.elapsed.stderr" \
		"$artifact" "$measurement_repetitions" -- "$@" \
		> "$log_prefix.observer.stdout" \
		2> "$log_prefix.observer.stderr"
	measurement_observer_status=$?
	set -e
	measurement_elapsed=0
	measurement_elapsed_status=255
	if test "$measurement_observer_status" -eq 0 &&
		jq -e '
			.schemaVersion == 1 and
			.clock == "time.monotonic_ns" and
			.clockInfo.monotonic == true and
			.clockInfo.adjustable == false and
			(.clockInfo.implementation | type) == "string" and
			(.clockInfo.implementation | length) > 0 and
			(.clockInfo.resolutionSeconds | type) == "number" and
			.clockInfo.resolutionSeconds > 0 and
			(.repetitions | type) == "number" and
			.repetitions == (.repetitions | floor) and
			.repetitions >= 1 and
			.completedRepetitions == .repetitions and
			.outputsIdentical == true and
			(.elapsedNanoseconds | type) == "number" and
			.elapsedNanoseconds > 0 and
			(.elapsedSeconds | type) == "string" and
			(.elapsedSeconds | test("^[0-9]+[.][0-9]{9}$")) and
			(.elapsedPerRepetitionSeconds | type) == "string" and
			(.elapsedPerRepetitionSeconds | test("^[0-9]+[.][0-9]{9}$")) and
			(.status | type) == "number" and
			.status == (.status | floor) and
			.inputExecutableBefore.exists == true and
			.inputExecutableBefore.executable == true and
			.inputExecutableBefore.sha256 == .inputExecutableAfter.sha256
		' "$log_prefix.elapsed.json" > /dev/null 2>&1; then
		measurement_elapsed=$(jq -r '.elapsedPerRepetitionSeconds' "$log_prefix.elapsed.json")
		measurement_elapsed_status=$(jq -r '.status' "$log_prefix.elapsed.json")
		measurement_elapsed_nanoseconds=$(jq -r '.elapsedNanoseconds' "$log_prefix.elapsed.json")
		expected_elapsed=$(awk -v value="$measurement_elapsed_nanoseconds" \
			-v repetitions="$measurement_repetitions" \
			'BEGIN { printf "%.9f", value / repetitions / 1000000000 }')
		if test "$measurement_elapsed" != "$expected_elapsed"; then
			measurement_observer_status=65
		fi
	else
		measurement_observer_status=65
	fi

	if test "$probe_expected" != -; then
		set +e
		"$artifact" "$probe_argument" \
			> "$log_prefix.elapsed-probe.stdout" \
			2> "$log_prefix.elapsed-probe.stderr"
		measurement_elapsed_probe_status=$?
		set -e
		printf '%s\n' "$measurement_elapsed_probe_status" > "$log_prefix.elapsed-probe.status"
	fi

	if test "$artifact" != -; then
		rm -f "$artifact"
	fi
	record_input_identity "$measurement_input" "$log_prefix.rss-input-before.txt"
	set +e
	/usr/bin/time -f '%M' -o "$log_prefix.rss-kib" \
		"$@" > "$log_prefix.rss.stdout" 2> "$log_prefix.rss.stderr"
	measurement_rss_status=$?
	set -e
	record_input_identity "$measurement_input" "$log_prefix.rss-input-after.txt"
	rss_kib=
	if test -r "$log_prefix.rss-kib"; then
		rss_kib=$(tr -d '[:space:]' < "$log_prefix.rss-kib")
	fi
	case "$rss_kib" in
	'' | *[!0-9]*) measurement_rss=0 ;;
	*) measurement_rss=$((rss_kib * 1024)) ;;
	esac
	if test "$artifact" != -; then
		if test -f "$artifact"; then
			{
				printf 'exists=true\n'
				printf 'executable=%s\n' "$(test -x "$artifact" && printf true || printf false)"
				printf 'size=%s\n' "$(file_size "$artifact")"
				printf 'sha256=%s\n' "$(sha256 "$artifact")"
			} > "$log_prefix.rss-artifact.txt"
		else
			printf 'exists=false\n' > "$log_prefix.rss-artifact.txt"
		fi
	fi
	if test "$probe_expected" != -; then
		set +e
		"$artifact" "$probe_argument" \
			> "$log_prefix.rss-probe.stdout" \
			2> "$log_prefix.rss-probe.stderr"
		measurement_rss_probe_status=$?
		set -e
		printf '%s\n' "$measurement_rss_probe_status" > "$log_prefix.rss-probe.status"
	fi
	printf '%s,%s,%s,%s,%s,%s,%s,%s\n' \
		"$stage" "$candidate" "$iteration" "$measurement_elapsed" \
		"$measurement_elapsed_status" "$measurement_rss" "$measurement_rss_status" \
		"$measurement_observer_status" \
		>> "$measurements"
}

require_measurement_success() {
	label=$1
	test "$measurement_observer_status" -eq 0 ||
		fail "$label elapsed observer failed with status $measurement_observer_status"
	require_empty_file "$measurement_logs/$stage-$candidate-$iteration.observer.stdout" \
		"$label elapsed observer wrote stdout"
	require_empty_file "$measurement_logs/$stage-$candidate-$iteration.observer.stderr" \
		"$label elapsed observer wrote stderr"
	test "$measurement_elapsed_status" -eq 0 ||
		fail "$label elapsed command failed with status $measurement_elapsed_status"
	test "$measurement_rss_status" -eq 0 ||
		fail "$label RSS command failed with status $measurement_rss_status"
	elapsed_input_sha256=$(jq -r '.inputExecutableAfter.sha256 // ""' "$log_prefix.elapsed.json")
	rss_input_before_sha256=$(awk -F= '$1 == "sha256" {print $2}' "$log_prefix.rss-input-before.txt")
	rss_input_after_sha256=$(awk -F= '$1 == "sha256" {print $2}' "$log_prefix.rss-input-after.txt")
	test -n "$elapsed_input_sha256" || fail "$label elapsed input identity is missing"
	test "$elapsed_input_sha256" = "$rss_input_before_sha256" ||
		fail "$label input executable changed between elapsed and RSS observations"
	test "$rss_input_before_sha256" = "$rss_input_after_sha256" ||
		fail "$label input executable changed during the RSS observation"
	grep -Fx 'executable=true' "$log_prefix.rss-input-before.txt" > /dev/null ||
		fail "$label RSS input is not executable"
	awk -v value="$measurement_elapsed" 'BEGIN { exit !(value > 0) }' ||
		fail "$label did not report positive monotonic elapsed time"
	test "$measurement_rss" -gt 0 || fail "$label did not report peak RSS"
}

require_measured_empty_output() {
	label=$1
	require_empty_file "$log_prefix.elapsed.stdout" "$label elapsed command wrote stdout"
	require_empty_file "$log_prefix.elapsed.stderr" "$label elapsed command wrote stderr"
	require_empty_file "$log_prefix.rss.stdout" "$label RSS command wrote stdout"
	require_empty_file "$log_prefix.rss.stderr" "$label RSS command wrote stderr"
}

require_measured_artifact_exact() {
	expected=$1
	label=$2
	expected_sha256=$(sha256 "$expected")
	test "$(jq -r '.artifact.exists // false' "$log_prefix.elapsed.json")" = true ||
		fail "$label elapsed command did not publish an artifact"
	test "$(jq -r '.artifact.executable // false' "$log_prefix.elapsed.json")" = true ||
		fail "$label elapsed command artifact is not executable"
	test "$(jq -r '.artifact.sha256 // ""' "$log_prefix.elapsed.json")" = "$expected_sha256" ||
		fail "$label elapsed command artifact bytes differ"
	grep -Fx 'exists=true' "$log_prefix.rss-artifact.txt" > /dev/null ||
		fail "$label RSS command did not publish an artifact"
	grep -Fx 'executable=true' "$log_prefix.rss-artifact.txt" > /dev/null ||
		fail "$label RSS command artifact is not executable"
	grep -Fx "sha256=$expected_sha256" "$log_prefix.rss-artifact.txt" > /dev/null ||
		fail "$label RSS command artifact bytes differ"
	cmp "$expected" "$artifact" > /dev/null || fail "$label final artifact bytes differ"
}

require_measured_artifact_published() {
	label=$1
	test "$(jq -r '.artifact.exists // false' "$log_prefix.elapsed.json")" = true ||
		fail "$label elapsed command did not publish an artifact"
	test "$(jq -r '.artifact.executable // false' "$log_prefix.elapsed.json")" = true ||
		fail "$label elapsed command artifact is not executable"
	test "$(jq -r '.artifact.size // 0' "$log_prefix.elapsed.json")" -gt 0 ||
		fail "$label elapsed command artifact is empty"
	grep -Fx 'exists=true' "$log_prefix.rss-artifact.txt" > /dev/null ||
		fail "$label RSS command did not publish an artifact"
	grep -Fx 'executable=true' "$log_prefix.rss-artifact.txt" > /dev/null ||
		fail "$label RSS command artifact is not executable"
	test -s "$artifact" || fail "$label final artifact is empty"
}

require_measured_probe() {
	label=$1
	for phase in elapsed rss; do
		test "$(cat "$log_prefix.$phase-probe.status")" -eq 0 ||
			fail "$label $phase artifact probe failed"
		cmp "$probe_expected" "$log_prefix.$phase-probe.stdout" > /dev/null ||
			fail "$label $phase artifact probe stdout differs"
		require_empty_file "$log_prefix.$phase-probe.stderr" \
			"$label $phase artifact probe wrote stderr"
	done
}

adjacent_b2=$workspace/bootstrap/b2/compiler
adjacent_b3=$workspace/bootstrap/b3/compiler
adjacent_b4=$workspace/bootstrap/b4/compiler
for adjacent_compiler in "$adjacent_b2" "$adjacent_b3" "$adjacent_b4"; do
	test -x "$adjacent_compiler" || fail "adjacent compiler input is not executable"
done
adjacent_b2_b3_directory=$workspace/measured-adjacent-b2-b3
adjacent_b3_b4_directory=$workspace/measured-adjacent-b3-b4
mkdir -p "$adjacent_b2_b3_directory" "$adjacent_b3_b4_directory"

round=0
while test "$round" -lt 9; do
	if test "$round" -lt 2; then
		iteration=$((round - 2))
	else
		iteration=$((round - 1))
	fi
	order='b2-b3 b3-b4'
	if test $((round % 2)) -ne 0; then
		order='b3-b4 b2-b3'
	fi
	for candidate in $order; do
		if test "$candidate" = b2-b3; then
			seed=$adjacent_b2
			expected=$adjacent_b3
			measured_output=$adjacent_b2_b3_directory/compiler
			label='B2-to-B3 adjacent compiler build'
		else
			seed=$adjacent_b3
			expected=$adjacent_b4
			measured_output=$adjacent_b3_b4_directory/compiler
			label='B3-to-B4 adjacent compiler build'
		fi
		measure_command adjacent-build "$candidate" "$iteration" \
			"$measured_output" - - \
			"$seed" build "$compiler_entry" \
				--output "$measured_output" \
				--qbe "$qbe" --cc "$cc" --target "$PROFILE"
		require_measurement_success "$label observation $iteration"
		require_measured_empty_output "$label observation $iteration"
		require_measured_artifact_exact "$expected" "$label observation $iteration"
	done
	round=$((round + 1))
done
require_no_intermediates "$adjacent_b2_b3_directory"
require_no_intermediates "$adjacent_b3_b4_directory"

compiler_native_directory=$workspace/measured-compiler-native
compiler_external_directory=$workspace/measured-compiler-external
mkdir -p "$compiler_native_directory" "$compiler_external_directory"
compiler_native_output=$compiler_native_directory/compiler
compiler_external_output=$compiler_external_directory/compiler

round=0
while test "$round" -lt 9; do
	if test "$round" -lt 2; then
		iteration=$((round - 2))
	else
		iteration=$((round - 1))
	fi
	order='native external'
	if test $((round % 2)) -ne 0; then
		order='external native'
	fi
	for candidate in $order; do
		if test "$candidate" = native; then
			measure_command compiler-build native "$iteration" \
				"$compiler_native_output" - - \
				"$output_compiler" build "$compiler_entry" \
					--output "$compiler_native_output" \
					--qbe "$qbe" --cc "$cc" --target "$PROFILE"
			require_measurement_success "Native compiler build observation $iteration"
			require_measured_empty_output "Native compiler build observation $iteration"
			require_measured_artifact_exact "$output_compiler" \
				"Native compiler build observation $iteration"
		else
			measure_command compiler-build external "$iteration" \
				"$compiler_external_output" - - \
				/bin/sh "$external_recipe" "$output_compiler" "$compiler_entry" \
				"$compiler_external_output" "$qbe" "$cc"
			require_measurement_success "external compiler recipe observation $iteration"
			require_measured_empty_output "external compiler recipe observation $iteration"
			require_measured_artifact_exact "$output_compiler" \
				"external compiler recipe observation $iteration"
		fi
	done
	round=$((round + 1))
done
require_no_intermediates "$compiler_native_directory"
require_no_intermediates "$compiler_external_directory"

mkdir -p "$workspace/external-trace"
strace -f -e trace=process -o "$evidence/external-recipe-process.trace" \
	/bin/sh "$external_recipe" "$output_compiler" "$compiler_entry" \
	"$workspace/external-trace/compiler" "$qbe" "$cc" \
	> "$evidence/external-recipe.stdout" \
	2> "$evidence/external-recipe.stderr" || fail "traced external compiler recipe failed"
require_empty_file "$evidence/external-recipe.stdout" "traced external compiler recipe wrote stdout"
require_empty_file "$evidence/external-recipe.stderr" "traced external compiler recipe wrote stderr"
cmp "$output_compiler" "$workspace/external-trace/compiler" > /dev/null ||
	fail "traced external compiler bytes differ"
grep execve "$evidence/external-recipe-process.trace" > "$evidence/external-recipe-process-inventory.txt"

application_native_directory=$workspace/measured-application-native
application_go_directory=$workspace/measured-application-go
mkdir -p "$application_native_directory" "$application_go_directory"
application_native_output=$application_native_directory/program
application_go_output=$application_go_directory/program

round=0
while test "$round" -lt 13; do
	if test "$round" -lt 2; then
		iteration=$((round - 2))
	else
		iteration=$((round - 1))
	fi
	order='native go'
	if test $((round % 2)) -ne 0; then
		order='go native'
	fi
	for candidate in $order; do
		if test "$candidate" = native; then
			measure_command application-build native "$iteration" \
				"$application_native_output" "$expected_stdout" 144 \
				"$output_compiler" build "$portable_config" \
					--output "$application_native_output" \
					--qbe "$qbe" --cc "$cc" --target "$PROFILE"
			require_measurement_success "Native application build observation $iteration"
			require_measured_empty_output "Native application build observation $iteration"
			require_measured_artifact_exact "$native_application" \
				"Native application build observation $iteration"
			require_measured_probe "Native application build observation $iteration"
		else
			measure_command application-build go "$iteration" \
				"$application_go_output" "$expected_stdout" 144 \
				"$reference_trb" build --compile --config "$portable_config" \
				--outfile "$application_go_output"
			require_measurement_success "optimized Go application build observation $iteration"
			printf 'executable -> %s\n' "$application_go_output" \
				> "$measurement_logs/application-build-go-$iteration.expected"
			cmp "$measurement_logs/application-build-go-$iteration.expected" \
				"$log_prefix.elapsed.stdout" > /dev/null ||
				fail "optimized Go elapsed build stdout differs at observation $iteration"
			cmp "$measurement_logs/application-build-go-$iteration.expected" \
				"$log_prefix.rss.stdout" > /dev/null ||
				fail "optimized Go RSS build stdout differs at observation $iteration"
			require_empty_file "$log_prefix.elapsed.stderr" \
				"optimized Go elapsed application build observation $iteration wrote stderr"
			require_empty_file "$log_prefix.rss.stderr" \
				"optimized Go RSS application build observation $iteration wrote stderr"
			require_measured_artifact_published \
				"optimized Go application build observation $iteration"
			require_measured_probe "optimized Go application build observation $iteration"
		fi
	done
	round=$((round + 1))
done
require_no_intermediates "$application_native_directory"

round=0
while test "$round" -lt 13; do
	if test "$round" -lt 2; then
		iteration=$((round - 2))
	else
		iteration=$((round - 1))
	fi
	order='native go'
	if test $((round % 2)) -ne 0; then
		order='go native'
	fi
	for candidate in $order; do
		executable=$native_application
		if test "$candidate" = go; then
			executable=$go_application
		fi
		measure_command application-runtime "$candidate" "$iteration" - - - "$executable" 144
		require_measurement_success "$candidate application runtime observation $iteration"
		cmp "$expected_stdout" "$log_prefix.elapsed.stdout" > /dev/null ||
			fail "$candidate elapsed runtime stdout differs at observation $iteration"
		cmp "$expected_stdout" "$log_prefix.rss.stdout" > /dev/null ||
			fail "$candidate RSS runtime stdout differs at observation $iteration"
		require_empty_file "$log_prefix.elapsed.stderr" \
			"$candidate elapsed runtime observation $iteration wrote stderr"
		require_empty_file "$log_prefix.rss.stderr" \
			"$candidate RSS runtime observation $iteration wrote stderr"
	done
	round=$((round + 1))
done

retained_count() {
	csv=$1
	stage=$2
	candidate=$3
	awk -F, -v stage="$stage" -v candidate="$candidate" \
		'NR > 1 && $1 == stage && $2 == candidate && $3 > 0 && $5 == 0 && $7 == 0 && $8 == 0 {count += 1} END {print count + 0}' \
		"$csv"
}

median_value() {
	csv=$1
	stage=$2
	candidate=$3
	column=$4
	count=$(retained_count "$csv" "$stage" "$candidate")
	test "$count" -gt 0 || fail "no retained $stage $candidate measurements"
	position=$(((count + 1) / 2))
	awk -F, -v stage="$stage" -v candidate="$candidate" -v column="$column" \
		'NR > 1 && $1 == stage && $2 == candidate && $3 > 0 && $5 == 0 && $7 == 0 && $8 == 0 {print $column}' \
		"$csv" | LC_ALL=C sort -n | sed -n "${position}p"
}

require_native_within_strongest() {
	label=$1
	native=$2
	comparison=$3
	percent=$4
	awk -v native="$native" -v comparison="$comparison" -v percent="$percent" 'BEGIN {
		strongest = native
		if (comparison < strongest) strongest = comparison
		exit !(native * 100 <= strongest * percent)
	}' || fail "$label exceeds the registered bound: native=$native comparison=$comparison"
}

require_spread() {
	label=$1
	first=$2
	second=$3
	percent=$4
	awk -v first="$first" -v second="$second" -v percent="$percent" 'BEGIN {
		minimum = first
		if (second < minimum) minimum = second
		maximum = first
		if (second > maximum) maximum = second
		exit !(maximum * 100 <= minimum * percent)
	}' || fail "$label exceeds the registered spread: $first, $second"
}

require_observations_below_catastrophic() {
	stage=$1
	candidate=$2
	column=$3
	baseline=$4
	awk -F, -v stage="$stage" -v candidate="$candidate" -v column="$column" -v baseline="$baseline" \
		'NR > 1 && $1 == stage && $2 == candidate && $3 > 0 && $5 == 0 && $7 == 0 && $8 == 0 && $column > baseline * 2 {exit 1}' \
		"$measurements" || fail "$stage $candidate observation exceeds the catastrophic bound"
}

test "$(retained_count "$measurements" adjacent-build b2-b3)" -eq 7 ||
	fail "B2-to-B3 adjacent compiler measurement count differs"
test "$(retained_count "$measurements" adjacent-build b3-b4)" -eq 7 ||
	fail "B3-to-B4 adjacent compiler measurement count differs"
test "$(retained_count "$measurements" compiler-build native)" -eq 7 ||
	fail "Native compiler measurement count differs"
test "$(retained_count "$measurements" compiler-build external)" -eq 7 ||
	fail "external compiler measurement count differs"
test "$(retained_count "$measurements" application-build native)" -eq 11 ||
	fail "Native application build measurement count differs"
test "$(retained_count "$measurements" application-build go)" -eq 11 ||
	fail "Go application build measurement count differs"
test "$(retained_count "$measurements" application-runtime native)" -eq 11 ||
	fail "Native application runtime measurement count differs"
test "$(retained_count "$measurements" application-runtime go)" -eq 11 ||
	fail "Go application runtime measurement count differs"

compiler_native_time=$(median_value "$measurements" compiler-build native 4)
compiler_external_time=$(median_value "$measurements" compiler-build external 4)
compiler_native_rss=$(median_value "$measurements" compiler-build native 6)
compiler_external_rss=$(median_value "$measurements" compiler-build external 6)
application_native_build_time=$(median_value "$measurements" application-build native 4)
application_go_build_time=$(median_value "$measurements" application-build go 4)
application_native_build_rss=$(median_value "$measurements" application-build native 6)
application_go_build_rss=$(median_value "$measurements" application-build go 6)
application_native_runtime_time=$(median_value "$measurements" application-runtime native 4)
application_go_runtime_time=$(median_value "$measurements" application-runtime go 4)
application_native_runtime_rss=$(median_value "$measurements" application-runtime native 6)
application_go_runtime_rss=$(median_value "$measurements" application-runtime go 6)
adjacent_b2_b3_time=$(median_value "$measurements" adjacent-build b2-b3 4)
adjacent_b3_b4_time=$(median_value "$measurements" adjacent-build b3-b4 4)
adjacent_b2_b3_rss=$(median_value "$measurements" adjacent-build b2-b3 6)
adjacent_b3_b4_rss=$(median_value "$measurements" adjacent-build b3-b4 6)

require_native_within_strongest "compiler build time" \
	"$compiler_native_time" "$compiler_external_time" 125
require_native_within_strongest "compiler build RSS" \
	"$compiler_native_rss" "$compiler_external_rss" 125
require_spread "adjacent compiler build time" "$adjacent_b2_b3_time" "$adjacent_b3_b4_time" 110
require_spread "adjacent compiler build RSS" "$adjacent_b2_b3_rss" "$adjacent_b3_b4_rss" 110
require_native_within_strongest "application build time" \
	"$application_native_build_time" "$application_go_build_time" 125
require_native_within_strongest "application build RSS" \
	"$application_native_build_rss" "$application_go_build_rss" 125
require_native_within_strongest "application runtime time" \
	"$application_native_runtime_time" "$application_go_runtime_time" 125
require_native_within_strongest "application runtime RSS" \
	"$application_native_runtime_rss" "$application_go_runtime_rss" 125

adjacent_strongest_time=$(awk -v a="$adjacent_b2_b3_time" -v b="$adjacent_b3_b4_time" \
	'BEGIN {if (a < b) print a; else print b}')
adjacent_strongest_rss=$(awk -v a="$adjacent_b2_b3_rss" -v b="$adjacent_b3_b4_rss" \
	'BEGIN {if (a < b) print a; else print b}')
compiler_strongest_time=$(awk -v a="$compiler_native_time" -v b="$compiler_external_time" \
	'BEGIN {if (a < b) print a; else print b}')
compiler_strongest_rss=$(awk -v a="$compiler_native_rss" -v b="$compiler_external_rss" \
	'BEGIN {if (a < b) print a; else print b}')
application_build_strongest_time=$(awk -v a="$application_native_build_time" -v b="$application_go_build_time" \
	'BEGIN {if (a < b) print a; else print b}')
application_build_strongest_rss=$(awk -v a="$application_native_build_rss" -v b="$application_go_build_rss" \
	'BEGIN {if (a < b) print a; else print b}')
application_runtime_strongest_time=$(awk -v a="$application_native_runtime_time" -v b="$application_go_runtime_time" \
	'BEGIN {if (a < b) print a; else print b}')
application_runtime_strongest_rss=$(awk -v a="$application_native_runtime_rss" -v b="$application_go_runtime_rss" \
	'BEGIN {if (a < b) print a; else print b}')

require_observations_below_catastrophic compiler-build native 4 "$compiler_strongest_time"
require_observations_below_catastrophic compiler-build native 6 "$compiler_strongest_rss"
require_observations_below_catastrophic compiler-build external 4 "$compiler_external_time"
require_observations_below_catastrophic compiler-build external 6 "$compiler_external_rss"
require_observations_below_catastrophic application-build native 4 "$application_build_strongest_time"
require_observations_below_catastrophic application-build native 6 "$application_build_strongest_rss"
require_observations_below_catastrophic application-build go 4 "$application_go_build_time"
require_observations_below_catastrophic application-build go 6 "$application_go_build_rss"
require_observations_below_catastrophic application-runtime native 4 "$application_runtime_strongest_time"
require_observations_below_catastrophic application-runtime native 6 "$application_runtime_strongest_rss"
require_observations_below_catastrophic application-runtime go 4 "$application_go_runtime_time"
require_observations_below_catastrophic application-runtime go 6 "$application_go_runtime_rss"
require_observations_below_catastrophic adjacent-build b2-b3 4 "$adjacent_strongest_time"
require_observations_below_catastrophic adjacent-build b2-b3 6 "$adjacent_strongest_rss"
require_observations_below_catastrophic adjacent-build b3-b4 4 "$adjacent_strongest_time"
require_observations_below_catastrophic adjacent-build b3-b4 6 "$adjacent_strongest_rss"

cat > "$evidence/medians.csv" <<EOF
metric,native,comparison,limit_percent
compiler-build-time,$compiler_native_time,$compiler_external_time,125
compiler-build-rss,$compiler_native_rss,$compiler_external_rss,125
adjacent-build-time,$adjacent_b2_b3_time,$adjacent_b3_b4_time,110
adjacent-build-rss,$adjacent_b2_b3_rss,$adjacent_b3_b4_rss,110
application-build-time,$application_native_build_time,$application_go_build_time,125
application-build-rss,$application_native_build_rss,$application_go_build_rss,125
application-runtime-time,$application_native_runtime_time,$application_go_runtime_time,125
application-runtime-rss,$application_native_runtime_rss,$application_go_runtime_rss,125
EOF
}

record_target_evidence() {
strip --strip-all -o "$workspace/native-first/program.stripped" "$native_application"
strip --strip-all -o "$workspace/go/program.stripped" "$go_application"
native_stripped_size=$(file_size "$workspace/native-first/program.stripped")
go_stripped_size=$(file_size "$workspace/go/program.stripped")
compiler_cost_check application-bytes-times-five "$((native_stripped_size * 5))" "$go_stripped_size" \
	>> "$evidence/cost-observations.txt" ||
	fail "Native portable-entry application is not at least 80% smaller than optimized Go"

{
	printf 'pre_implementation_revision=%s\n' "$PRE_IMPLEMENTATION_REVISION"
	printf 'candidate_revision=%s\n' "$(git -C "$candidate_root" rev-parse HEAD)"
	printf 'type_rb_revision=%s\n' "$TYPE_RB_REVISION"
	printf 'type_rb_version=%s\n' "$TYPE_RB_VERSION"
	printf 'root_qbe_sha256=%s\n' "$(sha256 "$root_qbe")"
	printf 'qbe_binary_sha256=%s\n' "$(sha256 "$qbe")"
	printf 'candidate_compiler_sha256=%s\n' "$(sha256 "$output_compiler")"
	printf 'measurement_controller_sha256=%s\n' "$(sha256 "$measurement_controller")"
	printf 'application_build_elapsed_repetitions=%s\n' \
		"$APPLICATION_BUILD_ELAPSED_REPETITIONS"
	printf 'application_runtime_elapsed_repetitions=%s\n' \
		"$APPLICATION_RUNTIME_ELAPSED_REPETITIONS"
	printf 'python_command=%s\n' "$python_command"
	printf 'python_command_sha256=%s\n' "$(sha256 "$python_command")"
	printf 'candidate_fixed_point_qbe_sha256=%s\n' \
		"$(awk -F= '$1 == "fixed_point_qbe_sha256" {print $2}' "$evidence/bootstrap/identities.txt")"
	printf 'portable_entry_qbe_sha256=%s\n' \
		"$(sha256 "$evidence/applications/portable-entry-first.ssa")"
	printf 'native_application_sha256=%s\n' "$(sha256 "$native_application")"
	printf 'go_application_sha256=%s\n' "$(sha256 "$go_application")"
	printf 'native_application_raw_bytes=%s\n' "$(file_size "$native_application")"
	printf 'native_application_stripped_bytes=%s\n' "$native_stripped_size"
	printf 'go_application_raw_bytes=%s\n' "$(file_size "$go_application")"
	printf 'go_application_stripped_bytes=%s\n' "$go_stripped_size"
} > "$evidence/identities.txt"

{
	printf 'verifier_revision=%s\n' "$(git -C "$verifier_root" rev-parse HEAD)"
	printf 'candidate_revision=%s\n' "$(git -C "$candidate_root" rev-parse HEAD)"
	printf 'profile=%s\n' "$PROFILE"
	printf 'qbe_target=amd64_sysv\n'
	printf 'runner_image=%s\n' "$RUNNER_IMAGE"
	uname -a
	"$reference_trb" version
	"$go_command" version
	"$go_command" env GOCACHE GOENV GOOS GOARCH
	"$python_command" --version
	"$python_command" -c \
		'import json, time; info = time.get_clock_info("monotonic"); print(json.dumps({"implementation": info.implementation, "resolutionSeconds": info.resolution, "monotonic": info.monotonic, "adjustable": info.adjustable}, sort_keys=True))'
	printf 'cc1_command=%s\n' "$cc1_command"
	printf 'collect2_command=%s\n' "$collect2_command"
	printf 'assembler_command=%s\n' "$assembler_command"
	printf 'architecture_assembler_command=%s\n' "$architecture_assembler_command"
	printf 'ld_lld_command=%s\n' "$ld_lld_command"
	"$cc" --version
	ld.lld --version
	"$qbe" -h
} > "$evidence/environment.txt" 2>&1

"$cc" -### -xassembler "$first_assembly" \
	-fuse-ld=lld -Wl,--gc-sections,--strip-all -lm -o "$workspace/cc-expansion-output" \
	> "$evidence/cc-expansion.stdout" 2> "$evidence/cc-expansion.stderr" ||
	fail "CC expansion probe failed"
require_empty_file "$evidence/cc-expansion.stdout" "CC expansion probe wrote stdout"

require_clean_revision "$candidate_root" "Linux target candidate after verification"
require_no_intermediates "$workspace"
printf 'linux-amd64-targets: passed\n'
}

test -x /usr/bin/time || fail "/usr/bin/time is required"

test -d "$candidate_root" || fail "candidate root does not exist"
test -f "$root_qbe" || fail "root QBE does not exist"
test -x "$qbe" || fail "QBE is not executable"
test -x "$cc" || fail "CC is not executable"
test -x "$reference_trb" || fail "reference trb is not executable"
test -x "$go_command" || fail "Go is not executable"
test -x "$external_recipe" || fail "external comparison recipe is not executable"
test -f "$measurement_controller" || fail "monotonic measurement controller does not exist"
file -L "$qbe" | grep -F 'ELF ' > /dev/null || fail "QBE must be a direct ELF executable"
file -L "$cc" | grep -F 'ELF ' > /dev/null || fail "CC must be a direct ELF executable"
test "$(command -v go)" = "$go_command" || fail "Go command does not match PATH"

resolve_cc_program() {
	program_name=$1
	program_path=$("$cc" "-print-prog-name=$program_name" 2> /dev/null || true)
	case "$program_path" in
	*/*) printf '%s\n' "$program_path" ;;
	'') ;;
	*) command -v "$program_path" 2> /dev/null || true ;;
	esac
}
cc1_command=$(resolve_cc_program cc1)
collect2_command=$(resolve_cc_program collect2)
assembler_command=$(command -v as)
architecture_assembler_command=$(command -v x86_64-linux-gnu-as 2> /dev/null || true)
ld_lld_command=$(command -v ld.lld)
test -x "$assembler_command" || fail "resolved assembler is not executable"
test -x "$ld_lld_command" || fail "resolved LLD is not executable"
for compiler_program in "$cc1_command" "$collect2_command" "$architecture_assembler_command"; do
	if test -n "$compiler_program"; then
		test -x "$compiler_program" || fail "resolved compiler child is not executable: $compiler_program"
	fi
done
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"
test ! -e "$output_compiler" || fail "output compiler already exists"

record_temporary_inventory() {
	mkdir -p "$evidence"
	if test -d "$workspace"; then
		find "$workspace" \( -name '*.trbn.*' -o -name '*.native-target-external.*' \) -print \
			> "$evidence/final-temporary-inventory.txt" 2>&1 || true
	else
		: > "$evidence/final-temporary-inventory.txt"
	fi
}
trap record_temporary_inventory 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

require_clean_revision "$candidate_root" "Linux target candidate"
test "$(tr -d '\n' < "$candidate_root/TYPE_RB_REVISION")" = "$TYPE_RB_REVISION" ||
	fail "candidate TypeRB revision pin differs"
test "$("$reference_trb" version)" = "$TYPE_RB_VERSION" || fail "reference TypeRB version differs"
"$go_command" version > /dev/null 2>&1 || fail "Go toolchain probe failed"
test "$(file_size "$root_qbe")" -eq "$ROOT_QBE_SIZE" || fail "root QBE size differs"
test "$(sha256 "$root_qbe")" = "$ROOT_QBE_SHA256" || fail "root QBE digest differs"

compiler_entry=$candidate_root/compiler/src/compiler.trb
seed_entry=$compiler_entry
if test "$seed_source_root" != "$candidate_root"; then
	# Only the exact accepted seed source may bridge the immutable root parser
	# to newer compiler syntax. This is setup, never a measured candidate input.
	require_clean_revision "$seed_source_root" "accepted seed source"
	test "$(git -C "$seed_source_root" rev-parse HEAD)" = \
		21f507e7ee7de2577f4137f6dfb9f732c14c1640 || fail "seed source revision differs"
	seed_entry=$seed_source_root/compiler/src/compiler.trb
	test -f "$seed_entry" || fail "seed compiler entry is missing"
fi
loop_entry=
if test -n "$loop_source_root"; then
	test "$seed_source_root" != "$candidate_root" || fail "loop source requires the accepted first seed source"
	require_clean_revision "$loop_source_root" "accepted loop source"
	test "$(git -C "$loop_source_root" rev-parse HEAD)" = \
		4e1d0b4aee97b9a5bd73a98f918b31d47985da25 || fail "loop source revision differs"
	loop_entry=$loop_source_root/compiler/src/compiler.trb
	test -f "$loop_entry" || fail "loop compiler entry is missing"
fi
boolean_entry=
if test -n "$boolean_source_root"; then
	test -n "$loop_source_root" || fail "Boolean source requires the accepted loop source"
	require_clean_revision "$boolean_source_root" "accepted boolean source"
	test "$(git -C "$boolean_source_root" rev-parse HEAD)" = \
		57cb41ad6be91716e31fa555ed8ea8c8ce7a5f51 || fail "boolean source revision differs"
	boolean_entry=$boolean_source_root/compiler/src/compiler.trb
	test -f "$boolean_entry" || fail "boolean compiler entry is missing"
fi
record_entry=
if test -n "$record_source_root"; then
	test -n "$boolean_source_root" || fail "record Array source requires the accepted Boolean source"
	require_clean_revision "$record_source_root" "accepted record source"
	test "$(git -C "$record_source_root" rev-parse HEAD)" = \
		566d00d67172460df0f57dff6d5fe03db3b67cdc || fail "record source revision differs"
	record_entry=$record_source_root/compiler/src/compiler.trb
	test -f "$record_entry" || fail "record compiler entry is missing"
fi
hash_entry=
if test -n "$hash_source_root"; then
	test -n "$record_source_root" || fail "Hash source requires the accepted record Array source"
	require_clean_revision "$hash_source_root" "accepted Hash source"
	test "$(git -C "$hash_source_root" rev-parse HEAD)" = \
		8a6d9ff73b14a97bca1b010ddaad6a38972b5373 || fail "Hash source revision differs"
	hash_entry=$hash_source_root/compiler/src/compiler.trb
	test -f "$hash_entry" || fail "Hash compiler entry is missing"
fi
iteration_entry=
if test -n "$iteration_source_root"; then
	test -n "$hash_source_root" || fail "Array iteration source requires the accepted Hash source"
	require_clean_revision "$iteration_source_root" "accepted Array iteration source"
	test "$(git -C "$iteration_source_root" rev-parse HEAD)" = \
		508f721f8964d67a5893e547d2e2fb3de5b20a63 || fail "Array iteration source revision differs"
	iteration_entry=$iteration_source_root/compiler/src/compiler.trb
	test -f "$iteration_entry" || fail "Array iteration compiler entry is missing"
fi
names_entry=
if test -n "$names_source_root"; then
	test -n "$iteration_source_root" || fail "compiler-name source requires the accepted Array iteration source"
	require_clean_revision "$names_source_root" "accepted compiler-name source"
	test "$(git -C "$names_source_root" rev-parse HEAD)" = \
		6ca79d22cde2ddba5fe836c66899b6e08a6511dc || fail "compiler-name source revision differs"
	names_entry=$names_source_root/compiler/src/compiler.trb
	test -f "$names_entry" || fail "compiler-name entry is missing"
	test "$(sha256 "$names_entry")" = 9d3fc404ea55f459ef4bf5417112f0bb47054077ea94f3f6a5d57fbc23958ad0 || fail "compiler-name entry digest differs"
fi
cycles_entry=
if test -n "$cycles_source_root"; then
	test -n "$names_source_root" || fail "module-cycle source requires the accepted compiler-name source"
	require_clean_revision "$cycles_source_root" "accepted module-cycle source"
	test "$(git -C "$cycles_source_root" rev-parse HEAD)" = \
		a0f9147d29d73ad8a0237c7c51a0a5993bbe3a36 || fail "module-cycle source revision differs"
	cycles_entry=$cycles_source_root/compiler/src/compiler.trb
	test -f "$cycles_entry" || fail "module-cycle entry is missing"
	test "$(sha256 "$cycles_entry")" = 19cd0afbd30f2113c3cfc5bf096ea9108ac9de8a2a835e63d68f8b49f535c116 || fail "module-cycle entry digest differs"
fi
portable_config=$candidate_root/corpus/portable-entry/portable-entry/trbconfig.jsonc
portable_source=$candidate_root/corpus/portable-entry/portable-entry/src/main.trb
failure_config=$candidate_root/corpus/portable-entry/runtime-failures/trbconfig.jsonc
failure_source=$candidate_root/corpus/portable-entry/runtime-failures/src/main.trb

for required_file in "$compiler_entry" "$portable_config" "$portable_source" "$failure_config" "$failure_source"; do
	test -f "$required_file" || fail "required source is missing: $required_file"
done
test "$(sha256 "$portable_config")" = f38bad94e483e16876514e5624e96e11a66eaeb8374e222b14a75c7072dddbf0 ||
	fail "portable-entry config digest differs"
test "$(sha256 "$portable_source")" = 9d23bdf23452e13a0a3dd112714a304cf59c37ebc5e10badd83a5938cccba397 ||
	fail "portable-entry source digest differs"
test "$(sha256 "$failure_config")" = 9a81d83d114df62bbe1f0f3e29404d01e6ae49945a0c3afe44b23fdeef228e98 ||
	fail "runtime-failure config digest differs"
test "$(sha256 "$failure_source")" = 66b47df1fe37b5c084be7a669542b13694d0afc7a5c28aa9570011d571e60526 ||
	fail "runtime-failure source digest differs"

mkdir -p \
	"$workspace/setup/root-era" \
	"$workspace/setup/first" \
	"$workspace/setup/current-runtime" \
	"$evidence/setup" \
	"$(dirname -- "$output_compiler")"

root_assembly=$workspace/setup/root-era/compiler.s
root_compiler=$workspace/setup/root-era/compiler
first_qbe=$workspace/setup/first/compiler.ssa
first_assembly=$workspace/setup/first/compiler.s
first_transition=$workspace/setup/first/compiler
runtime_qbe=$workspace/setup/current-runtime/compiler.ssa
runtime_assembly=$workspace/setup/current-runtime/compiler.s
runtime_transition=$workspace/setup/current-runtime/compiler

strace -f -e trace=process -o "$evidence/setup/root-qbe-process.trace" \
	"$qbe" -t amd64_sysv -o "$root_assembly" "$root_qbe" \
	> "$evidence/setup/root-qbe.stdout" \
	2> "$evidence/setup/root-qbe.stderr" || fail "root QBE translation failed"
require_empty_file "$evidence/setup/root-qbe.stdout" "root QBE translation wrote stdout"
require_empty_file "$evidence/setup/root-qbe.stderr" "root QBE translation wrote stderr"
test -s "$root_assembly" || fail "root QBE translation did not produce assembly"

strace -f -e trace=process -o "$evidence/setup/root-link-process.trace" \
	"$cc" -xassembler "$root_assembly" \
		-fuse-ld=lld \
		-Wl,--gc-sections,--strip-all \
		-lm \
		-o "$root_compiler" \
	> "$evidence/setup/root-link.stdout" \
	2> "$evidence/setup/root-link.stderr" || fail "root-era compiler link failed"
require_empty_file "$evidence/setup/root-link.stdout" "root-era compiler link wrote stdout"
require_empty_file "$evidence/setup/root-link.stderr" "root-era compiler link wrote stderr"
test -x "$root_compiler" || fail "root-era compiler was not produced"
require_tool_observed "$evidence/setup/root-link-process.trace" 'execve\("[^"]*/(as|x86_64-linux-gnu-as)"' \
	"root-era assembler"
require_tool_observed "$evidence/setup/root-link-process.trace" 'execve\("[^"]*/ld\.lld"' \
	"root-era LLD"

strace -f -e trace=process -o "$evidence/setup/root-emit-process.trace" \
	"$root_compiler" emit-qbe "$seed_entry" \
	> "$first_qbe" \
	2> "$evidence/setup/root-emit.stderr" || fail "root-era current-source emission failed"
require_empty_file "$evidence/setup/root-emit.stderr" "root-era current-source emission wrote stderr"
test -s "$first_qbe" || fail "root-era compiler emitted empty current-source QBE"
require_forbidden_processes_absent "$evidence/setup/root-emit-process.trace" \
	"root-era current-source emission"

strace -f -e trace=process -o "$evidence/setup/first-qbe-process.trace" \
	"$qbe" -t amd64_sysv -o "$first_assembly" "$first_qbe" \
	> "$evidence/setup/first-qbe.stdout" \
	2> "$evidence/setup/first-qbe.stderr" || fail "first-transition QBE translation failed"
require_empty_file "$evidence/setup/first-qbe.stdout" "first-transition QBE translation wrote stdout"
require_empty_file "$evidence/setup/first-qbe.stderr" "first-transition QBE translation wrote stderr"
test -s "$first_assembly" || fail "first-transition QBE translation did not produce assembly"

strace -f -e trace=process -o "$evidence/setup/first-link-process.trace" \
	"$cc" -xassembler "$first_assembly" \
		-fuse-ld=lld \
		-Wl,--gc-sections,--strip-all \
		-lm \
		-o "$first_transition" \
	> "$evidence/setup/first-link.stdout" \
	2> "$evidence/setup/first-link.stderr" || fail "first-transition link failed"
require_empty_file "$evidence/setup/first-link.stdout" "first-transition link wrote stdout"
require_empty_file "$evidence/setup/first-link.stderr" "first-transition link wrote stderr"
test -x "$first_transition" || fail "first current-source transition was not produced"
require_tool_observed "$evidence/setup/first-link-process.trace" 'execve\("[^"]*/ld\.lld"' \
	"first-transition LLD"

if test "$seed_source_root" != "$candidate_root"; then
	strace -f -e trace=process -o "$evidence/setup/logical-condition-process.trace" \
		"$first_transition" check "$candidate_root/compiler/conformance/valid/logical-short-circuit.trb" \
		> "$evidence/setup/logical-condition.stdout" \
		2> "$evidence/setup/logical-condition.stderr" || fail "seed bridge rejected logical conditions"
	printf 'ok\n' > "$evidence/setup/logical-condition.expected"
	cmp "$evidence/setup/logical-condition.expected" "$evidence/setup/logical-condition.stdout" > /dev/null ||
		fail "logical condition check stdout differs"
	require_empty_file "$evidence/setup/logical-condition.stderr" "logical condition check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/logical-condition-process.trace" "logical condition check"
	strace -f -e trace=process -o "$evidence/setup/elsif-process.trace" \
		"$first_transition" check "$candidate_root/compiler/conformance/valid/elsif-control.trb" \
		> "$evidence/setup/elsif.stdout" \
		2> "$evidence/setup/elsif.stderr" || fail "seed bridge rejected elsif"
	printf 'ok\n' > "$evidence/setup/elsif.expected"
	cmp "$evidence/setup/elsif.expected" "$evidence/setup/elsif.stdout" > /dev/null ||
		fail "elsif check stdout differs"
	require_empty_file "$evidence/setup/elsif.stderr" "elsif check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/elsif-process.trace" "elsif check"
fi

# The immutable first bridge knows elsif; this accepted source adds loop
# transfers before the candidate may use them. Keep the later candidate-runtime
# transition separate so future runtime changes still precede ordinary B2.
runtime_seed=$first_transition
if test -n "$loop_source_root"; then
	mkdir -p "$workspace/setup/loop-syntax"
	loop_qbe=$workspace/setup/loop-syntax/compiler.ssa
	loop_assembly=$workspace/setup/loop-syntax/compiler.s
	loop_transition=$workspace/setup/loop-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/loop-emit-process.trace" \
		"$first_transition" emit-qbe "$loop_entry" \
		> "$loop_qbe" 2> "$evidence/setup/loop-emit.stderr" || fail "loop source emission failed"
	require_empty_file "$evidence/setup/loop-emit.stderr" "loop source emission wrote stderr"
	test -s "$loop_qbe" || fail "loop source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/loop-emit-process.trace" "loop source emission"
	strace -f -e trace=process -o "$evidence/setup/loop-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$loop_assembly" "$loop_qbe" \
		> "$evidence/setup/loop-qbe.stdout" 2> "$evidence/setup/loop-qbe.stderr" || fail "loop QBE translation failed"
	require_empty_file "$evidence/setup/loop-qbe.stdout" "loop QBE translation wrote stdout"
	require_empty_file "$evidence/setup/loop-qbe.stderr" "loop QBE translation wrote stderr"
	test -s "$loop_assembly" || fail "loop assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/loop-link-process.trace" \
		"$cc" -xassembler "$loop_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$loop_transition" \
		> "$evidence/setup/loop-link.stdout" 2> "$evidence/setup/loop-link.stderr" || fail "loop transition link failed"
	require_empty_file "$evidence/setup/loop-link.stdout" "loop transition link wrote stdout"
	require_empty_file "$evidence/setup/loop-link.stderr" "loop transition link wrote stderr"
	test -x "$loop_transition" || fail "loop transition compiler is missing"
	require_tool_observed "$evidence/setup/loop-link-process.trace" 'execve\("[^"]*/ld\.lld"' "loop transition LLD"
	strace -f -e trace=process -o "$evidence/setup/loop-check-process.trace" \
		"$loop_transition" check "$candidate_root/compiler/conformance/valid/loop-transfer-control.trb" \
		> "$evidence/setup/loop-check.stdout" 2> "$evidence/setup/loop-check.stderr" || fail "loop bridge rejected transfers"
	printf 'ok\n' > "$evidence/setup/loop-check.expected"
	cmp "$evidence/setup/loop-check.expected" "$evidence/setup/loop-check.stdout" > /dev/null || fail "loop check stdout differs"
	require_empty_file "$evidence/setup/loop-check.stderr" "loop check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/loop-check-process.trace" "loop transfer check"
	runtime_seed=$loop_transition
fi
if test -n "$boolean_source_root"; then
	mkdir -p "$workspace/setup/boolean-syntax"
	boolean_qbe=$workspace/setup/boolean-syntax/compiler.ssa
	boolean_assembly=$workspace/setup/boolean-syntax/compiler.s
	boolean_transition=$workspace/setup/boolean-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/boolean-emit-process.trace" \
		"$loop_transition" emit-qbe "$boolean_entry" \
		> "$boolean_qbe" 2> "$evidence/setup/boolean-emit.stderr" || fail "boolean source emission failed"
	require_empty_file "$evidence/setup/boolean-emit.stderr" "boolean source emission wrote stderr"
	test -s "$boolean_qbe" || fail "boolean source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/boolean-emit-process.trace" "boolean source emission"
	strace -f -e trace=process -o "$evidence/setup/boolean-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$boolean_assembly" "$boolean_qbe" \
		> "$evidence/setup/boolean-qbe.stdout" 2> "$evidence/setup/boolean-qbe.stderr" || fail "boolean QBE translation failed"
	require_empty_file "$evidence/setup/boolean-qbe.stdout" "boolean QBE translation wrote stdout"
	require_empty_file "$evidence/setup/boolean-qbe.stderr" "boolean QBE translation wrote stderr"
	test -s "$boolean_assembly" || fail "boolean assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/boolean-link-process.trace" \
		"$cc" -xassembler "$boolean_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$boolean_transition" \
		> "$evidence/setup/boolean-link.stdout" 2> "$evidence/setup/boolean-link.stderr" || fail "boolean transition link failed"
	require_empty_file "$evidence/setup/boolean-link.stdout" "boolean transition link wrote stdout"
	require_empty_file "$evidence/setup/boolean-link.stderr" "boolean transition link wrote stderr"
	test -x "$boolean_transition" || fail "boolean transition compiler is missing"
	require_tool_observed "$evidence/setup/boolean-link-process.trace" 'execve\("[^"]*/ld\.lld"' "boolean transition LLD"
	strace -f -e trace=process -o "$evidence/setup/boolean-check-process.trace" \
		"$boolean_transition" check "$candidate_root/compiler/conformance/valid/boolean-array-values.trb" \
		> "$evidence/setup/boolean-check.stdout" 2> "$evidence/setup/boolean-check.stderr" || fail "Boolean bridge rejected Boolean arrays"
	printf 'ok\n' > "$evidence/setup/boolean-check.expected"
	cmp "$evidence/setup/boolean-check.expected" "$evidence/setup/boolean-check.stdout" > /dev/null || fail "boolean check stdout differs"
	require_empty_file "$evidence/setup/boolean-check.stderr" "boolean check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/boolean-check-process.trace" "Boolean Array check"
	runtime_seed=$boolean_transition
fi
if test -n "$record_source_root"; then
	mkdir -p "$workspace/setup/record-syntax"
	record_qbe=$workspace/setup/record-syntax/compiler.ssa
	record_assembly=$workspace/setup/record-syntax/compiler.s
	record_transition=$workspace/setup/record-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/record-emit-process.trace" \
		"$boolean_transition" emit-qbe "$record_entry" \
		> "$record_qbe" 2> "$evidence/setup/record-emit.stderr" || fail "record source emission failed"
	require_empty_file "$evidence/setup/record-emit.stderr" "record source emission wrote stderr"
	test -s "$record_qbe" || fail "record source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/record-emit-process.trace" "record source emission"
	strace -f -e trace=process -o "$evidence/setup/record-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$record_assembly" "$record_qbe" \
		> "$evidence/setup/record-qbe.stdout" 2> "$evidence/setup/record-qbe.stderr" || fail "record QBE translation failed"
	require_empty_file "$evidence/setup/record-qbe.stdout" "record QBE translation wrote stdout"
	require_empty_file "$evidence/setup/record-qbe.stderr" "record QBE translation wrote stderr"
	test -s "$record_assembly" || fail "record assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/record-link-process.trace" \
		"$cc" -xassembler "$record_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$record_transition" \
		> "$evidence/setup/record-link.stdout" 2> "$evidence/setup/record-link.stderr" || fail "record transition link failed"
	require_empty_file "$evidence/setup/record-link.stdout" "record transition link wrote stdout"
	require_empty_file "$evidence/setup/record-link.stderr" "record transition link wrote stderr"
	test -x "$record_transition" || fail "record transition compiler is missing"
	require_tool_observed "$evidence/setup/record-link-process.trace" 'execve\("[^"]*/ld\.lld"' "record transition LLD"
	strace -f -e trace=process -o "$evidence/setup/record-check-process.trace" \
		"$record_transition" check "$candidate_root/compiler/conformance/valid/record-array-values.trb" \
		> "$evidence/setup/record-check.stdout" 2> "$evidence/setup/record-check.stderr" || fail "record Array bridge rejected record Arrays"
	printf 'ok\n' > "$evidence/setup/record-check.expected"
	cmp "$evidence/setup/record-check.expected" "$evidence/setup/record-check.stdout" > /dev/null || fail "record check stdout differs"
	require_empty_file "$evidence/setup/record-check.stderr" "record check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/record-check-process.trace" "record Array check"
	runtime_seed=$record_transition
fi

if test -n "$hash_source_root"; then
	mkdir -p "$workspace/setup/hash-syntax"
	hash_qbe=$workspace/setup/hash-syntax/compiler.ssa
	hash_assembly=$workspace/setup/hash-syntax/compiler.s
	hash_transition=$workspace/setup/hash-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/hash-emit-process.trace" \
		"$record_transition" emit-qbe "$hash_entry" \
		> "$hash_qbe" 2> "$evidence/setup/hash-emit.stderr" || fail "hash source emission failed"
	require_empty_file "$evidence/setup/hash-emit.stderr" "hash source emission wrote stderr"
	test -s "$hash_qbe" || fail "hash source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/hash-emit-process.trace" "hash source emission"
	strace -f -e trace=process -o "$evidence/setup/hash-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$hash_assembly" "$hash_qbe" \
		> "$evidence/setup/hash-qbe.stdout" 2> "$evidence/setup/hash-qbe.stderr" || fail "hash QBE translation failed"
	require_empty_file "$evidence/setup/hash-qbe.stdout" "hash QBE translation wrote stdout"
	require_empty_file "$evidence/setup/hash-qbe.stderr" "hash QBE translation wrote stderr"
	test -s "$hash_assembly" || fail "hash assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/hash-link-process.trace" \
		"$cc" -xassembler "$hash_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$hash_transition" \
		> "$evidence/setup/hash-link.stdout" 2> "$evidence/setup/hash-link.stderr" || fail "hash transition link failed"
	require_empty_file "$evidence/setup/hash-link.stdout" "hash transition link wrote stdout"
	require_empty_file "$evidence/setup/hash-link.stderr" "hash transition link wrote stderr"
	test -x "$hash_transition" || fail "hash transition compiler is missing"
	require_tool_observed "$evidence/setup/hash-link-process.trace" 'execve\("[^"]*/ld\.lld"' "hash transition LLD"
	strace -f -e trace=process -o "$evidence/setup/hash-check-process.trace" \
		"$hash_transition" check "$candidate_root/compiler/conformance/valid/hash-values.trb" \
		> "$evidence/setup/hash-check.stdout" 2> "$evidence/setup/hash-check.stderr" || fail "Hash bridge rejected Hash values"
	printf 'ok\n' > "$evidence/setup/hash-check.expected"
	cmp "$evidence/setup/hash-check.expected" "$evidence/setup/hash-check.stdout" > /dev/null || fail "hash check stdout differs"
	require_empty_file "$evidence/setup/hash-check.stderr" "hash check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/hash-check-process.trace" "Hash check"
	runtime_seed=$hash_transition
fi

if test -n "$iteration_source_root"; then
	mkdir -p "$workspace/setup/iteration-syntax"
	iteration_qbe=$workspace/setup/iteration-syntax/compiler.ssa
	iteration_assembly=$workspace/setup/iteration-syntax/compiler.s
	iteration_transition=$workspace/setup/iteration-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/iteration-emit-process.trace" \
		"$hash_transition" emit-qbe "$iteration_entry" \
		> "$iteration_qbe" 2> "$evidence/setup/iteration-emit.stderr" || fail "iteration source emission failed"
	require_empty_file "$evidence/setup/iteration-emit.stderr" "iteration source emission wrote stderr"
	test -s "$iteration_qbe" || fail "iteration source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/iteration-emit-process.trace" "iteration source emission"
	strace -f -e trace=process -o "$evidence/setup/iteration-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$iteration_assembly" "$iteration_qbe" \
		> "$evidence/setup/iteration-qbe.stdout" 2> "$evidence/setup/iteration-qbe.stderr" || fail "iteration QBE translation failed"
	require_empty_file "$evidence/setup/iteration-qbe.stdout" "iteration QBE translation wrote stdout"
	require_empty_file "$evidence/setup/iteration-qbe.stderr" "iteration QBE translation wrote stderr"
	test -s "$iteration_assembly" || fail "iteration assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/iteration-link-process.trace" \
		"$cc" -xassembler "$iteration_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$iteration_transition" \
		> "$evidence/setup/iteration-link.stdout" 2> "$evidence/setup/iteration-link.stderr" || fail "iteration transition link failed"
	require_empty_file "$evidence/setup/iteration-link.stdout" "iteration transition link wrote stdout"
	require_empty_file "$evidence/setup/iteration-link.stderr" "iteration transition link wrote stderr"
	test -x "$iteration_transition" || fail "iteration transition compiler is missing"
	require_tool_observed "$evidence/setup/iteration-link-process.trace" 'execve\("[^"]*/ld\.lld"' "iteration transition LLD"
	for iteration_case in live control managed; do
		strace -f -e trace=process -o "$evidence/setup/iteration-$iteration_case-check-process.trace" \
			"$iteration_transition" check "$candidate_root/compiler/conformance/valid/array-iteration-$iteration_case.trb" \
			> "$evidence/setup/iteration-$iteration_case-check.stdout" 2> "$evidence/setup/iteration-$iteration_case-check.stderr" || fail "Array iteration bridge rejected $iteration_case"
		printf 'ok\n' > "$evidence/setup/iteration-$iteration_case-check.expected"
		cmp "$evidence/setup/iteration-$iteration_case-check.expected" "$evidence/setup/iteration-$iteration_case-check.stdout" > /dev/null || fail "iteration check stdout differs"
		require_empty_file "$evidence/setup/iteration-$iteration_case-check.stderr" "iteration check wrote stderr"
		require_forbidden_processes_absent "$evidence/setup/iteration-$iteration_case-check-process.trace" "Array iteration check"
	done
	runtime_seed=$iteration_transition
fi

if test -n "$names_source_root"; then
	mkdir -p "$workspace/setup/names-syntax"
	names_qbe=$workspace/setup/names-syntax/compiler.ssa
	names_assembly=$workspace/setup/names-syntax/compiler.s
	names_transition=$workspace/setup/names-syntax/compiler
	strace -f -e trace=process -o "$evidence/setup/names-emit-process.trace" \
		"$runtime_seed" emit-qbe "$names_entry" \
		> "$names_qbe" 2> "$evidence/setup/names-emit.stderr" || fail "names source emission failed"
	require_empty_file "$evidence/setup/names-emit.stderr" "names source emission wrote stderr"
	test -s "$names_qbe" || fail "names source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/names-emit-process.trace" "names source emission"
	strace -f -e trace=process -o "$evidence/setup/names-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$names_assembly" "$names_qbe" \
		> "$evidence/setup/names-qbe.stdout" 2> "$evidence/setup/names-qbe.stderr" || fail "names QBE translation failed"
	require_empty_file "$evidence/setup/names-qbe.stdout" "names QBE translation wrote stdout"
	require_empty_file "$evidence/setup/names-qbe.stderr" "names QBE translation wrote stderr"
	test -s "$names_assembly" || fail "names assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/names-link-process.trace" \
		"$cc" -xassembler "$names_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$names_transition" \
		> "$evidence/setup/names-link.stdout" 2> "$evidence/setup/names-link.stderr" || fail "names transition link failed"
	require_empty_file "$evidence/setup/names-link.stdout" "names transition link wrote stdout"
	require_empty_file "$evidence/setup/names-link.stderr" "names transition link wrote stderr"
	test -x "$names_transition" || fail "names transition compiler is missing"
	require_tool_observed "$evidence/setup/names-link-process.trace" 'execve\("[^"]*/ld\.lld"' "names transition LLD"
	# An old bridge checks the next accepted source before compiler self-use adopts cycles.
	names_check_entry=$compiler_entry
	if test -n "$cycles_source_root"; then names_check_entry=$cycles_entry; fi
	strace -f -e trace=process -o "$evidence/setup/names-check-process.trace" \
		"$names_transition" check "$names_check_entry" \
		> "$evidence/setup/names-check.stdout" 2> "$evidence/setup/names-check.stderr" || fail "compiler-name bridge rejected its successor source"
	printf 'ok\n' > "$evidence/setup/names-check.expected"
	cmp "$evidence/setup/names-check.expected" "$evidence/setup/names-check.stdout" > /dev/null || fail "compiler-name check stdout differs"
	require_empty_file "$evidence/setup/names-check.stderr" "compiler-name check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/names-check-process.trace" "compiler-name check"
	runtime_seed=$names_transition
fi

if test -n "$cycles_source_root"; then
	mkdir -p "$workspace/setup/module-cycles"
	cycles_qbe=$workspace/setup/module-cycles/compiler.ssa
	cycles_assembly=$workspace/setup/module-cycles/compiler.s
	cycles_transition=$workspace/setup/module-cycles/compiler
	strace -f -e trace=process -o "$evidence/setup/cycles-emit-process.trace" \
		"$runtime_seed" emit-qbe "$cycles_entry" \
		> "$cycles_qbe" 2> "$evidence/setup/cycles-emit.stderr" || fail "module-cycle source emission failed"
	require_empty_file "$evidence/setup/cycles-emit.stderr" "module-cycle source emission wrote stderr"
	test -s "$cycles_qbe" || fail "module-cycle source QBE is empty"
	require_forbidden_processes_absent "$evidence/setup/cycles-emit-process.trace" "module-cycle source emission"
	strace -f -e trace=process -o "$evidence/setup/cycles-qbe-process.trace" \
		"$qbe" -t amd64_sysv -o "$cycles_assembly" "$cycles_qbe" \
		> "$evidence/setup/cycles-qbe.stdout" 2> "$evidence/setup/cycles-qbe.stderr" || fail "module-cycle QBE translation failed"
	require_empty_file "$evidence/setup/cycles-qbe.stdout" "module-cycle QBE translation wrote stdout"
	require_empty_file "$evidence/setup/cycles-qbe.stderr" "module-cycle QBE translation wrote stderr"
	test -s "$cycles_assembly" || fail "module-cycle assembly is empty"
	strace -f -e trace=process -o "$evidence/setup/cycles-link-process.trace" \
		"$cc" -xassembler "$cycles_assembly" -fuse-ld=lld \
		-Wl,--gc-sections,--strip-all -lm -o "$cycles_transition" \
		> "$evidence/setup/cycles-link.stdout" 2> "$evidence/setup/cycles-link.stderr" || fail "module-cycle transition link failed"
	require_empty_file "$evidence/setup/cycles-link.stdout" "module-cycle transition link wrote stdout"
	require_empty_file "$evidence/setup/cycles-link.stderr" "module-cycle transition link wrote stderr"
	test -x "$cycles_transition" || fail "module-cycle transition compiler is missing"
	require_tool_observed "$evidence/setup/cycles-link-process.trace" 'execve\("[^"]*/ld\.lld"' "module-cycle transition LLD"
	strace -f -e trace=process -o "$evidence/setup/cycles-check-process.trace" \
		"$cycles_transition" check "$compiler_entry" \
		> "$evidence/setup/cycles-check.stdout" 2> "$evidence/setup/cycles-check.stderr" || fail "module-cycle bridge rejected current source"
	printf 'ok\n' > "$evidence/setup/cycles-check.expected"
	cmp "$evidence/setup/cycles-check.expected" "$evidence/setup/cycles-check.stdout" > /dev/null || fail "module-cycle check stdout differs"
	require_empty_file "$evidence/setup/cycles-check.stderr" "module-cycle check wrote stderr"
	require_forbidden_processes_absent "$evidence/setup/cycles-check-process.trace" "module-cycle check"
	python3 "$verifier_root/tools/bootstrap-module-cycles.py" \
		--compiler "$cycles_transition" --qbe "$qbe" --profile "$PROFILE" \
		--evidence "$evidence/setup/module-cycle-capabilities" --trace \
		> "$evidence/setup/module-cycle-capabilities.stdout" 2> "$evidence/setup/module-cycle-capabilities.stderr" || fail "module-cycle capability proof failed"
	for trace in "$evidence/setup/module-cycle-capabilities"/*/*.trace; do
		require_forbidden_processes_absent "$trace" "module-cycle capability proof"
	done
	runtime_seed=$cycles_transition
fi

strace -f -e trace=process -o "$evidence/setup/current-runtime-emit-process.trace" \
	"$runtime_seed" emit-qbe "$compiler_entry" \
	> "$runtime_qbe" \
	2> "$evidence/setup/current-runtime-emit.stderr" || fail "current-runtime QBE emission failed"
require_empty_file "$evidence/setup/current-runtime-emit.stderr" "current-runtime QBE emission wrote stderr"
test -s "$runtime_qbe" || fail "first transition emitted empty current-runtime QBE"
require_forbidden_processes_absent "$evidence/setup/current-runtime-emit-process.trace" \
	"current-runtime QBE emission"

strace -f -e trace=process -o "$evidence/setup/current-runtime-qbe-process.trace" \
	"$qbe" -t amd64_sysv -o "$runtime_assembly" "$runtime_qbe" \
	> "$evidence/setup/current-runtime-qbe.stdout" \
	2> "$evidence/setup/current-runtime-qbe.stderr" || fail "current-runtime QBE translation failed"
require_empty_file "$evidence/setup/current-runtime-qbe.stdout" "current-runtime QBE translation wrote stdout"
require_empty_file "$evidence/setup/current-runtime-qbe.stderr" "current-runtime QBE translation wrote stderr"
test -s "$runtime_assembly" || fail "current-runtime QBE translation did not produce assembly"

strace -f -e trace=process -o "$evidence/setup/current-runtime-link-process.trace" \
	"$cc" -xassembler "$runtime_assembly" \
		-fuse-ld=lld \
		-Wl,--gc-sections,--strip-all \
		-lm \
		-o "$runtime_transition" \
	> "$evidence/setup/current-runtime-link.stdout" \
	2> "$evidence/setup/current-runtime-link.stderr" || fail "current-runtime setup link failed"
require_empty_file "$evidence/setup/current-runtime-link.stdout" "current-runtime link wrote stdout"
require_empty_file "$evidence/setup/current-runtime-link.stderr" "current-runtime link wrote stderr"
test -x "$runtime_transition" || fail "current-runtime transition was not produced"
require_tool_observed "$evidence/setup/current-runtime-link-process.trace" 'execve\("[^"]*/ld\.lld"' \
	"current-runtime LLD"

{
	grep execve "$evidence/setup/root-qbe-process.trace"
	grep execve "$evidence/setup/root-link-process.trace"
	grep execve "$evidence/setup/root-emit-process.trace"
	grep execve "$evidence/setup/first-qbe-process.trace"
	grep execve "$evidence/setup/first-link-process.trace"
	if test "$seed_source_root" != "$candidate_root"; then
		grep execve "$evidence/setup/logical-condition-process.trace"
		grep execve "$evidence/setup/elsif-process.trace"
	fi
	if test -n "$loop_source_root"; then
		grep execve "$evidence/setup/loop-emit-process.trace"
		grep execve "$evidence/setup/loop-qbe-process.trace"
		grep execve "$evidence/setup/loop-link-process.trace"
		grep execve "$evidence/setup/loop-check-process.trace"
	fi
	if test -n "$boolean_source_root"; then
		grep execve "$evidence/setup/boolean-emit-process.trace"
		grep execve "$evidence/setup/boolean-qbe-process.trace"
		grep execve "$evidence/setup/boolean-link-process.trace"
		grep execve "$evidence/setup/boolean-check-process.trace"
	fi
	if test -n "$record_source_root"; then
		grep execve "$evidence/setup/record-emit-process.trace"
		grep execve "$evidence/setup/record-qbe-process.trace"
		grep execve "$evidence/setup/record-link-process.trace"
		grep execve "$evidence/setup/record-check-process.trace"
	fi
	if test -n "$hash_source_root"; then
		grep execve "$evidence/setup/hash-emit-process.trace"
		grep execve "$evidence/setup/hash-qbe-process.trace"
		grep execve "$evidence/setup/hash-link-process.trace"
		grep execve "$evidence/setup/hash-check-process.trace"
	fi
	if test -n "$iteration_source_root"; then
		grep execve "$evidence/setup/iteration-emit-process.trace"
		grep execve "$evidence/setup/iteration-qbe-process.trace"
		grep execve "$evidence/setup/iteration-link-process.trace"
		for iteration_case in live control managed; do
			grep execve "$evidence/setup/iteration-$iteration_case-check-process.trace"
		done
	fi
	if test -n "$names_source_root"; then
		grep execve "$evidence/setup/names-emit-process.trace"
		grep execve "$evidence/setup/names-qbe-process.trace"
		grep execve "$evidence/setup/names-link-process.trace"
		grep execve "$evidence/setup/names-check-process.trace"
	fi
	grep execve "$evidence/setup/current-runtime-emit-process.trace"
	grep execve "$evidence/setup/current-runtime-qbe-process.trace"
	grep execve "$evidence/setup/current-runtime-link-process.trace"
} > "$evidence/setup/process-inventory.txt"
{
	printf 'root_qbe_size=%s\n' "$(file_size "$root_qbe")"
	printf 'seed_source_revision=%s\n' "$(git -C "$seed_source_root" rev-parse HEAD)"
	printf 'seed_source_entry_sha256=%s\n' "$(sha256 "$seed_entry")"
	printf 'root_qbe_sha256=%s\n' "$(sha256 "$root_qbe")"
	printf 'root_era_compiler_size=%s\n' "$(file_size "$root_compiler")"
	printf 'root_era_compiler_sha256=%s\n' "$(sha256 "$root_compiler")"
	printf 'first_transition_qbe_size=%s\n' "$(file_size "$first_qbe")"
	printf 'first_transition_qbe_sha256=%s\n' "$(sha256 "$first_qbe")"
	printf 'first_transition_size=%s\n' "$(file_size "$first_transition")"
	printf 'first_transition_sha256=%s\n' "$(sha256 "$first_transition")"
	if test -n "$loop_source_root"; then
		printf 'loop_source_revision=%s\n' "$(git -C "$loop_source_root" rev-parse HEAD)"
		printf 'loop_source_entry_sha256=%s\n' "$(sha256 "$loop_entry")"
		printf 'loop_transition_qbe_size=%s\n' "$(file_size "$loop_qbe")"
		printf 'loop_transition_qbe_sha256=%s\n' "$(sha256 "$loop_qbe")"
		printf 'loop_transition_size=%s\n' "$(file_size "$loop_transition")"
		printf 'loop_transition_sha256=%s\n' "$(sha256 "$loop_transition")"
	fi
	if test -n "$boolean_source_root"; then
		printf 'boolean_source_revision=%s\n' "$(git -C "$boolean_source_root" rev-parse HEAD)"
		printf 'boolean_source_entry_sha256=%s\n' "$(sha256 "$boolean_entry")"
		printf 'boolean_transition_qbe_size=%s\n' "$(file_size "$boolean_qbe")"
		printf 'boolean_transition_qbe_sha256=%s\n' "$(sha256 "$boolean_qbe")"
		printf 'boolean_transition_size=%s\n' "$(file_size "$boolean_transition")"
		printf 'boolean_transition_sha256=%s\n' "$(sha256 "$boolean_transition")"
	fi
	if test -n "$record_source_root"; then
		printf 'record_source_revision=%s\n' "$(git -C "$record_source_root" rev-parse HEAD)"
		printf 'record_source_entry_sha256=%s\n' "$(sha256 "$record_entry")"
		printf 'record_transition_qbe_size=%s\n' "$(file_size "$record_qbe")"
		printf 'record_transition_qbe_sha256=%s\n' "$(sha256 "$record_qbe")"
		printf 'record_transition_size=%s\n' "$(file_size "$record_transition")"
		printf 'record_transition_sha256=%s\n' "$(sha256 "$record_transition")"
	fi
	if test -n "$hash_source_root"; then
		printf 'hash_source_revision=%s\n' "$(git -C "$hash_source_root" rev-parse HEAD)"
		printf 'hash_source_entry_sha256=%s\n' "$(sha256 "$hash_entry")"
		printf 'hash_transition_qbe_size=%s\n' "$(file_size "$hash_qbe")"
		printf 'hash_transition_qbe_sha256=%s\n' "$(sha256 "$hash_qbe")"
		printf 'hash_transition_size=%s\n' "$(file_size "$hash_transition")"
		printf 'hash_transition_sha256=%s\n' "$(sha256 "$hash_transition")"
	fi
	if test -n "$iteration_source_root"; then
		printf 'iteration_source_revision=%s\n' "$(git -C "$iteration_source_root" rev-parse HEAD)"
		printf 'iteration_source_entry_sha256=%s\n' "$(sha256 "$iteration_entry")"
		printf 'iteration_transition_qbe_size=%s\n' "$(file_size "$iteration_qbe")"
		printf 'iteration_transition_qbe_sha256=%s\n' "$(sha256 "$iteration_qbe")"
		printf 'iteration_transition_size=%s\n' "$(file_size "$iteration_transition")"
		printf 'iteration_transition_sha256=%s\n' "$(sha256 "$iteration_transition")"
	fi
	if test -n "$names_source_root"; then
		printf 'names_source_revision=%s\n' "$(git -C "$names_source_root" rev-parse HEAD)"
		printf 'names_source_entry_sha256=%s\n' "$(sha256 "$names_entry")"
		printf 'names_transition_qbe_size=%s\n' "$(file_size "$names_qbe")"
		printf 'names_transition_qbe_sha256=%s\n' "$(sha256 "$names_qbe")"
		printf 'names_transition_size=%s\n' "$(file_size "$names_transition")"
		printf 'names_transition_sha256=%s\n' "$(sha256 "$names_transition")"
	fi
	if test -n "$cycles_source_root"; then
		printf 'cycles_source_revision=%s\n' "$(git -C "$cycles_source_root" rev-parse HEAD)"
		printf 'cycles_source_entry_sha256=%s\n' "$(sha256 "$cycles_entry")"
		printf 'cycles_transition_qbe_size=%s\n' "$(file_size "$cycles_qbe")"
		printf 'cycles_transition_qbe_sha256=%s\n' "$(sha256 "$cycles_qbe")"
		printf 'cycles_transition_size=%s\n' "$(file_size "$cycles_transition")"
		printf 'cycles_transition_sha256=%s\n' "$(sha256 "$cycles_transition")"
	fi
	printf 'current_runtime_qbe_size=%s\n' "$(file_size "$runtime_qbe")"
	printf 'current_runtime_qbe_sha256=%s\n' "$(sha256 "$runtime_qbe")"
	printf 'current_runtime_transition_size=%s\n' "$(file_size "$runtime_transition")"
	printf 'current_runtime_transition_sha256=%s\n' "$(sha256 "$runtime_transition")"
} > "$evidence/setup/identities.txt"

/bin/sh "$verifier_root/tools/bootstrap-seed.sh" \
	--mode previous \
	--input "$runtime_transition" \
	--input-role transition \
	--qbe "$qbe" \
	--cc "$cc" \
	--profile "$PROFILE" \
	--runner-image "$RUNNER_IMAGE" \
	--workspace "$workspace/bootstrap" \
	--output "$output_compiler" \
	--evidence "$evidence/bootstrap" \
	--metadata "$evidence/bootstrap-metadata.json" \
	--asset-name native-target-candidate-linux-amd64 \
	--repository-root "$candidate_root" \
	> "$evidence/bootstrap.stdout" \
	2> "$evidence/bootstrap.stderr" || fail "candidate compiler chain failed"
printf 'bootstrap-seed: previous linux-amd64-v0 passed\n' > "$evidence/bootstrap.expected"
sed -n '$p' "$evidence/bootstrap.stdout" > "$evidence/bootstrap-marker.actual"
cmp "$evidence/bootstrap.expected" "$evidence/bootstrap-marker.actual" > /dev/null ||
	fail "candidate compiler chain success marker differs"
sed '$d' "$evidence/bootstrap.stdout" > "$evidence/bootstrap-checks.stdout"
test -s "$evidence/bootstrap-checks.stdout" || fail "candidate compiler chain reported no checks"
if grep -v '^ok$' "$evidence/bootstrap-checks.stdout" > /dev/null; then
	fail "candidate compiler chain reported unexpected checker output"
fi
require_empty_file "$evidence/bootstrap.stderr" "candidate compiler chain wrote stderr"
require_forbidden_processes_absent "$evidence/bootstrap/process.trace" \
	"closed candidate ordinary build"
require_tool_observed "$evidence/bootstrap/process.trace" 'execve\("[^"]*/ld\.lld"' \
	"closed candidate LLD"

ordinary_trace_directory=$workspace/ordinary-chain-traces
mkdir -p "$ordinary_trace_directory" "$evidence/ordinary-chain"
trace_ordinary_build() {
	label=$1
	seed=$2
	expected=$3
	build_directory=$ordinary_trace_directory/$label
	trace=$evidence/ordinary-chain/$label.process.trace
	mkdir -p "$build_directory"
	strace -f -e trace=process -o "$trace" \
		"$seed" build "$compiler_entry" \
			--output "$build_directory/compiler" \
			--qbe "$qbe" --cc "$cc" --target "$PROFILE" \
		> "$evidence/ordinary-chain/$label.stdout" \
		2> "$evidence/ordinary-chain/$label.stderr" || fail "$label traced ordinary build failed"
	require_empty_file "$evidence/ordinary-chain/$label.stdout" "$label traced ordinary build wrote stdout"
	require_empty_file "$evidence/ordinary-chain/$label.stderr" "$label traced ordinary build wrote stderr"
	cmp "$expected" "$build_directory/compiler" > /dev/null || fail "$label traced compiler bytes differ"
	require_closed_ordinary_trace "$trace" "$seed" "$label ordinary build"
	require_no_intermediates "$build_directory"
}

bootstrap_b1=$workspace/bootstrap/b1/compiler
bootstrap_b2=$workspace/bootstrap/b2/compiler
bootstrap_b3=$workspace/bootstrap/b3/compiler
bootstrap_b4=$workspace/bootstrap/b4/compiler
trace_ordinary_build b1-to-b2 "$bootstrap_b1" "$bootstrap_b2"
trace_ordinary_build b2-to-b3 "$bootstrap_b2" "$bootstrap_b3"
trace_ordinary_build b3-to-b4 "$bootstrap_b3" "$bootstrap_b4"
{
	cat "$evidence/ordinary-chain/b1-to-b2.process.trace.executables"
	cat "$evidence/ordinary-chain/b2-to-b3.process.trace.executables"
	cat "$evidence/ordinary-chain/b3-to-b4.process.trace.executables"
} > "$evidence/ordinary-chain/process-inventory.txt"

failure_order_directory=$workspace/native-target-failure-order
mkdir -p "$failure_order_directory"
probe_tool=$failure_order_directory/probe-tool
probe_marker=$failure_order_directory/probe-launched
cat > "$probe_tool" <<EOF
#!/bin/sh
/usr/bin/touch "$probe_marker"
exit 91
EOF
chmod 0755 "$probe_tool"
printf 'compiler: unsupported target profile\n' > "$failure_order_directory/unsupported.expected"
set +e
"$output_compiler" build "$failure_order_directory/missing-source.trb" \
	--output "$failure_order_directory/unsupported-output" \
	--qbe "$probe_tool" --cc "$probe_tool" --target unknown-v0 \
	> "$failure_order_directory/unsupported.stdout" \
	2> "$failure_order_directory/unsupported.stderr"
unsupported_status=$?
set -e
test "$unsupported_status" -eq 64 || fail "unknown target did not fail with status 64"
require_empty_file "$failure_order_directory/unsupported.stdout" "unknown target wrote stdout"
cmp "$failure_order_directory/unsupported.expected" "$failure_order_directory/unsupported.stderr" > /dev/null ||
	fail "unknown target diagnostic differs"
test ! -e "$probe_marker" || fail "unknown target launched an external tool"
test ! -e "$failure_order_directory/unsupported-output" || fail "unknown target published output"

publication_output=$failure_order_directory/publication-output
mkdir "$publication_output"
printf 'preserve-publication-directory\n' > "$publication_output/sentinel"
printf 'compiler: cannot publish output\n' > "$failure_order_directory/publication.expected"
set +e
"$output_compiler" build "$portable_config" \
	--output "$publication_output" \
	--qbe "$qbe" --cc "$cc" --target "$PROFILE" \
	> "$failure_order_directory/publication.stdout" \
	2> "$failure_order_directory/publication.stderr"
publication_status=$?
set -e
test "$publication_status" -eq 73 || fail "publication failure did not return status 73"
require_empty_file "$failure_order_directory/publication.stdout" "publication failure wrote stdout"
cmp "$failure_order_directory/publication.expected" "$failure_order_directory/publication.stderr" > /dev/null ||
	fail "publication failure diagnostic differs"
test "$(cat "$publication_output/sentinel")" = preserve-publication-directory ||
	fail "publication failure did not preserve the destination directory"
require_no_intermediates "$failure_order_directory"

compiler_size=$(file_size "$output_compiler")
compiler_cost_observe compiler-bytes "$compiler_size" >> "$evidence/cost-observations.txt"
{
	printf 'platform=linux-amd64\n'
	printf 'raw_compiler_bytes=%s\n' "$compiler_size"
} > "$evidence/compiler-size.txt"

mkdir -p \
	"$workspace/native-first" \
	"$workspace/native-second" \
	"$workspace/native-trace" \
	"$workspace/native-failure" \
	"$workspace/go" \
	"$workspace/go-failure" \
	"$evidence/applications"

native_application=$workspace/native-first/program
native_application_repeat=$workspace/native-second/program
native_application_traced=$workspace/native-trace/program
native_failure_application=$workspace/native-failure/program
go_application=$workspace/go/program
go_failure_application=$workspace/go-failure/program

require_successful_command \
	"$evidence/applications/native-first.stdout" \
	"$evidence/applications/native-first.stderr" \
	"first Native portable-entry build" \
	"$output_compiler" build "$portable_config" \
		--output "$native_application" --qbe "$qbe" --cc "$cc" --target "$PROFILE"
require_successful_command \
	"$evidence/applications/native-second.stdout" \
	"$evidence/applications/native-second.stderr" \
	"repeated Native portable-entry build" \
	"$output_compiler" build "$portable_config" \
		--output "$native_application_repeat" --qbe "$qbe" --cc "$cc" --target "$PROFILE"
cmp "$native_application" "$native_application_repeat" > /dev/null ||
	fail "repeated Native portable-entry bytes differ"

"$output_compiler" emit-qbe "$portable_config" \
	> "$evidence/applications/portable-entry-first.ssa" \
	2> "$evidence/applications/portable-entry-first.stderr" || fail "portable-entry QBE emission failed"
"$output_compiler" emit-qbe "$portable_config" \
	> "$evidence/applications/portable-entry-second.ssa" \
	2> "$evidence/applications/portable-entry-second.stderr" || fail "repeated portable-entry QBE emission failed"
require_empty_file "$evidence/applications/portable-entry-first.stderr" \
	"portable-entry QBE emission wrote stderr"
require_empty_file "$evidence/applications/portable-entry-second.stderr" \
	"repeated portable-entry QBE emission wrote stderr"
cmp "$evidence/applications/portable-entry-first.ssa" \
	"$evidence/applications/portable-entry-second.ssa" > /dev/null ||
	fail "portable-entry QBE is not deterministic"

require_go_build \
	"$evidence/applications/go.stdout" \
	"$evidence/applications/go.stderr" \
	"$evidence/applications/go.expected" \
	"$go_application" \
	"optimized Go portable-entry build" \
	"$reference_trb" build --compile --config "$portable_config" --outfile "$go_application"

strace -f -e trace=process -o "$evidence/applications/native-build-process.trace" \
	"$output_compiler" build "$portable_config" \
		--output "$native_application_traced" \
		--qbe "$qbe" \
		--cc "$cc" \
		--target "$PROFILE" \
	> "$evidence/applications/native-trace.stdout" \
	2> "$evidence/applications/native-trace.stderr" || fail "traced Native portable-entry build failed"
require_empty_file "$evidence/applications/native-trace.stdout" \
	"traced Native portable-entry build wrote stdout"
require_empty_file "$evidence/applications/native-trace.stderr" \
	"traced Native portable-entry build wrote stderr"
require_closed_ordinary_trace "$evidence/applications/native-build-process.trace" \
	"$output_compiler" "ordinary Native application build"
cp "$evidence/applications/native-build-process.trace.executables" \
	"$evidence/applications/native-build-process-inventory.txt"
cmp "$native_application" "$native_application_traced" > /dev/null ||
	fail "traced Native portable-entry bytes differ"

expected_stdout=$evidence/applications/portable-entry.expected
cat > "$expected_stdout" <<'EOF'
2
1
144
12
0
0
0
9007199254740991
-9007199254740991
-2
nan
EOF

for candidate_pair in "native:$native_application" "go:$go_application"; do
	candidate_label=$(printf '%s\n' "$candidate_pair" | cut -d: -f1)
	executable=$(printf '%s\n' "$candidate_pair" | cut -d: -f2-)
	"$executable" 144 \
		> "$evidence/applications/$candidate_label-runtime.stdout" \
		2> "$evidence/applications/$candidate_label-runtime.stderr" ||
		fail "$candidate_label portable-entry runtime failed"
	cmp "$expected_stdout" "$evidence/applications/$candidate_label-runtime.stdout" > /dev/null ||
		fail "$candidate_label portable-entry stdout differs"
	require_empty_file "$evidence/applications/$candidate_label-runtime.stderr" \
		"$candidate_label portable-entry runtime wrote stderr"
done
cmp "$evidence/applications/native-runtime.stdout" "$evidence/applications/go-runtime.stdout" > /dev/null ||
	fail "Native and Go portable-entry stdout differ"

require_successful_command \
	"$evidence/applications/native-failure-build.stdout" \
	"$evidence/applications/native-failure-build.stderr" \
	"Native runtime-failure build" \
	"$output_compiler" build "$failure_config" \
		--output "$native_failure_application" --qbe "$qbe" --cc "$cc" --target "$PROFILE"
require_go_build \
	"$evidence/applications/go-failure-build.stdout" \
	"$evidence/applications/go-failure-build.stderr" \
	"$evidence/applications/go-failure-build.expected" \
	"$go_failure_application" \
	"optimized Go runtime-failure build" \
	"$reference_trb" build --compile --config "$failure_config" --outfile "$go_failure_application"

printf 'mode,native_status,go_status,failure_class\n' > "$evidence/applications/runtime-failures.csv"
for failure_case in \
	"invalid-format:panic: invalid Integer" \
	"sign-only:panic: invalid Integer" \
	"integer-range:panic: Integer is outside the portable range" \
	"nan:panic: Float cannot be converted to Integer" \
	"infinity:panic: Float cannot be converted to Integer" \
	"float-range:panic: Integer is outside the portable range"; do
	mode=$(printf '%s\n' "$failure_case" | cut -d: -f1)
	expected_class=$(printf '%s\n' "$failure_case" | cut -d: -f2-)
	for candidate_pair in "native:$native_failure_application" "go:$go_failure_application"; do
		candidate_label=$(printf '%s\n' "$candidate_pair" | cut -d: -f1)
		executable=$(printf '%s\n' "$candidate_pair" | cut -d: -f2-)
		set +e
		"$executable" "$mode" \
			> "$evidence/applications/$candidate_label-$mode.stdout" \
			2> "$evidence/applications/$candidate_label-$mode.stderr"
		status=$?
		set -e
		test "$status" -ne 0 || fail "$candidate_label $mode unexpectedly succeeded"
		require_empty_file "$evidence/applications/$candidate_label-$mode.stdout" \
			"$candidate_label $mode wrote stdout"
		actual_class=$(sed -n '1p' "$evidence/applications/$candidate_label-$mode.stderr")
		test "$actual_class" = "$expected_class" || fail "$candidate_label $mode failure class differs"
		if test "$candidate_label" = native; then
			native_status=$status
			native_class=$actual_class
		else
			go_status=$status
			go_class=$actual_class
		fi
	done
	test "$native_class" = "$go_class" || fail "Native and Go $mode failure classes differ"
	printf '%s,%s,%s,%s\n' "$mode" "$native_status" "$go_status" "$native_class" \
		>> "$evidence/applications/runtime-failures.csv"
done

verify_binary_format
if test "$(compiler_cost_mode)" = strict; then
	run_formal_evidence
else
	printf 'cost_mode=mir-migration\ncomparative_measurements=not-run\n' > "$evidence/measurement-policy.txt"
fi
record_target_evidence
