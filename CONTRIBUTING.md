# Contributing to TypeRB Native

TypeRB Native targets a production-ready TypeRB-authored compiler and toolchain.
Contributions advance complete language and package coverage, practical tooling,
correctness, portability and measured performance. The
[MIR consolidation roadmap](docs/mir-consolidation.md) defines the active milestone.
Report implemented coverage and validation separately from the eventual goals.

## Proposing work

Plan work on the project board described in the
[project workflow](docs/project-workflow.md): an Initiative states a measurable
outcome and its Tasks are the reviewable changes, with milestone issues as their
parents. Record durable architectural decisions under `docs/decisions/`; keep
scoped implementation work and its acceptance conditions in the issue.

Record the behavior or ownership change, relevant reference revision, validation
and remaining gaps. Performance or backend experiments also record the hypothesis,
baseline and candidate configurations, measurements and acceptance or removal
conditions. Feature coverage does not need an experimental hypothesis to be useful.

Small fixes and documentation corrections do not need a separate experiment
proposal. Current MIR work follows the larger
[consolidation milestone](docs/mir-consolidation.md), including its explicit
temporary cost policy; individual helper moves do not need a new proposal.

Follow the [repository organization schedule](docs/repository-organization.md)
for source moves and naming cleanup. Inventory consumers before removing
recovery and ordinary compiler code, keep mechanical changes separate from optimization, and
preserve recovery and measurement coverage.

Keep documentation current rather than cumulative. A document states the present
contract, ownership or procedure; the PR and its issue record what a change
delivered, how it was validated and what remains. Do not append per-feature or
per-reference-update sections, dated checkpoint narratives, hand-counted totals
or test-by-test inventories that the code, tests or generated views already
state. When a section only describes history, remove it and link the immutable
revision that contains it.

Keep implementation drafts in the quick-feedback stage. After focused local
proof and negative-case review, mark cohesive PRs ready for complete correctness.
During basic-language completion, organize changes by complete language families:
syntax, checking, MIR ownership, execution, REPL and shared conformance belong
in the same integration candidate. Large PRs are appropriate when they close
that coherent contract; keep their commits and acceptance evidence reviewable.
Merge each coherent PR once its tiered acceptance passes rather than stacking
dependent PRs behind a long validation run. Measure progress by completed
language contracts and remaining gaps, not PR count.
During MIR consolidation, detailed comparative cost qualification belongs at
milestones; costs observed during migration do not establish qualification. See the
[CI validation stages](docs/ci-validation.md) for routing, manual runs, and
the fail-closed merge-acceptance check.

## Correctness

The TypeRB specification and accepted conformance behavior define expected
language semantics. The reference implementation is the bootstrap compiler and
a differential oracle; a discrepancy may reveal a bug on either side and must
be triaged against the specification. Unsupported behavior must produce a
deterministic diagnostic rather than a fallback with different semantics.

Reproducing the specification also reveals ambiguities and opportunities to
improve it. Propose them with the Specification proposal issue form, with a
generic reproduction and rationale, rather than implementing a deviation. The
maintainer decides whether to adopt a proposal and where it lands.

Performance does not justify changing portable integer behavior, Unicode
behavior, failure behavior, initialization order, source attribution, or other
TypeRB guarantees.

Repository-owned compiler and runtime implementation source must be TypeRB.
External backend and platform tools are allowed when their revisions, licenses,
invocations, and distribution costs are explicit. Do not introduce a permanent
Go, Rust, Zig, or C host implementation as an intermediate shortcut.

Use the pinned reference compiler revision recorded in `TYPE_RB_REVISION` for
checkpoint verification. A revision update is a reviewed compatibility change, not
an incidental tool upgrade.

Build a clean checkout of that revision with the declared executable version:

```sh
python3 tools/build-reference.py /path/to/type-rb /path/to/trb
```

