#!/bin/sh

set -eu

fail() {
	printf 'benchmarksgame-cache-control: %s\n' "$1" >&2
	exit 1
}

test "$#" -eq 1 || fail "one evidence path is required"
evidence=$1
test "$(uname -s)" = Linux || fail "Linux is required"
test -r /proc/meminfo || fail "/proc/meminfo is unavailable"
test -r /proc/swaps || fail "/proc/swaps is unavailable"
test "$(awk 'NR > 1 && NF > 0 { count += 1 } END { print count + 0 }' /proc/swaps)" -eq 0 ||
	fail "swap must be disabled before formal measurement"
test -x /usr/bin/sudo || fail "/usr/bin/sudo is required"
test -x /usr/bin/sync || fail "/usr/bin/sync is required"
test ! -e "$evidence" || fail "cache evidence path already exists"

{
	printf 'swap_devices=0\n'
	awk '$1 == "MemAvailable:" { print "mem_available_before_kib=" $2 }' /proc/meminfo
	awk '$1 == "Cached:" { print "cached_before_kib=" $2 }' /proc/meminfo
} > "$evidence"

/usr/bin/sync
/usr/bin/sudo -n /bin/sh -c 'printf 3 > /proc/sys/vm/drop_caches'

{
	awk '$1 == "MemAvailable:" { print "mem_available_after_kib=" $2 }' /proc/meminfo
	awk '$1 == "Cached:" { print "cached_after_kib=" $2 }' /proc/meminfo
} >> "$evidence"
