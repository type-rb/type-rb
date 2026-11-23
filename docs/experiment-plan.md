# Development and Validation Plan

## Engineering objective

Build a TypeRB-authored toolchain for production use, covering the reference
language, standard library and official packages, Go/Ruby/TypeScript emission and
execution, and Native AOT output. The [project roadmap](mir-consolidation.md)
includes stable public interfaces and supported releases as coverage matures.
Native performance targets are:

1. End-to-end application build time that outperforms equivalent Pure Go builds.
2. Generated-program execution time that outperforms equivalent Pure Go programs
   and is competitive with established statically typed implementations across
   representative portable workloads.
3. Deployed executable size smaller than equivalent Pure Go executables.

Use matched inputs, outputs, algorithms, concurrency and optimized toolchains.
These are measured targets; current results do not establish universal wins.

The compiler and runtime are implemented in TypeRB, reproduce themselves, and
must retain competitive build time and generated-code behavior once the
complete self-hosted toolchain is measured. Early checks establish this outcome
incrementally; they are not a sequence of throwaway demonstrations.

Secondary outcomes include compiler and runtime peak memory, startup latency,
toolchain distribution size, portability, diagnostics, correctness risk, and
maintenance cost.

The [performance engineering policy](performance-policy.md) governs corpus
selection, representative applications, input diversity and bounded CI cadence.

The identical-source comparison baseline is an optimized release executable
produced by the reference compiler's Go backend. It isolates backend and
runtime changes, but it is not the final execution-performance ceiling.
Cross-language runtime context uses pinned established implementations without
an intentionally unoptimized, unstripped, cold, or otherwise disadvantaged
configuration.

The active [MIR consolidation policy](mir-consolidation.md) supersedes the
per-checkpoint cost scheduling below during migration. Its fixed baselines and
correctness requirements remain binding; detailed cost qualification follows
the coherent milestone.

## Principles

- Correctness precedes performance.
- All candidates use the same TypeRB inputs, supported semantics, Native MIR
  corpus, benchmark policy, and, for same-target comparisons, the same target
  ABI profile.
- Unsupported behavior fails explicitly.
- Measurements include serialization, lowering, optimization, code generation,
  assembly, linking, runtime, and required external components.
- TypeRB semantic analysis and reusable optimization decisions belong to
  verified Native MIR and target-independent passes, not to a backend emitter.
- Quality and performance targets are recorded before reviewing a result.
- Microbenchmarks diagnose a phase; representative programs determine
  viability.

Before a checkpoint begins, its issue must record metric-specific non-inferiority
bounds, a minimum meaningful primary-metric improvement where the checkpoint is
expected to provide one, and catastrophic-regression limits. A miss identifies
required engineering work or an architectural decision; it does not by itself
end the native implementation. Targets cannot be weakened after results are
reviewed merely to label a checkpoint complete.

Follow [Optimization costs and trade-off evaluation](optimization-tradeoffs.md).
Ordinary acceptance retains existing limits; a preregistered, candidate-specific
diagnostic can measure benefit despite a cost miss. Any later acceptance-budget
change needs a separate reviewed decision and enforcement, not retrospective
relabeling. Track same-feature and frozen cumulative controls and actual Go
comparisons. Reassess after two size-only attempts without a current runtime
benefit assessment, rather than polishing small overruns indefinitely.

## Current implementation focus

Complete the [MIR consolidation milestone](mir-consolidation.md) through cohesive
families of typed operations, control flow, managed lifetimes and shared facts.
Use the [ordinary language coverage plan](native-language-coverage.md) to add
needed basic features and adopt them in the compiler while removing superseded
semantic owners and splitting large modules. Separate check/build/execution/REPL
coverage from recovery evidence. Detailed performance qualification follows the
coherent milestone; correctness and reproducibility remain blocking throughout.

## Candidate sequence

Backend candidates are not implemented to production completeness in parallel.
They advance through small shared checks, and only implementations with a clear
role continue to accumulate maintenance cost.

1. Use hand-authored bootstrap and MIR fixtures to validate the boundary.
2. Use QBE for the cheapest runtime and ABI feasibility check.
3. Consider Cranelift only if the QBE result leaves a measured development
   code-generation problem worth testing.
4. Add LLVM only after the corpus is representative enough to measure an
   optimization ceiling.
5. Attempt a direct emitter only if profiling shows codegen or toolchain
   overhead dominates and the MIR, layouts, and ABI have stabilized.

This order is a starting hypothesis, not a compatibility promise.

TinyGo may be measured once as a time-boxed calibration of the optimized Go
baseline. It is not a path to the required Go-independent compiler and is not
a checkpoint deliverable. A C emitter is likewise deferred unless later profiling
shows that it answers a specific question more cheaply than the selected
backend. Neither is built merely to populate a comparison table.

