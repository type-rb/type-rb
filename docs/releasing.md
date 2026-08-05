# Releasing TypeRB

TypeRB publishes prebuilt `trb` binaries for macOS and Linux on Arm64 and
x86-64. The Homebrew Formula selects the matching archive, so installing the
compiler does not require Go.

## Version convention

The version embedded in source builds is the next planned release followed by
`-dev`, such as `0.1.2-dev`. A release tag uses the corresponding stable version,
such as `v0.1.2`; release packaging replaces the development version in every
binary with `0.1.2`.

After publishing a release, advance the source version to the next planned
patch release in a pull request. Homebrew publishes only stable tagged releases
and never consumes a `-dev` version.

## One-time setup

1. Create the public GitHub repository `type-rb/homebrew-tap`.
2. Create a fine-grained personal access token that has `Contents: Read and
   write` access to that repository. Store it in the source repository as the
   Actions secret `HOMEBREW_TAP_TOKEN`.

The tap repository uses the standard layout:

```text
homebrew-tap/
└── Formula/
    └── trb.rb
```

## Release

Run the full checks, create the version tag, and push it:

```sh
go test ./...
./scripts/check-self-host.sh
release_version=X.Y.Z
git tag "v${release_version}"
git push origin "v${release_version}"
```

The release workflow then:

1. repeats the tests and bootstrap check;
2. builds four `trb_VERSION_OS_ARCH.tar.gz` binary archives;
3. writes `checksums.txt` and renders `trb.rb`;
4. creates or updates the GitHub release; and
5. commits the Formula to `type-rb/homebrew-tap`.

Users can then install with:

```sh
brew install type-rb/tap/trb
```

To render a Formula without making a release:

```sh
./scripts/package-release.sh X.Y.Z /tmp/type-rb-release
```

That directory contains the four archives, their checksums, and the rendered
Formula. TypeRB does not publish a Homebrew `HEAD` build; tagged releases keep
the installed compiler and Formula in sync.
