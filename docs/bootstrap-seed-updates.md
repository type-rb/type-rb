# Bootstrap seed updates

A seed is an already-built Native compiler used to build its successor. The
checked-in TypeRB compiler remains the implementation; the seed is neither
generated application code nor a new dependency on Go.

## Keep compatibility current, not the download URL floating

Refresh the seed before compiler implementation source adopts supported syntax
that the pinned seed cannot read, after relevant bootstrap/security fixes, or
at a deliberate stable development checkpoint. Check this boundary when adding
compiler implementation syntax, not only after a fresh checkout fails.

Use the latest **verified, accepted** source at that checkpoint. Do not publish
an unaccepted optimization candidate as a shortcut around its acceptance checks.
Refreshing on every commit is unnecessary: documentation and compatible source
changes do not inherently require a new seed. Expensive publication/verification
jobs remain manually dispatched; ordinary checkout builds always use an exact
immutable tag and digests, never a floating `latest` asset.

## Refresh sequence

The registered refresh is tracked by [issue #320](https://github.com/type-rb/type-rb-native/issues/320).
Preparation tooling alone does not mean that a release is published or that
checkout pins have switched. The current accepted handoff is recorded below.

1. Register a new tag, accepted source revision/closure, predecessor identities,
   target matrix and current bounds. Merge the reviewed observer changes first.
2. Dispatch `bootstrap-seed-refresh.yml` on main with `mode=prepare`. It verifies
   predecessor release state, strict manifest, checksums, asset digests and
   source-bound GitHub attestations **before** executing the old compiler.
3. Build two setup-only transitions from the previous Native seed, matching the
   existing worker-memory authority: the first generated compiler still carries
   the predecessor runtime. Both transitions permit the registered system linker;
   they remain process-traced and outside ordinary measurements. Then verify
   ordinary B2/B3/B4 builds, the complete existing corpus and registered adjacent
   measurement bounds (1.25 for adjacent generation medians, and 2.0 for every
   retained time/RSS/CPU observation against the strongest adjacent median).
   These are seed-generation checks, not the optimizer's 1.05 baseline contract.
   Record each ordinary Linux compiler build with a closed
   process allowlist. Darwin's inventory is boundary-only, not a dynamic trace.
4. Review both target results and paired bounds. Publish the four attested assets
   from that exact successful run as a new immutable experimental prerelease.
   Publication is a separate authorized operation; the preparation workflow
   itself never creates or edits a release.
5. Dispatch `mode=verify` from main. Fresh target jobs check out the release's
   exact source and download its actual published assets. Authenticate before
   execution, rebuild, compare the downloaded seed with the fixed point and
   rerun the corpus/measurement authorities. A failure leaves checkout pins alone.
6. In a separate PR, update checkout tag/digests and tag-scoped cache identity,
   run fresh core/CLI bootstrap on both targets, and update this status. Only
   then use the new syntax in compiler implementation source.

The v2 manifest records the predecessor tag/revision/manifest and target digests,
the source revision, QBE source/build identities and target compiler identities.
Unknown fields, altered identities, duplicate keys, invalid sizes, changed asset
sets, mutable/draft releases and incorrect checksum indexes fail closed.
Attestations bind compilers, manifest and checksums to the preparation workflow,
exact main revision and hosted runners. Manifest validation alone is not an
attestation check.

The three earlier published refreshes retain their source-era manifest caps of
350,000 bytes on Darwin arm64, 317,000 on Linux arm64 and 667,000 combined.
Current-source ordinary compiler validation uses the revised complete-compiler
budgets in [Decision 0029](decisions/0029-array-assignment-compiler-budget.md)
and [Decision 0030](decisions/0030-record-array-compiler-budget.md).
A future refresh must register its own exact source and manifest limits; this
decision does not republish a seed or alter historical release verification.
Refreshes change no ordinary relative limit, measurement baseline or runtime
performance claim. The legacy initial-root verifier remains unchanged.

## Retention and failure handling

Keep published tags/assets/attestations immutable and retain the initial root's
provenance. Later ordinary refreshes do not create another root QBE or invoke
the reference compiler. Historical release-integrity jobs keep their source-era
pins. An old target-recovery consumer is not silently repointed to a new release.

Compiler binaries stay outside Git. Retain compact accepted release identities
and durable verification links; temporary workflow packages/logs expire normally.
Never replace a published asset, retry until a favorable measurement hides an
earlier failure, or switch the default pin before post-publication verification.

## Earlier verified refreshes

Earlier registrations, preparation results, predecessor identities and exact
source-era limits remain in the [immutable handoff history](https://github.com/type-rb/type-rb-native/blob/f864151fa53a99a9491ca0268198d5ec914124fd/docs/bootstrap-seed-updates.md#registered-conditional-syntax-refresh).
The release manifests and verification runs remain the authority for each tag.
The current checkout handoff and refresh procedure are below and above.

## Registered module-cycle refresh

Task #706 registers `bootstrap-seed-2026-09-29-module-cycles` so implementation
modules can use mutually recursive imports. The compiler source is the accepted
Stage 2 implementation from PR #705 at
`a0f9147d29d73ad8a0237c7c51a0a5993bbe3a36`, validated by the
[complete acceptance run](https://github.com/type-rb/type-rb-native/actions/runs/36567058786).
The refresh workflow requires its exact `compiler/src` tree
`d811b0d7868f5e3701586fd563ef5b496eaba058`; a later source edit requires a new
reviewed registration before preparation. The attested full revision also records
the merged observer and fixtures used for that run.

The predecessor is the immutable compiler-name seed. Its exact revision,
manifest and both target digests remain authenticated in the release manifest.
Preparation and verification
retain Darwin arm64 on `macos-15` and Linux arm64 on `ubuntu-24.04-arm`, adjacent
median bounds of 1.25 and every-observation bounds of 2.0. MIR migration continues
to record exact compiler sizes without introducing an integration ceiling;
historical strict manifest caps remain unchanged.

In addition to the complete corpus and fixed points, both modes execute the
shared mutual-recursion and ordered-initializer contracts and reject value
cycles and unresolved indirect calls. Linux records each capability invocation's
process trace; Darwin retains its explicit boundary-only status. Ordinary CLI
CI exercises the same capability observer before publication.

The four exact attested assets are published immutably and have passed fresh
published-asset verification. Checkout builds and active consumers use the
verified seed recorded below; compiler source adoption retains ordinary and
recovery validation.

## Current verified checkout seed

Checkout builds pin [bootstrap-seed-2026-09-29-module-cycles](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-09-29-module-cycles),
an immutable experimental prerelease from accepted source
`04993b6bf598356a51143edd9dbde1a024c403cc`. Its compiler source tree is
`d811b0d7868f5e3701586fd563ef5b496eaba058` from the accepted module-cycle implementation.

- [Preparation and attestations](https://github.com/type-rb/type-rb-native/actions/runs/36579382624) passed both arm64 targets and all 28 retained generation observations. All four assets were authenticated against the exact source and hosted preparation workflow before publication.
- [Fresh published-asset verification](https://github.com/type-rb/type-rb-native/actions/runs/36582600586) passed both targets, including equality of each downloaded seed with B1/B2/B3/B4, the complete corpus, all 42 retained generation observations and ordinary Linux process boundaries. Both runs exercise the shared mutual-recursion and ordered-initializer contracts and reject value cycles and unproven indirect initialization calls. Darwin retains its explicit boundary-only inventory.
- Darwin compiler: 2,365,384 bytes, SHA-256 `54337b2b4e3aebc1a81260ad3be071008b88b6e30685d182db81a138f11ac216` (asset 598412238).
- Linux compiler: 2,321,904 bytes, SHA-256 `3627fd8c574a668173ac6bb25eb41371c0f6cf0fa9da1016eebfc08b078d4060` (asset 598412235).
- Combined: 4,687,288 bytes, recorded under the registered MIR migration and generation bounds.
- Manifest: SHA-256 `8d0b6a23b004c5a697715f8f0af454615edd7a69d6f7db9e65680e7914ec7b12` (asset 598412236).
- Checksum index: SHA-256 `4e48f0dcd6428dbea75bc48260cd7b7351438da940963037bc2f657dc807e210` (asset 598412237).

The immutable [compiler-name predecessor](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-09-12-compiler-names)
remains at `d7ffb9384229125216696a220c7f370422472178`. Its exact identities,
preparation/verification runs and earlier predecessor history remain in the
[source-era handoff record](https://github.com/type-rb/type-rb-native/blob/04993b6bf598356a51143edd9dbde1a024c403cc/docs/bootstrap-seed-updates.md#current-verified-checkout-seed).
Every historical tag retains its exact predecessor and size contracts.

Compiler binaries and detailed run artifacts remain outside Git. The current
seed and the registered historical consumer bridge permit compiler implementation
modules to adopt cyclic imports while retaining ordinary and recovery validation.

## Active CI consumers versus historical recovery

Current compiler-cost, worker-memory, formal runtime/build benchmark, daily/weekly
performance and Linux arm64 regression workflows use the exact module-cycle
seed and the shared strict download/authentication helper. Their manual seed input must match the recorded
source revision; an older or unknown tag fails rather than bypassing provenance.
Changing a setup seed does not move a frozen benchmark baseline or change any
measurement/acceptance bound. Full benchmark jobs remain manually dispatched.

Linux amd64 retains the original immutable root QBE. Its first setup compiler
builds exact accepted source `21f507e7ee7de2577f4137f6dfb9f732c14c1640` and must
accept logical-condition and `elsif` fixtures. That compiler then builds exact
accepted loop-transfer source `4e1d0b4aee97b9a5bd73a98f918b31d47985da25`; the new
bridge must accept the loop-transfer fixture. It then builds exact accepted
Boolean-capable source `57cb41ad6be91716e31fa555ed8ea8c8ce7a5f51` and checks the
Boolean Array fixture. It then builds exact accepted record-capable source
`566d00d67172460df0f57dff6d5fe03db3b67cdc` and checks the record Array fixture. It then builds accepted Hash source
`8a6d9ff73b14a97bca1b010ddaad6a38972b5373` and checks the Hash fixture.
It next builds accepted Array iteration source
`508f721f8964d67a5893e547d2e2fb3de5b20a63` and checks live iteration, control
transfer and managed-value fixtures. It then builds the accepted compiler-name
bridge `6ca79d22cde2ddba5fe836c66899b6e08a6511dc`, verifies its exact entry digest,
and uses it to check the accepted module-cycle source
`a0f9147d29d73ad8a0237c7c51a0a5993bbe3a36`. That next bridge verifies its exact
entry digest, checks the current candidate, and executes the shared cyclic-module
capability contracts under process tracing. Only the earlier compiler-name setup
transition retains the predecessor declaration spellings. The subsequent
candidate-runtime transition stays separate so a future runtime change still
precedes ordinary B2/B3/B4. These ordinary and measured generations remain the
candidate.

These compatibility bridges are setup-only. Exact clean source revisions and entry digests,
compiler/QBE identities, emission/QBE/link traces and capability-check output
are retained separately. It creates neither a new root asset nor Go recovery.
The arm64 comparison authenticates its verified seed independently. Optional
source arguments accept only their exact registered revisions. The Boolean
source requires both earlier sources, the record source requires all three,
the Hash source requires all four, and the Array iteration source requires all
five; the compiler-name source requires those six, and the module-cycle source
requires the compiler-name source. Omitted later arguments retain the earlier setup shapes. Omitting every source argument retains the historical
direct-current-source setup.

Initial publication/release-integrity workflows, retained manifests and earlier
results keep their source-era identities. Refreshing checkout bootstrap alone
does not establish that every historical recovery compiler understands newly
adopted implementation syntax; inspect these consumers before adopting it.
