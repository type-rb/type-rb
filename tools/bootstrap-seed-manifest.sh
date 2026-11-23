#!/bin/sh

set -eu

RELEASE_TAG=bootstrap-seed-2026-08-30
ROOT_ASSET=type-rb-native-bootstrap-root-qbe-v1.ssa
ROOT_KIND=configured-project-fixed-point-qbe
ROOT_SIZE=658639
ROOT_SHA256=62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a
DARWIN_ASSET=type-rb-native-bootstrap-darwin-arm64
LINUX_ASSET=type-rb-native-bootstrap-linux-arm64
MANIFEST_ASSET=type-rb-native-bootstrap-manifest-v1.json
CHECKSUM_ASSET=SHA256SUMS
QBE_SOURCE_URL=https://c9x.me/compile/release/qbe-1.3.tar.xz
QBE_SOURCE_SHA256=d587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320
MAX_COMPILER_SIZE=310000
MAX_COMBINED_COMPILER_SIZE=620000

usage() {
	cat >&2 <<'EOF'
usage:
  bootstrap-seed-manifest.sh create RELEASE_TAG NATIVE_REVISION \
    DARWIN_COMPILER LINUX_COMPILER DARWIN_METADATA LINUX_METADATA OUTPUT_DIRECTORY
  bootstrap-seed-manifest.sh verify RELEASE_TAG NATIVE_REVISION TARGET_ASSET \
    TARGET_COMPILER MANIFEST SHA256SUMS RELEASE_JSON
EOF
	exit 64
}

