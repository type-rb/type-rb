# TypeRB v0.5.0 implementation transition

Implementation is tracked by [the v0.5.0 transition initiative](https://github.com/type-rb/type-rb-native/issues/1164).

## Release outcome

TypeRB v0.5.0 makes the TypeRB-authored Native compiler the canonical
implementation in `type-rb/type-rb`. It must provide every feature in the
reviewed Go implementation release contract, subject only to the explicitly
approved v0.5 deferrals below, before the switch is released.
The existing Go implementation is preserved, with its Git history, in
`type-rb/type-rb-go` and maintained at matching release version numbers for the
time being. This is the accepted destination; it does not claim that current
Native builds have reached feature parity or authorize an immediate cutover.

Compiler implementation and program output are separate choices:

| Implementation | Go output | Ruby output | TypeScript output | Native executable output |
| --- | --- | --- | --- | --- |
| Canonical TypeRB-authored Native compiler | Required | Required | Required | Required |
| Retained Go compiler | Required | Required | Required | Not provided |

The retained Go implementation does not acquire a Native backend. A source
backend includes generation, build/run integration, applicable runtime and
package integration, and interactive behavior, not merely producing source
text. Explicit platform-specific APIs retain their documented applicability;
this policy does not make a Go-only platform API portable to every backend.
The Native executable path must implement the portable language and official
application contracts with its own qualified runtime providers.

## Repository ownership

- `type-rb/type-rb` owns the language specification, public documentation,
  portable standard/official sources, shared feature contract and conformance
  tests, and the canonical Native implementation after cutover.
- `type-rb/type-rb-go` owns the maintained Go implementation and its
  implementation-specific tests and build tooling. It consumes the same
  versioned shared contracts rather than maintaining an independent dialect or
  a separately edited copy of the specification and official packages.
- `type-rb/type-rb-native` remains the Native development repository until the
  migration gates pass. Then archive it as a historical repository, point users
  to the canonical project, and retain access to its history and immutable
  release, seed and evidence assets. Do not keep a third independent active
  implementation line.

## Complete feature contract

The initial versioned [v0.5 feature inventory](v0.5-feature-contract.md),
[manifest and result contract](../contracts/v0.5/README.md) record the full
review scope and explicitly unqualified baseline. They do not assert current
feature parity. Exact-candidate scenario mapping and qualification are tracked
by [task #1221](https://github.com/type-rb/type-rb-native/issues/1221).

Create and review a versioned manifest from the Go implementation's public
specification, source registries, CLI/help, tests, integrations and release
configuration. Record the exact source revisions and manifest identity.
Inventory the whole exposed surface; passing a sampled corpus or counting its
cases is not a completeness claim. Future roadmap features absent from the
reviewed Go release contract are not implicitly added to v0.5.0.

The manifest covers:

- language syntax, types, module/name resolution, effects, failure behavior,
  diagnostics and source origins;
- standard-library and official-package APIs, including Web, ORM, jobs and
  their composed application workflows;
- project configuration, dependency/lock behavior, local and remote packages,
  host ecosystems, declarations, generators, adapters and supported plugin
  protocols;
- every exposed CLI operation, REPL and retained state, formatting, linting,
  testing, compiler services, LSP and editor-facing contracts;
- Go, Ruby and TypeScript emission/execution and their supported environments;
- install/update/distribution paths, supported host targets, release metadata,
  provenance, support/security routing, and bootstrap/recovery requirements.

Each row states the observable contract, applicable implementation/output
modes, expected success and failure behavior, owning test command, exact
candidate/tool identities, result and remaining work owner. The shared
manifest also identifies additional Native executable requirements. Missing,
untested, blocked and contradictory rows remain visible; they cannot count as
passing or disappear from the denominator. Existing narrower inventories
remain useful evidence sources, not substitutes for the complete manifest.

The approved v0.5 exceptions are bounded in the manifest, not in result files:

- Native `-t` / `--test-name-pattern` matching is deferred pending the selection
  design in [#643](https://github.com/type-rb/type-rb-native/issues/643). File and
  directory selection remain required; unsupported options must be diagnosed.
- Native local `play` / `tour` execution is deferred. Both commands must report
  explicit unsupported diagnostics and must not launch the Go implementation
  implicitly. REPL and ordinary CLI formatting and all three source outputs
  remain required. The Go implementation retains local `play` / `tour`.
- At the v0.5 cutover, hosted Run, Transpile, Format and interactive tour
  execution may pause. Documentation must keep publishing current installation,
  CLI and implementation-difference guidance. Unavailable pages explain the
  pause and direct users to documentation and separately installed Go tools.
  This does not freeze the site or waive other distribution obligations.

[Scope preparation](https://github.com/type-rb/type-rb-native/issues/1255) records
these decisions; qualification remains #1221 and cutover rehearsal #1168.
Review each deferral at candidate freeze and again before restoring the feature.
It is not a permanent omission or an automatic waiver for subsequent releases.
Deferred obligations remain in the inventory, separate from passed evidence.

For the documentation-only site, use
`scripts/build-site.sh OUTPUT VERSION docs-only` or
`trb-site --docs-only --docs DOCUMENTATION_DIR --out OUTPUT --version VERSION`.
Use a fresh output directory: existing browser
runtime assets are rejected rather than silently carried into the deployment.
The current v0.4 publication keeps its interactive default until cutover.

After migration, `type-rb` continues owning and publishing the site and current
canonical documentation. Initially build `trb-site` from a fixed Go implementation
source revision in site CI only; record its source/toolchain/dependency identities
and generated-tool hash. Feed it canonical release documentation and navigation,
with the canonical grammar and pinned Node/Shiki highlighter. This does not
require shipping Go with Native applications. The generator's Go repository
must not become a second owner of documentation. Qualify links, versions and
installation instructions in the migration rehearsal. No WASM, worker protocol
or executable playground asset bundle is required in this mode.

The [post-v0.5 browser task](https://github.com/type-rb/type-rb-native/issues/1256)
first considers local Native play/tour using the REPL evaluator and
HTTP server, alongside language-neutral lesson data shared at an exact revision.
Keep fresh evaluation state, bounded execution and host-operation rejection;
validate loopback Host and Origin (also tracked for Go in
[#847](https://github.com/type-rb/type-rb/issues/847)) and distinguish evaluator,
worker and test timeouts. Run also returns generated source, so the three source backends are a
prerequisite. A browser build from Native TypeScript output remains a later
candidate after the compiler itself can be emitted and qualified with browser
I/O, cancellation and diagnostic adapters. Do not add a WASM backend or change
language semantics solely for tour. Restoration requires its own implementation
and evidence; this preparation does not promise its date.

Keep the release baseline explicit. A shared feature added before v0.5.0 must
enter the reviewed manifest and pass both implementations' applicable tests.
Do not silently chase a moving branch or remove a failing feature to qualify a
release. Changes to language/API contracts use the specification decision
process and update both implementations and first-party consumers.

## Meaning of equivalence

The shared specification and conformance suite are the authority. Compare
accepted and rejected programs, types, observable results and side effects,
failure/lifetime behavior, public APIs, command behavior and structured
protocols. Cover composed workflows, invalid input, runtime failure and
retained REPL behavior, as well as individual features. A known Go bug or a
mode inconsistency requires a reviewed correction, not automatic reproduction
as language semantics.

Generated source bytes, internal representations and implementation strategy
need not match. Human-readable diagnostic wording need only match where the
public contract or an existing required test specifies it; failure category,
origin and structured diagnostics remain observable contracts. Performance
is qualified separately under the existing criteria, not inferred from
functional equivalence. Required correctness, memory, target, bootstrap,
recovery and performance gates are not weakened by this transition.

## Versions and shared maintenance

Starting with the switch, publish matching TypeRB and Go implementation
release versions, beginning with v0.5.0. A matching version means both releases
satisfy the same shared contract, with the output difference and the explicit
v0.5 deferrals above. Record implementation identity, source revision, contract digest, artifacts and
provenance separately so equal version numbers never hide different binaries.
Pair subsequent releases, including implementation-specific fixes; no shared
feature is released on only one implementation under a supposedly equivalent
version. Shared specifications, package sources and conformance fixtures have
one canonical owner and are consumed at exact release identities.

Reassess the duration and scope of Go maintenance after practical Native
application use and release operations are stable. No automatic retirement
date or silent reduction in the promised shared functionality is established.

Before cutover, the Native repository retains its current independent
implementation identity and exact compatibility pins. This policy replaces
independent release numbering prospectively at v0.5.0; it does not rename old
tags, relabel existing seeds, update a pin without validation, or reinterpret
historical measurements. New immutable identities are required for new assets.

## Development and release gates

Prioritize complete practical application paths while closing the full
contract. Native previews may become useful before all v0.5.0 requirements
pass; publish their supported boundary explicitly. A preview does not waive
the final parity gate or require delaying useful application work until every
language edge case is complete.

The release and repository cutover require:

1. All non-deferred obligations in the reviewed full feature manifest pass for
   exact candidates on both implementations and every applicable output/target. There are no missing
   results, unexplained differences or silently excluded Go features.
2. The Native executable path passes its additional portable application and
   supported-target contracts. Ordinary self-hosting/fixed points and the
   separate recovery path have current evidence.
3. Required integration, memory/safety, diagnostic, performance and
   release/security qualification is complete. Migration CI alone is not
   final release qualification; historical criteria and failures retain their
   meaning.
4. A reversible rehearsal preserves Go and Native source history and checks
   tags, immutable seeds/assets, package sources and Go module identities.
   Verify Homebrew, OCI, installers, Pages, editor integrations, checksums and
   provenance against the new repository/implementation ownership.
5. Publish and verify the canonical v0.5.0 and matching Go release using the
   established release authorization and verified artifacts. Confirm that
   supported existing workflows still work with the replacement compiler.
6. Transfer or explicitly dispose of every active issue, PR, claim, workflow,
   consumer and support/security route before archiving `type-rb-native`.
   Verify old links and historical assets remain useful, publish the redirect
   notice, then archive. Do not archive the development repository first.

Work state, ownership, acceptance evidence and remaining dependencies belong
in the linked initiative and its tasks. Existing language, MIR, tooling and
application initiatives keep their criteria and ownership; this release
outcome connects their evidence without closing or duplicating them.
