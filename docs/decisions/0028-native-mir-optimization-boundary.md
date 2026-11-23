# Decision 0028: Native MIR Owns Portable Optimization Facts

## Status

Accepted for the self-hosted optimizer transition.

The cost-decision procedure was amended on 2026-09-07 by
[Optimization costs and trade-off evaluation](../optimization-tradeoffs.md).
It supersedes the blanket requirement to recover every added byte before
investigating another useful pass. Remove duplicate/superseded emitter ownership
and track cumulative cost; separately justify useful optimizer code. Historical
ceilings and failed outcomes below remain unchanged, as do ordinary CI limits.
A bounded diagnostic is not an acceptance allowance.

## Context

The bootstrap pipeline established a distinct verified Native MIR before QBE
lowering. The later self-hosted compiler deliberately used a smaller direct-QBE
path to reach behavioral closure, reproducible fixed points, multi-target
execution, compact distribution, and measurable application performance.

That path now carries several portable facts in emitter state or discovers them
while emitting QBE. Examples include a loop local's nonnegative state, bounded
derived indices, inline-check budgets, Array-header reuse, and GC safe-point
placement. These optimizations have produced useful performance evidence and do
not change TypeRB semantics. However, extending the same organization to
general range, alias, effect, and loop analysis would make the QBE adapter own
TypeRB optimizer behavior. A later LLVM or other adapter would then need to
duplicate those decisions, and backend comparisons would no longer isolate the
backend.

QBE itself has not been shown to be the limiting backend. Accepted improvements
on the same QBE path demonstrate substantial remaining headroom. The immediate
architectural question is therefore ownership of analysis, not backend
replacement.

## Decision

Portable semantic facts and reusable optimization decisions belong to verified
Native MIR analysis and target-independent passes.

The initial fact families are:

- portable Integer range and nonnegativity;
- index normalization and bounds relationships;
- loop induction, step, and bound relationships;
- call allocation, mutation, and safe-point effects;
- Array length and data-header stability; and
- root-publication and GC-safety requirements.

The exact internal record names and representation remain unstable. Facts are
derived from checked program structure and verified control flow, not from QBE
text. Transforms preserve explicit checked arithmetic, traps, source origins,
evaluation order, allocation behavior, and runtime capabilities.

Backend adapters consume verified post-optimization MIR. They may perform
target legalization, ABI lowering, instruction selection, temporary naming,
and backend-local peepholes that need no reconstructed TypeRB semantics. They
must not inspect source tokens or emitted backend text to rediscover the facts
listed above.

The current direct-QBE self-hosted path is migration evidence, not a parallel
long-term architecture. Already registered narrow experiments may complete.
After that boundary, new non-trivial semantic analysis is added to MIR rather
than the emitter. Migration uses bounded vertical slices and removes the
superseded emitter ownership as each optimization family moves.

QBE remains the active adapter. LLVM is added only after the shared MIR path and
representative corpus cover scalar, Array, allocation, and I/O behavior. Its
first purpose is to measure an optimization ceiling over identical MIR and ABI
semantics.

## Structural compactness policy

The final objectives remain unchanged: the self-hosted Native path must match
or improve the optimized Go backend's end-to-end build time and generated
artifact size, and generated programs should match or improve established
statically typed implementations on representative workloads.

Per-optimization size limits remain binding for ordinary emitter candidates.
Introducing the MIR foundation is a structural change and may use a separate,
temporary compiler-size envelope only under all of these conditions:

1. measure the smallest useful MIR skeleton before selecting the envelope;
2. freeze the envelope and build-time, RSS, fixed-point, generated-artifact,
   and catastrophic bounds before judging the implementation;
3. state which direct-emitter logic the slice supersedes and when it is removed;
4. retain measurements at each migration checkpoint; and
5. do not present the temporary envelope as a relaxed final compactness goal.

Issue #197 selected the first structural envelope from the measured minimal
foundation: 302,000 bytes on Darwin arm64, 272,000 bytes on Linux arm64, and
574,000 bytes combined. Issue #221 then measured the smallest complete scalar
connection after rejecting an additive flat carrier and a two-pass checker.
It freezes the connection envelope at 317,000 Darwin arm64 bytes, 290,000
Linux arm64 bytes, and 607,000 bytes combined. Only the exact
foundation-to-connection transition may use its 1.07 compiler-size and 1.15
build-time ratios; subsequent ordinary changes return to the 1.05 relative
limits. RSS remains limited to 1.05 and every retained observation remains
subject to the 2.0 catastrophic limit.

Issue #225 measures the first complete control-flow connection before changing
the policy. It freezes 334,000 Darwin arm64 bytes, 310,000 Linux arm64 bytes,
and 644,000 bytes combined. Linux amd64 remains at 310,000 bytes. Only the exact
scalar-to-control-flow transition may use 1.08 compiler-size and 1.25 build-time
ratios; the measured maximum build ratio is 1.233645. RSS and all correctness,
fixed-point, artifact, process, cleanup, and catastrophic requirements remain
unchanged. The checked-in policy and three exact markers make each exceptional
transition machine-readable.

Issue #230 measures the first complete `Array<Integer>` reduction carried by
verified induction and accumulator block parameters. It freezes one-time
ceilings of 350,000 Darwin arm64 bytes, 317,000 Linux arm64 bytes, 667,000
bytes combined, and 1,120,000 bytes of target-neutral compiler QBE. The
corresponding code-section ceilings are 250,904 Mach-O `__text` bytes and
253,424 ELF `.text` bytes. Exact compiler and compiler-test source digests
identify the pre-publication measured candidate. This is not a new ratio
exception: compiler size, build time, and RSS retain the ordinary 1.05 limit,
and every retained observation remains subject to the 2.0 catastrophic limit.
The allowance applies only to removal of reduction token/control and
mutable-stack emission ownership.

Within that already measured envelope, moving a same-family decision into its
first explicit MIR pass may retain a small pre-registered compiler-QBE or
code-section cost when the complete compiler does not grow, the generated
workload QBE and code sections strictly shrink, and a material runtime gain is
measured without an RSS regression. This does not create another allowance:
the exact cost remains part of the same recovery obligation and must disappear
before the next portable fact family.

None of these allowances is an optimization budget. The complete increase
above the pre-MIR compiler must be recovered by the end of the portable range,
index, and induction migration, before work begins on the next portable fact
family.
Removing superseded direct-emitter ownership is part of completing that
migration.

## Initial scope

The first implementation is not a general optimizer. It contains only the
function, block, value, operation, origin, verifier, and fact machinery needed
to carry one existing hot scalar/Array loop from the checked self-hosted
frontend through QBE. It must retain the current QBE ABI, runtime, target
profiles, diagnostics, self-hosted fixed point, and differential corpus.

The next slices migrate current range/index decisions, loop facts, call effects,
Array-header reuse, and GC-safety decisions one family at a time. General SSA
construction, broad alias analysis, interprocedural optimization, production
LLVM support, and user-visible backend selection remain deferred until measured
evidence justifies them.

## Consequences

The transition adds short-term compiler structure and can temporarily increase
the self-hosted compiler artifact under an explicitly registered envelope. In
return, optimization correctness becomes independently verifiable, QBE remains
a small adapter, and a later backend comparison can measure backend quality
instead of comparing two different TypeRB optimizers.

Existing QBE optimizations remain useful: their tests, profiles, acceptance
evidence, and semantic conditions become the migration oracle. Rejected
experiments also remain useful because they identify transformations whose
runtime benefit did not justify their compactness or build cost.
