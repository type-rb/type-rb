#!/bin/sh

set -eu

MAX_PEAK_HEAP_BYTES=4194304
MAX_SMOKE_SECONDS=4.25
MIN_FORMAL_ALLOCATED_BYTES=32212254720
MAX_FORMAL_RSS_BYTES=67108864
MAX_FORMAL_QUARTILE_GROWTH_BYTES=8388608
MAX_FORMAL_SLOPE_BYTES_PER_MINUTE=1048576
STAT_PREFIX=type-rb-native-gc-stat-v1
PHASE_MARKER=native-memory-soak-phase
SUCCESS_MARKER=native-memory-soak-ok

usage() {
	cat >&2 <<'EOF'
usage: runtime-memory-soak.sh smoke|formal|asan|valgrind COMPILER QBE CC
       darwin-arm64-v0|linux-arm64-v0 WORKSPACE EVIDENCE
EOF
	exit 64
}

fail() {
	printf 'runtime-memory-soak: %s\n' "$1" >&2
	exit 1
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
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

require_stdout() {
	awk -v phases="$1" -v phase_marker="$PHASE_MARKER" -v success_marker="$SUCCESS_MARKER" '
		NR <= phases && $0 != phase_marker { failed = 1 }
		NR == phases + 1 && $0 != success_marker { failed = 1 }
		END { exit (failed || NR != phases + 1) }
	' "$2" || fail "workload stdout differs"
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

	cat > "$evidence/collector-statistics.txt" <<EOF
collections=$collections
automatic_collections=$automatic
allocated_bytes=$allocated
reclaimed_bytes=$reclaimed
live_bytes=$live
peak_heap_bytes=$peak
EOF
}

test "$#" -eq 7 || usage
mode=$1
compiler=$2
qbe=$3
cc=$4
profile=$5
workspace=$6
evidence=$7

case "$mode" in
smoke)
	phases=1
	iterations=5000000
	;;
formal)
	phases=60
	iterations=5000000
	;;
asan | valgrind)
	phases=1
	iterations=50000
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
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_directory/../compiler-cost.sh"
compiler_cost_mode > /dev/null || exit 64
template=$script_directory/workload.trb
analyzer=$script_directory/analyze-rss.awk
test -f "$template" || fail "workload template is missing"
test -f "$analyzer" || fail "RSS analyzer is missing"
test "$(grep -F -c 'run_memory_soak(60, 5000000)' "$template")" -eq 1 ||
	fail "registered workload invocation is not unique"

mkdir -p "$workspace" "$evidence"
{
	printf 'controller=/bin/sh tools/runtime-memory-soak/runtime-memory-soak.sh\n'
	printf 'mode=%s\n' "$mode"
	printf 'compiler=%s\n' "$compiler"
	printf 'qbe=%s\n' "$qbe"
	printf 'cc=%s\n' "$cc"
	printf 'profile=%s\n' "$profile"
	printf 'workspace=%s\n' "$workspace"
	printf 'evidence=%s\n' "$evidence"
	if test "$mode" = asan; then
		printf 'asan_options=detect_leaks=1:halt_on_error=1:exitcode=97\n'
		printf 'lsan_options=exitcode=98:report_objects=1\n'
	fi
	if test "$mode" = valgrind; then
		printf 'valgrind_options=--leak-check=full --show-leak-kinds=all --errors-for-leak-kinds=definite,indirect,possible --error-exitcode=97\n'
	fi
} > "$evidence/invocation.txt"
source=$workspace/workload.trb
sed "s/run_memory_soak(60, 5000000)/run_memory_soak($phases, $iterations)/" "$template" > "$source"

"$compiler" check "$source" > "$workspace/check.stdout" 2> "$workspace/check.stderr"
test "$(cat "$workspace/check.stdout")" = ok || fail "compiler source check output differs"
test ! -s "$workspace/check.stderr" || fail "compiler source check wrote stderr"
"$compiler" emit-qbe "$source" > "$workspace/program-first.ssa" 2> "$workspace/emit-first.stderr"
"$compiler" emit-qbe "$source" > "$workspace/program-second.ssa" 2> "$workspace/emit-second.stderr"
test ! -s "$workspace/emit-first.stderr" || fail "first QBE emission wrote stderr"
test ! -s "$workspace/emit-second.stderr" || fail "second QBE emission wrote stderr"
cmp "$workspace/program-first.ssa" "$workspace/program-second.ssa" >/dev/null ||
	fail "repeated QBE emission differs"

