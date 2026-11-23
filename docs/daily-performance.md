# Daily performance and benchmark strategy

Status: adopted for the daily diagnostic layer. Existing correctness, safety,
self-hosting, compiler cost and optimization acceptance authorities are unchanged.

The [performance engineering policy](performance-policy.md) defines broader
application coverage, input sensitivity, measurement interpretation and CI cost
budgets. Its opt-in representative suite has a separate result series and does
not expand this daily job or change its existing scores.

## Purpose and cadence

The primary benchmark page answers what has changed, which supported workload
areas remain slow, how the identical TypeRB source compares through the Go backend, and how
three numeric kernels compare with the registered hand-written Pure Go programs. It is not a language ranking. Daily data does not need a new complete
seven-implementation cohort or a manual prose update.

- PRs retain their applicable acceptance checks. A daily snapshot is never a
  dependency of PR acceptance, merge, cleanup, or starting the next task.
- `daily-performance.yml` checks main at 21:23 UTC (06:23 JST) once daily. GitHub
  may delay scheduled jobs. Relevant unchanged sources skip measurement. The
  independent state upload is renewed even on unchanged days.
- Manual dispatch can force an immediate refresh. Only main can measure and
  publish. One run may be active; pending requests coalesce through Actions
  concurrency without cancelling the active cohort.
- `weekly-performance.yml` measures Native, Pure Go, C, C++, Rust and Java on
  the three daily numeric inputs, at 20:23 UTC Saturday (05:23 JST Sunday),
  only after relevant changes or a forced manual dispatch. It is independent of
  the daily workflow, with its own state/evidence artifacts and 25-minute limit.
- Complete Benchmarks Game comparisons remain manual, on a separate page.
  Run one for a concrete cross-language or large-input question, or before a
  performance claim that needs it. There is no required monthly refresh.
- Long-lived memory, worker stability and sanitizer checks keep their own
  existing triggers. Short speed measurements cannot replace them.
- The separate [weekly development progress report](../tools/development-progress/README.md)
  displays reviewed language-probe gaps, recent PR CI feedback time, the latest
  daily compiler self-build observation and same-cohort Native/Pure Go ratios.
  It compiles no programs and sets no acceptance threshold.

The first four-role run completed in about 5 minutes 40 seconds, with 2 minutes
10 seconds spent measuring. It shared current/previous compiler preparation;
subsequent runs can require more preparation. Adding Pure Go has a provisional
one-minute incremental budget, to be checked with a real run.

The operational target remains a 10–15 minute daily update, with a 25-minute job safety
limit. This is an operational budget, not a duration guarantee or a
requirement to wait after merging. Calibrate using the first real daily runs;
reduce redundant work before broadening the suite.

## Coverage

The machine-readable contract is `tools/daily-performance/suite.json`.
Numeric and runtime workloads reuse the existing TypeRB bodies and checked-in
output oracles:

| Area | Workload | Daily input |
| --- | --- | --- |
| Integer, branches, Array updates | fannkuch-redux | 10 |
| Float operations | n-body | 1,000,000 |
| Numeric loops | spectral-norm | 5,500 |
| Array traversal | Integer reduction | 100,000 elements, 1,000 rounds |
| Allocation and GC | memory workload | one phase, 5,000,000 iterations |
| String/Array/state interactions | worker workload | one phase, 40,000 batches |
| Fresh-process startup | minimal CLI | one output line |

Memory and worker inputs change only their single registered invocation.
Separate untimed Native probes report whether automatic GC actually occurred.
Their runtime observations have instrumentation disabled. A missing GC signal
is a coverage gap, not evidence of healthy collector performance.

Language-area workloads live in `benchmarks/features/`. Each is a small,
deterministic program for one ordinary-language family, with a checked-in
output and a hand-written Pure Go program using the same algorithm. Its
`family` names the matching entry in `tools/native-language-cases.json`, so the
benchmark page shows each area's assessment next to that family's language
coverage. They use features newer than the frozen Native baseline and set
`frozenBaseline: false`; they compare with the previous measurement, TypeRB Go
and Pure Go. Add a workload only when ordinary Native support for its features
exists and both TypeRB outputs match the Pure Go output; do not extend language
semantics just to port a benchmark.

