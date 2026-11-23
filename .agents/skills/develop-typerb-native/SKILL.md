---
name: develop-typerb-native
description: Implement or review TypeRB Native compiler, runtime, recovery, and performance changes.
---

# Develop TypeRB Native

Complete the requested Native change with evidence appropriate to its affected
surface. Work within the task's stated endpoint and budget. A review-only request
stays read-only; a PR-only request includes the requested implementation and
verification, then stops before merge or further development.
For documentation edits, use this skill only when its Native-specific boundaries
or validation guidance are needed.

The project targets practical production use and complete TypeRB toolchain
coverage. Use `docs/mir-consolidation.md` for the goal, current scope and performance
qualification. Current unsupported features and unstable interfaces are gaps to
close; do not treat the project's former experimental scope as a feature ceiling.

Feature selection also follows the accepted
[v0.5.0 transition contract](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md): complete Go implementation functionality,
three source backends plus Native output, and practical application previews
within that release scope. Apply only the explicit approved deferrals in the
canonical manifest; keep their required companion diagnostics and all other
obligations. Deferred work is not a passing result. Use the shared contract inventory and existing
initiatives; do not infer full parity from language-case counts or duplicate
another owner's tasks. The retained Go implementation has no Native backend
requirement. Preserve current pins and quality gates until qualified cutover.

## Choose the relevant guidance

`AGENTS.md` owns repository-wide constraints. Read the matching guidance below
when the change needs it; do not load every reference before routine work.
All `docs/` and configuration paths below are relative to the repository root.

| Changed surface | Read when needed |
| --- | --- |
| Feature selection or coverage claims | `docs/native-language-coverage.md`; preserve ordinary check/build/execution/REPL versus recovery distinctions. |
| Work selected from the project board, waiting work or completion reconciliation | `docs/project-workflow.md` for eligibility, claims, WIP and flow checkpoints connecting PR evidence, next actions and initiative completion. |
| Compiler, MIR, runtime, driver, or target implementation | [Compiler constraints](references/compiler.md) and the decisions for the affected boundary. |
| Ordinary compiler source, recovery, self-hosting, or seed handoff | [Bootstrap and recovery](references/bootstrap.md), including recovery-enabled suites for compiler-source changes. |
| Source decomposition, names, or removal | `docs/repository-organization.md` and the affected consumer inventory; applicable recovery and cost checks still apply. |
| Performance experiments or retained results | [Measurements and retention](references/measurements.md); use `docs/experiment-plan.md` when defining a material experiment. |
| Reference pin or compatibility | `docs/type-rb-compatibility.md`, `docs/versioning.md`, and the cross-repository procedure in `CONTRIBUTING.md`. |
| Documentation, instructions, or metadata only | Check changed text, links, metadata and the applicable documentation CI. Compiler/bootstrap/benchmark runs are not implied. |

Read `docs/mir-consolidation.md` for the current larger milestone, permitted
intermediate cost regressions and integration/qualification distinction.
Use `README.md` for orientation when project context is missing. Small fixes and
documentation corrections do not need a new experiment proposal. For material
experiments and source-organization slices, use the owning plan's registration
requirements and establish scope and exit criteria before implementation.

## Propose specification improvements

Reproducing the reference exposes the specification in detail. When a contract
is ambiguous, contradicts its own documentation, differs between modes, is more
complex than its purpose needs, prevents an efficient implementation, or gives
poor diagnostics, propose an improvement:

- Keep implementing the pinned reference behavior. Do not ship a Native-only
  deviation while the proposal is open.
- Open a Specification proposal issue. Include a minimal generic reproduction,
  the observed behavior at `TYPE_RB_REVISION`, the proposed contract, the reason
  (clarity, portability, performance, diagnostics or simplicity), and the effect
  on existing programs.
- Set Attention to Needs decision. The maintainer decides whether to adopt the
  change and where it lands. Posting it to another repository also needs that
  decision.
- Link the proposal from the task, PR or scenario that found it, such as a
  `triage` expectation in a scenario manifest.

## Verify the affected surface

Colocate unit tests as `some_test.trb` beside `some.trb`, classifying them by the
contract they verify rather than their import count or setup machinery. Follow
`docs/compiler-source-layout.md` for feature/system suites and shared `testing`
helpers. Move unit tests with their owner; never import one test file from
another or allow production to depend on test helpers. Source staging and cache
selection must agree about these repository roles.

