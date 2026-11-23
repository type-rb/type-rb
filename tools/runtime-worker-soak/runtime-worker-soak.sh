#!/bin/sh

set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_directory/../compiler-cost.sh"
compiler_cost_mode > /dev/null || exit 64

MAX_PEAK_HEAP_BYTES=4194304
MAX_SMOKE_SECONDS=2.25
MIN_FORMAL_ALLOCATED_BYTES=32212254720
# Initial state: Array handle/backing (40 + 512) and WorkerState (24).
# Each batch: two Array handles/backings (2 * (40 + 2048)), WorkerBatch (32),
# and eight retry Strings (8 * (19 + 33)). Original payloads are static.
EXPECTED_INITIAL_ALLOCATED_BYTES=576
EXPECTED_BATCH_ALLOCATED_BYTES=4624
STAT_PREFIX=type-rb-native-gc-stat-v1
PHASE_MARKER=native-worker-phase
SUCCESS_MARKER=native-worker-ok

usage() {
	printf '%s\n' \
		'usage: runtime-worker-soak.sh smoke|formal|asan|valgrind COMPILER QBE CC' \
		'       REFERENCE_TRB|- darwin-arm64-v0|linux-arm64-v0 WORKSPACE EVIDENCE' >&2
	exit 64
}

fail() {
	printf 'runtime-worker-soak: %s\n' "$1" >&2
	exit 1
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

sha256() {
	if command -v sha256sum > /dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

require_unsigned() {
	case "$2" in
	'' | *[!0-9]*) fail "$1 is not an unsigned integer: $2" ;;
	esac
}

stat_value() {
	awk -F, -v prefix="$STAT_PREFIX" -v wanted="$1" '
		$1 == prefix && $2 == wanted {
			count += 1
			value = $3
		}
		END {
			if (count != 1) exit 1
			print value
		}
	' "$2"
}

summary_value() {
	awk -F= -v wanted="$1" '
		$1 == wanted {
			count += 1
			value = $2
		}
		END {
			if (count != 1) exit 1
			print value
		}
	' "$2"
}

require_stdout() {
	awk -v phases="$1" -v phase_marker="$PHASE_MARKER" -v success_marker="$SUCCESS_MARKER" '
		NR <= phases && $0 != phase_marker { failed = 1 }
		NR == phases + 1 && $0 != success_marker { failed = 1 }
		END { exit (failed || NR != phases + 1) }
	' "$2" || fail "$3 stdout differs"
}

require_collector_statistics() {
	log=$1
	collections=$(stat_value collections "$log") || fail "collection count is missing or repeated"
	automatic=$(stat_value automatic-collections "$log") || fail "automatic collection count is missing or repeated"
	allocated=$(stat_value allocated-bytes "$log") || fail "allocated byte count is missing or repeated"
	reclaimed=$(stat_value reclaimed-bytes "$log") || fail "reclaimed byte count is missing or repeated"
	live=$(stat_value live-bytes "$log") || fail "live byte count is missing or repeated"
	peak=$(stat_value peak-heap-bytes "$log") || fail "peak heap byte count is missing or repeated"

	require_unsigned collections "$collections"
	require_unsigned automatic-collections "$automatic"
	require_unsigned allocated-bytes "$allocated"
	require_unsigned reclaimed-bytes "$reclaimed"
	require_unsigned live-bytes "$live"
	require_unsigned peak-heap-bytes "$peak"
	test "$collections" -gt 0 || fail "no collection was reported"
	test "$automatic" -gt 0 || fail "no automatic collection was reported"
	test "$reclaimed" -gt 0 || fail "no reclaimed bytes were reported"
	test "$live" -eq 0 || fail "final managed live bytes are not zero"
	test "$allocated" -eq "$((reclaimed + live))" || fail "allocation accounting does not close"
	test "$peak" -le "$MAX_PEAK_HEAP_BYTES" || fail "peak managed heap exceeds 4 MiB"

	{
		printf 'collections=%s\n' "$collections"
		printf 'automatic_collections=%s\n' "$automatic"
		printf 'allocated_bytes=%s\n' "$allocated"
		printf 'reclaimed_bytes=%s\n' "$reclaimed"
		printf 'live_bytes=%s\n' "$live"
		printf 'peak_heap_bytes=%s\n' "$peak"
	} > "$evidence/collector-statistics.txt"
}

require_trace() {
	log=$1
	minimum=$2
	awk -v minimum_observations="$minimum" -f "$trace_analyzer" "$log" \
		> "$evidence/gc-trace-analysis.txt" || fail "managed-runtime trace validation failed"
	trace_collection=$(summary_value final_collection "$evidence/gc-trace-analysis.txt") ||
		fail "final trace collection is missing"
	stats_collection=$(summary_value collections "$evidence/collector-statistics.txt") ||
		fail "final stats collection is missing"
	test "$trace_collection" -eq "$stats_collection" ||
		fail "trace and stats final collection counts differ"
}

run_sampled_process() {
	label=$1
	program=$2
	stdout=$3
	stderr=$4
	series=$5
	printf 'elapsed_seconds,rss_bytes,completed_phases,fd_count,thread_count\n' > "$series"
	start_uptime=$(awk '{print $1}' /proc/uptime)
	if test "$label" = native; then
		TYPE_RB_NATIVE_RUNTIME_STATS=1 TYPE_RB_NATIVE_RUNTIME_TRACE=1 \
			"$program" > "$stdout" 2> "$stderr" &
	else
		"$program" > "$stdout" 2> "$stderr" &
	fi
	program_pid=$!
	while kill -0 "$program_pid" 2> /dev/null; do
		state=$(awk '$1 == "State:" {print $2}' "/proc/$program_pid/status" 2> /dev/null || true)
		test "$state" != Z || break
		rss_kib=$(awk '$1 == "VmRSS:" {print $2}' "/proc/$program_pid/status" 2> /dev/null || true)
		threads=$(awk '$1 == "Threads:" {print $2}' "/proc/$program_pid/status" 2> /dev/null || true)
		fd_count=$(find "/proc/$program_pid/fd" -mindepth 1 -maxdepth 1 -print 2> /dev/null | wc -l | tr -d ' ')
		final_state=$(awk '$1 == "State:" {print $2}' "/proc/$program_pid/status" 2> /dev/null || true)
		if test -n "$rss_kib" && test -n "$threads" && test "$fd_count" -gt 0 && \
			test -n "$final_state" && test "$final_state" != Z && kill -0 "$program_pid" 2> /dev/null; then
			now_uptime=$(awk '{print $1}' /proc/uptime)
			elapsed=$(awk -v start="$start_uptime" -v now="$now_uptime" 'BEGIN {printf "%.2f", now - start}')
			completed=$(awk -v marker="$PHASE_MARKER" '$0 == marker {count += 1} END {print count + 0}' "$stdout")
			printf '%s,%s,%s,%s,%s\n' \
				"$elapsed" "$((rss_kib * 1024))" "$completed" "$fd_count" "$threads" >> "$series"
		fi
		sleep 0.25
	done
	set +e
	wait "$program_pid"
	runtime_status=$?
	set -e
	end_uptime=$(awk '{print $1}' /proc/uptime)
	elapsed=$(awk -v start="$start_uptime" -v end="$end_uptime" 'BEGIN {printf "%.2f", end - start}')
	printf 'elapsed_seconds=%s\n' "$elapsed" > "$evidence/$label-runtime.txt"
	test "$runtime_status" -eq 0 || fail "$label formal workload failed with status $runtime_status"
}

test "$#" -eq 8 || usage
mode=$1
compiler=$2
qbe=$3
cc=$4
reference_trb=$5
profile=$6
workspace=$7
evidence=$8

case "$mode" in
smoke)
	phases=1
	batches=40000
	minimum_trace_observations=1
	build_reference=1
	;;
