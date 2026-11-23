#!/bin/sh
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "usage: $0 VERSION OUTPUT_DIRECTORY [trb|trbg]" >&2
  exit 2
fi

version=${1#v}
output_dir=$2
command_name=${3:-trb}
case "$command_name" in
  trb|trbg) ;;
  *)
    echo "invalid release command: $command_name" >&2
    exit 2
    ;;
esac
case "$version" in
  ""|*[!0-9A-Za-z.-]*)
    echo "invalid version: $version" >&2
    exit 2
    ;;
esac

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
module_path=$(cd "$repository_root" && go list -m -f '{{.Path}}')
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
staging_root=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-release.XXXXXX")
trap 'rm -rf "$staging_root"' EXIT HUP INT TERM

for target in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
  target_os=${target%_*}
  target_arch=${target#*_}
  staging_dir="$staging_root/$target"
  target_cache=${GOCACHE:-$staging_root/go-cache-$target}
  archive="${command_name}_${version}_${target}.tar.gz"
  mkdir -p "$staging_dir" "$target_cache"
  (
    cd "$repository_root"
    CGO_ENABLED=0 GOCACHE="$target_cache" GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X $module_path/internal/cli.Version=$version" \
      -o "$staging_dir/$command_name" "./cmd/$command_name"
  )
  cp "$repository_root/LICENSE" "$staging_dir/LICENSE"
  tar -cf - -C "$staging_dir" "$command_name" LICENSE | gzip -n > "$output_dir/$archive"
done

(
  cd "$output_dir"
  for archive in \
    "${command_name}_${version}_darwin_arm64.tar.gz" \
    "${command_name}_${version}_darwin_amd64.tar.gz" \
    "${command_name}_${version}_linux_arm64.tar.gz" \
    "${command_name}_${version}_linux_amd64.tar.gz"; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$archive"
    else
      shasum -a 256 "$archive"
    fi
  done > checksums.txt
)

if [ "$command_name" = trb ]; then
  "$repository_root/scripts/render-homebrew-formula.sh" \
    "$version" "$output_dir/checksums.txt" "$output_dir/trb.rb"
fi

printf 'release artifacts written to %s\n' "$output_dir"
