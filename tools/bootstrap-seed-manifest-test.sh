#!/bin/sh

set -eu

fail() {
	printf 'bootstrap-seed-manifest-test: %s\n' "$1" >&2
	exit 1
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
manifest_tool=$script_directory/bootstrap-seed-manifest.sh
task_tmp=$(printenv TMPDIR 2>/dev/null || printf /tmp)
test_root=$(mktemp -d "$task_tmp/type-rb-native-bootstrap-manifest.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

darwin=$test_root/darwin
linux=$test_root/linux
printf 'synthetic-darwin-compiler\n' > "$darwin"
printf 'synthetic-linux-compiler\n' > "$linux"
chmod 0755 "$darwin" "$linux"

darwin_sha=$(sha256 "$darwin")
linux_sha=$(sha256 "$linux")
darwin_size=$(file_size "$darwin")
linux_size=$(file_size "$linux")

jq -n -S \
	--arg sha "$darwin_sha" \
	--argjson size "$darwin_size" \
	'{
		profile: "darwin-arm64-v0",
		os: "darwin",
		architecture: "arm64",
		runnerImage: "macos-15",
		qbeTarget: "arm64_apple",
		ccBoundary: "system-cc",
		asset: "type-rb-native-bootstrap-darwin-arm64",
		mode: "0755",
		size: $size,
		sha256: $sha,
		attestationSubjectSha256: $sha,
		qbeBinarySize: 400000,
		qbeBinarySha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}' > "$test_root/darwin.json"

jq -n -S \
	--arg sha "$linux_sha" \
	--argjson size "$linux_size" \
	'{
		profile: "linux-arm64-v0",
		os: "linux",
		architecture: "arm64",
		runnerImage: "ubuntu-24.04-arm",
		qbeTarget: "arm64",
		ccBoundary: "system-cc",
		asset: "type-rb-native-bootstrap-linux-arm64",
		mode: "0755",
		size: $size,
		sha256: $sha,
		attestationSubjectSha256: $sha,
		qbeBinarySize: 390000,
		qbeBinarySha256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}' > "$test_root/linux.json"

revision=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
release_tag=bootstrap-seed-2026-08-30
output=$test_root/release

/bin/sh "$manifest_tool" create \
	"$release_tag" "$revision" \
	"$darwin" "$linux" \
	"$test_root/darwin.json" "$test_root/linux.json" \
	"$output"

# Diagnostic preparation is deliberately ineligible for release packaging.
jq '.diagnosticOnly = true' "$test_root/darwin.json" > "$test_root/diagnostic.json"
set +e
/bin/sh "$manifest_tool" create \
	"$release_tag" "$revision" "$darwin" "$linux" \
	"$test_root/diagnostic.json" "$test_root/linux.json" "$test_root/diagnostic-release" \
	> "$test_root/diagnostic.stdout" 2> "$test_root/diagnostic.stderr"
diagnostic_status=$?
set -e
test "$diagnostic_status" -ne 0 || fail "diagnostic metadata was accepted for release"
grep -F 'target metadata shape is invalid' "$test_root/diagnostic.stderr" > /dev/null ||
	fail "diagnostic metadata did not fail closed"

manifest=$output/type-rb-native-bootstrap-manifest-v1.json
checksums=$output/SHA256SUMS
manifest_sha=$(sha256 "$manifest")
manifest_size=$(file_size "$manifest")
checksums_sha=$(sha256 "$checksums")
checksums_size=$(file_size "$checksums")

jq -n -S \
	--arg tag "$release_tag" \
	--arg darwinSha "$darwin_sha" \
	--arg linuxSha "$linux_sha" \
	--arg manifestSha "$manifest_sha" \
	--arg checksumsSha "$checksums_sha" \
	--argjson darwinSize "$darwin_size" \
	--argjson linuxSize "$linux_size" \
	--argjson manifestSize "$manifest_size" \
	--argjson checksumsSize "$checksums_size" \
	'{
		tag_name: $tag,
		draft: false,
		prerelease: true,
		immutable: true,
		assets: [
			{
				name: "type-rb-native-bootstrap-root-qbe-v1.ssa",
				size: 658639,
				digest: "sha256:62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a"
			},
			{
				name: "type-rb-native-bootstrap-darwin-arm64",
				size: $darwinSize,
				digest: ("sha256:" + $darwinSha)
			},
			{
				name: "type-rb-native-bootstrap-linux-arm64",
				size: $linuxSize,
				digest: ("sha256:" + $linuxSha)
			},
			{
				name: "type-rb-native-bootstrap-manifest-v1.json",
				size: $manifestSize,
				digest: ("sha256:" + $manifestSha)
			},
			{
				name: "SHA256SUMS",
				size: $checksumsSize,
				digest: ("sha256:" + $checksumsSha)
			}
		]
	}' > "$test_root/release.json"

/bin/sh "$manifest_tool" verify \
	"$release_tag" "$revision" \
	type-rb-native-bootstrap-darwin-arm64 \
	"$output/type-rb-native-bootstrap-darwin-arm64" \
	"$manifest" "$checksums" "$test_root/release.json"

/bin/sh "$manifest_tool" verify \
	"$release_tag" "$revision" \
	type-rb-native-bootstrap-linux-arm64 \
	"$output/type-rb-native-bootstrap-linux-arm64" \
	"$manifest" "$checksums" "$test_root/release.json"

jq '.unexpected = true' "$manifest" > "$test_root/invalid-manifest.json"
set +e
/bin/sh "$manifest_tool" verify \
	"$release_tag" "$revision" \
	type-rb-native-bootstrap-darwin-arm64 \
	"$output/type-rb-native-bootstrap-darwin-arm64" \
	"$test_root/invalid-manifest.json" "$checksums" "$test_root/release.json" \
	> "$test_root/invalid.stdout" 2> "$test_root/invalid.stderr"
invalid_status=$?
set -e
test "$invalid_status" -ne 0 || fail "unknown manifest field was accepted"

printf 'changed\n' >> "$output/type-rb-native-bootstrap-linux-arm64"
set +e
/bin/sh "$manifest_tool" verify \
	"$release_tag" "$revision" \
	type-rb-native-bootstrap-linux-arm64 \
	"$output/type-rb-native-bootstrap-linux-arm64" \
	"$manifest" "$checksums" "$test_root/release.json" \
	> "$test_root/digest.stdout" 2> "$test_root/digest.stderr"
digest_status=$?
set -e
test "$digest_status" -ne 0 || fail "changed compiler digest was accepted"

printf 'bootstrap-seed-manifest-test: passed\n'
