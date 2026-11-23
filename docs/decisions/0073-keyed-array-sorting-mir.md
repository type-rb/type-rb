# Stable key-based Array sorting

Implement the reference's `Array.sort_by` and `sort_by_descending` as structured
value-producing blocks with exactly one parameter. Keys must have portable
Integer, Float or String natural order; values may have any supported Array
element type. Preserve the existing restrictions on escaping block transfers,
extra arguments and `with_index`.

## Evaluation and identity

Evaluate the receiver once and retain its identity during live Array traversal.
Evaluate one key per visited element, including elements appended during the
block. Retain the visited value before evaluating the key, so replacing its
source slot or reassigning the block parameter cannot redirect the result.
Capture keys as values; the comparison loop never re-executes the block.
Return an Array with independent outer storage and shallow element identity.

Collect visited values and evaluated keys in separate fresh Arrays through the
existing checked iteration/MIR machinery. Extend the natural-order merge builder
with paired source/destination key buffers. Comparisons read keys; each selected
write moves both value and key. Take the left run on ties, keep NaNs last in both
directions and retain signed-zero order. Reuse the bounded internal String
comparison from [decision 0072](0072-array-sorting-mir.md).

Every loop, edge, read, write and copy is ordinary verified MIR. The backend
receives no new sort operation, source recognition or callback dispatch. Managed
values and keys remain live across both collection growth and scratch allocation.
The construction uses O(n log n) comparisons and O(n) auxiliary storage for n
visited elements. This is an algorithmic bound, not a performance measurement.
The checked projection records the key type for REPL evaluation and rejects
unsupported or forged source/key combinations.

## Verification and remaining boundaries

Exercise primitive, nullable, union, record, nested-Array and callable elements;
empty inputs; key effects, live mutations, captures, narrowing and nested blocks;
Unicode/NUL keys; Float NaNs and signed-zero ties; and rejection cases. Erase
source and checked projections and reorder MIR blocks before execution. Force
collection around allocations, reject omitted roots, and compare stable element
identities against an independent oracle across 68 lengths. Retained REPL tests
cover failure recovery, declaration remapping and replay.

The exact reference includes [TypeRB #773](https://github.com/type-rb/type-rb/pull/773),
which fixes descending NaN order in the REPL. Safe navigation on block iteration
remains a defect in [TypeRB #774](https://github.com/type-rb/type-rb/issues/774);
Native continues to reject that syntax and the shared registry records the gap.
Sliced iteration, safe collection APIs and other basic-language work remain open.

The immutable bootstrap seed is unchanged. Ordinary fixed points and full
recovery remain separate requirements; the compiler implementation stays in the
supported snapshot source subset. This does not claim completed language parity
or qualified performance.
