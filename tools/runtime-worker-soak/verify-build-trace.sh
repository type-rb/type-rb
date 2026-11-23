#!/bin/sh

set -eu

usage() {
	printf '%s\n' 'usage: verify-build-trace.sh TRACE COMPILER QBE CC setup|ordinary' >&2
	exit 64
}

fail() {
	printf 'verify-build-trace: %s\n' "$1" >&2
	exit 1
}

resolve_cc_program() {
	program=$("$cc" -print-prog-name="$1")
	case "$program" in
	/*) printf '%s\n' "$program" ;;
	*) command -v "$program" ;;
	esac
}

require_observed() {
	program=$1
	label=$2
	grep -Fx "$program" "$inventory" > /dev/null ||
		fail "$label was not observed at $program"
}

test "$#" -eq 5 || usage

trace=$1
compiler=$2
qbe=$3
cc=$4
role=$5
inventory=$trace.executables

test "$role" = setup || test "$role" = ordinary || usage

test "$(uname -s)" = Linux || fail "Linux is required"
for command_name in awk file grep ld.lld; do
	command -v "$command_name" > /dev/null 2>&1 || fail "$command_name is required"
done
test -s "$trace" || fail "process trace is empty"
for program in "$compiler" "$qbe" "$cc"; do
	test -x "$program" || fail "$program is not executable"
	file -L "$program" | grep -F 'ELF ' > /dev/null ||
		fail "$program is not a direct ELF executable"
done

collect2=$(resolve_cc_program collect2)
assembler=$(resolve_cc_program as)
system_assembler=$(command -v as)
system_linker=$(resolve_cc_program ld)
linker=$(command -v ld.lld)

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
			if ($0 ~ /= 0$/ && pid in pending) print pending[pid]
			delete pending[pid]
		}
	}
' "$trace" > "$inventory"
test -s "$inventory" || fail "successful process inventory is empty"

while IFS= read -r executable; do
	case "$executable" in
	"$compiler" | "$qbe" | "$cc" | "$collect2" | "$assembler" | \
		"$system_assembler" | "$linker") ;;
	"$system_linker")
		test "$role" = setup ||
			fail "ordinary build launched the system linker: $executable"
		;;
	*) fail "unregistered executable: $executable" ;;
	esac
done < "$inventory"

require_observed "$compiler" compiler
require_observed "$qbe" QBE
require_observed "$cc" "C driver"
require_observed "$collect2" collect2
if ! grep -Fx "$assembler" "$inventory" > /dev/null &&
	! grep -Fx "$system_assembler" "$inventory" > /dev/null; then
	fail "assembler was not observed at a registered path"
fi
if test "$role" = ordinary; then
	require_observed "$linker" LLD
elif ! grep -Fx "$linker" "$inventory" > /dev/null &&
	! grep -Fx "$system_linker" "$inventory" > /dev/null; then
	fail "setup linker was not observed at a registered path"
fi

if grep -E 'execve\("[^"]*/(go|trb|sh|bash|dash|zsh)"|compiler-recovery' \
	"$trace" > /dev/null; then
	fail "trace contains Go, reference trb, a shell, or a recovery generator"
fi
if grep -F -- '--source-content' "$trace" > /dev/null; then
	fail "trace used the hidden source-content adapter"
fi