JSON and varied working sets are covered by the separate
[application workload view](capabilities/benchmarks/representative.html), whose
measurements run on demand rather than daily. I/O and service latency remain
coverage gaps. The
compiler's own bootstrap cost remains separate from the application's
clean-output build timings and sizes shown here. Its daily self-compilation
measurement is reported separately.

## Comparison and measurement

Four TypeRB roles run serially on one logical CPU of a fresh Linux arm64 hosted runner:
current Native, the previously measured Native revision, a frozen Native
baseline, and the exact compatible TypeRB Go revision in `TYPE_RB_REVISION`.
These four roles compile the same TypeRB source bytes. Each Native chain is closed through
the existing bootstrap verifier from an exact published seed. Identical role
revisions share a compiler preparation inside one run. The frozen source retains
its compatible `bootstrap-seed-2026-09-09-record-arrays` input because current
seeds no longer implement its historical compiler intrinsic names. Both published
seed packages receive the same manifest and attestation verification. The
preparation evidence records each input hash and preserves generation stdout,
stderr and process traces, including failures. This does not replace the frozen
source or skip its subsequent ordinary generations and correctness checks.
Preparation explicitly selects
`bootstrap-seed.sh --measurement-policy diagnostic`: the Native generations,
fixed-point/QBE identity checks, valid and invalid corpus, process/tool boundaries,
cleanup controls and size observations still run. It omits the separate legacy
compiler timing series (four warmups and fourteen retained self-builds for a
transition input). Application build/runtime observations below are unchanged.
The evidence marks these compiler medians as excluded, records total preparation
time per revision, and includes `diagnosticOnly` metadata that release packaging
rejects. Standalone seed validation keeps its default measurement policy. If no
compatible previous snapshot exists, that role uses current Native and the page
shows no previous comparison. The frozen baseline is not automatically advanced.

Pure Go additionally runs fannkuch-redux (10), n-body (1,000,000) and
spectral-norm (5,500), using exactly the registered upstream sources in
`benchmarks/benchmarksgame/pure-go/`. It uses the same installed Go toolchain as
the TypeRB Go backend, `go build -trimpath`, the same output oracles, CPU and
sample policy. All measured programs have `GOMAXPROCS=1`. Source SHA-256,
upstream revision, Go version and tool executable identity accompany the data.
This comparison includes implementation differences; it is not a general language
ranking. The other four cases explicitly have no Pure Go comparison.

The page uses buttons for metrics and comparison targets. Each workload shows
its numbers and a metric-specific assessment (faster/slower, less/more memory,
smaller/larger). Differences under 5% are described as about the same, without
claiming statistical equivalence. Missing, failed and below-resolution values
never receive favorable assessments. Same-run Native/Pure Go ratios are available
in trends; old snapshots without Pure Go remain valid historical evidence.

Runtime uses one warmup and five retained fresh processes. Application builds
use one warmup and three retained clean-output builds with a warm Go build
cache; this is not a cold-cache build claim. Role order rotates each round.
Filesystem caches stay warm, swap is disabled, and measurements do not run
concurrently on the measurement host. Warmup in a separate process does not
warm a later process's JIT or heap.

Every successful application build, including the warmup, retains its complete
output in the evidence artifact before the next build can overwrite it. Raw
build observations record its SHA-256, byte count and path relative to the
measurement directory under `artifacts/sha256/`. Identical bytes share a stored
file; differing outputs remain separate, without normalization. Retention runs
outside the timed interval and a storage failure aborts snapshot publication.
Failed builds retain their original logs and status without borrowing an earlier
build's artifact. Runtime and GC observations identify the final build's
unstripped executable; `program.stripped` is a separate size-only copy. This
provenance does not change measurement or classification, and older snapshots
without these fields remain readable.

After workload measurement, the current Native compiler emits QBE for its
same-revision compiler source once, recording wall/CPU time, peak RSS, byte size
and SHA-256. It then builds that source three times (one warmup, two retained)
with the registered QBE and C compiler, recording the retained median and range
of the full build cost. Output binary size and hash must agree across all three
builds. This is self-compilation telemetry, not the multi-generation bootstrap
or a Pure Go comparison. Each emission has a 90-second timeout and each build
a 120-second timeout; failure is published explicitly. The large QBE text and
repeated binaries are removed after their hashes and sizes are recorded, while
timing logs remain in the 90-day evidence artifact. The daily page shows the
latest values and up to ten recent observations; old snapshots without this
field remain valid.