formal)
	phases=60
	batches=120000
	minimum_trace_observations=400
	build_reference=1
	;;
asan | valgrind)
	phases=1
	batches=400
	minimum_trace_observations=1
	build_reference=0
	;;
*) usage ;;
esac

case "$profile" in
darwin-arm64-v0)
	qbe_target=arm64_apple
	expected_system=Darwin
	;;
linux-arm64-v0)
	qbe_target=arm64
	expected_system=Linux
	;;
*) usage ;;
esac

test "$(uname -s)" = "$expected_system" || fail "target profile does not match the host"
test -x "$compiler" || fail "compiler is not executable"
test -x "$qbe" || fail "QBE is not executable"
test -x "$cc" || fail "CC is not executable"
if test "$build_reference" -eq 1; then
	test -x "$reference_trb" || fail "reference TypeRB compiler is not executable"
fi
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"

template=$script_directory/workload.trb
config_template=$script_directory/trbconfig.jsonc
trace_analyzer=$script_directory/analyze-gc-trace.awk
process_analyzer=$script_directory/analyze-process-series.awk
test -f "$template" || fail "workload template is missing"
test -f "$config_template" || fail "reference configuration is missing"
test -f "$trace_analyzer" || fail "GC trace analyzer is missing"
test -f "$process_analyzer" || fail "process-series analyzer is missing"
test "$(grep -F -c 'run_worker_lifecycle(60, 120000, 128)' "$template")" -eq 1 ||
	fail "registered workload invocation is not unique"

