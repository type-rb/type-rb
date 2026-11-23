#!/bin/sh

# Source this file from the controller checkout. Resolve each source checkout
# independently; the historical alternative is not a production compatibility
# alias. Incomplete or duplicate projects must not silently select a baseline.
native_compiler_project_directory() (
	if test "$#" -ne 1 || test ! -d "$1"; then
		printf '%s\n' 'compiler-project: expected one existing repository root' >&2
		exit 1
	fi
	project=$1/compiler
	if test -e "$project/gate4" || test -L "$project/gate4"; then
		if test -e "$project/src" || test -L "$project/src" ||
			test -e "$project/trbconfig.jsonc" || test -L "$project/trbconfig.jsonc"; then
			printf '%s\n' 'compiler-project: ambiguous current and historical layouts' >&2
			exit 1
		fi
		project=$project/gate4
	fi
	if test ! -f "$project/src/compiler.trb" || test ! -f "$project/trbconfig.jsonc"; then
		printf '%s\n' 'compiler-project: incomplete compiler project' >&2
		exit 1
	fi
	printf '%s\n' "$project"
)

# Recovery suites must use the configuration belonging to the source checkout.
native_recovery_project_config() (
	if test "$#" -ne 1 || test ! -d "$1"; then
		printf '%s\n' 'recovery-project: expected one existing repository root' >&2
		exit 1
	fi
	current=$1/recovery
	historical=$1/trbconfig.reference.jsonc
	if test -e "$current" || test -L "$current"; then
		if test -e "$historical" || test -L "$historical" ||
			test ! -f "$current/trbconfig.jsonc" || test ! -f "$current/src/driver/main.trb"; then
			printf '%s\n' 'recovery-project: ambiguous or incomplete current layout' >&2
			exit 1
		fi
		printf '%s\n' "$current/trbconfig.jsonc"
	else
		if test ! -f "$historical" || test ! -f "$1/src/recovery_driver.trb"; then
			printf '%s\n' 'recovery-project: incomplete historical layout' >&2
			exit 1
		fi
		printf '%s\n' "$historical"
	fi
)

# Bootstrap validation must use the fixture from the same frozen checkout as
# the compiler, including its original location before the corpus rename.
native_configured_fixture_directory() (
	if test "$#" -ne 1 || test ! -d "$1"; then
		printf '%s\n' 'configured-fixture: expected one existing repository root' >&2
		exit 1
	fi
	current=$1/corpus/configured-project
	historical=$1/corpus/gate6k
	if test -e "$current" || test -L "$current"; then
		if test -e "$historical" || test -L "$historical"; then
			printf '%s\n' 'configured-fixture: ambiguous current and historical layouts' >&2
			exit 1
		fi
		fixture=$current/configured-project
	else
		fixture=$historical/configured-project
	fi
	if test ! -f "$fixture/trbconfig.jsonc"; then
		printf '%s\n' 'configured-fixture: incomplete configured project' >&2
		exit 1
	fi
	printf '%s\n' "$fixture"
)