## Self-hosted MIR optimization transition

The [MIR consolidation milestone](mir-consolidation.md) supersedes the per-slice
transition envelopes. [Decision 0028](decisions/0028-native-mir-optimization-boundary.md)
keeps TypeRB facts and optimizations in verified MIR. The transition size
markers are retired; the earlier transition sequence, its markers and
source-era bounds are in the
[prior version of this plan](https://github.com/type-rb/type-rb-native/blob/04c6ca7263c066fd13e83b3faa09c4e80d707c13/docs/experiment-plan.md#self-hosted-mir-optimization-transition).
LLVM remains deferred until the shared path and benchmark corpus cover scalar,
Array, allocation and I/O behavior; its first role is a bounded
optimization-ceiling comparison over the same MIR and ABI.

## Repository organization

Use the [current ownership map](repository-organization.md) and the
[MIR consolidation milestone](mir-consolidation.md). Combine connected semantic
ownership moves and retire their superseded code with the consumers. Keep recovery,
ordinary fixed points, lifetime and target correctness blocking; defer detailed
performance qualification until the coherent milestone.

Completed experimental contracts are available in the [historical record](history.md).

## Correctness checks

Each supported feature requires:

- reference-backend differential tests;
- valid and invalid MIR fixtures;
- boundary-value tests for layout and arithmetic;
- deterministic diagnostics for unsupported input;
- reproducible output checks; and
- randomized or fuzz validation when a verifier or encoder accepts structured
  untrusted input.

A candidate fails correctness if it obtains performance by weakening TypeRB
integer ranges, Unicode behavior, failure semantics, initialization order,
source attribution, or another portable guarantee.

## Measurements

### Build measurements

Report separately:

- reference frontend and snapshot production;
- snapshot validation and Native MIR lowering;
- optimization;
- backend code generation;
- assembly and object writing;
- linking; and
- total cold, warm, and incremental build time.

Also record peak compiler RSS and every process executed by the build.

### Runtime measurements

Depending on the workload, report:

- startup latency;
- steady-state throughput or completion time;
- latency distribution rather than only the best result;
- allocation count where available; and
- peak runtime RSS.

### Size measurements

Report:

- raw and stripped executable size;
- compressed artifact size when relevant;
- static and dynamic runtime dependencies;
- backend sidecars, assembler, linker, and required SDK components; and
- complete toolchain distribution size.

An executable that relies on a shared VM or uncounted runtime is not directly
comparable to a standalone binary without reporting both views.

## Benchmark record

Every published result should include:

- TypeRB, native repository, runtime, and backend revisions;
- exact commands, release flags, stripping, path metadata, and configuration;
- hardware, operating system, architecture, and toolchain versions;
- cache state and environment constraints;
- input corpus revision;
- warmup, repetition count, aggregation, and variance; and
- raw machine-readable results.

Store results under a date- and experiment-specific directory only after the
first executable benchmark exists. Do not commit placeholder result files.
Follow the [evidence lifecycle policy](evidence-retention.md): only registered
active reports and observation tables stay in main. Replace a result and retire
its predecessor in the same PR, retaining significant decisions and pinned
historical links. Archive generated payloads only when they have a durable use.
Preserve active measurement cohorts, frozen baselines and seed consumers.

## Backend selection policy

A backend implementation remains active only when it:

- passes the current correctness and reproducibility checks;
- satisfies the pre-registered non-inferiority and catastrophic-regression
  limits;
- achieves the pre-registered minimum improvement in at least one primary
  outcome before product feasibility, or has a concrete diagnostic role in
  reaching that outcome;
- has a credible path for the next required target and runtime feature;
- does not impose disproportionate distribution, security, or maintenance
  costs.

One production default is preferred. Separate development and release backends
remain possible only when their end-to-end advantages are both material and
stable. Experimental backends should not become user-visible configuration
merely because they win a microbenchmark.

A secondary improvement may justify a bounded diagnostic experiment, but it
does not pass product feasibility when all three primary outcomes miss their
registered checks.

## Reassessment policy

A missed checkpoint triggers diagnosis of the MIR, runtime, backend, or build
pipeline and a recorded plan to close the gap. Backend adapters may be replaced
or removed when another implementation serves their role better. The native
implementation itself is reconsidered only when evidence exposes a fundamental
conflict with portable TypeRB semantics, safe implementation, or sustainable
self-hosting—not merely because an early implementation needs optimization.

Temporary bootstrap surfaces still have no compatibility guarantee. Remove
them when the independent frontend replaces them, and retain generally useful
benchmark methodology, conformance tests, and architectural findings.
