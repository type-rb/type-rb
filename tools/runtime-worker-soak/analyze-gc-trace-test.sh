#!/bin/sh

set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
analyzer=$script_directory/analyze-gc-trace.awk
temporary=$(mktemp -d "${TMPDIR:-/tmp}/runtime-worker-gc-trace-test.XXXXXX")
trap 'rm -rf "$temporary"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

printf '%s\n' \
	'type-rb-native-gc-trace-v1,collection,64' \
	'type-rb-native-gc-trace-v1,automatic,1' \
	'type-rb-native-gc-trace-v1,live-bytes,4096' \
	'type-rb-native-gc-trace-v1,next-target-bytes,1048576' \
	'type-rb-native-gc-trace-v1,root-count,9' \
	'type-rb-native-gc-trace-v1,root-capacity,64' \
	'type-rb-native-gc-trace-v1,collection,91' \
	'type-rb-native-gc-trace-v1,automatic,0' \
	'type-rb-native-gc-trace-v1,live-bytes,0' \
	'type-rb-native-gc-trace-v1,next-target-bytes,1048576' \
	'type-rb-native-gc-trace-v1,root-count,0' \
	'type-rb-native-gc-trace-v1,root-capacity,64' \
	'type-rb-native-gc-stat-v1,collections,91' \
	> "$temporary/valid.log"
awk -v minimum_observations=2 -f "$analyzer" "$temporary/valid.log" > "$temporary/valid.txt"
grep -F 'observation_count=2' "$temporary/valid.txt" > /dev/null
grep -F 'maximum_sampled_live_bytes=4096' "$temporary/valid.txt" > /dev/null

sed 's/root-capacity,64/root-capacity,128/' "$temporary/valid.log" > "$temporary/capacity.log"
if awk -v minimum_observations=2 -f "$analyzer" "$temporary/capacity.log" > /dev/null 2>&1; then
	printf 'capacity mutation unexpectedly passed\n' >&2
	exit 1
fi

sed '6d' "$temporary/valid.log" > "$temporary/incomplete.log"
if awk -v minimum_observations=2 -f "$analyzer" "$temporary/incomplete.log" > /dev/null 2>&1; then
	printf 'incomplete observation unexpectedly passed\n' >&2
	exit 1
fi

sed 's/collection,64/collection,65/' "$temporary/valid.log" > "$temporary/interval.log"
if awk -v minimum_observations=2 -f "$analyzer" "$temporary/interval.log" > /dev/null 2>&1; then
	printf 'sampling-interval mutation unexpectedly passed\n' >&2
	exit 1
fi

printf 'analyze-gc-trace-test: passed\n'
