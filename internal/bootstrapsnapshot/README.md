# Internal bootstrap snapshots

This package exports a deliberately bounded, versioned data format from checked
TypeRB IR. It is not a general backend or a public compiler extension API.
Unsupported source constructs return explicit diagnostics.

Versions 3 and 4 lower `if` and `while` Boolean condition trees containing
short-circuit `&&`, `||`, and nested `!` through existing branches, jumps and
block parameters. The left operand runs once; the right runs only when needed.
Loop backedges return to the original condition entry, and lexical bindings
are carried to each continuation explicitly. No format opcode is added.

Logical value expressions outside these condition trees, including logical
subexpressions in call arguments, remain unsupported. Supporting them requires
carrying enclosing expression temporaries across blocks, not simply making
binary instruction emission eager. Version 2 is unchanged. These are snapshot
coverage limits, not restrictions on ordinary TypeRB language semantics.

Versions 3 and 4 also lower statement `break` and `next` in `while` through
existing jump edges. Transfers select the nearest loop exit or original
condition entry and carry the current values of the loop's outer bindings;
body-local bindings do not escape. Conditional transfers use the same checked
conditional lowering. A terminating loop body no longer requires an implicit
backedge, so an early `return` keeps its method target. Function and closure
lowerers do not inherit enclosing loop targets. Version 2 and unsupported
iteration-block constructs retain their existing boundaries.

Version 4 also lowers direct Array `each` and `each.with_index` statements
through existing Array operations and control-flow edges. It retains the source
once, reads its live length and element before each block, and uses an internal
cursor independent of mutable block parameters. Appending and replacing elements
remain visible; rebinding the source variable does not retarget the traversal.
Outer bindings, including those shadowed by block parameters, cross loop edges
explicitly. `next` advances the cursor, `break` leaves the nearest loop, and
`return` retains its method or closure target. Closures can capture managed
iteration elements and values referenced only within an iteration body.

This slice requires one block parameter for `each` and two for `with_index`.
It uses the existing version-4 Array element types: Integer, Boolean, String,
function values, nested Arrays, and supported records. Float Arrays, Range and
Iterable sources, batches, and result-producing iteration remain outside this
snapshot subset. Versions 2 and 3 keep their earlier coverage. No snapshot
opcode or format version is added, and ordinary language behavior is unchanged.
