# Architecture

## Purpose

TypeRB Native develops a TypeRB-specific native compiler and runtime while
keeping the supported language and reference compiler independent. Its
engineering objective is a self-hosted implementation that removes Go from the
ordinary bootstrap and application-build path while matching or improving the
practical tradeoff among build time, execution performance, and deployed binary
size after all required tooling is counted.

The implementation is written in TypeRB. Native execution
and self-hosting are separate checkpoints, but both belong to the intended
path. The Go reference compiler bootstraps early artifacts and remains a
differential oracle. The completed compiler and runtime owned by this
repository are written in TypeRB and reproduce themselves without Go in the
ordinary release/bootstrap path.

## Complete toolchain goal

The [MIR consolidation roadmap](mir-consolidation.md) records the longer goal:
full TypeRB behavior, Go/Ruby/TypeScript emission and execution, standard-library
and official-package coverage, and Native output outperforming equivalent Pure
Go applications in measured runtime, application size and build time. Current
Native coverage remains partial. Preserve high-level checked semantics for the
source backends while lowering Native layout/ABI details through its MIR path.

## Ownership boundary

The [reference TypeRB repository](https://github.com/type-rb/type-rb) owns:

- syntax and normative language semantics;
- parsing, name resolution, type checking, and diagnostics;
- package resolution and portable standard-library contracts;
- the reference typed IR and supported Go, Ruby, and TypeScript backends; and
- the canonical cross-backend conformance behavior.

This repository owns the TypeRB-authored implementation and its development:

- the independent TypeRB-authored frontend, growing toward full reference coverage;
- Go/Ruby/TypeScript emission and execution in that implementation as future work;
- bootstrap snapshot validation and lowering;
- Native MIR and its verifier;
- native data layout and target ABI profiles;
- optimization and backend adapters;
- object emission and linker integration;
- the Native runtime and platform support for the standard library and packages; and
- native correctness, portability, and performance measurements.

The normal reference TypeRB build, test, and release paths must not depend on
this repository. A language-level change discovered here belongs in the
reference repository's normal design and review process. The reference
implementation's narrow snapshot producer remains a recovery and differential
surface, not part of ordinary Native compilation.

### Reference-repository independence

The reference repository is consumer-neutral. A temporary producer there must
be justified, named, documented, tested, and diagnosed solely as a
reference-compiler capability; it must be understandable without knowing that
TypeRB Native exists. Reference code, documentation, changelog entries, commit
messages, and pull requests must not contain this repository's name, experimental
checkpoint names, backend or runtime roadmap, consumer-specific aliases, integration
commands, revision pins, or bridge retirement policy.

This repository owns the other side of that boundary: the exact producer
command it invokes, the snapshot version used by each checkpoint, the pinned merged
reference revision, compatibility coordination, and the condition for removing
the bridge. An upstream change remains narrow, internal, versioned, data-only,
and removable; downstream urgency does not turn it into a public TypeRB API.

## Implementation-language boundary

Repository-owned executable compiler and runtime source is written in TypeRB.
Go, Rust, Zig, C, or another existing implementation language is not introduced
as the permanent host for those components. Generated C, assembly, object
files, or backend IR are outputs rather than maintained implementation source.

External tools remain allowed and must be accounted for. QBE or LLVM, an
assembler, a linker, an SDK, and system libraries do not violate self-hosting;
they are explicit dependencies of a TypeRB-authored compiler in the same way a
linker can be a dependency of another self-hosted language implementation.

## Pipeline

```text
TypeRB source
    -> Native file/project loader, lexer, parser, resolver, checker
    -> verified Native MIR and target-independent passes
    -> QBE adapter and generated managed runtime
    -> external QBE, assembler, and linker
    -> executable
```

Each boundary must preserve source origins so diagnostics and runtime failures
can eventually refer to authored TypeRB source.

Every accepted ordinary function requires a verified MIR body. The
[MIR status](native-mir-optimization-status.md) records current ownership and
remaining optimization work.

### Current compiler source ownership

The ordinary entry is [compiler/src/compiler.trb](../compiler/src/compiler.trb).
Its explicit transitive import closure is registered in
`recovery/src/compiler/layout.trb`.

The import closure changes as language coverage grows; the generated layout,
not a hand-maintained module table, is the exact inventory. Its main responsibility
boundaries are:

| Source group | Responsibility |
| --- | --- |
| Lexer, parser, resolution and checked declarations | Authored syntax, names, semantic types, diagnostics and source origins. |
| `mir_*` and checked construction modules | Typed operations, control flow, effects, liveness, proof plans and independent verification. |
| `qbe_*` and runtime modules | QBE/ABI adaptation of verified MIR, managed runtime code and target output. |
| `compiler/src/compiler.trb` | Entry orchestration, final checking and emission lifetime. |

Expression, call, collection, member, statement, control and iteration checking
have explicit mutually recursive owners under `frontend/checking`; see the
[directory and dependency map](compiler-source-layout.md). The CLI/REPL reuses
checked source identities and core runtime behavior. Calls, including standard
package and host adapters, carry typed signatures and conservative effects in
MIR. The backend consumes verified plans and does not recover source-level
Array, Range, iteration or root facts. Future decomposition should follow these
responsibilities without duplicating the canonical compiler closure.

Official package providers under `frontend/providers` attach declarations only
to loader-owned package identities. Concrete Web parameter schemas generate
ordinary TypeRB bodies with private aliases for canonical record and field
types. Speculative template analysis validates the schema without modifying
shared source syntax; ordinary checking materializes the executable body.
Those bodies use the normal resolver, checker, verified MIR and REPL;
the backend receives no Web parsing or record-construction rules. The pinned
portable package sources remain unchanged. Unsettled cross-mode contracts stay
explicitly unsupported and visible in the shared coverage registry.

File-route discovery reads configured application sources and records handler,
pattern and middleware identities before checking. The loader-owned testing
provider generates ordinary TypeRB wrappers with resolved import bindings;
private TypeRB code handles dispatch and protocol responses. The generated
dispatcher belongs to the application compilation unit because it calls that
application's handlers. Other package units retain their cycle checks. Neither
MIR nor the backend reads route filenames or reconstructs route semantics.
The reference testing dispatcher has no REPL runtime adapter, and the owned
Native entry preserves that rejection. REPL startup still validates the configured
route graph, including projects without an executable main. Network serving and
network cancellation remain separate runtime contracts in the coverage inventory.

Endpoint class-body declarations retain source spans and resolve the direct
superclass and marker functions through loader-owned identities. Checked
metadata binds each contract to one local route and canonical response/input
types; authored aliases remain available for the pinned generator's spelling
restrictions. These declarations never become executable bodies. The OpenAPI
exporter lives with checked-program semantics, validates catalog identities and
consumes checked declaration facts,
including JSON field names and portable scalar representations. Generation-only
schema restrictions do not change ordinary checking. The CLI writes a
deterministic document without invoking a target toolchain, handler or server.
MIR and QBE neither read endpoint contracts nor reconstruct their policy.

Web recovery uses the loader-owned `Runtime.recover<T>` callback intrinsic.
The frontend requires runtime loader ownership. The MIR verifier checks the
callback, fallback and result as one concrete type identity and retains
conservative effects. Portable TypeRB code places separate core and outer
boundaries, so a failed handler returns through the root middleware's `Next`
while a failed root middleware abandons its remaining statements. HEAD response
suppression remains part of the dispatcher.

The QBE runtime isolates libc `setjmp`/`longjmp` in a callback trampoline;
ordinary functions have no returns-twice call. Its immutable frame records the
previous scope, caller root count, closure and fallback before entering the
callback. Both completion paths restore the scope and root count without saving
a movable root-buffer pointer. Integer/pointer and Float callbacks have distinct
ABI adapters. Target tests check the 1024-byte, 16-byte-aligned jump environment
against the system headers and exercise nested failures and normal returns.
Language failures use this scope when present and retain their diagnostics and
exit behavior otherwise. Allocation failure and invalid internal object state
remain process-fatal. This internal mechanism does not introduce a public catch
syntax. Managed deadlines use a separate owned scope with MIR checkpoints and
resource-stack unwinding; see [Native deadlines](native-deadlines.md) for the
foreign-call and ordinary-application qualification boundary.

Scoped File acquisition uses a parser-owned trailing block and checked lexical
captures. It does not create an authored callback value. The frontend checks
immutable resource borrows and transitively reached calls; MIR independently
verifies non-escaping resources, synchronous call edges, typed captures and the
structured acquisition operation. The backend consumes that operation to acquire,
invoke its known body and close exactly once before publishing a Result.
File and directory cleanup frames share the recovery unwind stack.

File/Dir value and error construction stays in checked TypeRB source. Only the
loader-owned private filesystem adapter can request the narrow libc operations;
matching authored names or imports cannot obtain that privilege. The REPL uses
the same checked bodies and a CLI-owned synchronous scope callback, clearing its
private descriptor payload before returning to the host's close operation.
See [file and directory operations](native-filesystem.md) for the public API and
its validation boundary.

The CLI/REPL under `compiler/cli/` is outside the ordinary core closure.
Snapshot recovery derives a temporary flattened source
from the canonical modules using
[strict closure validation](../recovery/src/compiler/source.trb); it does not
replace file-root imports in ordinary self-hosting. The
[organization guide](repository-organization.md) tracks further extraction
and removal of superseded implementation.

The ordinary self-hosting sequence
starts from a previous Native seed, records any setup-only transitions, and
then compares repeated full Native-built generations:

```text
previous Native seed -> setup transitions -> B2 -> B3 -> B4
compare(B2, B3, B4)   -> exact same-basename compiler and QBE bytes
```

Published native releases use a previously released native compiler as their
seed. Building the bootstrap seed from Go is a recovery/development path, not
an ordinary release requirement. Recovery and differential tests use the
reference frontend's versioned snapshot, strict validation, and the separate
snapshot-MIR/QBE path. They are mandatory coverage but do not substitute for
ordinary Native-to-Native generation.

## Bootstrap snapshot

The bootstrap snapshot is a deterministic, versioned, target-neutral, data-only
interchange for recovery. It is distinct from both the reference typed IR
and Native MIR.

It may eventually contain:

- normalized control flow and typed values;
- stable symbol and module identities;
- target-independent literals and semantic operations;
- explicit traps and required runtime capabilities; and
- source identifiers and spans.

It must not contain:

- parser, resolver, or mutable compiler objects;
- Go pointers, interfaces, callbacks, or process-local identities;
- unchecked or unresolved source;
- filesystem, network, environment, or process capabilities;
- backend-native instructions; or
- an implicit package-extension mechanism.

The consumer validates schema versions, resource limits, required features, and
all structural invariants. Unknown or unsupported input fails explicitly. While
experimental, producer and consumer revisions may be pinned exactly and the
format may change without compatibility adapters.

Hand-authored fixtures and the producer bridge established this boundary in
the early checks. Their [contracts](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/gate-reference.md#gates) remain available;
they are not unfinished prerequisites for today's ordinary frontend.

## Native MIR

Native MIR is an internal control-flow and value representation designed for
verification, optimization, layout, and code generation. It is owned by this
repository and is not a public TypeRB API or serialized package protocol.

The MIR should expose TypeRB semantics explicitly rather than relying on a
backend to infer them. Examples include checked integer operations, nullable or
tagged representations, failure traps, direct and indirect calls, allocation,
source origins, and runtime capability requirements.

Backend-specific instructions, object-format details, and linker behavior stay
below the MIR boundary. Existing backends must not force backend-specific
concepts into portable TypeRB source.

### Optimization ownership

Native MIR owns facts whose validity follows from TypeRB semantics and verified
control flow. This includes Integer ranges and nonnegativity, index validity,
loop induction and bounds, call allocation and mutation effects, Array-header
stability, and GC safe-point requirements. Target-independent passes consume
those facts to retain, move, combine, or remove semantic operations such as
range checks, negative-index normalization, bounds checks, header loads, and
root publication.

A backend adapter consumes verified MIR after those decisions. It may legalize
operations for a target ABI, choose backend instructions, assign backend
temporaries, and apply a backend-local peephole that does not require recovering
TypeRB semantics. It must not inspect source tokens or emitted backend text to
rediscover portable facts. Adding a second adapter before this boundary exists
would duplicate optimizer behavior and make a backend comparison ambiguous.

Numeric Array functions now use the same general typed CFG as other managed
functions. The former signature-selected induction/reduction builders, positional
six-block adapter and dedicated emitter are removed. `compiler/src/mir/analysis/array_loops.trb`
derives natural-loop membership, SSA forwarding, checked unit-step induction,
Array identity and stable storage from verified edges and instructions. Plans name
the preheader, owner, size operation and proved reads; verification re-derives
those plans before `compiler/src/backend/qbe/emit/arrays.trb` may reuse headers or omit access checks.
Block storage order and source tokens are not proof inputs. Unknown aliases,
changed guards or indexes, calls and allocation/mutation outside the admitted
operation set retain checks. Integer arithmetic overflow checks remain explicit.

This admits nonzero and subtracting reductions, multiple parameters,
branches and lexical transfers without a special Array function shape. The
ordinary direct body emitter and token-bound Array/header analysis are retired;
independent low-level verifier fixtures are not a second emission route.
The [MIR milestone](mir-consolidation.md) permits measured intermediate cost
regressions while preserving correctness, recovery and lifetime checks. Daily
and weekly measurements guide optimization; formal Pure Go comparison follows
the coherent milestone. Historical strict limits and rejected experiments remain
in their immutable decisions and results, not as current PR gates.

## Backend adapters

QBE is the current backend and consumes verified, target-neutral MIR.
Other entries below are possible experiments, not implemented adapters.
Target lowering selects a versioned ABI profile for an operating system and
architecture. Backend comparisons on the same target use the same profile.

| Candidate | Experimental role |
| --- | --- |
| Cranelift | Balanced fast-codegen candidate for development and AOT builds |
| LLVM | Optimizing ceiling for release-oriented measurements |
| QBE | Current Native backend |
| Direct emitter | Limited lower-bound experiment for compile time and size |

The architecture permits comparison; it does not promise long-term support for
multiple backends. A production decision should prefer one default. A second
backend remains only if a distinct use case demonstrates a durable advantage
large enough to justify its correctness and maintenance matrix.

An ABI profile defines calling conventions, symbol identity, data layout,
unwind behavior, and GC metadata for one target. Small backend-specific shims
may implement the profile, but they must not fork runtime or language semantics.
Objects from different backend implementations could be linked only after they
conform to the same profile. Per-function mixed code generation and tiered JIT
compilation are therefore deferred.

## Runtime and ABI

A machine-code backend does not provide TypeRB's runtime. The full-language,
production-use goal requires implemented and validated solutions for:

- strings, bytes, arrays, hashes, records, enums, unions, and nullable values;
- classes, interfaces, closures, and generic representation;
- allocation, ownership, garbage collection, and stack maps;
- failure traps, stack unwinding, and TypeRB source traces;
- module initialization and symbol visibility;
- filesystem, process, time, networking, and other portable runtime services;
- cancellation, scheduling, and concurrency; and
- native package integration, lifecycle, and error conversion through a
  separately accepted TypeRB design.

The initial runtime covered static data, scalar values, simple aggregate layout,
observable output and deterministic process failure. The current ordinary runtime
also manages Strings, Arrays, records and Hash values; remaining coverage is
tracked in the [ordinary language plan](native-language-coverage.md). Ordinary
Strings store byte length and code-point count separately, with UTF-8 data after
the managed header. I/O and Hash equality use bytes; language size/index use code
points. Non-ASCII indexing can allocate, with its lifetime effects owned by MIR.
The runtime shares one bounded UTF-8 decoder between scalar decoding and
counting. When a String's byte length equals its point count, every decoding unit
has width one, including malformed single-byte units. Splitting such a String
reuses each contiguous fragment's byte length as its point count. Other Strings
retain bounded decoding; exactly one ASCII delimiter lets the runtime count only
the shorter fragment and derive the larger count from the original String.
All paths copy fragment bytes and preserve ordinary temporary roots and
independent mutable result Arrays.
Portable Integer conversion validates and accumulates digits in one bounded
pass. After the portable magnitude overflows, a validation-only tail preserves
invalid-format priority without further arithmetic. Failure leaves the output
slot untouched.

Snapshot
aggregate recovery retains a separate heap-free layer, keeping record and
tagged-value semantics independent of allocation strategy.

StringBuilder owns a traced descriptor containing its backing allocation and
used-byte count. Capacity belongs to private byte storage and is never exposed
as an immutable String. Appends root the owner and source across geometric
growth; snapshots copy initialized bytes and compute Unicode length after
joining fragments. Clearing retains at most the initial small buffer and drops
larger backing storage. JSON parsing reuses one such scratch builder, clearing it
after each independent string snapshot. The JSON implementation alone may load the checked byte-span
adapter. MIR independently verifies its argument and mutation contract, while
the runtime checks byte bounds before copying.

Managed snapshot recovery uses an exact-root, non-moving tracing collector for dynamic Strings,
Arrays, closures, and recursively reference-containing aggregates.
Heap-free snapshot aggregates remain unboxed. Managed roots use compiler-emitted
shadow-stack frames, and heap descriptors identify managed fields without
placing target layouts in the bootstrap snapshot. See
[Decision 0005](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/decisions/0005-managed-runtime-and-tracing-gc.md).

The first collector is single-threaded and stop-the-world. Concurrency,
generational or moving collection, finalizers, and weak references are deferred
implementation choices rather than new language promises.

The ordinary self-hosted emitter reuses that collector for its supported
dynamic Strings, Arrays, and reference-containing records. Its current root
representation is one exact managed-reference stack with per-function
watermarks, loop compaction, alias roots, and managed-return preservation.
Managed values loaded from containers retain independent roots across later
allocation: replacing an element may remove the owner's last reference to a
still-used alias. Verified MIR supplies explicit live-before sets. Record field
identities and types have one verified MIR owner, also consumed by GC
descriptors. Constructor operands and returned projections participate
in the same live-before analysis. The collector does not conservatively scan the machine stack. Fixed descriptors,
Array-backing reclamation, and deterministic pacing are therefore shared by
compiler-generated applications rather than being confined to the earlier
snapshot adapter. Versioned statistics remain an internal
environment-controlled test path; they do not add a portable TypeRB API. See
the [runtime memory stability plan](runtime-memory-stability.md).

Runtime semantics are shared across backend candidates. Target-specific ABI
profiles and small shims may differ, but the runtime must not be independently
reimplemented for every code generator.

## Linking and toolchain independence

Removing the Go toolchain does not automatically produce a self-contained
toolchain. An experiment may still use an assembler, system linker, bundled
linker, SDK, C ABI library, or backend sidecar. Reports must distinguish:

- no Go toolchain requirement;
- no external compiler requirement;
- no external linker requirement; and
- a single self-contained `trb` distribution.

Every required component counts toward build time and toolchain distribution
size. Dynamically supplied system libraries must be identified rather than
silently excluded from comparisons.

The ordinary compiler invokes explicit QBE and C-toolchain paths directly,
without a shell, and publishes the executable only after successful code
production and cleanup. Every replacement generation loads the canonical
TypeRB file-root closure; a derived flat compiler is recovery-only. Repeated
same-basename generations must reach exact compiler and QBE bytes.

Internal profiles cover Darwin arm64, Linux arm64, and Linux amd64. They select
QBE lowering and explicit linker/libm policy without forking frontend or
runtime semantics. The experimental immutable seed release supplies verified
previous-Native assets for arm64; the registered amd64 recovery starts from its
verified target-neutral root QBE and separately records setup transitions.
Neither boundary constitutes stable target or distribution support.

File-root and configured-project loading preserve explicit declaration
ownership and load only the supported closure. Internal indexes accelerate
lookup without becoming public collections or package APIs. Portable process
and math roots remain a bounded integration, not a general package manager.
The [capability map](https://type-rb.github.io/type-rb-native/) records exact
coverage; [historical architecture checkpoints](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/gate-reference.md#architecture-checkpoint-history)
retain the original experiment-to-decision mapping and seed provenance.

## Source organization

The [organization guide](repository-organization.md) separates ordinary
compiler responsibilities from snapshot/recovery adapters, runtime generation
and verification support. Compiler decomposition continues alongside MIR work.
The schedule preserves the canonical closure, explicit recovery boundary, and
historical evidence; it does not create a second compiler or wait for promotion.

## Stability and promotion

No MIR, ABI profile, snapshot, object, cache, command, or runtime API in this
repository is stable. Official TypeRB packages must not depend on it.

Promotion to a supported TypeRB target is a separate decision. It would require
representative portable conformance, source-mapped diagnostics and failures,
runtime and package boundaries, reproducible builds, primary-platform support,
an end-to-end advantage after the complete toolchain is counted, and a
reproducible self-hosted compiler build whose ordinary path does not use Go.

The bootstrap bridge remains recovery-only and removable when its retained
consumers have replacements, not because removal is the default project outcome.
Checks expose correctness, performance, and maintenance problems early enough to
improve the shared MIR, runtime, backend, or build pipeline before those choices
become public contracts.
