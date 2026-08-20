# Releasing TypeRB

TypeRB publishes prebuilt `trb` binaries for macOS and Linux on Arm64 and
x86-64. The Homebrew Formula selects the matching archive, so installing the
compiler does not require Go. Each stable release also publishes the Linux
binary in a public multi-platform OCI image at `ghcr.io/type-rb/trb`.

## Version convention

The version embedded in source builds is the next planned release followed by
`-dev`, written as `X.Y.Z-dev`. A release tag uses the corresponding stable
version `vX.Y.Z`; release packaging embeds `X.Y.Z` in every binary.

After publishing a release, the Release workflow advances the source version
to the next planned patch release through an automatically merged pull request
and refreshes Pages. No human review is required for this mechanical change,
while the main branch retains its pull-request requirement. Homebrew publishes
only stable tagged releases and never consumes a `-dev` version.

## One-time setup

1. Create the public GitHub repository `type-rb/homebrew-tap`.
2. Create a fine-grained personal access token that has `Contents: Read and
   write` access to that repository. Store it in the source repository as the
   Actions secret `HOMEBREW_TAP_TOKEN`.
3. Create a separate fine-grained personal access token with `Contents: Read
   and write` and `Pull requests: Read and write` access to `type-rb/type-rb`.
   Store it as `RELEASE_SOURCE_TOKEN`. The workflow uses it only to create and
   merge the development-version pull request; the main branch ruleset remains
   unchanged.
4. After the first workflow-created `type-rb/trb` container package exists,
   set its visibility to Public. Keep it linked to `type-rb/type-rb`; public
   images can then be pulled without registry authentication.

The tap repository uses the standard layout:

```text
homebrew-tap/
└── Formula/
    └── trb.rb
```

## Release

Add a user-facing entry to `CHANGELOG.md` before tagging. Use a level-two
heading in this exact form:

```text
## X.Y.Z - YYYY-MM-DD
```

Group the entry by user impact rather than copying a pull-request list. The
release workflow requires a non-empty matching section and publishes its body
as the GitHub Release notes.

Run the full checks, create the version tag, and push it:

```sh
go test ./...
release_version=X.Y.Z
git tag "v${release_version}"
git push origin "v${release_version}"
```

The release workflow then:

1. repeats the compiler tests;
2. verifies that the source and changelog match the tag;
3. builds four `trb_VERSION_OS_ARCH.tar.gz` binary archives;
4. writes `checksums.txt` and renders `trb.rb`;
5. creates or updates the GitHub release using the changelog entry;
6. commits the Formula to `type-rb/homebrew-tap`; and
7. publishes `ghcr.io/type-rb/trb` for Linux Arm64 and x86-64 with an SBOM and
   build-provenance attestation; and
8. opens and merges a pull request for the next patch `-dev` version, then
   dispatches Pages.

Users can then install with:

```sh
brew install type-rb/tap/trb
```

Verify the published container and its exact release version:

```sh
docker run --rm "ghcr.io/type-rb/trb:${release_version}"
```

The package page must show the release tags and provenance for the image
digest. Container publication must succeed before the workflow advances the
source development version.

## Visual Studio Code extension

The compiler Release workflow does not publish the Visual Studio Code
extension. When a language release requires a new extension version, publish
the extension only after the matching compiler release and Homebrew Formula
have been verified.

From synchronized `main`, verify and package the extension:

```sh
npm ci --prefix editors/vscode
npm test --prefix editors/vscode
npm run package --prefix editors/vscode
```

Confirm that `editors/vscode/package.json`, its lockfile, changelog, and README
declare the intended extension version and minimum TypeRB version. Inspect the
generated `editors/vscode/dist/typerb.vsix`, then publish it with the
Marketplace credentials owned by the maintainer:

```sh
npm exec --prefix editors/vscode -- vsce publish
```

Verify the Marketplace listing and install the published version against the
new stable `trb` binary. The checked-in `vscode.yml` workflow tests and packages
the extension but intentionally does not hold Marketplace credentials or
publish it.

To render a Formula without making a release:

```sh
./scripts/package-release.sh X.Y.Z /tmp/type-rb-release
```

That directory contains the four archives, their checksums, and the rendered
Formula. TypeRB does not publish a Homebrew `HEAD` build; tagged releases keep
the installed compiler and Formula in sync.

Prepare and test the container root files locally from those artifacts:

```sh
./scripts/prepare-container-rootfs.sh \
  X.Y.Z /tmp/type-rb-release /tmp/type-rb-release/container
mkdir -p dist
cp -R /tmp/type-rb-release/container dist/container
docker build \
  --file packaging/container/Dockerfile \
  --build-arg TYPERB_VERSION=X.Y.Z \
  --build-arg SOURCE_REVISION="$(git rev-parse HEAD)" \
  --tag type-rb/trb:X.Y.Z .
docker run --rm type-rb/trb:X.Y.Z
```
