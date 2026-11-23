# TypeRB Compatibility

TypeRB Native currently follows exact reference revisions during development. The
current source and semantic oracle is TypeRB
`cc8c734f73f5149464838825cdbb2165058ed26d` (`0.4.12-dev`, including the shared
Unicode 17 OIDC hostname policy, declared standalone Go dependencies, portable
Process text decoding and explicit executable launch, joined Jobs shutdown and portable Math APIs), recorded
in `TYPE_RB_REVISION`. This declares one exact reference identity during Native
development, without claiming a supported version range.

The machine-readable
[`compatibility/current.json`](../compatibility/current.json) records this
exact mapping beside the independent Native `0.1.0-dev` implementation
identity. Its strict schema and CI validation keep TypeRB, bootstrap, MIR,
runtime ABI, backend, target, and evidence identities separate. The current
target list contains the internal Darwin arm64 and Linux arm64 seed
profiles plus the independently recovered and verified internal Linux
amd64 profile. See
[Native versioning and compatibility](versioning.md) for bump and release
rules.

Each reference update is reviewed in its own PR, which records the incorporated
TypeRB changes and their Native effect. Update notes written before this policy
remain in the [prior version of this document](https://github.com/type-rb/type-rb-native/blob/04c6ca7263c066fd13e83b3faa09c4e80d707c13/docs/type-rb-compatibility.md).

## Release integration

Treat a new reference release as a compatibility update through a Native PR.
After the public release workflow succeeds, resolve the stable tag to its exact
merged commit. Update `TYPE_RB_REVISION`, the manifest, maintained current
workflow/controller identities and generated coverage views together. Build
through `tools/build-reference.py` and rerun the shared language cases against
the exact reference AST. Review changed outcomes before editing expectations;
reference fixes can expose Native gaps that must remain visible.

Complete the PR's required correctness, recovery, target and memory authorities
before merging. Preserve frozen experiment pins, immutable seed assets and
historical evidence. A future release detector can prepare this PR, but must
not update the default branch or rewrite conformance expectations automatically.
This consumer-owned procedure requires no Native-specific reference API or
release hook in the TypeRB repository.

## Bundled official sources

[`vendor/type-rb/official`](../vendor/type-rb/official/provenance.json) retains the
reference's complete `internal/official/packages` tree, the URL, JSON, JSONC and time
standard-library source wrappers and license unchanged.
The provenance record identifies the repository, exact commit, source paths,
byte lengths and SHA-256 hashes. A generated TypeRB catalog embeds these exact
sources and canonical names, so ordinary compilers load reachable official
modules through the same declaration graph without a repository or reference
checkout at runtime. The complete portable HTTP, URL, Web value and portable
middleware sources have ordinary import coverage. URL component encoding/decoding and ordered query operations
use a Native-owned TypeRB adapter with the pinned nominal types and errors.
JSON values and JSONC parsing use a Native-owned TypeRB adapter. Typed JSON
providers cover finite records, nullable values, containers, raw enums, newtypes
and the six portable time value types. Generic and recursive record schemas
remain incomplete or retain the pinned explicit rejection.
Web Request/Response values, middleware composition, CORS and secure headers use
the unchanged package bodies. Their internal
`Runtime.fail<T>` service is bound by loader ownership to a verified MIR
signature and the checked REPL evaluator. Authored source names and import paths
cannot grant internal access. Loader-owned providers specialize `Context#params<T>`
and `Request#query<T>` for admitted record schemas through ordinary checked TypeRB
bodies. Scalar values, string-backed enums, nullable fields and query Arrays
(at most one non-nullable Array field) preserve the pinned errors and field identities. Integer-backed enums and
multiple non-nullable Array fields await the cross-mode decision in
[#1175](https://github.com/type-rb/type-rb-native/issues/1175). Time-valued parameters
and `Context#bind<T>` compose these providers with typed JSON body binding.
Context state, routes and networking remain incomplete. Packages requiring other unimplemented platform or semantic
providers fail explicitly. The shared ordinary-language registry records the delivered subsets and remaining
import and codec gaps.

The complete pinned time wrappers retain Date, TimeOfDay, DateTime, Instant,
Duration and TimeZone declaration identities. Calendar arithmetic, ISO-shaped
parsing, formatting, TZif decoding and local/UTC conversion are ordinary TypeRB.
Two independently verified host adapters transfer wall-clock seconds/nanoseconds
and bounded zone-file bytes; their [system dependencies](native-targets.md#time-services)
remain explicit. JSON and Web codecs use the same value constructors and typed
parsers. Reference range and execution-mode inconsistencies remain tracked in
[#1187](https://github.com/type-rb/type-rb-native/issues/1187) and
[#1188](https://github.com/type-rb/type-rb-native/issues/1188). Codec import-name
restrictions remain under [#1189](https://github.com/type-rb/type-rb-native/issues/1189);
retaining those gaps does not establish full time-service parity.

After changing the reference pin, synchronize from a Git checkout containing
that exact commit:

```sh
python3 tools/official-sources.py --reference-checkout /path/to/type-rb --write
python3 tools/official-sources.py --reference-checkout /path/to/type-rb --check
```

The tool also regenerates `compiler/src/project/official_catalog.trb`. Never
edit embedded source strings separately from the pinned bundle. Internal
imports remain inaccessible to authored code; an authored path spelling does
not confer compiler-owned source identity.

The tool reads pinned Git objects instead of working files or the checkout's
current HEAD. It requires no network or reference executable. PR quick and Main
validation compare the complete file inventory and bytes, including provenance,
with the pinned reference. Modified, missing, additional or symlinked bundle
files and altered catalogs fail validation even if the local hashes were
updated to match a change.
Keep this comparison in the reference-update procedure; never edit vendored
sources as a separate fork.

## Current declaration-import mapping

The self-hosted frontend preserves canonical declaration identity for its
implemented declaration families:

| TypeRB behavior | Current Native behavior |
| --- | --- |
| Named import | Selects an exact supported record, class, interface, enum, function, alias, module or constant declaration |
| Named `as` alias | Changes only the local binding; canonical declaration identity remains exact |
| Bare project import | Selects one matching supported record, class, interface, enum, generic nominal, alias, module or constant root |
| Bare `as` alias | Selects the same unique declaration first, then changes its local binding |
| Root key | Removes ASCII `_` from the logical final path segment and folds ASCII case; declaration names fold ASCII case without removing `_` |
| Directory entry | A resolved `name/index` module uses `name` as its logical root segment; `name` and `name/index` can resolve to that same module |
| Direct/index conflict | Rejects a resolved graph containing both `name` and `name/index`; there is no precedence between two loaded module identities |
| Duplicate identity | Rejects importing the same declaration identity again, including through a different alias or equivalent index path |
| Binding and usage | Rejects duplicate local bindings, local-declaration collisions, missing exports, and unused aliases under their local names |

Bare imports do not create lowercase namespaces and never import every export.
A top-level function remains available through an exact named import but cannot
become a bare root. Zero matches, multiple matching declaration roots, and a
function-only match produce deterministic diagnostics.

Nested and reopened modules, owned declarations, runtime constants, generic
nominals and transparent aliases retain lexical identity. Classes, interfaces,
newtypes, general package activation and project-aware formatter rewrites remain
coverage work. Selected compiler-owned standard packages have explicit mappings;
unsupported imports do not fall back to a namespace or loaded-identity precedence
model. Repository-owned TypeRB source is still formatted and checked by the
pinned reference compiler.

## Source pins and bootstrap seeds

The quick PR job validates reference checkout configuration before downloading
or building the reference compiler. `tools/compatibility_manifest.py` inventories
every reference checkout in the maintained workflows, their post-checkout identity
checks, and the Linux amd64 controller pin. Missing, added, or changed consumers require
an explicit validator update. Mutation tests exercise each checkout separately.
The later executable version check remains required before matrix fan-out.

Snapshot validation, PR validation, worker memory, formal runtime/build benchmarks, and
the Linux amd64 workflow follow `TYPE_RB_REVISION`. Daily and weekly workflows
derive it from the checked-out file before their reference checkout.
Retired experiments keep their historical pins in the
[retired workflows](retired-experiment-tools.md).

This is a strict check of the maintained block-mapping and shell spellings,
not a general YAML or shell interpreter. Post-checkout Git identity and the
reference executable version are still checked when the workflows run.

The reference pin and a Native bootstrap seed answer different questions:

- `TYPE_RB_REVISION` selects the exact syntax, semantics, formatter, and
  differential oracle used for current development.
- A Native seed is an executable predecessor used to compile the current
  TypeRB-authored compiler. Its source-era metadata records provenance; it does
  not constrain the source revision that a later compatible compiler may
  accept.
- The immutable
  [`bootstrap-seed-2026-08-30`](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-08-30)
  remains unchanged. When its embedded runtime or link policy predates current
  source, compatibility uses separately identified setup-only Native
  transitions before proving exact current B2/B3/B4 fixed points. The
  transitions remain Go-free and outside candidate timing and size claims;
  they do not replace or relabel the seed.

A later seed is warranted only by a concrete distribution need demonstrated by
the compatibility chain. The existing attested seed reaches the exact
`0.4.4-dev` fixed point on both targets through two setup-only transitions.
That confirms bootstrap feasibility without making the older embedded runtime
free: a future seed containing the current runtime can remove both transitions.
Revision alignment alone does not warrant replacing or relabelling the seed.
The Linux amd64 target demonstrates the complementary case: it is recovered
from the immutable seed release's target-neutral root QBE, not from an amd64
seed executable. Its exact current chain and workflow are therefore recorded
as target-chain evidence in compatibility schema version 2, while the
immutable seed manifest remains unchanged. See the
[Historical Linux amd64 result](../results/2026-08-31-gate6n-linux-amd64/README.md) and
[Decision 0026](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/decisions/0026-recovered-target-chain-evidence.md).
Native SemVer is independently defined. TypeRB compatibility ranges, stable
installation and release-support policies remain implementation work toward the
[production-use goal](mir-consolidation.md). The current schema can express only
this exact verified TypeRB revision; broader claims require evidence and an
explicit schema/policy update.