program=$workspace/program
if test "$mode" = asan; then
	test "$expected_system" = Linux || fail "the registered ASan/LSan run uses Linux arm64"
	"$qbe" -t "$qbe_target" -o "$workspace/program.s" "$workspace/program-first.ssa"
	"$cc" -x assembler "$workspace/program.s" -fsanitize=address,leak \
		-Wl,--gc-sections -o "$program"
else
	"$compiler" build "$source" --output "$program" --qbe "$qbe" --cc "$cc" --target "$profile" \
		> "$workspace/build.stdout" 2> "$workspace/build.stderr"
	test ! -s "$workspace/build.stdout" || fail "Native build wrote stdout"
	test ! -s "$workspace/build.stderr" || fail "Native build wrote stderr"
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

stdout=$evidence/stdout.txt
runtime_log=$evidence/runtime-stderr.txt

if test "$mode" = formal; then
	test "$expected_system" = Linux || fail "the registered formal soak uses Linux arm64"
	rss_samples=$evidence/rss-samples.csv
	printf 'elapsed_seconds,rss_bytes,completed_phases\n' > "$rss_samples"
	start_uptime=$(awk '{print $1}' /proc/uptime)
	TYPE_RB_NATIVE_RUNTIME_STATS=1 "$program" > "$stdout" 2> "$runtime_log" &
	program_pid=$!
	while kill -0 "$program_pid" 2>/dev/null; do
		state=$(awk '$1 == "State:" {print $2}' "/proc/$program_pid/status" 2>/dev/null || true)
		test "$state" != Z || break
		rss_kib=$(awk '$1 == "VmRSS:" {print $2}' "/proc/$program_pid/status" 2>/dev/null || true)
		if test -n "$rss_kib"; then
			now_uptime=$(awk '{print $1}' /proc/uptime)
			elapsed=$(awk -v start="$start_uptime" -v now="$now_uptime" 'BEGIN {printf "%.2f", now - start}')
			completed=$(awk -v marker="$PHASE_MARKER" '$0 == marker {count += 1} END {print count + 0}' "$stdout")
			printf '%s,%s,%s\n' "$elapsed" "$((rss_kib * 1024))" "$completed" >> "$rss_samples"
		fi
		sleep 0.25
	done
	set +e
	wait "$program_pid"
	runtime_status=$?
	set -e
	end_uptime=$(awk '{print $1}' /proc/uptime)
	formal_elapsed=$(awk -v start="$start_uptime" -v end="$end_uptime" 'BEGIN {printf "%.2f", end - start}')
	printf 'elapsed_seconds=%s\n' "$formal_elapsed" > "$evidence/runtime.txt"
	test "$runtime_status" -eq 0 || fail "formal workload failed with status $runtime_status"
	awk -f "$analyzer" "$rss_samples" > "$evidence/rss-analysis.txt"
	maximum_rss=$(awk -F= '$1 == "maximum_rss_bytes" {print $2}' "$evidence/rss-analysis.txt")
	quartile_growth=$(awk -F= '$1 == "quartile_median_growth_bytes" {print $2}' "$evidence/rss-analysis.txt")
	slope=$(awk -F= '$1 == "fitted_rss_slope_bytes_per_minute" {print $2}' "$evidence/rss-analysis.txt")
	require_unsigned maximum-rss "$maximum_rss"
	test "$maximum_rss" -le "$MAX_FORMAL_RSS_BYTES" || fail "formal maximum RSS exceeds 64 MiB"
	awk -v value="$quartile_growth" -v limit="$MAX_FORMAL_QUARTILE_GROWTH_BYTES" 'BEGIN {exit !(value <= limit)}' ||
		fail "formal quartile median RSS growth exceeds 8 MiB"
	awk -v value="$slope" -v limit="$MAX_FORMAL_SLOPE_BYTES_PER_MINUTE" 'BEGIN {exit !(value <= limit)}' ||
		fail "formal fitted RSS slope exceeds 1 MiB per minute"
