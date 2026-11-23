#!/bin/sh
set -eu
script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_directory/compiler-cost.sh"
unset NATIVE_MIR_COST_MODE
test "$(compiler_cost_mode)" = strict
test "$(compiler_cost_check compiler-bytes 417000 417000)" = \
	'metric=compiler-bytes mode=strict actual=417000 limit=417000 status=within-limit'
if compiler_cost_check compiler-bytes 417001 417000; then
	exit 1
fi
NATIVE_MIR_COST_MODE=mir-migration
export NATIVE_MIR_COST_MODE
test "$(compiler_cost_check compiler-bytes 432048 417000)" = \
	'metric=compiler-bytes mode=mir-migration actual=432048 limit=417000 status=exceeded'
test "$(compiler_cost_check smoke-seconds 2.26 2.25)" = \
	'metric=smoke-seconds mode=mir-migration actual=2.26 limit=2.25 status=exceeded'
# Invalid measurements and modes are errors even when costs are advisory.
for invalid in '' nan inf -1 1e3 10x; do
	if compiler_cost_check compiler-bytes "$invalid" 417000; then exit 1; fi
	if compiler_cost_check compiler-bytes 1 "$invalid"; then exit 1; fi
done
if compiler_cost_check compiler-bytes 1 0; then exit 1; fi
for invalid in '' migration STRICT; do
	NATIVE_MIR_COST_MODE=$invalid
	if compiler_cost_mode; then exit 1; fi
	if compiler_cost_check compiler-bytes 1 417000; then exit 1; fi
done
NATIVE_MIR_COST_MODE=strict
if compiler_cost_check compiler-bytes 432048 417000; then exit 1; fi
# A check must not overwrite the caller's own mode, metric or measurements.
cost_actual=keep
compiler_cost_check compiler-bytes 1 417000 > /dev/null
test "$cost_actual" = keep
test "$(compiler_cost_observe compiler-bytes 2133296)" = \
	'metric=compiler-bytes actual=2133296 status=observed'
for invalid in '' -1 abc; do
	if compiler_cost_observe compiler-bytes "$invalid" 2>/dev/null; then exit 1; fi
done
if compiler_cost_observe compiler-bytes 2>/dev/null; then exit 1; fi
printf '%s\n' 'Compiler cost modes passed'