mkdir -p "$workspace/native" "$workspace/reference/src" "$evidence"
source=$workspace/workload.trb
sed "s/run_worker_lifecycle(60, 120000, 128)/run_worker_lifecycle($phases, $batches, 128)/" \
	"$template" > "$source"
cp "$source" "$workspace/reference/src/main.trb"
cp "$config_template" "$workspace/reference/trbconfig.jsonc"

{
	printf 'controller=/bin/sh tools/runtime-worker-soak/runtime-worker-soak.sh\n'
	printf 'mode=%s\n' "$mode"
	printf 'compiler=%s\n' "$compiler"
	printf 'qbe=%s\n' "$qbe"
	printf 'cc=%s\n' "$cc"
	printf 'reference_trb=%s\n' "$reference_trb"
	printf 'profile=%s\n' "$profile"
	printf 'workspace=%s\n' "$workspace"
	printf 'evidence=%s\n' "$evidence"
	printf 'runtime_stats=1\n'
	printf 'runtime_trace=1\n'
} > "$evidence/invocation.txt"

"$compiler" check "$source" > "$workspace/native/check.stdout" 2> "$workspace/native/check.stderr"
test "$(cat "$workspace/native/check.stdout")" = ok || fail "compiler source check output differs"
test ! -s "$workspace/native/check.stderr" || fail "compiler source check wrote stderr"
"$compiler" emit-qbe "$source" > "$workspace/native/program-first.ssa" 2> "$workspace/native/emit-first.stderr"
"$compiler" emit-qbe "$source" > "$workspace/native/program-second.ssa" 2> "$workspace/native/emit-second.stderr"
test ! -s "$workspace/native/emit-first.stderr" || fail "first QBE emission wrote stderr"
test ! -s "$workspace/native/emit-second.stderr" || fail "second QBE emission wrote stderr"
cmp "$workspace/native/program-first.ssa" "$workspace/native/program-second.ssa" > /dev/null ||
	fail "repeated QBE emission differs"

native_program=$workspace/native/program
if test "$mode" = asan; then
	test "$expected_system" = Linux || fail "the registered ASan/LSan run uses Linux arm64"
	"$qbe" -t "$qbe_target" -o "$workspace/native/program.s" "$workspace/native/program-first.ssa"
	"$cc" -x assembler "$workspace/native/program.s" -fsanitize=address,leak \
		-Wl,--gc-sections -o "$native_program"
else
	"$compiler" build "$source" --output "$native_program" --qbe "$qbe" --cc "$cc" --target "$profile" \
		> "$workspace/native/build.stdout" 2> "$workspace/native/build.stderr"
	test ! -s "$workspace/native/build.stdout" || fail "Native build wrote stdout"
	test ! -s "$workspace/native/build.stderr" || fail "Native build wrote stderr"
fi

cp "$compiler" "$workspace/compiler.stripped"
if test "$expected_system" = Darwin; then
	/usr/bin/strip -x "$workspace/compiler.stripped"
else
	/usr/bin/strip --strip-all "$workspace/compiler.stripped"
fi
stripped_compiler_size=$(file_size "$workspace/compiler.stripped")
printf '%s\n' "$stripped_compiler_size" > "$evidence/compiler-size-bytes.txt"
compiler_cost_observe compiler-bytes "$stripped_compiler_size" >> "$evidence/cost-observations.txt"

