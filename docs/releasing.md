# Releasing TypeRB

During the v0.5 alpha series, publish a same-version pair from
[type-rb/type-rb-go](https://github.com/type-rb/type-rb-go) and this repository.
The [v0.5 transition contract](native-transition.md) defines the alpha and
final release gates.

1. Set the Go companion version and verify that its Go module path is
   `github.com/type-rb/type-rb-go`. Merge the release preparation, tag the
   merged commit, and verify the prerelease assets and checksums.
2. Set the Native source version in `NATIVE_VERSION`, the Native CLI, and
   `compatibility/current.json`. Update the changelog, release notes and
   available platform scope. Verify the candidate with Main validation.
3. Tag the exact validated Native main commit with the matching alpha tag.
   The Native release workflow verifies the public Go companion, the shared
   manifest digest, and Main validation. It checks the archives and publishes
   them as a GitHub prerelease, along with checksums and build identity.
4. Open a pull request updating both formulas in
   [type-rb/homebrew-tap](https://github.com/type-rb/homebrew-tap).
   Verify a new install, upgrade, side-by-side use, and the rollback path.
5. Verify the published formulas and [canonical site](https://type-rb.github.io/)
   after the tap update. Keep older tags and archives available at their
   original URLs.