This verifies the source identity and embeds the exact version in
`compatibility/current.json`, including for shallow release-source checkouts.
See the [release integration procedure](docs/type-rb-compatibility.md#release-integration)
when advancing the reference after a TypeRB release.

## Cross-repository changes

Keep the reference TypeRB repository independent of this project. When a
temporary producer change is required upstream:

- define and name it only in terms of reference-compiler behavior;
- keep it internal, narrow, versioned, data-only, and removable;
- do not mention TypeRB Native, experimental checkpoint names, native-backend plans, or
  consumer-specific aliases in upstream code, diagnostics, tests,
  documentation, changelog entries, commits, or pull requests; and
- record the integration command, snapshot compatibility mapping, exact merged revision,
  compatibility note, and removal condition in this repository.

Before opening the upstream pull request, audit both its diff and proposed
title and body for project terminology. Update `TYPE_RB_REVISION` and CI only
after the upstream change is merged, then run the complete native checks with
the exact pinned compiler.

## Validation scope

Choose checks using [CI validation](docs/ci-validation.md) and the actual changed
paths. Documentation-only corrections need text, link, metadata and applicable
documentation checks; they do not require compiler recovery or benchmark runs.
Code changes retain their required correctness, recovery, target, memory and
cost authorities. After relevant checks pass, repeat or expand them for new
changes, failures or unresolved concerns rather than by default.

For compiler-source changes, also enable the recovery/QBE environment described
in [the compiler recovery guidance](.agents/skills/develop-typerb-native/references/bootstrap.md).
An optional test that skips recovery does not count as recovery evidence.
Hosted CI owns integration verification: PR acceptance for the planned lanes,
then `Main validation` for the lanes a tiered PR defers. Locally, run the
affected units, positive/negative reference comparisons, MIR and REPL checks,
and the ordinary Native build when compiler source changes. Run full local
recovery only to diagnose a recovery or platform failure, such as a red
`Main validation`. Merge only after PR acceptance passes.

Before pushing a code or CLI change, run the local preflight with the pinned
reference compiler:

```sh
TYPE_RB_CHECKOUT=/path/to/type-rb tools/preflight.sh /path/to/pinned/trb
```

It repeats the quick checks that need only that compiler, cheapest first, and
stops at the first failure: whitespace, generated coverage views, reference
identity, recovery layout, planner tests, Unicode data, formatting, root and
compiler types, the CLI and core closure, the recovery snapshot subset and the
project scenarios. A documentation-only change set stops after the generated
views. `--full` adds the reference language fixtures and the root and compiler
units. The preflight does not replace the focused checks below or hosted CI.
The reference checkout must contain the pinned commit for the authoritative
official-source comparison; when omitted, it defaults to `.type-rb`.

Keep the edit loop local and narrow. For example, after a MIR Array change,
run its QBE-backed compiler tests with the pinned reference compiler and QBE:

```sh
TYPE_RB_NATIVE_ROOT="$PWD" TYPE_RB_NATIVE_QBE=/path/to/qbe \
  /path/to/pinned/trb test --config compiler/trbconfig.jsonc \
  -t 'Array loop CFG MIR'
```

Use the matching test name for another feature family. Run formatting, type
checking, reference/Native behavior cases and the ordinary Native fixed point
as the batch becomes coherent; leave the cross-platform recovery, target, CLI
and memory authorities to hosted CI. Start the next local batch while a PR
validates instead of waiting for each hosted job to finish.
Use an ordinary Native compiler built from the active worktree for differential
probes; a core artifact in another checkout may predate the source under test.
For newly accepted syntax, compare check, build, execution and REPL against the
exact reference revision: a successful reference check alone does not prove
its output backend can execute the source.

For any compiler-source edit, also check the recovery snapshot subset before
publishing. This quick check uses the same canonical source copy and snapshot
version as CI, and catches syntax that ordinary Native regeneration accepts but
the recovery compiler cannot yet read. It also checks the exported file size
and source, type and function counts against the recovery decoder's bounds:

```sh
tools/check-bootstrap-snapshot.sh /path/to/pinned/trb
```

When a compiler source import changes or a module is added, renamed or
removed, synchronize the recovery inventories from the canonical import closure
before the focused checks:

```sh
python3 tools/recovery_layout_sync.py --write
python3 tools/recovery_layout_sync.py --check
```

Quick CI repeats the check. It covers the recovery layout rows and imports,
one recovery mutation per module and the own-frontend module list. A module
without a string literal or two adjacent record fields needs a hand-written
mutation. The generated inventories are not a separate behavioral change; the
hosted recovery suite verifies the resulting closure.

When checked binding or diagnostic behavior changes, scan the reviewed
conformance sources with an already built Native compiler before rerunning the
long recovery suite. This checks all valid, runtime-failing and compile-failing
source expectations in seconds; it does not replace MIR, runtime or recovery
verification:

```sh
python3 tools/check-conformance-sources.py /path/to/native-compiler
```

For source and compatibility validation, run the maintained root checks:

```sh
trb fmt --check .
trb check --config recovery/trbconfig.jsonc
tools/check-native-cli.sh /path/to/trb
TYPE_RB_NATIVE_ROOT="$PWD" trb test --config recovery/trbconfig.jsonc
python3 -m unittest tools/compatibility_manifest_test.py
python3 tools/compatibility_manifest.py --reference-trb /path/to/pinned/trb
```

`NATIVE_VERSION` identifies this implementation independently from TypeRB.
When changing it, `TYPE_RB_REVISION`, the compatibility manifest, bootstrap or
target identities, or their evidence, keep the canonical files consistent and
follow the bump rules in [Native versioning and compatibility](docs/versioning.md).

## Backend changes

Backend candidates share Native MIR, conformance inputs, and benchmark policy.
Same-target comparisons share a versioned target ABI profile. Keep
candidate-specific code behind the backend boundary. Do not add language syntax
or expose a backend name in a user-facing TypeRB API solely for an experiment.

## Benchmark claims

Benchmark pull requests must include enough information to reproduce the
result:

- repository revisions and exact commands;
- hardware, operating system, and toolchain versions;
- cold, warm, or incremental cache state;
- repetitions, aggregation method, and raw measurements;
- phase timings where available; and
- executable, runtime, linker, library, and sidecar sizes.

Compare against an optimized Go release baseline. A backend-only microbenchmark
may diagnose code generation, but it does not establish an end-to-end TypeRB
advantage.

## Language and workflow

Use English for committed documentation, code comments, commit messages, and
pull request text. Submit changes through focused pull requests and keep the
default branch passing its documented checks.
