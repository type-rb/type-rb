# 0014: Stable Array assignment positions

## Context

Arrays are shared reference values. A pending element assignment must survive
right-hand-side calls that grow the same Array, including growth that replaces
its backing storage. Evaluation order must also agree across generated code
and the typed-IR REPL. Retaining a physical element address or reevaluating a
receiver after such a call does not implement this contract.

Negative indices add a separate question: when does `-1` become a nonnegative
position? Reading an old last element but writing a new last element can move
an update merely because the right-hand side appended a value. For compound
and short-circuit assignments this can separate the calculation or condition
from the position being updated.

## Decision

Ordinary, compound and short-circuit Array element assignments retain the
same Array identity and nonnegative position throughout the statement:

1. Evaluate the receiver and index, in that order, exactly once. If evaluating
   the index changes the Array, use its length after that evaluation.
2. Normalize a negative index using that length and check its bounds. An
   initially invalid target fails before evaluating the right-hand side.
3. For `=`, evaluate the right-hand side. For compound assignments, first read
   the old element value once, then evaluate the right-hand side and apply the
   operator to the saved old value. Do not reread it after the right-hand side.
4. For `&&=` and `||=`, test that saved Boolean. When it short-circuits, skip
   both the right-hand side and the write. These operators do not introduce
   truthiness or nullable initialization.
5. Immediately before writing, check that the retained nonnegative position
   exists in the retained Array's current storage. Write there, or fail if it
   is now out of bounds. Do not normalize the original negative index again,
   redirect the write, or extend the Array automatically.

Failure or an enclosing control-flow transfer before the write leaves that
write unperformed. Earlier side effects remain; assignment is not a
transaction. An ordinary `Result::Err` value is still a value unless consumed
by an operation such as `try` that transfers control.

Nested receivers are evaluated as ordinary reads. In `rows[f()][g()]`, retain
the inner Array obtained by `rows[f()]` before evaluating `g()`. Replacing the
outer entry during the right-hand side does not change the retained receiver.
Aliases of the retained Array observe the final write.

This fixes a position, not an element's identity. `shift`, `unshift`, or
reordering can change the value at that position. If the right-hand side
empties and rebuilds the Array, writing succeeds when the position is valid
again; there is no element-generation tracking.

## Examples

Start each statement below with a fresh `[1, 2]`. Let `grow(values)` append
`3` to the same Array and return `9`:

```trb
values[-1] = grow(values)                # [1, 9, 3]
values[values.size() - 1] = grow(values)  # [1, 9, 3]
values[-1] += grow(values)               # [1, 11, 3]
```

If the right-hand side instead removes the last element and returns `9`, both
`=` and `+=` fail at the final bounds check, leaving `[1]`. If it changes the
old last value to `20` and returns `9`, `+=` still writes `11`, because its old
value was already read as `2`.

## Alternatives

Keeping a raw negative index and normalizing at every access is coherent as a
getter/setter model. However, an append can make a compound assignment read
one position and write another. A short-circuit condition can similarly
trigger a write to a newly appended position. Stable nonnegative positions
avoid this movement for append-only changes.

Evaluating the entire right-hand side before the target is also coherent for
ordinary assignment, but changes receiver and index side-effect order and
requires separate rules for short-circuit assignment. Using one retained
position rule across the existing assignment forms is preferred.

Initial bounds checking and final bounds checking are both explicit parts of
the decision. Capturing a position by itself would not determine when an
initially invalid target fails.

## Implementation boundary

Typed lowering captures the receiver, requested index, validated position,
and any old value before right-hand-side control flow. The final store uses
current storage. Backends must not substitute a cached element pointer for
the Array identity, or let target-language assignment rules change the order.
No new source syntax, public API, persistent element-reference type or
rollback mechanism is introduced.
