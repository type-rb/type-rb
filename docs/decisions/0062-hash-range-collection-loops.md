# Hash snapshot iteration and Range materialization

## Behavior

Implement the existing TypeRB `Hash.each` and `Range<Integer>.to_a()` contracts
in ordinary Native files and the REPL. Hash iteration binds exactly a key and a
value. It visits a shallow entry snapshot, with unspecified enumeration order.
Adding or deleting entries, replacing scalar values, or rebinding the receiver
inside the body cannot change that snapshot. Managed values preserve their
identity. Hash indexed iteration and transforms remain rejected, as in the
reference implementation.

Range materialization returns a fresh Array. Inclusive/exclusive endpoints,
empty and reversed ranges use the existing iteration rules. Bounds are evaluated
once. An inclusive portable maximum is appended and exits without computing a
successor outside the Integer domain. There is no artificial iteration limit.

## Representation

The syntax projection names its optional second parameter neutrally: it is an
Array/Range ordinal or a Hash value. The checked receiver kind and canonical
Hash type determine both parameter types. Ordinary iteration and loop transfers
validate parameter roles; transforms continue to admit only Array and Range.

Hash keys and values are projected into two fresh Arrays before executing user
code. Both projections traverse the same runtime table slots. The receiver,
keys and values are explicit managed MIR operands with independently verified
root plans. Deletion and table rebuild during the body cannot invalidate them.
The body reads each matching pair by a private cursor through ordinary Array
instructions and typed loop edges.

Range materialization lives in a small frontend lowering module. It shares the
iteration endpoint test, carries its result in a private SSA binding and appends
through ordinary Array instructions. No synthetic source iteration, new opcode
or runtime entry point is required. QBE consumes executable MIR alone.

## Evidence and remaining work

Forty new shared cases cover key/value representations, snapshots under
mutation and rebinding, shallow aliases, nested loops, lexical transfers,
generic receivers, retained loop captures, Unicode/NUL, optional calls, fresh arrays and portable
Integer extrema. Previously rejected Hash iteration and Range materialization
cases now pass. Rejection cases enforce parameter count and supported receiver
forms. The registry, feature inventory and Capabilities projection move together.

Independent controls erase frontend data, reorder MIR blocks, force collection
before allocation/calls, and reject forged receiver/key plans and missing live
roots. Ordinary CLI checks additionally preserve Unicode/NUL snapshots and
retained REPL enums, and verify complete GC accounting after repeated traversal.
The immutable-seed and full recovery authorities remain required. This closes
these collection contracts, not the full basic-language milestone; safe APIs,
joining, sorting and sliced iteration remain in the completion plan.
