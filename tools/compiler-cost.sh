#!/bin/sh

# Strict is the standalone/default contract. Integration explicitly selects the
# temporary MIR migration policy; an exceeded observation is never called pass.
compiler_cost_mode() {
	case "${NATIVE_MIR_COST_MODE-strict}" in
	strict | mir-migration) printf '%s\n' "${NATIVE_MIR_COST_MODE-strict}" ;;
	*) printf '%s\n' 'invalid compiler cost mode' >&2; return 64 ;;
	esac
}

compiler_cost_check() (
	test "$#" -eq 3 || exit 64
	cost_metric=$1
	cost_actual=$2
	cost_limit=$3
	cost_mode=$(compiler_cost_mode) || exit 64
	awk -v actual="$cost_actual" -v limit="$cost_limit" 'BEGIN {
		number = "^[0-9]+([.][0-9]+)?$"
		exit !(actual ~ number && limit ~ number && actual >= 0 && limit > 0)
	}' || { printf '%s\n' 'invalid compiler cost observation' >&2; exit 64; }
	cost_status=within-limit
	if ! awk -v actual="$cost_actual" -v limit="$cost_limit" 'BEGIN {exit !(actual <= limit)}'; then
		cost_status=exceeded
	fi
	printf 'metric=%s mode=%s actual=%s limit=%s status=%s\n' \
		"$cost_metric" "$cost_mode" "$cost_actual" "$cost_limit" "$cost_status"
	test "$cost_status" = within-limit || test "$cost_mode" = mir-migration
)

# Retired transition markers no longer set compiler size limits. Size remains
# retained evidence; daily measurements track its trend.
compiler_cost_observe() (
	test "$#" -eq 2 || exit 64
	awk -v actual="$2" 'BEGIN { exit !(actual ~ "^[0-9]+([.][0-9]+)?$") }' ||
		{ printf '%s\n' 'invalid compiler cost observation' >&2; exit 64; }
	printf 'metric=%s actual=%s status=observed\n' "$1" "$2"
)
