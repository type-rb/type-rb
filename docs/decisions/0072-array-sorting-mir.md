# Stable natural Array sorting

Implement the pinned reference's non-destructive `Array.sort()` and
`sort_descending()` for non-nullable Integer, Float and String elements.
The result has independent storage. Equal elements retain their input order,
including negative/positive zero; NaNs follow ordinary numbers in both directions.
Other element types, extra arguments and authored String ordering operators
remain rejected. Key-based sorting is a separate collection transform.

## Verified construction

Build a bottom-up stable merge sort from ordinary typed MIR control flow,
bindings, comparisons, Array copies, loads and stores. Two rooted copies supply
alternating source/destination buffers, with no allocation per merge pass.
Take from the left run on ties. Clamp run boundaries and width growth by testing
the remaining distance before addition, preserving the portable Integer range.
This takes O(n log n) comparisons and O(n) auxiliary storage.

The checker keeps sorting inline while its caller has fewer than 32 completed
MIR blocks, before guarded-loop expansion. This is a code-placement budget,
not a safety rule. Once a caller is
already large, it shares a typed MIR helper for each value/key type and direction.
Natural-sort input copies remain in the caller, and key sorting still collects
its values and keys before the call. Helpers use the same control-flow builder,
call-effect analysis, verifier and root plans as authored functions. This bounds
caller CFG growth and avoids threading unrelated caller bindings through sorting.
No new sort instruction, runtime type dispatch or source-dependent backend path
is introduced.

String comparison extends the existing internal String comparison operation
with one verified strict-order mode. Its adapter compares bounded byte spans
and uses length for equal prefixes; embedded NUL does not terminate comparison.
Byte order agrees with Unicode code-point order for valid UTF-8 and preserves
the pinned reference's behavior on invalid bytes constructed by byte escapes.
The operation does not allocate or introduce a public String ordering operator.
Float direction is applied only after checking NaNs.

The backend consumes explicit comparison and Array operations. It does not
recognize source-level sort calls, choose an algorithm or reconstruct run bounds.
Private bindings cross the same typed edges and root analysis as authored locals.
The REPL retains value identities in independently allocated result storage and
uses the same ordering rules, including failure recovery and replay.

## Verification and remaining work

Paired cases cover ordinary check/build/run and retained REPL behavior. MIR tests
erase source and reorder blocks before executing with forced collection, reject
forged comparison operands/modes/failure edges, and reject missing scratch roots.
The CLI checks Unicode/NUL storage, all lengths from zero through 67 against an
independent oracle, complete reclamation and retained independent copies.

The initial reference REPL reversed NaN placement in descending order.
[TypeRB #773](https://github.com/type-rb/type-rb/pull/773) fixes that defect, and
the accepted reference update reviews its changed observation. Existing
presentation differences remain explicit. [Decision 0073](0073-keyed-array-sorting-mir.md)
extends the same merge machinery to key-based sorting; safe collection APIs
remain part of basic-language completion.

The immutable bootstrap seed remains unchanged. The core constructs sort MIR
using its previously supported source subset; no new seed syntax is required.
Full recovery, ordinary fixed points and target correctness remain integration
requirements. This does not establish complete language coverage or performance
qualification.
