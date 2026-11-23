#!/bin/sh

set -eu

fail() {
	printf 'benchmarksgame-cpuset-delegation: %s\n' "$1" >&2
	exit 1
}

test "$#" -eq 1 || fail "one evidence path is required"
evidence=$1
test "$(uname -s)" = Linux || fail "Linux is required"
test -x /usr/bin/id || fail "/usr/bin/id is required"
test -x /usr/bin/sudo || fail "/usr/bin/sudo is required"
test -x /usr/bin/tee || fail "/usr/bin/tee is required"
test -d "$(dirname -- "$evidence")" || fail "evidence parent does not exist"
test ! -e "$evidence" || fail "evidence path already exists"
test ! -e "$evidence.sudo.stdout" || fail "sudo stdout evidence already exists"
test ! -e "$evidence.sudo.stderr" || fail "sudo stderr evidence already exists"

user_id=$(/usr/bin/id -u)
case "$user_id" in
'' | *[!0-9]*) fail "current user ID is not numeric" ;;
esac

user_service=/sys/fs/cgroup/user.slice/user-$user_id.slice/user@$user_id.service
{
	printf 'user_id=%s\n' "$user_id"
	printf 'user_service=%s\n' "$user_service"
	printf 'current_cgroup=%s\n' "$(awk -F: '$1 == 0 { print $3 }' /proc/self/cgroup)"
} > "$evidence"

: > "$evidence.sudo.stdout"
: > "$evidence.sudo.stderr"

enable_cpuset() {
	label=$1
	cgroup_path=$2
	controllers=$cgroup_path/cgroup.controllers
	subtree_control=$cgroup_path/cgroup.subtree_control
	test -r "$controllers" || fail "$label cgroup controllers are unavailable"
	test -r "$subtree_control" || fail "$label subtree control is unavailable"
	grep -qw cpuset "$controllers" || fail "cpuset is not available to $label"
	{
		printf '%s_path=%s\n' "$label" "$cgroup_path"
		printf '%s_available_controllers=%s\n' "$label" "$(cat "$controllers")"
		printf '%s_subtree_control_before=%s\n' "$label" "$(cat "$subtree_control")"
	} >> "$evidence"
	if ! grep -qw cpuset "$subtree_control"; then
		set +e
		printf '+cpuset\n' |
			/usr/bin/sudo -n /usr/bin/tee "$subtree_control" >> "$evidence.sudo.stdout" 2>> "$evidence.sudo.stderr"
		delegation_status=$?
		set -e
		test "$delegation_status" -eq 0 || fail "could not delegate cpuset to $label"
	fi
	printf '%s_subtree_control_after=%s\n' "$label" "$(cat "$subtree_control")" >> "$evidence"
	grep -qw cpuset "$subtree_control" || fail "cpuset remains unavailable to $label"
}

enable_cpuset user_service "$user_service"
benchexec_slice=$user_service/benchexec.slice
if test -d "$benchexec_slice"; then
	printf 'benchexec_slice_present=true\n' >> "$evidence"
	enable_cpuset benchexec_slice "$benchexec_slice"
else
	printf 'benchexec_slice_present=false\n' >> "$evidence"
fi

printf 'benchmarksgame-cpuset-delegation: cpuset is delegated\n'