Wall time includes launcher overhead. GNU time records CPU and maximum RSS;
RSS is not a simultaneous sum of a process tree. Short startup CPU/RSS values
can be below resolution and appear unavailable. Startup wall values are a
launch-inclusive diagnostic, not isolated language startup latency.

Every runtime observation checks exact output, empty stderr and exit status.
Build failures, output differences and timeouts are published as failed cases,
with no passing runtime median. A timeout stops further observations of that
role/case, preserving the failure and continuing other cases. The controller
kills its owned process group on timeout. A complete numeric regression still
publishes normally. Infrastructure failure keeps the last complete snapshot
and publishes the failed attempt when state is available.

Five percent is a visual screening band only. It changes no acceptance bound
and is not a significance test. Small changes require focused, prospectively
bounded A/B confirmation. Preserve all samples; do not repeat until passing.
Minimum/maximum retained wall times help expose noise. Trends show ratios to
the frozen compiler measured in the same cohort, not causal differences between
absolute timings from different hosted machines.

## Publication and retention

The daily workflow has no repository write permission. It uploads a compact
state artifact and raw evidence. The existing Pages workflow runs after the
daily workflow completes, reads data only from this repository's main-branch
daily workflow, validates it, and composes a static deployment. Source changes
still go through PRs; daily measurements do not push to main or create data PRs.
Unrelated Pages updates restore the daily state instead of resetting the page.
Main pushes also run only the lightweight Pages composition, so newly merged
performance changes can be labelled unmeasured immediately without starting a
compiler or benchmark. This deployment is not a merge-completion dependency.

The state carries the last snapshot, latest attempt and up to 60 historical
snapshots. Raw measurements, derived source, toolchain identity, compiler closure
and process evidence are retained as Actions artifacts for 90 days. The page
links each run; old raw artifacts expire. Promote evidence through the existing
result archival policy before making a durable formal claim.

Workload/source/oracle/controller changes start a new measurement series.
Only the current series is graphed together. Performance input fingerprints
include compiler/runtime sources, compatible compiler pins, daily tooling and
reused workload sources; prose-only and compiler test-only edits do not require
another timing run. Failed API retrieval does not masquerade as empty history.
If no retained state exists, the page explicitly starts without daily values.

After the first bounded adoption period, review runtime cost, false alarms,
detected regressions and decisions informed. Replay a known regression/fix pair
and use identical-binary comparisons before relying on this diagnostic for small
changes. This calibration is separate from formal acceptance; the daily page
does not claim it has already established a statistical detection guarantee.

## Weekly external comparisons

The weekly page is a small-input diagnostic covering runtime and peak RSS, not
a replacement for the large-input formal cohort. Each run measures all six
implementations on the same host and one logical CPU. It reuses the upstream
archive and per-program checksums registered for the formal comparison, including
its C `-O3`, C++ `-O3`, Rust optimization level 3 and Go `-trimpath` policies.
Tool versions and executable hashes are recorded. Java uses the installed JDK 21
and `-XX:ActiveProcessorCount=1`; fresh-process JVM startup and JIT are included
in every sample, so this is not steady-state service throughput.

One warmup and five retained runtime observations use the same daily inputs,
output checks and 30-second per-process timeout. Runtime roles rotate each round.
Four clean-output build observations (one warmup, three retained) remain raw
evidence; weekly build timings and artifact sizes are not compared on the page.
Compiler warnings remain in build logs; runtime stderr must be empty. Failed
cases stay visible. Infrastructure failures retain the previous snapshot.

Weekly source/state artifacts use `weekly-performance-evidence` and
`weekly-performance-state`; retention matches daily (90 days of raw artifacts,
up to 60 state snapshots). Pages restores each workflow's state separately and
shows the weekly date/revision on the daily page. Weekly completion cannot
replace daily results or their failure status.

If optional weekly artifact retrieval fails during Pages composition, the weekly
section falls back to the last retrievable public weekly snapshot with an explicit
warning. If neither source is available it shows a pending/ unavailable state.
This fallback never writes workflow state and does not block daily publication.
