# Releasing TypeRB

TypeRB uses a source-building Homebrew Formula. Release archives are generated
with `git archive` and deterministic gzip metadata, rather than using GitHub's
automatically generated source archives, so the Formula checksum remains under
the project's control.

## One-time setup

1. Create the public GitHub repository `type-rb/homebrew-tap`.
2. Give the source repository a `HOMEBREW_TAP_TOKEN` Actions secret whose token
   can update that repository.
3. Choose the project license. Add the license file to this repository and the
   matching `license` declaration to `packaging/homebrew/trb.rb.in`.

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
git tag v0.1.0
git push origin v0.1.0
```

The release workflow then:

1. repeats the tests and bootstrap check;
2. creates `type-rb-VERSION.tar.gz` and its checksum;
3. renders and uploads `trb.rb`;
4. creates the GitHub release; and
5. commits the Formula to `type-rb/homebrew-tap` when the tap token is set.

Users can then install with:

```sh
brew install type-rb/tap/trb
```

To render a Formula without making a release:

```sh
./scripts/render-homebrew-formula.sh 0.1.0 SHA256 /tmp/trb.rb
```
