#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: $0 VERSION RELEASE_DIRECTORY OUTPUT_DIRECTORY" >&2
	exit 2
fi

version=${1#v}
release_dir=$2
output_dir=$3

case "$version" in
	""|*[!0-9A-Za-z.-]*)
		echo "invalid version: $version" >&2
		exit 2
		;;
esac

for arch in amd64 arm64; do
	archive="$release_dir/trb_${version}_linux_${arch}.tar.gz"
	destination="$output_dir/linux/$arch"
	if [ ! -f "$archive" ]; then
		echo "release archive not found: $archive" >&2
		exit 1
	fi
	mkdir -p "$destination"
	tar -xzf "$archive" -C "$destination" trb
	chmod 0755 "$destination/trb"
done

printf 'container root files written to %s\n' "$output_dir"
