# Ordinary Runtime Memory Stability

The ordinary self-hosted compiler now emits managed Strings, Arrays, and
reference-containing records through the exact-root tracing model established
by the managed snapshot recovery path. This stage closes the process-lifetime allocation policy that was
acceptable for behavioral self-hosting but unsuitable for persistent Web and
Job processes.

The scope and thresholds were frozen publicly in
[issue #104](https://github.com/type-rb/type-rb-native/issues/104) before the
implementation was measured. This document describes the implementation and
evidence path; it does not relax those criteria or claim that the experimental
runtime is production-ready.

## Runtime integration

The self-hosted emitter uses a single-threaded, stop-the-world, non-moving
mark-sweep collector. Managed objects have a 16-byte allocation header, and the
descriptor word immediately preceding the payload carries the mark bit.
Static String objects use a zero descriptor sentinel and are never linked into
the heap.

Marking uses an explicit worklist instead of recursive calls, so reference depth
does not consume the machine call stack. Objects are marked before enqueueing
to handle cycles and shared references once. Scalar objects need no worklist
entry. A 64-pointer local buffer covers narrow graphs; wider frontiers grow raw
storage with checked byte counts and fatal allocation-failure handling. That
storage cannot trigger collection and is released when the root traversal ends.
It is temporary collector overhead, separate from managed-heap byte accounting.

The compiler emits:

- one exact managed-reference stack shared by allocation results, managed
  aliases, and logical function root segments;
- a per-function root-count watermark, loop-backedge compaction that
  repopulates the current managed locals, and return-value preservation across
  watermark restoration;
- fixed descriptors containing managed field offsets and payload size;
- distinct scalar and managed Array descriptors; and
- a final post-`main` collection when the internal statistics switch is set.

The root representation contains only compiler-proven managed references; the
collector does not conservatively scan machine stack memory. Allocation and
managed-return paths push their result before another collection can occur.
Managed record-field and Array-element loads push an alias root before the
owning container can leave the current logical segment. Parameters remain
covered by their caller unless the callee directly reassigns its binding.
Every emitted return shares one epilogue that restores the entry watermark and
then re-pushes a managed result for the caller. Long-running loops restore the
watermark and repopulate live local roots at each header, preventing root-stack
growth across iterations.

Array backing storage is included in pacing and allocation accounting, and is
released before the Array handle. A collection leaves objects in place, so the
QBE ABI does not require forwarding updates. The next heap target is at least
1 MiB and otherwise twice the post-sweep live heap plus 64 KiB.

Managed object requests from 16 through 64 bytes use eight-byte size classes
within 4 KiB storage pages. Each page has a 64-byte header and each cell has an
eight-byte owner prefix outside the visible object layout. Available pages and
freed cells are linked per class and per page respectively; full pages leave
the available list, and the final cell release immediately frees its page.
Larger objects retain direct system allocation. Array backing stores and the
exact-root buffer remain separate allocations. These raw storage operations
cannot trigger collection or a managed callback.

Released four-element Array backing buffers enter a bounded reuse cache. It
retains at most 8192 ordinary libc allocations, or 256 KiB of payload plus
allocator overhead. Reuse clears all 32 bytes, and growth continues to use
`realloc` on the original libc pointer. Larger buffers and entries exceeding
the cap are freed immediately. Cached storage is untraced and never keeps its
previous elements alive. Final statistics/trace reclamation also flushes this
cache; ordinary execution relies on process teardown for final raw storage.

Collector accounting and pacing count the exact requested object and Array
backing bytes. They exclude page headers, owner prefixes, class rounding and
unused cells, as well as released backing buffers retained in the cache.
Thus logical heap statistics are not physical storage or RSS
measurements. Empty pages are never cached, and final reclamation must release
all page storage as well as every directly allocated object and Array buffer.

The versioned `TYPE_RB_NATIVE_RUNTIME_STATS` switch is internal test machinery.
It reports collection count, automatic collection count, cumulative allocated
and reclaimed bytes, final live bytes, and peak managed heap to stderr after
the authored `main` frame has returned. Ordinary programs do not emit the
report, and TypeRB gains no source-visible GC operation.

The separate `TYPE_RB_NATIVE_RUNTIME_TRACE` switch is also internal evidence
machinery and is read once at process startup. It samples every 64th automatic
collection and the final manual collection, reporting the collection number,
automatic/manual role, post-sweep live bytes, next heap target, logical-root
count, and root capacity. It leaves the stats v1 schema unchanged and has no
output or behavioral effect when disabled.

The exact-root buffer is one raw runtime allocation whose capacity follows
maximum logical root depth rather than cumulative allocation. The registered
workload stays at its initial 64-word, 512-byte capacity, which remains globally
reachable until process exit. Valgrind exposes and records it explicitly rather
than suppressing reachable memory; every managed object and every Array backing
allocation must still be reclaimed.

## Registered workload

[`tools/runtime-memory-soak/workload.trb`](../tools/runtime-memory-soak/workload.trb)
is one authored workload shared by every measurement mode. Each iteration:

1. creates a dynamic String;
2. creates and grows a four-element `Array<Integer>`;
3. carries the String across a nested allocating call;
4. constructs a managed record containing both references;
5. consumes the fields through another call; and
6. returns only an Integer checksum, dropping the last managed root.

Each phase runs an allocation-idle Integer loop, prints one fixed phase marker,
and contributes to the final checked success line. The canonical invocation is
60 phases of 5,000,000 iterations. The harness derives the one-phase CI smoke
and 50,000-iteration oracle input by replacing only that exact invocation.

## Automated evidence

[`runtime-memory-soak.sh`](../tools/runtime-memory-soak/runtime-memory-soak.sh)
checks deterministic QBE emission, application output, collector invariants,
runtime, compiler size, and artifact identities. The normal pull-request
workflow runs the 5,000,000-iteration Darwin arm64 smoke after closing the
self-hosted compiler chain.

The former manually dispatched
[`runtime-memory-formal.yml`](https://github.com/type-rb/type-rb-native/blob/d1c151d02e051c6557acb479a9e9d7c713226858/.github/workflows/runtime-memory-formal.yml)
is retired; run `runtime-memory-soak.sh` in its formal mode to reproduce it. It
started from the published ordinary Native seeds. Because those seeds predate
the current emitter and embedded linker policy, each target first builds two
setup-only, Go-free transitions from the current compiler sources. The first
introduces the current TypeRB-authored emitter through the published seed's
runtime; the second carries the current embedded runtime and link policy. The
second transition then builds candidate B2, B2 builds B3, and B3 builds B4.
Both transitions are identified separately and excluded from candidate timing
and size claims; B2/B3/B4 compiler and QBE bytes must converge exactly on
Darwin and Linux arm64. A future published seed containing the current runtime
can omit these compatibility transitions. The workflow then records:

- a production-linked smoke on both targets;
- one dedicated Linux arm64 QBE output linked with Clang ASan/LSan;
- a reduced Linux arm64 Valgrind Memcheck run with all leak classes visible;
- the 300,000,000-iteration Linux arm64 production soak;
- raw RSS samples every 250 ms, annotated with the latest authored phase
  marker count;
- first- and last-temporal-quartile median RSS after six warmup phases;
- an ordinary least-squares RSS slope in bytes per minute;
- exact revisions, source, tools, commands, hashes, sizes, timings, runtime
  statistics, and process inventory; and
- the per-target and combined stripped compiler size.

The analyzer reports both trend values independently. Passing the absolute RSS
ceiling cannot substitute for either trend limit. The dedicated sanitizer link
intercepts allocator and linked-toolchain behavior; it is not described as
instrumenting QBE instructions.

## Persistent worker extension

[Issue #150](https://github.com/type-rb/type-rb-native/issues/150) registers a
second, persistent-process verification layer without adding a public Web or
Job API. Its [single authored workload](../tools/runtime-worker-soak/workload.trb)
keeps one worker state alive, retains a bounded 64-entry recent-payload cache,
and processes deterministic success, retry, terminal-failure, and cancellation
paths. Native and optimized Go compile the exact same TypeRB source.

The [persistent worker harness](../tools/runtime-worker-soak/README.md) runs a
40,000-batch CI smoke on Darwin and Linux arm64. Its dispatch-only Linux formal
mode now runs 60 phases of 120,000 batches: 921,600,000 original jobs,
979,200,000 processed attempts, and exactly 33,292,800,576 managed bytes:
576 initial bytes plus 4,624 bytes per batch. Every harness mode checks this
allocation formula, while formal mode also requires at least 30 GiB allocated. It
retains the sampled internal GC trace, 250 ms Native and Go RSS/descriptor/thread
series, ASan/LSan output, and Valgrind leak-class inventory. Formal acceptance
requires at least 400 complete GC observations, no more than 128 KiB post-sweep
live bytes at any sampled automatic collection, a fixed 64-word root capacity,
zero final live bytes and roots, a Native RSS maximum below 64 MiB, both existing
RSS trend limits, and no post-warmup descriptor or thread growth.

## Current status

The reviewed
[Darwin/Linux arm64 result](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-08-30-runtime-memory-stability-darwin-linux-arm64/README.md)
passes every frozen Stage 1 criterion. The 300,000,000-iteration production
soak allocates and reclaims 42,300,000,000 managed bytes, ends with zero live
managed bytes, and records zero RSS growth or fitted slope. ASan/LSan reports no
finding, while Valgrind reports no lost or possibly lost allocation and only
the enumerated 512-byte globally reachable exact-root buffer.

Persistent service lifecycles, concurrent mutation, closure cycles in the
self-hosted subset, finalizers, weak references, and non-memory resource
cleanup remain separate work. The bounded allocation result must not be
presented as proof for those later surfaces.

The persistent worker extension narrows the first item to the registered
single-threaded worker lifecycle. The
[formal result](../results/2026-09-01-persistent-worker-memory-darwin-linux-arm64/README.md)
processes 460,800,000 original jobs, allocates and reclaims 33,926,400,576
managed bytes, ends with zero live bytes, retains a fixed 64-word root buffer,
and records flat Native RSS quartiles with stable descriptor and thread counts.
ASan/LSan and Memcheck report no leak or memory error. This evidence is not a
claim about concurrency, real network servers, finalizers, weak references, or
arbitrary external-resource cleanup.
