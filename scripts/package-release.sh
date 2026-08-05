#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 VERSION OUTPUT_DIRECTORY" >&2
  exit 2
fi

version=${1#v}
output_dir=$2
case "$version" in
  ""|*[!0-9A-Za-z.-]*)
    echo "invalid version: $version" >&2
    exit 2
    ;;
esac

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
staging_root=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-release.XXXXXX")
trap 'rm -rf "$staging_root"' EXIT HUP INT TERM

for target in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
  target_os=${target%_*}
  target_arch=${target#*_}
  staging_dir="$staging_root/$target"
  archive="trb_${version}_${target}.tar.gz"
  mkdir -p "$staging_dir"
  (
    cd "$repository_root"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X github.com/type-rb/type-rb/internal/cli.Version=$version" \
      -o "$staging_dir/trb" ./cmd/trb
  )
  cp "$repository_root/LICENSE" "$staging_dir/LICENSE"
  tar -cf - -C "$staging_dir" trb LICENSE | gzip -n > "$output_dir/$archive"
done

(
  cd "$output_dir"
  for archive in \
    "trb_${version}_darwin_arm64.tar.gz" \
    "trb_${version}_darwin_amd64.tar.gz" \
    "trb_${version}_linux_arm64.tar.gz" \
    "trb_${version}_linux_amd64.tar.gz"; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$archive"
    else
      shasum -a 256 "$archive"
    fi
  done > checksums.txt
)

"$repository_root/scripts/render-homebrew-formula.sh" \
  "$version" "$output_dir/checksums.txt" "$output_dir/trb.rb"

printf 'release artifacts written to %s\n' "$output_dir"