reference_program=$workspace/reference/program
if test "$build_reference" -eq 1; then
	(
		cd "$workspace/reference"
		"$reference_trb" check --config trbconfig.jsonc > check.stdout 2> check.stderr
		"$reference_trb" build --compile --config trbconfig.jsonc --outfile "$reference_program" \
			> build.stdout 2> build.stderr
	)
	test "$(cat "$workspace/reference/check.stdout")" = 'checked 1 file(s) for mode go' ||
		fail "reference source check output differs"
	test ! -s "$workspace/reference/check.stderr" || fail "reference source check wrote stderr"
	printf 'executable -> %s\n' "$reference_program" > "$workspace/reference/build.expected"
	cmp "$workspace/reference/build.expected" "$workspace/reference/build.stdout" > /dev/null ||
		fail "reference build output differs"
	test ! -s "$workspace/reference/build.stderr" || fail "reference build wrote stderr"
	test -x "$reference_program" || fail "reference executable was not published"
fi

native_stdout=$evidence/native-stdout.txt
native_stderr=$evidence/native-stderr.txt
reference_stdout=$evidence/reference-stdout.txt
reference_stderr=$evidence/reference-stderr.txt

if test "$mode" = formal; then
	test "$expected_system" = Linux || fail "the registered formal soak uses Linux arm64"
	run_sampled_process native "$native_program" "$native_stdout" "$native_stderr" \
		"$evidence/native-process-series.csv"
	run_sampled_process reference "$reference_program" "$reference_stdout" "$reference_stderr" \
		"$evidence/reference-process-series.csv"
	awk -v enforce_native=1 -f "$process_analyzer" "$evidence/native-process-series.csv" \
		> "$evidence/native-process-analysis.txt" || fail "Native process-series validation failed"
	awk -v enforce_native=0 -f "$process_analyzer" "$evidence/reference-process-series.csv" \
		> "$evidence/reference-process-analysis.txt" || fail "reference process-series validation failed"
elif test "$mode" = valgrind; then
	test "$expected_system" = Linux || fail "the registered Valgrind run uses Linux arm64"
	command -v valgrind > /dev/null 2>&1 || fail "Valgrind is not installed"
	set +e
	LC_ALL=C TYPE_RB_NATIVE_RUNTIME_STATS=1 TYPE_RB_NATIVE_RUNTIME_TRACE=1 valgrind \
		--leak-check=full --show-leak-kinds=all \
		--errors-for-leak-kinds=definite,indirect,possible \
		--error-exitcode=97 --log-file="$evidence/valgrind.txt" \
		"$native_program" > "$native_stdout" 2> "$native_stderr"
	runtime_status=$?
	set -e
	test "$runtime_status" -eq 0 || fail "Valgrind workload failed with status $runtime_status"
	grep -E 'definitely lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" > /dev/null ||
		fail "Valgrind found definitely lost bytes"
	grep -E 'indirectly lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" > /dev/null ||
		fail "Valgrind found indirectly lost bytes"
	grep -E 'possibly lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" > /dev/null ||
		fail "Valgrind found possibly lost bytes"
	grep -E 'ERROR SUMMARY: 0 errors from 0 contexts' "$evidence/valgrind.txt" > /dev/null ||
		fail "Valgrind reported an error"
	if grep -E 'still reachable: 0 bytes in 0 blocks' "$evidence/valgrind.txt" > /dev/null; then
		printf 'still_reachable_bytes=0\nstill_reachable_blocks=0\njustification=none\n' \
			> "$evidence/valgrind-still-reachable.txt"
	else
		grep -E 'still reachable: 512 bytes in 1 blocks' "$evidence/valgrind.txt" > /dev/null ||
			fail "Valgrind found an unexpected still-reachable allocation"
		printf '%s\n' \
			'still_reachable_bytes=512' \
			'still_reachable_blocks=1' \
			'justification=bounded runtime temporary-root storage remains globally reachable until process exit' \
			> "$evidence/valgrind-still-reachable.txt"
	fi
