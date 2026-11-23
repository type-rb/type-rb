# Performance engineering policy

## Goal and scope of claims

TypeRB should be competitive with established fast implementations in ordinary
applications, across representative inputs and sustained workloads. Existing
benchmark wins are useful, source-qualified milestones. They do not establish
that every program, input size, architecture or metric is faster.

Keep three complementary views:

1. Small language/runtime workloads diagnose mechanisms and detect regressions.
2. Input sensitivity checks vary sizes, distributions and object lifetimes.
3. Application workloads combine parsing, allocation, data structures, error
   handling and output, with end-to-end costs visible.

Select workloads from intended uses and observed user problems before inspecting
performance results. Record uncovered use cases, including unsupported features.
Do not select or remove cases because a particular implementation wins or loses.
Update the corpus as actual usage changes, preserving historical suite versions,
inputs, outcomes and source identities. New coverage has its own result series;
it does not rewrite the daily suite or a completed qualification.

## Representative coverage

The opt-in [representative suite](../tools/representative-performance/README.md)
adds three application-shaped workloads: log parsing/grouping/report generation,
JSON parsing/transformation/serialization, and a persistent stateful worker.
Its sensitivity matrix varies Hash size and lookup hit rate, UTF-8 string length,
Array size/order/duplicates, and allocation lifetime/live-set size.

These are reproducible synthetic workloads. Input preparation occurs at runtime;
size and distribution are executable arguments, so each case uses the same
compiled program. Small, medium and large refer to this suite's registered
sizes, not universal definitions of a large production working set.

The first application cases prepare in-memory inputs and write checksums. File
and network input, service queueing/latency, parallel scaling, production traces,
and additional architectures remain coverage gaps. Extend these when ordinary
language and package support exists; do not expose compiler-only intrinsics or
change language semantics to make a benchmark executable. Existing memory and
worker soaks separately check long-lived resource stability. The representative
suite does not replace them.

## Comparisons and metrics

Use equivalent inputs, observable outputs, error behavior, numeric precision,
lifetime requirements and concurrency. A same-algorithm comparison diagnoses
compiler/runtime behavior. An application comparison also permits idiomatic,
efficient standard-library use in each language. Label which question a control
answers; do not deliberately disadvantage controls or present library differences
as isolated backend effects. Go is the initial application control; extend Rust
and other implementations after the application contracts stabilize. The
existing cross-language numeric suite remains independently useful.

Retain exact commands, sources, compiler/tool identities, environment, outputs
and every observation, including first processes, failed cases and timeouts.
Interleave comparison roles on the same host. Keep instrumentation probes
separate from ordinary timings. A local diagnostic does not establish a hosted
or cross-platform victory. Statistical uncertainty and below-resolution values
remain unknown. Never retry unchanged valid observations until a desired result
appears; investigate a measurement defect before registering a corrected run.

Report end-to-end wall time, CPU, throughput and peak RSS independently. Keep
application build and compiler self-build costs separate. For repeated work,
report the observed distribution and worst observation, not only an average.
Sparse samples cannot support tail estimates. Observer-side batch completion
intervals include output/pipe/scheduling overhead and are not request latency.
Coalesced output invalidates these interval estimates. Request p95/p99 requires
a separate load test with request boundaries, load/queueing policy and enough
observations; process runtime percentiles cannot substitute for it.

Show individual cases and the slowest relative cases alongside any aggregate.
Several large wins must not hide a substantial regression elsewhere. Coverage
and comparative performance are separate outcomes: correctly measuring a slower
case successfully closes a coverage gap but does not establish a speed win.

## Development and CI cost

Develop cohesive changes and investigate relevant cases locally. Publish one
validated batch instead of waiting for a new PR/CI cycle for each helper or
measurement. Existing required correctness and integration authorities remain.

| Trigger | Additional performance coverage | Blocking role |
| --- | --- | --- |
| Ordinary PR CI | Small exact-output smoke using the already built Native CLI; synthetic controller tests | Correctness only; no speed comparison |
| Performance-related local development | Affected inputs and controls | Evidence for the proposed change |
| Existing asynchronous daily/weekly measurements | Existing representative trend cases | Investigate material changes; no new per-PR timing gate |
| Cohesive milestone or release preparation | Full input matrix, application cases, sustained loads and applicable platforms | Required evidence for the claim being made |

