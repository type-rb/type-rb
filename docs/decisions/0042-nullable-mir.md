# Nullable values in ordinary MIR

## Semantic ownership

Optional identities retain the exact payload type. Internal Nil is distinct
from Integer zero and Boolean false and cannot be authored as a type annotation.
`nullable_types.trb` owns assignability and common-type queries. Arrays and Hashes
recursively resolve their element/value types rather than enumerating nesting
levels. Unannotated collections with Nil and a different base type require
union support; they are not silently given an optional type.

`nullable_flow.trb` owns direct lexical and stable record-field facts. Each
binding replacement invalidates previous field facts. Facts apply to direct
nil-comparison branches, a short-circuit RHS, and the continuation after a
returning guard. Conditional/loop assignments do not export a precise assigned
value type to a parent path. Repeated bodies invalidate earlier facts for
bindings they replace, and loop-local shadowing does not invalidate an outer
binding. Compound conditions do not introduce additional narrowing beyond the
pinned reference contract. Class/callback and general pattern flow remain gaps.

Safe navigation evaluates its receiver once and lowers to ordinary typed
blocks. Only the present edge evaluates the member and authored arguments.
A Void call joins without a result; a value call joins a nullable result,
flattening an already optional member result. A nonnullable receiver uses the
ordinary member operation. These rules and parsed suffix boundaries also drive
the checked REPL evaluator; QBE does not inspect source or narrowing facts.

## MIR and representation

Four typed operations make absence, injection, nil testing and checked extraction
explicit. Their verifier checks operand availability, exact payload identity,
result type and operation shape. Extraction checks for nil at runtime, so a
forged control-flow assumption cannot turn into an unchecked pointer load.
Numeric optional widening maps a present Integer to Float through ordinary CFG
edges and leaves absence unchanged.

The initial private ABI uses zero for absence and a managed one-slot box for a
present value. A descriptor traces the payload when it is managed. Injection is
an allocation safepoint; shared liveness and root plans retain both the payload
and its owners. Records, Arrays, Hashes, calls, returns and defaults keep one
word per optional value. This representation prioritizes correctness and a
uniform ABI. Escape analysis, scalar replacement and specialized layouts are
future MIR optimizations; this integration is not a performance qualification.

## Evidence and compatibility

Focused tests erase frontend metadata, reverse physical block order and force
collection before allocation/calls. Negative cases exercise stale binding and
field facts, malformed operations, unannotated Nil, unchecked member access and
immutable aliases. The ordinary CLI fixture and shared case registry also
exercise the REPL and retained values. Capabilities is generated from that
registry and keeps remaining gaps visible. The subsequent
[checked submission decision](0043-checked-repl-submissions.md) covers retained
REPL assignment flow and direct assignment display. Statement-control result
display and Hash separators still differ and remain explicit shared-case gaps.

The pinned Go reference accepts nullable numeric widening but omits a necessary
conversion in its typed IR. [TypeRB PR 692](https://github.com/type-rb/type-rb/pull/692)
corrects the generated targets and REPL. The isolated
`reference-compiler-fixed/nullable-numeric-widening.source` fixture tests this
correction without replacing the exact reference pin or changing existing
shared expectations. The separate
[loop-backedge bug](https://github.com/type-rb/type-rb/issues/693) in the reference
checker was fixed by [TypeRB PR 695](https://github.com/type-rb/type-rb/pull/695).
Native conservatively invalidates pre-loop facts for replaced bindings. These
reference fixes do not change the exact reference pin used by this integration.

The implementation remains readable by the immutable seed and snapshot v4.
New syntax is not yet used in compiler implementation sources. The ordinary
closure has 65 modules; exact recovery import boundaries and mutation controls
include every added dependency. Ordinary core/CLI fixed points and separate
recovery remain required before acceptance.
