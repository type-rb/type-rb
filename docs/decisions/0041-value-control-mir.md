# Value-producing control flow in ordinary MIR

## Ownership

Full `if`/`elsif`/`else`, scalar `case`, ternary expressions and conditional
`return`/`break`/`next` use parsed branch regions shared by checking and the REPL.
These regions are frontend syntax metadata. QBE never reads them to recover
control flow, evaluate a branch or determine a result type.

The checker resolves lexical transfer targets and computes the common type of
branches that produce values. Equal supported types join directly; Integer and
Float join as Float. Every value-producing full control currently requires an
`else`. Scalar case selectors are Integer or String, with literal alternatives.
Enum, union, nullable, exhaustive case and pattern-binding support remain separate
coverage work. General statement modifiers are not added by conditional transfers.

`mir_value_control.trb` constructs ordinary blocks and typed jump arguments.
Each completing branch passes its current outer bindings and result to an exit
block. Once the checker knows the common type, that exit performs any numeric
conversion and jumps to the common result block. This avoids editing previous
instructions or relying on source order in an adapter. Branch-local bindings do
not escape; immutable reference capabilities survive the value join.

A lexical transfer contributes no result operand. If every reachable branch
transfers, the checker uses internal `Never` to propagate that fact through the
containing expression. No fabricated value is passed to a call or join, and later
operands do not execute. Unreachable source still receives syntax and type
checking. A short-circuit Boolean expression retains its completing short path
even when the evaluated right side always transfers. `Never` is not a public
type annotation.

## Validation and compatibility

Ordinary conformance and REPL fixtures cover value joins, side effects, widening,
managed aliases, scalar case alternatives and conditional lexical transfers.
MIR tests erase source and control metadata, reverse physical block storage,
force collection before allocation/calls, and reject an unavailable join result.
The shared language registry records each ordinary path independently.

The pinned reference has a parser defect around a full control expression inside
parentheses or call arguments. The public reference fix is
[TypeRB PR 691](https://github.com/type-rb/type-rb/pull/691). Native regression
tests cover the corrected expression nesting, while the shared parity probes use
forms accepted by the exact pin. This change does not replace the reference pin.

Default initializer bodies cannot use `return` to escape through their private
MIR helper. Full control initializer parity remains outside the pinned reference
contract.

The compiler implementation remains readable by the immutable checkout seed and
snapshot v4. Compiler self-use of the newly admitted value-control syntax needs
an accepted seed refresh. Ordinary fixed points and separate recovery checks
remain required; neither a seed refresh nor a performance qualification follows
from language coverage alone.

## Decomposition boundary

The new value-join builder is independent of recursive source checking. Splitting
the mutually recursive expression and statement checker into separate modules
would currently create a compiler-module import cycle, which Native rejects.
Further decomposition needs a representation or dispatch boundary that removes
that cycle. Copying helpers or introducing forwarding modules does not resolve it.