Use `docs/ci-validation.md` and the maintained commands in `CONTRIBUTING.md`.
The CI plan for the actual changed paths determines required authorities; do
not skip them because a change looks small. Complete focused local checks and
the required CI, fixing failures caused by the change. After they pass, repeat
or broaden checks only for new edits, failures, or unresolved concerns.
Run `tools/preflight.sh /path/to/pinned/trb` before each push of code; it runs
the reference-only quick checks, including the recovery snapshot subset, and
stops at the first failure.
Use hosted CI as the integration authority. Under the tiered gate, merge after
PR acceptance; `Main validation` runs the deferred complete lanes, and a red
main run takes priority over further feature merges. Run full local recovery
only to diagnose recovery or platform failures.
While a run is in progress, inspect completed failures as well as unfinished
jobs; an early CLI or target failure can coexist with a long-running Native job.
After changing checked binding or diagnostic behavior, run
`tools/check-conformance-sources.py .trb/bootstrap/core/compiler` after an
ordinary Native build for a quick conformance source scan. This script uses
the core compiler's `--source-content` protocol; the `bin/trbn` CLI does not
implement that protocol. Keep the full recovery run as the authority.
For basic-language coverage, combine syntax, checking, MIR, execution and REPL
into a coherent family rather than opening a PR for each internal step. A larger
PR with reviewable commits and complete acceptance evidence is appropriate.
When probing reference language semantics, run the exact `TYPE_RB_REVISION`
compiler used by compatibility checks; an installed `trb` may be a different
release and can disagree on check, generated code, or execution.
Use a Native compiler rebuilt from the active worktree, and compare all four
ordinary paths for new syntax. A cached compiler in a neighboring checkout
may accept or reject a different language contract.
When compiler imports or modules change, run `python3 tools/recovery_layout_sync.py --write`
and `--check` instead of editing the recovery layout, mutations or own-frontend
module list by hand.
Updating the generated import inventory alone does not change recovery
orchestration; validate it with the focused check and hosted full recovery.

For root TypeRB source checks, use the exact `TYPE_RB_REVISION` compiler:

```sh
trb fmt --check .
trb check --config recovery/trbconfig.jsonc
TYPE_RB_NATIVE_ROOT="$PWD" trb test --config recovery/trbconfig.jsonc
```

The default root configuration runs the example; root verification explicitly
uses `recovery/trbconfig.jsonc`, and the compiler suite uses
`compiler/trbconfig.jsonc`. For compiler-source changes, load the bootstrap
reference above: successful optional tests without the recovery/QBE environment
do not establish recovery coverage.

## Finish within the requested scope

At a waiting, handoff or completion boundary, apply the affected work's
`docs/project-workflow.md` flow checkpoint. Reconcile batch-PR acceptance and
open children before declaring an initiative complete; keep unresolved work
actionable without repeating unchanged reports or reopening closed experiments.

Report the implemented subset, meaningful validation, relevant measurements,
and remaining limitations. Fix routine issues within the authorized task
without introducing a new approval checkpoint. If a skill instruction requires
a pause, identify the exact file and instruction, and explain the unresolved
decision; do not turn an optional recommendation into a mandatory approval.

Board work that preserves specified behavior and the approved scope
needs no confirmation for routine steps. This includes reference conformance,
MIR and performance improvements, and maintenance. Merge green PRs, clean up,
update the reference pin through its procedure, mark an initiative Done when
current evidence meets its recorded completion criteria, activate the next
Approved initiative in a free slot, and continue. Follow
`docs/project-workflow.md` for evidence and lifecycle rules. Stop only for its
listed maintainer decisions, such as a specification change or retargeting an
initiative.

For performance and maintenance candidates, assess measured benefit, code quality
and maintenance cost together. When the evidence supports an overall improvement
and required correctness and CI pass, record the trade-off and merge without
another confirmation even if numerical targets remain unmet. Simpler code can
justify little or no speed gain; added complexity needs a useful compensating
benefit. Follow `docs/optimization-tradeoffs.md` for the adoption record. Preserve
misses, unknowns, thresholds and initiative completion criteria.

When the user has authorized continued development, pursue the active milestone
through cohesive ownership changes within that authorization and budget. Avoid
turning each helper move or size overrun into a separate acceptance milestone.
Honor later limits such as one small task or stopping with an open PR. Use
issue-closing keywords only when the reviewed PR completes the Task or Bug;
never closing-link its parent initiative. Follow `docs/project-workflow.md` for
closure and reopening after a revert. Even a negated closing keyword can close
an issue on merge.