elif test "$mode" = valgrind; then
	test "$expected_system" = Linux || fail "the registered Valgrind run uses Linux arm64"
	command -v valgrind >/dev/null 2>&1 || fail "Valgrind is not installed"
	set +e
	LC_ALL=C TYPE_RB_NATIVE_RUNTIME_STATS=1 valgrind \
		--leak-check=full --show-leak-kinds=all --errors-for-leak-kinds=definite,indirect,possible \
		--error-exitcode=97 --log-file="$evidence/valgrind.txt" \
		"$program" > "$stdout" 2> "$runtime_log"
	runtime_status=$?
	set -e
	test "$runtime_status" -eq 0 || fail "Valgrind workload failed with status $runtime_status"
	grep -E 'definitely lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" >/dev/null || fail "Valgrind found definitely lost bytes"
	grep -E 'indirectly lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" >/dev/null || fail "Valgrind found indirectly lost bytes"
	grep -E 'possibly lost: 0 bytes in 0 blocks' "$evidence/valgrind.txt" >/dev/null || fail "Valgrind found possibly lost bytes"
	grep -E 'ERROR SUMMARY: 0 errors from 0 contexts' "$evidence/valgrind.txt" >/dev/null || fail "Valgrind reported an error"
	if grep -E 'still reachable: 0 bytes in 0 blocks' "$evidence/valgrind.txt" >/dev/null; then
		printf 'still_reachable_bytes=0\nstill_reachable_blocks=0\njustification=none\n' \
			> "$evidence/valgrind-still-reachable.txt"
	else
		grep -E 'still reachable: 512 bytes in 1 blocks' "$evidence/valgrind.txt" >/dev/null ||
			fail "Valgrind found an unexpected still-reachable allocation"
		cat > "$evidence/valgrind-still-reachable.txt" <<'EOF'
still_reachable_bytes=512
still_reachable_blocks=1
justification=bounded runtime temporary-root storage remains globally reachable until process exit
EOF
	fi
else
	if test "$mode" = asan; then
		ASAN_OPTIONS=detect_leaks=1:halt_on_error=1:exitcode=97 \
		LSAN_OPTIONS=exitcode=98:report_objects=1 \
		/usr/bin/time -p /usr/bin/env TYPE_RB_NATIVE_RUNTIME_STATS=1 "$program" \
			> "$stdout" 2> "$runtime_log"
	else
		/usr/bin/time -p /usr/bin/env TYPE_RB_NATIVE_RUNTIME_STATS=1 "$program" \
			> "$stdout" 2> "$runtime_log"
	fi
	elapsed=$(awk '$1 == "real" {print $2}' "$runtime_log")
	test -n "$elapsed" || fail "runtime elapsed time is missing"
	printf 'elapsed_seconds=%s\n' "$elapsed" > "$evidence/runtime.txt"
	if test "$mode" = smoke; then
		compiler_cost_check smoke-seconds "$elapsed" "$MAX_SMOKE_SECONDS" \
			>> "$evidence/cost-observations.txt" ||
			fail "smoke runtime exceeds 5x the registered 0.85-second baseline"
	fi
	if test "$mode" = asan; then
		if grep -E 'AddressSanitizer|LeakSanitizer|detected memory leaks' "$runtime_log" >/dev/null; then
			fail "ASan/LSan reported an error"
		fi
	fi
fi

require_stdout "$phases" "$stdout"
require_collector_statistics "$runtime_log"
if test "$mode" = formal; then
	allocated=$(awk -F= '$1 == "allocated_bytes" {print $2}' "$evidence/collector-statistics.txt")
	test "$allocated" -ge "$MIN_FORMAL_ALLOCATED_BYTES" || fail "formal allocation is below 30 GiB"
fi

cp "$source" "$evidence/workload.trb"
cp "$workspace/program-first.ssa" "$evidence/program.ssa"
if test -f "$workspace/program.s"; then
	cp "$workspace/program.s" "$evidence/program.s"
fi
{
	printf 'mode=%s\n' "$mode"
	printf 'profile=%s\n' "$profile"
	printf 'phases=%s\n' "$phases"
	printf 'iterations_per_phase=%s\n' "$iterations"
	printf 'compiler_size=%s\n' "$(file_size "$compiler")"
	printf 'stripped_compiler_size=%s\n' "$stripped_compiler_size"
	printf 'compiler_sha256=%s\n' "$(sha256 "$compiler")"
	printf 'qbe_sha256=%s\n' "$(sha256 "$qbe")"
	printf 'workload_sha256=%s\n' "$(sha256 "$source")"
	printf 'qbe_output_sha256=%s\n' "$(sha256 "$workspace/program-first.ssa")"
	printf 'program_sha256=%s\n' "$(sha256 "$program")"
} > "$evidence/identities.txt"
{
	uname -a
	"$cc" --version
	"$qbe" -h
	if test "$mode" = valgrind; then
		valgrind --version
	fi
} > "$evidence/environment.txt" 2>&1

printf 'runtime-memory-soak: %s %s passed\n' "$mode" "$profile"
