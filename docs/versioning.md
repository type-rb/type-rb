# Native Versioning and Compatibility

TypeRB Native ships matching public versions with the Go companion starting
with `0.5.0-alpha.1`. The exact Go reference compiler used by Native CI
continues to have its own pinned version and revision.

The canonical inputs are:

- [`NATIVE_VERSION`](../NATIVE_VERSION), containing exactly one SemVer value;
- [`TYPE_RB_REVISION`](../TYPE_RB_REVISION), selecting the exact reference
  source and semantic oracle;
- [`compatibility/current.json`](../compatibility/current.json), declaring the
  exact verified combination; and
- [`compatibility/schema-v3.json`](../compatibility/schema-v3.json), defining
  the strict machine-readable shape.

The compatibility record is descriptive, not a dependency resolver. Version 3
allows only `supportMode: "exact"`. A TypeRB version or revision absent from the
record is not claimed to work, even if it happens to compile.

Schema version 2 separates target executables present in the immutable seed
manifest from additional current target chains recovered from an attested
target-neutral root. Every recovered target requires its own exact Native
revision, result path, workflow run, and profile identity. This prevents a
newly verified target from being mislabeled as an asset in an older immutable
seed release. Schema version 1 remains retained for historical records but
cannot express recovered target-chain evidence.

Schema version 3 additionally records the current backend variant and patch
digests. The upstream identity is verified against the retained seed provenance;
the current patch identity is verified against `tools/qbe/backend.json` and the
patch bytes. Historical seed manifests and schemas remain unchanged.

## v0.5.0 transition boundary

The accepted [canonical transition contract](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md) replaces independent
release numbering prospectively when the Native implementation moves into
`type-rb` at v0.5.0. That release and the retained `type-rb-go` release share the
version and reviewed shared feature contract. Go emits the three source
languages only; Native additionally emits Native executables. Record exact
implementation, source, artifact and contract identities separately.

The independent version numbering applied to pre-cutover development.
Historical tags, manifests, seed assets, and measurements retain their original
identities. Decision 0020 describes those identities.

## Independent identities

The Native version identifies the compiler implementation, CLI,
runtime, target profiles, and distribution as one releasable implementation.
It does not identify a TypeRB language edition. TypeRB remains the same
language contract implemented by the reference compiler and this implementation.

The manifest keeps these compatibility axes separate:

- exact TypeRB version and Git revision;
- compiler protocol stability;
- bootstrap snapshot schema and immutable seed manifest;
- Native MIR stability;
- runtime ABI stability;
- external backend source identity;
- target and CC boundary profiles; and
- exact conformance, fixed-point, and bootstrap evidence.

`null` protocol, MIR, and runtime ABI versions are intentional. Those surfaces
are currently unstable, so assigning a reusable compatibility number would be
a stronger promise than the evidence supports. Target profile suffixes remain
internal development identities and do not by themselves establish production
support.

## Bump rules

Native and TypeRB changes are reviewed independently.

- A TypeRB version or revision advance updates `TYPE_RB_REVISION`, the exact
  `typeRB` identity, and fresh conformance evidence. It never copies the TypeRB
  number into `NATIVE_VERSION`.
- A Native implementation release receives its own SemVer bump when compiler,
  CLI, runtime, target, or distribution behavior changes. While Native remains
  in `0.x`, a minor bump may include intentional incompatible development
  changes; patch bumps are reserved for compatible fixes and evidence or
  metadata corrections included in a release.
- Documentation, tests, and unpublished experiment results do not require a
  version bump by themselves.
- Changing the exact TypeRB claim in a distributed Native release is a visible
  compatibility change and therefore requires a Native release bump, but the
  two numbers remain unrelated.

A compatibility range may be introduced only by a later reviewed schema and
decision backed by conformance evidence for every claimed boundary. Current
schema version 2 cannot express a range, so tooling must fail instead of
inferring one.

## Releases, deprecation, and security fixes

The tracked manifest deliberately has no current Native Git revision or
artifact checksum: either would become self-referential or stale on the commit
that updates it. An immutable release adds, outside this source declaration:

- the exact source Git revision;
- the compatibility-manifest digest;
- every compiler and sidecar size and SHA-256;
- target, runner, QBE, system-toolchain, and signing or attestation identity;
  and
- the release's exact TypeRB and bootstrap records.

Published artifacts and seed assets are never replaced in place. A corrected
seed uses a new tag and manifest. A security fix uses a new Native version and
new immutable artifacts; if the fix affects bootstrap trust or generated
compiler bytes, it also uses new seed assets. Existing experimental versions
are retained as historical identities rather than silently reclassified.

There is no stable deprecation window in `0.x`. A future support or
installation policy must separately define supported versions, notice
periods, recovery, and security maintenance before any such guarantee is
advertised.

## Validation

Run the permanent validator and mutation tests from the repository root:

```sh
python3 -m unittest tools/compatibility_manifest_test.py
python3 tools/compatibility_manifest.py --reference-trb /path/to/pinned/trb
```

The validator rejects duplicate, missing, and unknown JSON members and checks
the manifest against `NATIVE_VERSION`, `TYPE_RB_REVISION`, the pinned compiler's
reported version, snapshot decoder and workflow, retained bootstrap release
manifest, self-hosted target definitions, QBE source identity, and referenced
evidence. CI runs both commands with the compiler built from the exact pinned
TypeRB revision.

The decision boundary is recorded in
[Decision 0020](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/decisions/0020-independent-native-versioning.md).
