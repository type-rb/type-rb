# TypeRB Native

Use English for committed documentation, code comments, commit messages, and
pull request text. The project targets production use: full TypeRB language,
standard-library and official-package coverage, Go/Ruby/TypeScript emission and
execution, and Native output. Follow `docs/mir-consolidation.md` for the current
milestone and measured Pure Go goals. Describe incomplete coverage and unstable
interfaces as current gaps, not permanent exclusions from that objective.

For release scope and feature selection, also follow the accepted
[v0.5.0 transition contract](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md). Full Go implementation parity is a release
gate, including source backends, packages and tooling, subject only to the
scoped maintainer-approved v0.5 deferrals in the canonical manifest. Deferrals
remain visible and never count as passing parity. The retained Go compiler
has no Native output requirement. Practical application slices remain a
priority within that full scope. Current exact pins, historical identities,
quality gates and initiative ownership stay in force until their documented
transition; no planning task authorizes an early archive or release.

## Scope and completion

Carry the user's requested task through its stated completion point. Use existing
authorization for routine implementation choices, reversible checks, and fixes
within that scope. Ask only for missing information or an unresolved decision
that materially changes the result or requires new authority. Work taken from
the project board continues through merge, cleanup and the next eligible task
without further approval, and stops only for the maintainer decisions in the
workflow's authority table. This includes reference conformance, MIR and
performance improvements, and maintenance that preserve specified behavior and
the approved scope. Use the development skill's trade-off guidance to adopt
overall improvements even when numerical performance targets remain unmet.
When an initiative meets its recorded completion criteria, record current
evidence and mark it Done under the project workflow.
A request to prepare a PR for review ends with an open PR; it does not authorize
a merge or another development slice. Later task and budget limits narrow
earlier standing direction to continue.

Read documents relevant to the affected surface. Use the
[development skill](.agents/skills/develop-typerb-native/SKILL.md) for compiler,
MIR, runtime, recovery, or performance work and its conditional reference map
for specialized constraints. For routine documentation edits, check the changed
text and applicable documentation CI. For code, complete the authorities in
[CI validation](docs/ci-validation.md); avoid redundant runs after they pass
unless a new change, failure, or unresolved concern warrants them.
Use focused local proof during development and hosted CI for integration.
Before pushing code, run `tools/preflight.sh /path/to/pinned/trb` so quick CI
failures that need only the reference compiler surface locally first.
During alpha development the PR gate is tiered: merge once PR acceptance passes;
`Main validation` then runs the deferred complete lanes. When it is red, fixing
or reverting main comes before merging more feature PRs. Run full local recovery
only to diagnose a recovery or platform failure. Until basic-language coverage is
complete, prefer coherent language families in reviewable PRs over separate PRs
for each implementation step; follow `CONTRIBUTING.md` for the cadence.

## Public repository boundary

Keep every outward-facing artifact explainable solely from public information
in this repository or other public sources. Never publish private names, URLs,
local paths, quotations, data, implementation details, or provenance. Use
self-contained generic reproductions, and inspect the complete diff and
outward-facing text before publication.

## Implementation invariants

- `type-rb/type-rb` at `TYPE_RB_REVISION` is the language and conformance source
  of truth. Preserve exact semantics, origins, and deterministic diagnostics;
  reject unsupported input instead of introducing a native-only dialect,
  unchecked fallback, or `Any` escape hatch.
- When reproducing the reference exposes an ambiguity, inconsistency or
  improvement opportunity, propose a specification change instead of deviating.
  Follow the development skill's specification proposal procedure.
- Keep the reference repository consumer-neutral. Native snapshot compatibility mappings, backend
  plans, pins, bridge compatibility and retirement conditions belong here.
  Reference changes must be justified by reference semantics, without Native
  terminology or consumer aliases. Follow `CONTRIBUTING.md` for that workflow.
- Write repository-owned compiler and runtime implementation in TypeRB.
  External backend/toolchain/system dependencies are allowed with explicit roles
  and costs. Ordinary self-hosting must reproduce from Native source; recovery
  through the Go reference compiler is separate evidence.
- Put TypeRB facts and optimizations in verified Native MIR and target-independent
  passes. Backend adapters consume them; they must not rediscover ranges, index
  validity, loop structure, call effects, Array-header stability, or GC safety.
  Finish registered direct-QBE experiments as migration evidence and remove
  superseded emitter ownership before broadening the fact family.
- Keep snapshots, MIR, ABI profiles, and runtime interfaces internal and unstable
  until an explicit promotion decision. Stable public CLI/ABI contracts and
  supported releases are goals; establish their compatibility and security
  policies as coverage and validation mature. Implement the dependencies in
  `docs/mir-consolidation.md` without claiming unverified support.

## Conditional project guidance

- For work taken from the project board, follow `docs/project-workflow.md`:
  implement only eligible Todo tasks under Active initiatives and claim before
  starting. Use closing keywords only for tasks the PR completes, never their
  parent initiative. Main failures take priority over feature merges; follow
  the workflow's completion and reopening rules. Its authority table lists what
  needs a maintainer decision. A direct request keeps its own scope and
  completion point. Apply its flow checkpoints at work start, waiting/handoff,
  merge and completion boundaries so that task acceptance, owners and next
  actions remain aligned with the delivered work.
- For language coverage, follow `docs/native-language-coverage.md`: pair basic
  syntax with MIR checks and verified compiler self-use. Extend the shared
  reference/Native case contract and regenerate the Capabilities detail view
  when ordinary behavior changes. Deferred optimizer acceptance does not block
  language work.
- Documents describe the current contract. Record how a change was delivered in
  its PR and issue; do not append per-feature or per-update narratives, dated
  status or hand-counted totals to docs. Regenerate generated views instead.
- For external dependency changes, update `docs/native-dependencies.json` and
  the applicable source pins in the same PR. Regenerate the GitHub Pages
  dependency register with `tools/dependency-pages.py --write` and verify
  `--check`. Distinguish current dependencies from candidates; retain target ABI,
  linked-library, configuration and license evidence with release artifacts.
- For source organization, follow `docs/repository-organization.md`, including
  recovery sources and symbols. At accepted optimization checkpoints,
  advance a bounded cleanup or record its concrete blocker within the task's
  scope. Preserve applicable recovery/measurement checks and immutable history.
- Before adopting new syntax in compiler source or changing seeds, follow
  `docs/bootstrap-seed-updates.md`. Verify actual published immutable assets
  before changing exact pins; unaccepted candidates and floating downloads are
  not replacement seeds.
- Current development follows `docs/mir-consolidation.md`: prioritize the complete
  MIR ownership milestone through cohesive changes. Integration uses explicit
  migration cost observations; temporary performance/size regressions are allowed.
  Keep correctness, safety and reproducibility blocking. Reserve detailed cost
  qualification for coherent milestones and use daily/weekly trends in between.
- For measurements, follow the active milestone policy and
  `docs/optimization-tradeoffs.md`. Retain failed observations and fixed baselines;
  report application and compiler/toolchain costs separately. Migration CI is not
  final performance qualification. Historical strict contracts stay explicit.