fail() {
	printf 'bootstrap-seed-manifest: %s\n' "$1" >&2
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

validate_revision() {
	printf '%s\n' "$1" | grep -E '^[0-9a-f]{40}$' >/dev/null ||
		fail "Native revision must be a full lowercase Git SHA"
}

validate_metadata_shape() {
	metadata=$1
	jq -e '
		type == "object" and
		(keys == [
			"architecture",
			"asset",
			"attestationSubjectSha256",
			"ccBoundary",
			"mode",
			"os",
			"profile",
			"qbeBinarySha256",
			"qbeBinarySize",
			"qbeTarget",
			"runnerImage",
			"sha256",
			"size"
		]) and
		(.architecture | type == "string") and
		(.asset | type == "string") and
		(.attestationSubjectSha256 | test("^[0-9a-f]{64}$")) and
		(.ccBoundary | type == "string") and
		(.mode | type == "string") and
		(.os | type == "string") and
		(.profile | type == "string") and
		(.qbeBinarySha256 | test("^[0-9a-f]{64}$")) and
		(.qbeBinarySize | type == "number" and . > 0 and floor == .) and
		(.qbeTarget | type == "string") and
		(.runnerImage | type == "string") and
		(.sha256 | test("^[0-9a-f]{64}$")) and
		(.size | type == "number" and . > 0 and floor == .) and
		(.attestationSubjectSha256 == .sha256)
	' "$metadata" >/dev/null || fail "target metadata shape is invalid"
}

validate_target_metadata() {
	metadata=$1
	compiler=$2
	expected_profile=$3
	expected_os=$4
	expected_runner=$5
	expected_qbe_target=$6
	expected_asset=$7

	validate_metadata_shape "$metadata"
	jq -e \
		--arg profile "$expected_profile" \
		--arg os "$expected_os" \
		--arg runner "$expected_runner" \
		--arg qbeTarget "$expected_qbe_target" \
		--arg asset "$expected_asset" \
		--arg mode 0755 \
		'
			.profile == $profile and
			.os == $os and
			.architecture == "arm64" and
			.runnerImage == $runner and
			.qbeTarget == $qbeTarget and
			.ccBoundary == "system-cc" and
			.asset == $asset and
			.mode == $mode
		' "$metadata" >/dev/null || fail "$expected_asset metadata values are invalid"

	expected_size=$(jq -r '.size' "$metadata")
	expected_sha256=$(jq -r '.sha256' "$metadata")
	test "$(file_size "$compiler")" -eq "$expected_size" ||
		fail "$expected_asset size does not match metadata"
	test "$(sha256 "$compiler")" = "$expected_sha256" ||
		fail "$expected_asset digest does not match metadata"
	test "$expected_size" -le "$MAX_COMPILER_SIZE" ||
		fail "$expected_asset exceeds the size bound"
}

checksum_entry() {
	name=$1
	checksums=$2
	count=$(awk -v wanted="$name" '$2 == wanted { count += 1 } END { print count + 0 }' "$checksums")
	test "$count" -eq 1 || fail "checksum entry count differs for $name"
	awk -v wanted="$name" '$2 == wanted { print $1 }' "$checksums"
}

create_manifest() {
	test "$#" -eq 7 || usage
	release_tag=$1
	native_revision=$2
	darwin_compiler=$3
	linux_compiler=$4
	darwin_metadata=$5
	linux_metadata=$6
	output_directory=$7

	test "$release_tag" = "$RELEASE_TAG" || fail "unexpected release tag"
	validate_revision "$native_revision"
	for path in "$darwin_compiler" "$linux_compiler" "$darwin_metadata" "$linux_metadata"; do
		test -f "$path" || fail "required input is missing: $path"
	done
	test ! -e "$output_directory" || fail "output directory already exists"
	command -v jq >/dev/null 2>&1 || fail "jq is required"

	validate_target_metadata \
		"$darwin_metadata" "$darwin_compiler" \
		darwin-arm64-v0 darwin macos-15 arm64_apple "$DARWIN_ASSET"
	validate_target_metadata \
		"$linux_metadata" "$linux_compiler" \
		linux-arm64-v0 linux ubuntu-24.04-arm arm64 "$LINUX_ASSET"

	darwin_size=$(jq -r '.size' "$darwin_metadata")
	linux_size=$(jq -r '.size' "$linux_metadata")
	test $((darwin_size + linux_size)) -le "$MAX_COMBINED_COMPILER_SIZE" ||
		fail "combined compiler assets exceed the size bound"

	mkdir -p "$output_directory"
	cp "$darwin_compiler" "$output_directory/$DARWIN_ASSET"
	cp "$linux_compiler" "$output_directory/$LINUX_ASSET"
	chmod 0755 "$output_directory/$DARWIN_ASSET" "$output_directory/$LINUX_ASSET"

	jq -n -S \
		--arg releaseTag "$release_tag" \
		--arg nativeRevision "$native_revision" \
		--arg rootAsset "$ROOT_ASSET" \
		--arg rootKind "$ROOT_KIND" \
		--arg rootSha256 "$ROOT_SHA256" \
		--arg qbeSourceUrl "$QBE_SOURCE_URL" \
		--arg qbeSourceSha256 "$QBE_SOURCE_SHA256" \
		--argjson rootSize "$ROOT_SIZE" \
		--slurpfile darwin "$darwin_metadata" \
		--slurpfile linux "$linux_metadata" \
		'{
			schemaVersion: 1,
			status: "experimental",
			releaseTag: $releaseTag,
			nativeRevision: $nativeRevision,
			root: {
				kind: $rootKind,
				asset: $rootAsset,
				size: $rootSize,
				sha256: $rootSha256
			},
			backend: {
				name: "QBE",
				version: "1.3",
				sourceUrl: $qbeSourceUrl,
				sourceSha256: $qbeSourceSha256,
				builds: [
					{
						runnerImage: $darwin[0].runnerImage,
						binarySize: $darwin[0].qbeBinarySize,
						binarySha256: $darwin[0].qbeBinarySha256
					},
					{
						runnerImage: $linux[0].runnerImage,
						binarySize: $linux[0].qbeBinarySize,
						binarySha256: $linux[0].qbeBinarySha256
					}
				]
			},
			targets: [
				($darwin[0] | del(.qbeBinarySize, .qbeBinarySha256)),
				($linux[0] | del(.qbeBinarySize, .qbeBinarySha256))
			]
		}' > "$output_directory/$MANIFEST_ASSET"

	darwin_sha256=$(sha256 "$output_directory/$DARWIN_ASSET")
	linux_sha256=$(sha256 "$output_directory/$LINUX_ASSET")
	manifest_sha256=$(sha256 "$output_directory/$MANIFEST_ASSET")
	cat > "$output_directory/$CHECKSUM_ASSET" <<EOF
$ROOT_SHA256  $ROOT_ASSET
$darwin_sha256  $DARWIN_ASSET
$linux_sha256  $LINUX_ASSET
$manifest_sha256  $MANIFEST_ASSET
EOF
	chmod 0644 "$output_directory/$MANIFEST_ASSET" "$output_directory/$CHECKSUM_ASSET"
	printf 'bootstrap-seed-manifest: created %s\n' "$output_directory"
}

