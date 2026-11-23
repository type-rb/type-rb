#!/bin/sh
# Release validation is setup, never an ordinary compiler child.
set -eu
test "$#" -eq 4 || exit 64
tag=$1
revision=$2
asset=$3
destination=$4
case "$asset" in type-rb-native-bootstrap-darwin-arm64|type-rb-native-bootstrap-linux-arm64) ;; *) exit 64 ;; esac
printf '%s\n' "$revision" | grep -E '^[0-9a-f]{40}$' >/dev/null || exit 64
case "$tag" in
bootstrap-seed-2026-08-30)
    test "$revision" = 0058818314977633c50393796ef9b9f8f1fda50f || exit 64
    manifest=type-rb-native-bootstrap-manifest-v1.json
    signer=bootstrap-seed-initial.yml
    ;;
bootstrap-seed-2026-09-07|bootstrap-seed-2026-09-08|bootstrap-seed-2026-09-08-loop-transfers|bootstrap-seed-2026-09-08-boolean-arrays|bootstrap-seed-2026-09-09-record-arrays|bootstrap-seed-2026-09-10-hash|bootstrap-seed-2026-09-11-array-iteration|bootstrap-seed-2026-09-12-compiler-names|bootstrap-seed-2026-09-29-module-cycles)
    manifest=type-rb-native-bootstrap-manifest-v2.json
    signer=bootstrap-seed-refresh.yml
    ;;
*) exit 64 ;;
esac
test ! -e "$destination" || exit 64
mkdir -p "$destination"
tool_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
gh release download "$tag" -R type-rb/type-rb-native --dir "$destination" \
    --pattern "$asset" --pattern "$manifest" --pattern SHA256SUMS
gh api "repos/type-rb/type-rb-native/releases/tags/$tag" > "$destination/release.json"
if test "$tag" = bootstrap-seed-2026-08-30; then
    /bin/sh "$tool_root/bootstrap-seed-manifest.sh" verify "$tag" "$revision" "$asset" \
        "$destination/$asset" "$destination/$manifest" "$destination/SHA256SUMS" "$destination/release.json"
else
    python3 "$tool_root/bootstrap-seed-release.py" verify "$tag" "$revision" "$asset" "$destination" "$destination/release.json"
fi
for subject in "$asset" "$manifest" SHA256SUMS; do
    gh attestation verify "$destination/$subject" -R type-rb/type-rb-native \
        --signer-workflow "type-rb/type-rb-native/.github/workflows/$signer" \
        --source-digest "$revision" --source-ref refs/heads/main \
        --deny-self-hosted-runners --format json > "$destination/$subject.attestation.json"
done
# No downloaded binary is executed or made executable before all checks pass.
chmod 0755 "$destination/$asset"
