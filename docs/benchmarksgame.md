# Reproducible Language Benchmark Plan

This plan implements the bounded benchmark work registered in
[issue #103](https://github.com/type-rb/type-rb-native/issues/103). It follows
[Decision 0023](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/decisions/0023-reproducible-benchmark-layers.md) and keeps the
identical-TypeRB comparison separate from cross-language implementation
context.

## Daily status and occasional detailed comparisons

[Daily performance](daily-performance.md) is now the primary Pages view for
current status and improvements. It uses a bounded diagnostic suite and updates
independently of this full seven-implementation snapshot. These formal runtime
and build workflows remain manual and are run for a concrete comparison or
publication question, not for every merge or a mandatory monthly refresh.
Daily measurements do not change the acceptance authorities below.

## Status

The capability corpus and formal runtime and build/distribution layers cover
three numeric cases. The [current Linux arm64 result](../results/2026-09-10-benchmarksgame-runtime-loop-local-headers-linux-arm64/README.md)
retains every registered observation at accepted Native revision `8980b597`,
including assignment recovery and subsequent MIR Array-header improvements.
One-core spectral-norm takes 2.25684 seconds versus Pure Go's 3.21985 seconds,
29.91% less wall time. N-body and fannkuch take 7.87414 and 61.8302 seconds;
both recover the earlier published regressions and improve beyond the former
pre-regression snapshot, while remaining slower than Pure Go.

The [same-source build result](../results/2026-09-10-benchmarksgame-build-loop-local-headers-linux-arm64/README.md)
finds that Native uses 38.2% to 39.7% of TypeRB Go's build wall time. All 546
runtime and 78 build observations are retained. Separate local Native A/B
cohorts assess individual optimization effects; changes between publication
dates do not establish causal attribution. These three programs do not establish
general language parity.

## Runtime objective and benchmark expansion

The long-term execution objective is to match or exceed established statically
typed language implementations on representative portable workloads. The
identical-TypeRB Go path remains the primary backend control because it holds
source and semantics constant; it does not define the final Native performance
ceiling. Cross-language rows remain exact implementation context rather than a
composite language ranking.

The current three cases are the first numeric slice, not a broad application
suite. Expansion proceeds in this order:

1. rerun all three runtime and build cases from one current Native revision and
   one current compatible TypeRB Go revision before replacing the published
   snapshot;
2. add allocation and collection pressure, with `binary-trees` as the first
   candidate once its specification is expressible without semantic changes;
3. add byte, String, and I/O workloads from `fasta` and
   `reverse-complement`, followed by Hash and String pressure from
   `k-nucleotide` and binary-output numeric work from `mandelbrot`;
4. admit `regex-redux` and `pidigits` only after the corresponding portable
   Regex and arbitrary-precision surfaces exist; and
5. keep persistent worker, eventual Web/Job service, leak, latency, and soak
   measurements in a separate application-lifecycle layer.

Each addition must pin its upstream specification, license, source revision,
input, exact output, toolchains, and measurement policy before results are
reviewed. Pull-request optimization checks may use bounded Native-to-Native A/B
runs, while the larger cross-language suite runs periodically from a complete
revision. Profiles and generated-code evidence select optimization work; the
project does not weaken TypeRB semantics or trade away its measured build-time
and compactness advantages merely to improve a benchmark.

## Pinned upstream boundary

- project: [The Computer Language Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/);
- site version: `25.03`;
- repository revision:
  [`40296663ed350d5fe4a6ab5e367bab61cb77c219`](https://salsa.debian.org/benchmarksgame-team/benchmarksgame/-/commit/40296663ed350d5fe4a6ab5e367bab61cb77c219);
- source archive path: `public/download/benchmarksgame-sourcecode.zip` at that
  revision;
- source archive SHA-256:
  `aabcf6726cdc14f0f45b99e5daba48584f94bbb48883fd3711a1d040474d1cb4`;
- measurement procedure:
  [How programs are measured](https://benchmarksgame-team.pages.debian.net/benchmarksgame/how-programs-are-measured.html);
- license: [Revised BSD](https://benchmarksgame-team.pages.debian.net/benchmarksgame/license.html).

The repository retains the two upstream license instances that cover the
adapted TypeRB programs in [`benchmarks/benchmarksgame/licenses`](../benchmarks/benchmarksgame/licenses).

## Admitted TypeRB cases

| Case | Correctness input | Performance input | Published performance output |
| --- | ---: | ---: | --- |
| `fannkuch-redux` | `7` | `12` | `3968050`, then `Pfannkuchen(12) = 65` |
| `n-body` | `1000` | `50000000` | `-0.169075164`, then `-0.169059907` |
| `spectral-norm` | `100` | `5500` | `1.274224153` |

The sources and complete correctness/performance expected outputs are under
[`benchmarks/benchmarksgame`](../benchmarks/benchmarksgame). Their admission
does not weaken a specification:

- `fannkuch-redux` uses mutable Integer Arrays, remainder, permutation
  generation, process arguments, and exact Integer output;
- `n-body` uses binary64 Arrays grouped in a record, the published symplectic
  integration order, square root, and exact nine-place output; and
- `spectral-norm` keeps the four published matrix functions, ten power-method
  iterations, square root, and exact nine-place output.

The Float programs render nine decimal places in user source by rounding a
finite value to an Integer-scaled billionth and padding the fractional digits.
The published values are far inside the portable Integer range and are not
halfway cases. This uses only existing portable TypeRB behavior and remains
identical in both backend paths.

## Primary backend-pair procedure

For each case:

1. hash the authored source and expected output;
2. check the source with both compilers;
3. build the same source with the pinned optimized Go path and with the
   canonical self-hosted Native compiler;
4. run the correctness input and require zero status, empty stderr, exact
   published stdout, and byte identity between backends;
5. perform two untimed compiler warmups, then retain eleven alternating clean
   builds for compiler wall time, CPU time, and peak RSS;
6. validate the performance output before timing runtime, perform two runtime
   warmups, and retain eleven alternating observations for wall time, CPU time,
   and peak RSS; and
7. record raw and stripped executable size plus the complete compiler,
   generator, linker, runtime, shared-library, and required distribution
   inventory.

The first run is never reported. Alternation order changes each observation so
one backend is not systematically favored by thermal or host drift. Failures,
timeouts, and memory-limit outcomes remain in raw data. Medians and the full
per-case distribution are reported; no aggregate score is calculated.

[`tools/benchmarksgame-verify.sh`](../tools/benchmarksgame-verify.sh) owns the
fast correctness boundary and is run by ordinary pull-request CI. The formal
measurement controllers extend this boundary without changing its sources or
oracles.

## Cross-language context selection

The initial arm64-compatible context uses these exact paths from the pinned
archive:

| Case | C | C++ | Go | Rust | Java |
| --- | --- | --- | --- | --- | --- |
| `fannkuch-redux` | `fannkuchredux.gcc` | `fannkuchredux.gpp-3.gpp` | `fannkuchredux.go-8.go` | `fannkuchredux.rust-2.rust` | `fannkuchredux.java-3.java` |
| `n-body` | `nbody.gcc` | `nbody.gpp-2.gpp` | `nbody.go-8.go` | `nbody.rust` | `nbody.java-4.java` |
| `spectral-norm` | `spectralnorm.gcc-8.gcc` | `spectralnorm.gpp` | `spectralnorm.go-8.go` | `spectralnorm.rust-3.rust` | `spectralnorm.java-8.java` |

These variants compile on Darwin arm64 and reproduce every small published
oracle with the locally recorded toolchains. They avoid x86-only SIMD
requirements; Linux arm64 remains part of the formal run rather than an
inference from that check. Some upstream implementations create threads, so
the one-core and four-core lanes are published separately. The one-core lane
does not rewrite source to remove threading; BenchExec constrains the complete
process tree and records the resulting overhead.

[`context-sources.tsv`](../benchmarks/benchmarksgame/context-sources.tsv) pins
each archive path and source hash.
[`tools/benchmarksgame-context-verify.sh`](../tools/benchmarksgame-context-verify.sh)
refuses a different archive, extracts only those 15 sources, compiles them,
records the complete commands and tool versions, and requires exact published
output for every small input. This is a correctness and toolchain-capability
check, not a performance result.

The preregistered
[`formal runtime controller`](../tools/benchmarksgame-formal/README.md) pins
BenchExec `runexec` 3.35, runs correctness before timing, rotates all seven
candidates through two warmup and eleven retained rounds, and preserves every
failure and raw process-tree metric. One-core and four-core lanes are separate.
This controller measures complete fresh processes only. Its current
[formal result](../results/2026-09-10-benchmarksgame-runtime-loop-local-headers-linux-arm64/README.md)
publishes all one-core and four-core raw observations, independently reproduced
medians, and exact artifact identities. Compiler measurements and complete
artifact/distribution inventory use the separate
[formal build controller](../tools/benchmarksgame-build-formal/README.md). It
measures alternating clean outputs through both TypeRB backends, verifies every
measured artifact, process-traces representative builds, and separates
controlled payloads from platform prerequisites and deploy artifacts. Its
current
[formal result](../results/2026-09-10-benchmarksgame-build-loop-local-headers-linux-arm64/README.md)
publishes all raw observations, independently reproduced medians, artifact
variants, process closure, dynamic dependencies, and distribution totals. See
[Decision 0024](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/decisions/0024-benchexec-runtime-controller.md) and
[Decision 0027](decisions/0027-formal-build-distribution-controller.md). The
dispatch-only [runtime workflow](../.github/workflows/benchmarksgame-formal.yml)
and [build workflow](../.github/workflows/benchmarksgame-build-formal.yml)
isolate each case on a fresh runner and retain complete result trees even when
an observation fails.

## Native optimization A/B procedure

The cross-language result is a durable reference point, not a same-host baseline
for every compiler optimization. The daily measurement compares each main
revision with the previously measured one on the same host. The former
`native-runtime-ab` controller, its registered contracts and accepted or
rejected results remain in the
[immutable experiment history](https://github.com/type-rb/type-rb-native/blob/f864151fa53a99a9491ca0268198d5ec914124fd/docs/benchmarksgame.md#native-optimization-ab-procedure).
New portable facts and transforms belong in verified Native MIR as described by
[Decision 0028](decisions/0028-native-mir-optimization-boundary.md).

Builds use release optimization without unsafe fast-math substitutions: C and
C++ use `-O3`, Go uses `go build -trimpath`, Rust uses `rustc -C opt-level=3`,
and Java uses `javac` followed by the same recorded JVM for every run. The
selected n-body C++ source additionally needs `-include cstdlib` with current
Clang because it uses `std::size_t` without including that header. This is a
recorded compiler-compatibility flag, not a source rewrite. Exact compiler
versions and complete command lines are result inputs. Raw and stripped
artifacts are distinct; Java application size and required JRE distribution
size are both explicit.

Formal context runs use pinned BenchExec `runexec` 3.35 on a fresh Linux arm64
hosted runner, with no other project workload in that job. The harness checks
cgroup support, validates all output
before timing, drops the Linux page cache before every measured process, uses
two warmup and eleven retained rounds, prevents network access during
execution, and records every timeout or failure. One-core and four-core results
use the same binaries and inputs.

## Measurement-host procedure

For an optimization A/B, first register the candidate and baseline revisions,
source bytes, inputs, output oracle, target, sample count and stop conditions.
Run both candidates on the same host, with no other project build, profiler or
benchmark running during timed observations. Use the registered controller's
warmups and alternating order. Keep every raw observation, including failures;
do not retry individual slow samples or select the best cohort.

For a formal cross-language result, use the dispatch-only workflow's fresh
runner for one case. The controller runs candidates serially under its cgroup
CPU and memory limits, with swap disabled and the registered cache policy.
Review the retained host identity, cgroup/setup checks, command traces,
resource-limit outcomes and full sample spread before promoting a claim. A
hosted runner isolates this job from other project jobs, but does not establish
that the underlying physical machine had no competing workload. Describe that
limit explicitly instead of treating a small wall-time difference as certain.

If setup isolation fails, another project workload ran on the measurement host,
or the recorded run shows material CPU/memory contention or outliers beyond its
registered contract, retain the evidence and mark that cohort inconclusive.
After removing the cause, run a new complete cohort under the same declared
policy; never silently discard one observation. Compare Native and Pure Go
within one cohort. Absolute times from separate hosts or dates are trend
signals, not causal evidence for one compiler change.

## Interpretation limits

These cases have insignificant I/O and primarily test tight numeric kernels.
They do not represent application architecture, package ecosystems, latency
under load, persistent services, allocation behavior, or developer
productivity. The primary layer supports backend claims only. The context layer
supports statements about these exact implementations, inputs, host, and
toolchains only.
