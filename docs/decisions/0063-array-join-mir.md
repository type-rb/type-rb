# Array joining through MIR

## Reference contract

`Array<String>.join(separator: String): String` follows the exact pinned
reference declaration and receiver-evaluation contract. Evaluate and retain the
receiver first, evaluate the separator once, then read the retained Array's
current elements. Argument effects may grow, shorten, clear or replace elements;
rebinding the receiver variable does not redirect the operation. Empty Arrays
still evaluate the separator. Optional absence skips it, and lexical transfers
leave the operation unexecuted.

Joining needs no mutable receiver and never changes its Array or Strings. No
implicit element conversion, default separator, nullable-element overload or
nested-Array flattening is introduced. Preserve String bytes, including NUL and
invalid UTF-8. Decode the completed byte sequence for its code-point count:
separate fragments can form a valid character after joining.

## Typed ownership and runtime

MIR instruction 49 has an available `Array<String>` first operand, an available
`String` separator, a `String` result, zero payload and no resumable failure edge.
Independent verification rejects every other shape. Its allocation/failure
effects require both operands in the live-root plan even when dead after the
operation. The Array transitively retains its elements.

The adapter emits one runtime call without source inspection. The runtime reads
the retained Array only after arguments have finished, measures its total bytes
with checked addition, allocates the final String once and copies each fragment.
One final UTF-8 scan handles cross-fragment sequences. Work is linear in element
count plus output bytes. No user callback runs between sizing and copying in
the current single-mutator, nonmoving runtime; future mutation/GC models must
revisit that assumption explicitly.

A verified single-use String Array plan may omit the container and its initial
backing storage. It admits at most four already available fragments, one
same-block join and complete uses consisting only of the construction appends
and that join. Additional uses, escaping containers, mutation, unrelated effects
and allocating separators retain ordinary materialization. The canonical MIR
and its conservative effects stay intact; verification independently derives
both the plan and the corresponding fragment roots.

The adapter reserves the bounded pointer workspace once in the function prologue
and passes it to the same byte-joining runtime used by ordinary Arrays. The
workspace is not a managed object or a GC root. Exact root plans directly retain
each String owner and the separator across result allocation, including repeated
fragment aliases. Only eliminated operations lose their internal allocation
safepoints; every remaining failure and full-result UTF-8 scan stays unchanged.

The retained REPL gathers its stored String values and uses ordinary `join` for
assembly. This is verified CLI self-use: the current Native core compiles the
CLI after its immutable-seed fixed point. The core implementation itself remains
in the preceding seed and recovery snapshot subset; no seed change is needed.

## Acceptance

Shared ordinary check/build/run/REPL cases cover empty/singleton Arrays, Unicode,
embedded NUL, split UTF-8 sequences, argument mutations/rebinding, optional calls,
closures, nested transforms and lexical transfers. Negative cases reject wrong
element types, separators and arities.

Independent MIR tests erase source, reorder block storage, force collection and
forge types, operands, modes, failure edges and root plans. Ordinary CLI controls
collect immediately before final allocation, check complete reclamation, bound
total allocation for a large join without timing assertions, exercise both size
guards using a reduced test-only capacity, and retain results across REPL input
and rejected calls. Recovery closure and mutation inventories include the runtime
module. This completes joining, not sorting, safe APIs or the full language.
