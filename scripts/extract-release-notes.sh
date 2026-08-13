#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 3 ]; then
	echo "usage: $0 VERSION CHANGELOG OUTPUT" >&2
	exit 2
fi

version="$1"
changelog="$2"
output="$3"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "release version is not a semantic version: $version" >&2
	exit 1
fi
if [ ! -f "$changelog" ]; then
	echo "changelog not found: $changelog" >&2
	exit 1
fi

temporary="$(mktemp)"
trap 'rm -f "$temporary"' EXIT
found=false
expected_prefix="## ${version} - "

while IFS= read -r line || [ -n "$line" ]; do
	if [[ "$line" == "## "* ]]; then
		if [ "$found" = true ]; then
			break
		fi
		if [[ "$line" == "$expected_prefix"* ]]; then
			date="${line#"$expected_prefix"}"
			if [[ ! "$date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
				echo "invalid changelog date for $version: $date" >&2
				exit 1
			fi
			found=true
		fi
		continue
	fi
	if [ "$found" = true ]; then
		printf '%s\n' "$line" >> "$temporary"
	fi
done < "$changelog"

if [ "$found" = false ]; then
	echo "CHANGELOG.md has no release section for $version" >&2
	exit 1
fi
if ! grep -q '[^[:space:]]' "$temporary"; then
	echo "CHANGELOG.md release section for $version is empty" >&2
	exit 1
fi

mkdir -p "$(dirname "$output")"
mv "$temporary" "$output"
trap - EXIT