validate_manifest_shape() {
	manifest=$1
	jq -e \
		--arg releaseTag "$RELEASE_TAG" \
		--arg rootAsset "$ROOT_ASSET" \
		--arg rootKind "$ROOT_KIND" \
		--arg rootSha256 "$ROOT_SHA256" \
		--arg qbeSourceUrl "$QBE_SOURCE_URL" \
		--arg qbeSourceSha256 "$QBE_SOURCE_SHA256" \
		--arg darwinAsset "$DARWIN_ASSET" \
		--arg linuxAsset "$LINUX_ASSET" \
		--argjson rootSize "$ROOT_SIZE" \
		--argjson maxCompilerSize "$MAX_COMPILER_SIZE" \
		--argjson maxCombinedSize "$MAX_COMBINED_COMPILER_SIZE" \
		'
		def sha256: type == "string" and test("^[0-9a-f]{64}$");
		def exact_target_keys:
			keys == [
				"architecture",
				"asset",
				"attestationSubjectSha256",
				"ccBoundary",
				"mode",
				"os",
				"profile",
				"qbeTarget",
				"runnerImage",
				"sha256",
				"size"
			];
		type == "object" and
		(keys == [
			"backend",
			"nativeRevision",
			"releaseTag",
			"root",
			"schemaVersion",
			"status",
			"targets"
		]) and
		.schemaVersion == 1 and
		.status == "experimental" and
		.releaseTag == $releaseTag and
		(.nativeRevision | test("^[0-9a-f]{40}$")) and
		(.root | type == "object") and
		(.root | keys == ["asset", "kind", "sha256", "size"]) and
		.root.kind == $rootKind and
		.root.asset == $rootAsset and
		.root.size == $rootSize and
		.root.sha256 == $rootSha256 and
		(.backend | type == "object") and
		(.backend | keys == ["builds", "name", "sourceSha256", "sourceUrl", "version"]) and
		.backend.name == "QBE" and
		.backend.version == "1.3" and
		.backend.sourceUrl == $qbeSourceUrl and
		.backend.sourceSha256 == $qbeSourceSha256 and
		(.backend.builds | type == "array" and length == 2) and
		([.backend.builds[].runnerImage] == ["macos-15", "ubuntu-24.04-arm"]) and
		(all(.backend.builds[];
			type == "object" and
			(keys == ["binarySha256", "binarySize", "runnerImage"]) and
			(.binarySha256 | sha256) and
			(.binarySize | type == "number" and . > 0 and floor == .)
		)) and
		(.targets | type == "array" and length == 2) and
		(all(.targets[];
			type == "object" and
			exact_target_keys and
			.architecture == "arm64" and
			.ccBoundary == "system-cc" and
			.mode == "0755" and
			(.sha256 | sha256) and
			(.attestationSubjectSha256 == .sha256) and
			(.size | type == "number" and . > 0 and floor == . and . <= $maxCompilerSize)
		)) and
		(.targets[0].profile == "darwin-arm64-v0") and
		(.targets[0].os == "darwin") and
		(.targets[0].runnerImage == "macos-15") and
		(.targets[0].qbeTarget == "arm64_apple") and
		(.targets[0].asset == $darwinAsset) and
		(.targets[1].profile == "linux-arm64-v0") and
		(.targets[1].os == "linux") and
		(.targets[1].runnerImage == "ubuntu-24.04-arm") and
		(.targets[1].qbeTarget == "arm64") and
		(.targets[1].asset == $linuxAsset) and
		((.targets[0].size + .targets[1].size) <= $maxCombinedSize)
		' "$manifest" >/dev/null || fail "manifest shape or fixed values are invalid"
}

verify_release_asset() {
	release_json=$1
	name=$2
	size=$3
	digest=$4
	jq -e \
		--arg name "$name" \
		--arg digest "sha256:$digest" \
		--argjson size "$size" \
		'
			[.assets[] | select(.name == $name and .size == $size and .digest == $digest)]
			| length == 1
		' "$release_json" >/dev/null || fail "release asset metadata differs for $name"
}

