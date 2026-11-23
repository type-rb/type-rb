# Ordinary Native language coverage

The shared case registry, [`tools/native-language-cases.json`](../tools/native-language-cases.json),
is the source of truth for ordinary check, build, execution and REPL coverage.
The generated [family inventory](native-language-feature-inventory.md) and the
[Capabilities detail view](capabilities/README.md) show current probes, differences
and uncovered semantic contracts. These are test inventories with explicit gaps,
not complete language support. [Issue #454](https://github.com/type-rb/type-rb-native/issues/454)
owns basic-language completion.

This document holds the coverage contract only. Record delivered behavior in the
PR, the owning issue and the case registry rather than appending feature
narratives here. Per-feature notes written before this policy remain in the
[prior version of this document](https://github.com/type-rb/type-rb-native/blob/04c6ca7263c066fd13e83b3faa09c4e80d707c13/docs/native-language-coverage.md).

## Current development priority

The [basic-language completion plan](basic-language-completion.md) groups the
remaining implementation and verification work into one completion milestone.

Complete useful language families together with their MIR dependencies, instead
of waiting for individual unsupported programs to be reported. TypeRB at
`TYPE_RB_REVISION` remains the semantic authority; no Native-only dialect is
introduced. Fix wrong acceptance, wrong results and unsafe behavior as soon as
the shared probes reveal them.

Order the remaining families by their dependencies, as recorded in the
generated inventory. It keeps wider source interop and package syntax visible as
later phases, with reasons; do not count these phase labels as implemented
support. Keep raw byte operations explicit where source decoding or terminal
editing needs them; character indexing and terminal cell widths remain separate
contracts.

The [MIR consolidation milestone](mir-consolidation.md) coordinates the shared
ownership work. Correctness, memory safety, process and reproducibility checks
remain required. Temporary performance/size regressions are observed during
integration; detailed qualification occurs at coherent milestones. A feature
need not manufacture a runtime speedup or a separate size budget revision to
justify its existence. Final performance goals remain unchanged.

## Coverage is path-specific

The generated family inventory and the [Capabilities detail view](capabilities/README.md) use
[`tools/native-language-cases.json`](../tools/native-language-cases.json).
Each row describes one bounded example, not full support for a feature or a
percentage of the TypeRB language. The exact reference revision, executable
hashes and registry hash are recorded with observations.

The registry maps every concrete statement/expression node in the pinned
reference AST to a family. The oracle job checks the AST hash and scans all
non-test Go files in its directory for unclassified syntax nodes. This detects
syntax-inventory drift; it does not prove that every grammar combination,
method, type rule or boundary case has been tested. Families therefore also
record uncovered semantic contracts. Review those against public reference
language and standard-library documentation when expanding or changing the pin.

Both compilers execute the same authored sources, including project/import
fixtures. A case marked `projectCheck` checks its complete configured source
graph, which is required for file-route discovery; ordinary cases retain their
file-entry check. Route fixtures may use parameter brackets and Unicode names,
while every fixture path remains relative and excludes traversal components.
Each implementation has separate reviewed expectations for ordinary
check, build, execution and REPL. Positive cases, static rejection cases and
runtime failures are all intentional. Native acceptance of reference-invalid
record, Array or Hash equality is a bug, not extra support. Some reference REPL
submissions also fail despite successful ordinary execution; those limitations
are visible separately and must not be attributed only to Native.

A rejected REPL submission may leave the interactive session at exit status
zero. Output and diagnostics determine the result, not exit status alone.
Snapshot/recovery coverage cannot establish ordinary check/build/run/REPL support.

The regression command checks each implementation against its reviewed outcomes.
It can pass with known gaps; its `parityGaps` field summarizes differences between
those reviewed expectations, and `uncoveredContracts` identifies untested basic
contracts. `--require-parity` additionally rejects both known differences and
untested basic contracts. It requires both compilers, the reference AST and the
complete registry, and is intentionally failing until basic parity is achieved.
Neither mode infers full language coverage from the number of passing examples.

Frontend diagnostic wording is checked exactly for each implementation but need
not match between compilers. Matching rejection does not establish diagnostic
parity. [Issue #455](https://github.com/type-rb/type-rb-native/issues/455) tracks
message detail, real source columns and terminal presentation separately.
Runtime-failure fixtures may check the exact first
stderr line, status and stdout, omitting unstable reference panic stack frames.
Execution parity and its display use that same first-line contract when the
other implementation records full stderr. Two full-stderr expectations still
compare exactly; each implementation's regression checks keep its own contract.
Invalid UTF-8 output retains normalized raw bytes alongside an escaped display;
it cannot silently compare equal to valid text. Process timeouts always fail.

### Reproducing and maintaining the inventory

```sh
python3 tools/native-language-coverage-test.py
python3 tools/native-language-coverage.py --native /absolute/path/to/trbn --jobs 2
python3 tools/native-language-coverage.py --reference /absolute/path/to/trb \
  --reference-ast /path/to/pinned-reference/internal/ast/ast.go --jobs 2
python3 tools/native-language-coverage.py --native /absolute/path/to/trbn \
  --reference /absolute/path/to/trb \
  --reference-ast /path/to/pinned-reference/internal/ast/ast.go --require-parity
```

Build the reference executable from `TYPE_RB_REVISION`. CI verifies every
reviewed reference outcome in the quick oracle job. Native CLI jobs exercise
the same cases without invoking a reference compiler. Reports are short-lived
CI artifacts; do not add raw observations to `results/`. The suite is a
correctness contract, not a performance benchmark. Each subprocess has a
30-second timeout with owned-process-group cleanup; `--jobs` bounds concurrent
isolated cases and leaves reporting order deterministic.

Use `--case ID` for a focused probe and `--observe` to inspect changed Native
behavior. Observation still checks reference expectations and process failures;
it never rewrites expectations. Review all four paths before accepting a change.
Then regenerate all public views from that same reviewed registry:

```sh
python3 tools/native-language-coverage.py --feature-table > docs/native-language-feature-inventory.md
python3 tools/native-language-coverage.py --pages-data > docs/capabilities/ordinary-language.js
```

CI checks exact generated contents with `--check-feature-table` and
`--check-pages-data`. Review the related broad `capabilities/catalog.js`
entries when behavior changes, preserving their stated scope and separating
snapshot evidence from ordinary support. A Pages capability update needs no
formal benchmark rerun when runtime benchmark evidence has not changed.

## Feature delivery contract

Work within the active milestone and record exact reference/Native identities,
semantics and coverage in each cohesive PR. Register new semantic scope or material
experiments when needed; routine ownership moves do not need individual budgets.
Then deliver parser/checker behavior, checked/MIR representation and validation,
mechanical lowering, CLI/REPL coverage, and reference differential tests.
Preserve source diagnostics, evaluation order, branch-local bindings, portable
failures and managed-root lifetimes. Do not add semantic analysis to QBE text.

For `break` and `next`, explicitly validate loop targets, nested control flow,
backedges, induction updates and root cleanup. Existing bounds/header proofs
must account for each new path; unproved cases retain their checks. General
control-flow MIR need not be completed before the feature, but the affected
semantic ownership and verifier cannot be skipped.

After feature acceptance, follow the [seed update boundary](bootstrap-seed-updates.md)
before using the syntax in compiler implementation source. Add the matching
snapshot recovery coverage when that source begins using it. Then remove the
actual flags, nesting or representation workaround that motivated the feature.
Do not count a parser-only change or a source rewrite without bootstrap tests
as completion. Keep each feature and its self-use adoption independently clear.

## Checkpoint reporting

Report the ordinary coverage cases added, remaining gaps, compiler-source
cleanup enabled, MIR invariants verified, bootstrap/recovery status and measured
costs separately in the PR and the owning issue. Update the case registry and
regenerate its views when behavior changes; keep planned work distinct from
accepted behavior. Publish Pages coverage at accepted checkpoints without
rerunning the formal runtime benchmarks unless accepted runtime evidence has
actually changed.