Keep incremental ordinary CI cost near zero, with one to two minutes as a design
budget for new checks. Reuse the existing compiler artifact and build the shared
workload executable once per target. Do not add compiler generations, comparison
toolchains, large inputs, repeated timing runs or schedules to ordinary PRs.
Record smoke elapsed time and fail on timeout or incorrect output; its wall time
is an operational observation, not a competitive performance threshold.

New broad coverage initially runs locally or explicitly at milestones. Promote
only a small set of cases with demonstrated regression-detection value into
continuous measurement, within an explicit runtime budget. Adding a benchmark
does not automatically add it to daily or weekly CI. Inspect noisy alerts and
profile reproducible regressions before adding another expensive check.

### Publication is independent of measurement cadence

The benchmark Pages application view lists every registered representative input,
including unmeasured cases. It groups them by workload family and shows runtime,
CPU, peak RSS and throughput against same-cohort controls. Each observation retains
its date, compiler revisions, environment and evidence link. Missing measurements
are unavailable, and related source changes mark the previous result as awaiting
refresh. These results do not enter the existing daily workload score.

The initial public matrix and later milestone or relevant-change refreshes use
the manual, main-only `representative-performance.yml` workflow. It has no
schedule and is independent of PR acceptance. The existing daily and weekly
cadences are unchanged. Select future weekly cases only after reviewing actual
runtime and regression-detection value within a stated budget.

Pages composes retained results without compiling or measuring. It preserves the
latest complete representative snapshot when a later attempt fails, displays that
failure separately, and retains up to twelve complete compact snapshots. The
published snapshot remains a fallback after the 90-day raw-artifact retention
expires; its original date and evidence-expiry limit stay visible. Local diagnostics
remain separate from hosted observations and are not automatically published.

## Maintenance loop

For a reproducible practical slowdown, preserve the reproducer, find the costly
phase with profiling/generated code, improve the general mechanism, then check
the original application and relevant sensitivity cases. Retain a compact
regression case when it adds coverage. Assess speed, memory, build cost and code
quality together under [optimization trade-offs](optimization-tradeoffs.md).
Review corpus relevance and maintenance cost at milestones, not after each
minor edit. The performance investigation is complete when its evidence and
remaining gaps are recorded, not when every conceivable benchmark has been won.

## Related practice

- [Go performance monitoring](https://go.dev/wiki/PerformanceMonitoring) combines
  microbenchmarks and end-to-end applications, relative comparisons and a corpus
  that changes with usage.
- [Rust compiler performance testing](https://rustc-dev-guide.rust-lang.org/tests/perf.html)
  tracks compilation separately from the
  [runtime benchmark suite](https://github.com/rust-lang/rustc-perf/tree/main/collector/runtime-benchmarks).
- [V8's retirement of Octane](https://v8.dev/blog/retiring-octane) documents cases
  where benchmark-specific improvements regressed real applications.

### Fixed process batches and observed dispersion

Representative measurements default to the original `registered` inputs. The
opt-in `sustained-v1` profile repeats fixed batches from `suite.json` to reduce
the fraction of startup overhead. Batch counts are selected before execution;
there is no runtime calibration or retry-until-pass. Working-set sizes, outputs,
first-process separation and existing deadlines remain explicit. Different
profiles are different input series, including when the case IDs are shared.
Historical measurements retain their own batch counts, dates and source hashes.

Report retained wall, CPU, RSS and throughput ranges alongside medians. Minimum
and maximum describe the observed samples, not confidence intervals. Overlap is
an uncertainty warning; non-overlap does not establish significance. Keep median
screening labels and numerical thresholds unchanged, and do not infer missing
historical ranges. Method diagnostics use a separately registered same-executable
comparison and never replace or erase earlier candidate observations. Routine
CI retains its small correctness smoke; longer profiles remain opt-in.