verify_manifest() {
	test "$#" -eq 7 || usage
	release_tag=$1
	native_revision=$2
	target_asset=$3
	target_compiler=$4
	manifest=$5
	checksums=$6
	release_json=$7

	test "$release_tag" = "$RELEASE_TAG" || fail "unexpected release tag"
	validate_revision "$native_revision"
	for path in "$target_compiler" "$manifest" "$checksums" "$release_json"; do
		test -f "$path" || fail "required input is missing: $path"
	done
	case "$target_asset" in
	"$DARWIN_ASSET" | "$LINUX_ASSET") ;;
	*) fail "unknown target asset" ;;
	esac
	command -v jq >/dev/null 2>&1 || fail "jq is required"

	validate_manifest_shape "$manifest"
	test "$(jq -r '.nativeRevision' "$manifest")" = "$native_revision" ||
		fail "manifest Native revision differs"

	target_index=$(jq -r --arg asset "$target_asset" '
		.targets | to_entries[] | select(.value.asset == $asset) | .key
	' "$manifest")
	test -n "$target_index" || fail "target is missing from manifest"
	target_size=$(jq -r --argjson index "$target_index" '.targets[$index].size' "$manifest")
	target_sha256=$(jq -r --argjson index "$target_index" '.targets[$index].sha256' "$manifest")
	test "$(file_size "$target_compiler")" -eq "$target_size" || fail "downloaded target size differs"
	test "$(sha256 "$target_compiler")" = "$target_sha256" || fail "downloaded target digest differs"

	test "$(wc -l < "$checksums" | tr -d ' ')" -eq 4 || fail "SHA256SUMS line count differs"
	test "$(checksum_entry "$ROOT_ASSET" "$checksums")" = "$ROOT_SHA256" ||
		fail "root checksum entry differs"
	test "$(checksum_entry "$DARWIN_ASSET" "$checksums")" = \
		"$(jq -r '.targets[0].sha256' "$manifest")" || fail "Darwin checksum entry differs"
	test "$(checksum_entry "$LINUX_ASSET" "$checksums")" = \
		"$(jq -r '.targets[1].sha256' "$manifest")" || fail "Linux checksum entry differs"
	test "$(checksum_entry "$MANIFEST_ASSET" "$checksums")" = "$(sha256 "$manifest")" ||
		fail "manifest checksum entry differs"

	jq -e \
		--arg tag "$release_tag" \
		--arg root "$ROOT_ASSET" \
		--arg darwin "$DARWIN_ASSET" \
		--arg linux "$LINUX_ASSET" \
		--arg manifest "$MANIFEST_ASSET" \
		--arg checksums "$CHECKSUM_ASSET" \
		'
			.tag_name == $tag and
			.draft == false and
			.prerelease == true and
			.immutable == true and
			([.assets[].name] | sort) == ([$root, $darwin, $linux, $manifest, $checksums] | sort)
		' "$release_json" >/dev/null || fail "release state or asset set is invalid"

	verify_release_asset "$release_json" "$ROOT_ASSET" "$ROOT_SIZE" "$ROOT_SHA256"
	verify_release_asset \
		"$release_json" "$DARWIN_ASSET" \
		"$(jq -r '.targets[0].size' "$manifest")" \
		"$(jq -r '.targets[0].sha256' "$manifest")"
	verify_release_asset \
		"$release_json" "$LINUX_ASSET" \
		"$(jq -r '.targets[1].size' "$manifest")" \
		"$(jq -r '.targets[1].sha256' "$manifest")"
	verify_release_asset \
		"$release_json" "$MANIFEST_ASSET" \
		"$(file_size "$manifest")" "$(sha256 "$manifest")"
	verify_release_asset \
		"$release_json" "$CHECKSUM_ASSET" \
		"$(file_size "$checksums")" "$(sha256 "$checksums")"

	printf 'bootstrap-seed-manifest: verified %s\n' "$target_asset"
}

test "$#" -gt 0 || usage
command=$1
shift
case "$command" in
create) create_manifest "$@" ;;
verify) verify_manifest "$@" ;;
*) usage ;;
esac
