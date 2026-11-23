# 0075: Streamed Array and Range batches in MIR

The reference `each_slice(size)` contract applies to ordinary Array and
`Range<Integer>` receivers, including safe calls and `with_index`. Evaluate the
receiver once, then the Integer size once, before testing even an empty source.
An absent safe receiver skips both the size expression and the body. A zero
literal is a static error; a nonpositive runtime value takes an explicit failure
edge. A size expression that transfers control never starts an iteration.

Each batch owns fresh shallow Array storage. The requested size stays fixed;
there is no initial-length hint or whole-source conversion. Full batches observe
later changes to the retained source, including growth and shrinking. Observing
source exhaustion before a partial batch sets a persistent exhaustion flag, so
changes made by that final block cannot restart the iterator. Rebinding the
source variable does not change the retained object. Range traversal compares
the captured endpoints and exclusion bit without computing `end + 1`.

`sliced_iteration.trb` builds an outer batch loop and an inner filling loop with
ordinary typed MIR edges, scalar comparisons, Range fields and Array operations.
The filling loop finishes before the authored body is checked, leaving the outer
loop as the lexical owner of `break` and `next`; `return` keeps its function
owner. Indexed blocks count batches. Managed receivers, batch elements, aliases
and captures use existing independently verified root plans.

The syntax plan records the size expression separately from the receiver and
block. The checker validates those boundaries and the receiver/parameter types;
MIR-only scratch slots do not consume lexical declaration identities. The
existing Boolean guard instruction gains closed reason 3 for invalid batch
sizes, retaining its available-Boolean and failure-block requirements. QBE only
adapts these operations; it neither reconstructs batching nor analyzes source.
The REPL owns a retained cursor with the same exhaustion contract.

Shared ordinary/REPL cases cover live and exhausted sources, fresh shallow
batches, portable Range extrema, once-only effects, safe calls, lexical transfers,
generic and nullable elements, captures and invalid forms. Independent controls
erase source, reorder MIR blocks, mutate size regions/guard operands, force
collection and check bounded streaming and retained REPL replay. Ordinary Native
fixed points and recovery include the new canonical module. Iterable conversion,
concurrent mapping and remaining collection APIs stay in their owning inventory
groups. This advances the basic-language and MIR milestones without claiming
full coverage or final performance qualification.
