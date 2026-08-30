#!/usr/bin/env bash

set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT

checksum="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
for target in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
	printf '%s  trb_9.8.7_%s.tar.gz\n' "$checksum" "$target"
done > "$temporary/checksums.txt"

formula="$temporary/trb.rb"
"$root_dir/scripts/render-homebrew-formula.sh" 9.8.7 "$temporary/checksums.txt" "$formula"

grep -Fq 'releases/download/v9.8.7/trb_9.8.7_darwin_arm64.tar.gz' "$formula"
grep -Fq 'IO.puts("installed with Homebrew")' "$formula"
if grep -Fq 'io.puts("installed with Homebrew")' "$formula"; then
	echo "Homebrew smoke source uses a legacy package namespace" >&2
	exit 1
fi

project="$temporary/project"
mkdir -p "$project/src"
awk '
	$0 == "    (testpath/\"src/main.trb\").write <<~TRB" { capture = 1; next }
	capture && $0 == "    TRB" { complete = 1; exit }
	capture { sub(/^      /, ""); print }
	END { if (!capture || !complete) exit 1 }
' "$formula" > "$project/src/main.trb"

cat > "$project/trbconfig.jsonc" <<'JSON'
{
  "name": "brew-smoke-test",
  "version": "0.1.0",
  "mode": "go",
  "sourceDir": "src",
  "outDir": "build",
  "packageManagement": "external",
  "go": {
    "module": "example.com/brew-smoke-test",
    "version": "1.27",
    "rootPackage": "main"
  }
}
JSON

GOCACHE="$temporary/go-cache" go build -o "$temporary/trb" "$root_dir/cmd/trb"
"$temporary/trb" fmt "$project/src/main.trb"
"$temporary/trb" build --config "$project/trbconfig.jsonc"
grep -Fq 'fmt.Println("installed with Homebrew")' "$project/build/main.go"
