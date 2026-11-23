# Validation by changed surface

The `Pull request validation` entry has no path filter. Its planning job tests
and executes `tools/ci-plan.mjs` against the complete merge-base-to-head delta,
including deletions and both sides of renames. Git paths and change statuses are
streamed with NUL separators; large evidence inventories, unusual filenames and
unknown paths cannot silently truncate or bypass validation. Git failures reject planning.

## Active MIR integration mode

[Issue #439](mir-consolidation.md) establishes an explicit `mir-migration` mode
for PR/main integration. All applicable correctness, recovery, target identity,
process, memory-lifetime, sanitizer and cleanup jobs remain required. Compiler
size is recorded as an observation without a limit, and smoke-time limits write
observations with their actual exceeded status; neither stops correctness
verification. Standalone tools default to `strict`, and unknown modes or
malformed measurements fail. Migration changes omit the amd64 repeated formal
timing series; binary-format, dependency, generation, output and failure checks
still execute.

The strict comparative cost contract (the static String compactness workflow,
the MIR transition size markers and the runtime A/B controller) is retired; see
[retired experiment controllers](retired-experiment-tools.md). Daily and weekly
measurements track compiler and application costs. Green CI must not be
reported as a performance-qualified result.

## Tiered gate during alpha development

`NATIVE_CI_GATE: tiered` in the PR workflow lets ordinary compiler, CLI and
conformance edits merge after the pre-merge lanes: planning, quick (including
the snapshot-v4 subset check), complete non-recovery compiler units, documentation,
tooling, the CLI build on Darwin
and Linux arm64 (including the conformance source scan), the Ubuntu x64 target
and memory. Native recovery, CLI cache invalidation and the arm64 regression
then run in `Main validation`.

The planner output `complete` keeps those lanes before merge whenever a PR
touches a path only they execute: workflows and `tools/ci-*`, root recovery
sources other than the generated import and mutation inventories, fixtures,
corpora, benchmarks, compatibility pins, bootstrap and cache inputs, target
scripts, or any unknown path. Main always plans with the complete gate. Set
`NATIVE_CI_GATE: complete` to restore complete pre-merge validation for every PR.

A red `Main validation` opens or updates the `Main validation is failing` issue
and closes it after a later run passes. Fix or revert main before merging more
feature PRs; development on branches continues. Release, seed and milestone
decisions use a main commit with a passing `Main validation` run.

For an ordinary main push, the expensive per-module recovery mutations run for
compiler modules changed since the last passing main validation. The canonical
compiler emission, missing/malformed controls for every module, all other
recovery stages, and the independent compiler suite still run. An unknown source
path or recovery-harness change selects every module. A scheduled main run each
day selects every module and all complete lanes, so unchanged modules retain a
daily mutation check. PRs that require complete recovery also select every
module. The selected module names are recorded in the main plan output.

## Linux prerequisites

Main and PR CLI, target and memory jobs verify their required host tools before
building. An installed unversioned LLD remains selected; when it is absent, the
Ubuntu 24.04 LLD 18 executable can supply a job-local `ld.lld` alias. This uses
the same major version selected by Ubuntu's `lld` metapackage. No system alias
is overwritten. Only missing tools or sanitizer archives require package
acquisition through the existing OS repositories. Failed index downloads stop
preparation instead of accepting stale indexes or silently retrying.
Privileged acquisition runs under its own watchdog inside `sudo`, before the
outer controller's timeout, so root-owned package workers are also bounded.

The prerequisite receipt retains resolved tool paths, versions, commands and
failures even when a later build cannot start. A real C link/run verifies LLD,
and memory jobs also compile and execute an AddressSanitizer probe. Existing
Native, process-trace, memory and sanitizer checks still run independently;
prerequisite success is not Native qualification.

## Required authorities

| Changed surface | Required PR validation |
| --- | --- |
| Markdown, development metadata, static documentation, registered results and exact documentation generators | Planning and documentation |
| The two exact planning files | Their unconditional planning tests and documentation |
| Exact synthetic tool-test files listed in `toolingTests` | Planning and macOS tooling; project/policy shell tests also run Linux quick tooling |
| Existing compiler unit-test modules listed in `compilerTestInputs` | Complete compiler units, quick, Native, CLI, tooling and target correctness; no unchanged-binary worker-memory measurements |
| Existing language registry and its two exact generated views, modified in a tiered PR | Planning, reference language quick checks, documentation and unchanged Darwin/Linux CLI builds; no complete compiler units |
| Exact CLI adapter, launcher, build helper and CLI-test inputs listed in `cliInputs` | Planning, quick, complete compiler units and Darwin/Linux CLI artifacts |
| Ordinary compiler, conformance, execution workflows and measurement policy | Full applicable correctness, tooling, CLI, target and memory authorities |
| Other code or unknown files | Complete Native correctness, tooling and CLI/target checks; memory according to the conservative rules in the planner |
| Mixed changes | The union of applicable authorities, with core changes restoring the core lane |

These are exact allowlists for executable exceptions, not filename suffix rules
for arbitrary tests. A new CLI file or neighboring tool defaults to the code
lane until its consumers and executing authority are reviewed. Production
measurement controllers, toolchain pins, suite controllers and stage-recording
code retain code validation. Tool-test-only routing is valid because the
independent tooling job actually executes each listed test. Suite-controller
and workspace-ownership tests run on Linux in unconditional planning and on
macOS in tooling, moving the former Native step without duplicating it.
Project and transition-policy shell tests also retain their Linux quick-check
authority in addition to the macOS tooling checks. For changes limited to those
tests, quick skips reference checkout/build, compatibility, formatting/types and
TypeRB units; the Linux shell tests need none of those compiler inputs.

The eleven reviewed compiler unit-test modules are excluded from ordinary
reference/Native compiler builds and CLI source staging. Changing only those
modules cannot change the measured compiler or worker binary. Full correctness
and target checks still execute; only worker-memory measurements are omitted. Conformance fixtures, new test paths, project configurations,
production source and measurement policies retain conservative routing. A mixed
non-exempt code change restores the previous compiler measurement requirements;
changing routing/execution workflows still exercises the full graph.

CLI adapters are outside the ordinary compiler source closure. The CLI workflow
builds the current core from the pinned Native seed, verifies fixed points, runs
the CLI/REPL test scripts listed in `.github/workflows/native-cli.yml` and
packages Darwin/Linux artifacts. CLI-only changes still run reference
formatting/type checks and quick tests; changing core source alongside an
adapter restores the core lanes.

The tiered PR gate has one narrower registry exception. A modification to
`tools/native-language-cases.json`, alone or with modifications to
`docs/native-language-feature-inventory.md` and
`docs/capabilities/ordinary-language.js`, sets `registry_only`. Every changed
path must match this list and have Git status `M`. Additions, deletions, renames,
type changes, missing change statuses and any other mixed path keep ordinary
routing. Complete-gate PRs and main never use the exception.

Registry-only quick retains the pinned reference build and identity checks,
registry validation, reference AST coverage, every reviewed check/build/execute/
REPL observation and generated-view checks. It skips unrelated compiler-source
formatting/types, snapshot, root/MIR, recovery-layout, tooling and project-scenario
checks. The complete compiler unit job is skipped. Documentation and the existing
Darwin/Linux CLI jobs remain required, including fresh Native fixed points and
all Native language observations. CLI workflow scope and main/daily validation
are unchanged. Acceptance rejects missing or inconsistent registry-only outputs
and missing, failed or skipped required jobs.

The CLI smoke (`build`) and cache-invalidation (`cache`) jobs run independently
on Darwin and Linux arm64, so a failed smoke does not require rerunning the
cache job. The build job verifies a fresh core and CLI fixed point. The cache
job checks core and CLI content-key invalidation through the build's `--plan`
path, then performs one real CLI-only rebuild under concurrent callers to
verify core reuse and atomic publication. Its rebuild has a 600-second
watchdog and both jobs have a 90-minute limit; a timeout is a failure and
terminates the owned builder process group. Cache reuse, invalidation, failure
atomicity and concurrent-caller assertions remain required.

The documentation authority checks evidence retention, skill metadata, public
path hygiene, the capability catalog and benchmark explorer. Pages does not
repeat those checks in a separate PR workflow. Its main-push deployment and
manual publication workflow retain verification before upload/deploy. Changing
the Pages workflow requires documentation validation, not compiler benchmarks.

## Scheduling and acceptance

1. **Plan and quick feedback.** Planning tests are unconditional. Code or CLI
   changes build the pinned reference compiler, validate canonical compatibility
   metadata, formatting/core types and root/focused MIR units. Quick also replays
   the shared language cases and the frozen project scenario manifests against
   the pinned reference and requires their recorded reference outcomes. No complete
   recovery or comparative claim comes from quick feedback. The explicit `quick`
   output also selects Linux-only tooling steps for project/policy test edits;
   code and CLI plans require complete quick feedback except for the exact
   registry-only route described above.
   The independent `Complete compiler units` job runs the entire
   `trb test --config compiler/trbconfig.jsonc` suite on Darwin arm64 with the
   exact reference pin and QBE, without recovery, alongside quick. The runner
   matches the compiler tests' arm64_apple assembly and linker assumptions.
   It compiles the full test executable once, then runs every discovered test
   source with two bounded workers and isolated temporary directories. The
   measured long-running files start first to avoid an idle worker at the end;
   these scheduling hints never restrict discovery or skip renamed/new files.
   The suites are split by their guarantees into colocated unit tests and feature
   or compiler system tests; each discovered source runs exactly once.
   Structured events must
   report every selected test passing, with no missing or duplicate identity.
   It retains the full log and compile, execution and total times as an artifact.
   Its success is required for every code or CLI
   PR, including drafts, except for the registry-only route; planning rejects
   missing or malformed routing.
2. **Independent tooling.** The former Native `Verify bootstrap seed tooling`
   commands run unchanged on macOS in the separate `CI tooling controls` workflow.
   Its synthetic checks need no compiled candidate, so it can start after
   planning without waiting for quick feedback. This removes it from the serial
   path before recovery. Recovery-artifact consumers remain in the Native job after recovery joins.
   The retired source-era benchmark controllers are no longer rebuilt or tested
   against current source; see [their preserved versions](retired-experiment-tools.md).
3. **Correctness and CLI.** On ready PRs, the planned Native, target, memory
   and CLI jobs start alongside quick after planning; under the tiered gate
   they run in their pre-merge scope unless `complete` is planned. Quick failure
   still fails acceptance, even if the other jobs succeed. Parallel startup saves
   wall time on passing PRs but may use more runner time on a failing quick job.
   Development drafts retain source/CLI checks in quick feedback and defer the
   complete jobs.
   The project-test contract runs against the pinned reference in quick feedback
   and against the ordinary Native compiler in both CLI artifact jobs. Its
   reviewed manifest preserves exact failure categories and source locations;
   unrelated compilation errors cannot pass negative cases. The approved
   name-filter deferral is reported separately and never counts as passing
   parity. Additional CLI and value controls exercise selection, event order,
   continuation, standalone output preservation and managed assertion payloads.
   `fixtures/projects/native-test-values.json` freezes reference value events
   with the exact pin and source hashes. Quick feedback replays every group
   against the reference; CLI jobs verify every reviewed group without a
   reference executable. Source or pin changes require a new reference replay
   and review of the recorded observations before updating this contract.
4. **Acceptance.** `Native CI acceptance` checks every planned authority,
   including tooling and CLI. A tiered PR summary states that the deferred
   lanes run on main after merge. Failed, cancelled, missing or skipped required
   jobs reject acceptance. Unexpected execution of a disabled authority also
   rejects the plan/result mismatch.

Drafts receive quick, applicable tooling and documentation feedback. Their
acceptance job succeeds only when those selected jobs pass and the complete jobs
are skipped; its summary explicitly says this is partial draft feedback. GitHub
does not allow a draft PR to merge. Marking ready triggers complete validation;
converting back to draft cancels the old run. New commits cancel superseded PR
work. Cancelled measurements are not accepted results.

Develop complete language families with focused local units, reference/Native
positive and negative cases, MIR ownership and retained REPL checks. Include an
ordinary Native build when compiler source changes. Hosted CI is the complete
integration authority: PR acceptance for the planned lanes, then `Main
validation` for deferred lanes. Full local recovery is only for
recovery/platform diagnosis, including a red `Main validation`.
Scheduling-only workflow changes run controller tests and hosted full CI; local
recovery does not exercise changed job dependencies or draft reporting.
Optional local suites without recovery variables remain partial evidence. Do
not merge while required PR checks are pending. Merge each coherent PR once its
acceptance passes instead of stacking dependent PRs behind a long run.

`Main validation` runs the applicable Native, target, CLI, memory, tooling and
documentation lanes on main. It plans the two-dot delta from the most recent
main commit whose `Main validation` passed (the empty tree when none is an
ancestor), so a failed or skipped run's changes are planned again until a run
passes. One push run executes at a time; a newer pending push replaces an older
pending one, which batches rapid merges without losing their changes. Scheduled
full runs use a separate concurrency group so a push cannot replace them.
The reusable CLI workflow also separates events, preserving both Main runs and
isolating manual CLI runs. Only superseded PR or manual CLI runs cancel active
CLI work; Main serialization remains with its caller workflow.
Unknown or invalid revisions fail planning rather than skip checks. Manual
workflow controls remain available.

Full multi-language benchmark refreshes remain manual. During MIR migration
they supply milestone qualification separately from integration correctness;
ordinary compiler edits do not wait for detailed costs.

The representative application suite follows the
[performance policy](performance-policy.md). The existing Native CLI build job
builds its shared workload once and checks small exact-output inputs within a
two-minute step deadline. The existing tooling job runs synthetic controller
tests. Neither job compares speed or builds comparison compilers. The broad
input matrix remains opt-in local/milestone work; daily/weekly schedules and
their workload counts are unchanged.

## Focused compiler preflight feedback

Before making a compiler PR ready, use the existing standalone workflows when
a target-size or compiler-cost question needs earlier feedback. Do not dispatch
another copy of a comparison that is already running for that exact candidate.
[Issue #425](https://github.com/type-rb/type-rb-native/issues/425) tracks this path;
it changes no acceptance dependency or measurement limit.

Start from a clean, committed and pushed PR branch. Record its exact commit and
the baseline chosen in the registered comparison; do not replace that baseline
with a newer main revision between observations. The baseline must be an ancestor
of the candidate. Local ordinary fixed points and relevant correctness checks
remain prerequisites; record which full authorities are still pending.

```sh
test -z "$(git status --porcelain)"
candidate_ref=$(git symbolic-ref --short HEAD)
candidate_revision=$(git rev-parse HEAD)
test "$(git ls-remote --heads origin "refs/heads/$candidate_ref" | cut -f1)" = "$candidate_revision"
```

For a cost comparison, set `baseline_revision` to its registered full commit ID
and verify `git merge-base --is-ancestor "$baseline_revision" "$candidate_revision"`.

For the Linux amd64 artifact and its arm64 target-neutral control, dispatch the
existing target controller:

```sh
gh workflow run linux-amd64-targets.yml --ref "$candidate_ref"
```

Select the newly created dispatch by its run ID, then use
`gh run view RUN_ID --json event,headBranch,headSha,status,conclusion` to verify
that it is a manual run of the expected candidate commit. If the branch moved
or several runs are ambiguous, resolve the identity before interpreting results;
do not dispatch again merely to obtain an easier match. Retain the run ID,
verifier/policy revision, baseline and toolchain identities with the evidence.

Inspect completed target artifacts as soon as they are available. Report actual
fixed-point executable sizes and their enforced limits: shorter source or QBE
Strings can compress less well and produce a larger compiler. For latency work,
record the first actionable failure time separately from full-suite completion.
Host-local or emulated checks are diagnostics, not hosted runtime comparisons.

Retain failed and partial evidence and clean only owned workspaces. Apply the
registered run budget rather than retrying an unchanged failure. Passing this
preflight does not make a draft mergeable or replace fresh complete PR acceptance;
the formal multi-language benchmark remains a separate manual operation.

## Recovery suite

`tools/ci-run-suites.mjs` runs the root and compiler recovery suites as two owned
process groups with separate logs and `status.json`; one failure keeps the
peer's evidence, and cancellation terminates only owned process groups. The root
suite records ordered phase receipts through `tools/recovery-stage.py`; missing,
reordered or incomplete receipts reject an otherwise successful run. These are
diagnostic phase durations, not controlled measurements.

The B0-to-B3 recovery and B1-to-B4 ordinary generation chains stay sequential.
B0 selects its bootstrap emitter before generating QBE; it does not first
generate and discard ordinary managed output. Its emission, QBE and link
durations are printed separately. Source-closure preparation validates every
module path, creates the distinct parent directories in one command, then
writes the sources only after directory creation succeeds.
Independent generation commands and per-module boundary controls run with
`TYPE_RB_NATIVE_RECOVERY_JOBS` concurrency (default `2`, accepted `1` to `4`;
hosted CI uses `4`). Each module keeps its missing-module and malformed-module
checks; selected modules also run the QBE mutation check. Each root invocation
prints per-module preparation, emission, missing/malformed and total wall
durations in inventory order, including total time for failed controls. A negative
duration marks an unavailable or unselected phase; totals include assertion/cleanup
overhead and parallel durations must not be summed as stage elapsed time.
The complete module controls remain required. Each root invocation
owns a fresh `mktemp -d` workspace
validated by `tools/recovery-workspace.mjs` before later consumers use it and
before cleanup. Do not remove generation identity, mutation or differential
checks merely because they are slow; measure phase costs before rescheduling.

## Protection and review

The main ruleset requires the uniquely named `Native CI acceptance` check from
GitHub Actions. Existing PR-only, no-force-push and no-deletion rules remain.
Do not require conditionally omitted workflow names individually or interpret
a green skipped job as accepted compiler evidence. Review relevant base changes
before merging; update and revalidate when correctness, measurement baseline
or the CI contract changes. An unrelated documentation merge alone does not
invalidate the compiler evidence.

Routing and tests cannot replace review of semantic proof boundaries. Preserve
raw MIR verification, mutation/effect exclusions, negative cases, compactness
and measured thresholds before accepting a compiler optimization.
