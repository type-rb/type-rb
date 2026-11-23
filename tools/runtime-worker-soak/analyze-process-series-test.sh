#!/bin/sh

set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
analyzer=$script_directory/analyze-process-series.awk
temporary=$(mktemp -d "${TMPDIR:-/tmp}/runtime-worker-process-series-test.XXXXXX")
trap 'rm -rf "$temporary"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

printf '%s\n' \
	'elapsed_seconds,rss_bytes,completed_phases,fd_count,thread_count' \
	'0.00,10485760,0,3,1' \
	'0.25,12582912,6,3,1' \
	'0.50,12582912,7,3,1' \
	'0.75,12582912,8,3,1' \
	'1.00,12582912,9,3,1' \
	'1.25,12582912,10,3,1' \
	'1.50,12582912,11,3,1' \
	'1.75,12582912,12,3,1' \
	'2.00,12582912,13,3,1' \
	> "$temporary/valid.csv"
awk -v enforce_native=1 -f "$analyzer" "$temporary/valid.csv" > "$temporary/valid.txt"
grep -F 'post_warmup_sample_count=8' "$temporary/valid.txt" > /dev/null
grep -F 'maximum_fd_count=3' "$temporary/valid.txt" > /dev/null

sed 's/2.00,12582912,13,3,1/2.00,12582912,13,4,1/' "$temporary/valid.csv" > "$temporary/fd-growth.csv"
if awk -v enforce_native=1 -f "$analyzer" "$temporary/fd-growth.csv" > /dev/null 2>&1; then
	printf 'descriptor-growth mutation unexpectedly passed\n' >&2
	exit 1
fi

sed 's/elapsed_seconds,rss_bytes,completed_phases,fd_count,thread_count/elapsed_seconds,rss_bytes,completed_phases/' "$temporary/valid.csv" > "$temporary/header.csv"
if awk -v enforce_native=1 -f "$analyzer" "$temporary/header.csv" > /dev/null 2>&1; then
	printf 'header mutation unexpectedly passed\n' >&2
	exit 1
fi

sed 's/2.00,12582912,13,3,1/2.00,12582912,13,0,1/' "$temporary/valid.csv" > "$temporary/exit-race.csv"
if awk -v enforce_native=1 -f "$analyzer" "$temporary/exit-race.csv" > /dev/null 2>&1; then
	printf 'exit-boundary mutation unexpectedly passed\n' >&2
	exit 1
fi

printf 'analyze-process-series-test: passed\n'