else
	if test "$mode" = asan; then
		ASAN_OPTIONS=detect_leaks=1:halt_on_error=1:exitcode=97 \
		LSAN_OPTIONS=exitcode=98:report_objects=1 \
		/usr/bin/time -p /usr/bin/env TYPE_RB_NATIVE_RUNTIME_STATS=1 TYPE_RB_NATIVE_RUNTIME_TRACE=1 \
			"$native_program" > "$native_stdout" 2> "$native_stderr"
	else
		/usr/bin/time -p /usr/bin/env TYPE_RB_NATIVE_RUNTIME_STATS=1 TYPE_RB_NATIVE_RUNTIME_TRACE=1 \
			"$native_program" > "$native_stdout" 2> "$native_stderr"
		/usr/bin/time -p "$reference_program" > "$reference_stdout" 2> "$reference_stderr"
	fi
	native_elapsed=$(awk '$1 == "real" {print $2}' "$native_stderr")
	test -n "$native_elapsed" || fail "Native runtime elapsed time is missing"
	printf 'elapsed_seconds=%s\n' "$native_elapsed" > "$evidence/native-runtime.txt"
	if test "$mode" = smoke; then
		compiler_cost_check smoke-seconds "$native_elapsed" "$MAX_SMOKE_SECONDS" \
			>> "$evidence/cost-observations.txt" ||
			fail "smoke runtime exceeds the registered 2.25-second ceiling"
		reference_elapsed=$(awk '$1 == "real" {print $2}' "$reference_stderr")
		test -n "$reference_elapsed" || fail "reference runtime elapsed time is missing"
		printf 'elapsed_seconds=%s\n' "$reference_elapsed" > "$evidence/reference-runtime.txt"
	fi
	if test "$mode" = asan; then
		if grep -E 'AddressSanitizer|LeakSanitizer|detected memory leaks' "$native_stderr" > /dev/null; then
			fail "ASan/LSan reported an error"
		fi
	fi
fi

require_stdout "$phases" "$native_stdout" Native
if test "$build_reference" -eq 1; then
	require_stdout "$phases" "$reference_stdout" reference
	cmp "$native_stdout" "$reference_stdout" > /dev/null || fail "Native and reference stdout differ"
	test ! -s "$reference_stderr" || {
		if test "$mode" = formal; then
			fail "reference formal run wrote stderr"
		fi
	}
fi
require_collector_statistics "$native_stderr"
require_trace "$native_stderr" "$minimum_trace_observations"

allocated=$(summary_value allocated_bytes "$evidence/collector-statistics.txt") ||
	fail "allocated byte summary is missing"
if test "$mode" = formal; then
	test "$allocated" -ge "$MIN_FORMAL_ALLOCATED_BYTES" || fail "formal allocation is below 30 GiB"
fi
expected_allocated=$((EXPECTED_INITIAL_ALLOCATED_BYTES + phases * batches * EXPECTED_BATCH_ALLOCATED_BYTES))
test "$allocated" -eq "$expected_allocated" ||
	fail "allocation differs from the exact worker workload"

cp "$source" "$evidence/workload.trb"
cp "$workspace/native/program-first.ssa" "$evidence/program.ssa"
if test -f "$workspace/native/program.s"; then
	cp "$workspace/native/program.s" "$evidence/program.s"
fi
{
	printf 'mode=%s\n' "$mode"
	printf 'profile=%s\n' "$profile"
	printf 'phases=%s\n' "$phases"
	printf 'batches_per_phase=%s\n' "$batches"
	printf 'original_jobs_per_batch=128\n'
	printf 'processed_attempts_per_batch=136\n'
	printf 'compiler_size=%s\n' "$(file_size "$compiler")"
	printf 'stripped_compiler_size=%s\n' "$stripped_compiler_size"
	printf 'compiler_sha256=%s\n' "$(sha256 "$compiler")"
	printf 'qbe_sha256=%s\n' "$(sha256 "$qbe")"
	printf 'workload_sha256=%s\n' "$(sha256 "$source")"
	printf 'qbe_output_sha256=%s\n' "$(sha256 "$workspace/native/program-first.ssa")"
	printf 'native_program_sha256=%s\n' "$(sha256 "$native_program")"
	if test "$build_reference" -eq 1; then
		printf 'reference_trb_version=%s\n' "$("$reference_trb" version)"
		printf 'reference_program_sha256=%s\n' "$(sha256 "$reference_program")"
	fi
} > "$evidence/identities.txt"
{
	uname -a
	"$cc" --version
	"$qbe" -h
	if test "$mode" = valgrind; then
		valgrind --version
	fi
} > "$evidence/environment.txt" 2>&1

printf 'runtime-worker-soak: %s %s passed\n' "$mode" "$profile"
