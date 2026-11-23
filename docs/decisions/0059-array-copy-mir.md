# Array edges and shallow copies through MIR

Status: implemented subset under the basic-language completion milestone.

Ordinary files and the REPL support Array `empty?`, `first`, `last`, `dup`,
`reverse` and `slice(Range<Integer>)`. Empty edge access fails explicitly.
Copies allocate a fresh outer Array and retain the same element identities;
mutating a copied outer Array cannot modify the source's length or slots.
Managed inner values remain shared. A readonly binding constrains operations
through that binding; it does not freeze aliases or recursively freeze elements.

Slice evaluation captures the receiver before evaluating the Range. Endpoints
run once in source order. Rebinding the source variable does not retarget the
call, while mutation of the retained Array is visible when the call executes.
An absent optional receiver skips its arguments. Ranges require nonnegative,
ordered limits; exclusive ends may equal size, inclusive ends must be smaller.
Checks precede inclusive increment and allocation, including portable extrema.

Existing MIR length, scalar comparison, Boolean failure guard and checked index
operations own edge access. Guard reason 2 denotes an empty Array. New internal
MIR operation 47 owns shallow copying with closed modes 0 (dup), 1 (reverse),
and 2 (slice). It requires an available Array, the same result type, an available
Integer Range only for slice, and no resumable failure edge. Allocation and
terminal failure effects are explicit; independent verification rejects malformed
shapes and root plans that omit retained operands.

The backend selects the exact scalar or managed element descriptor and calls a
bounded runtime. That runtime allocates one capacity-sized buffer, copies raw
element slots, and preserves Float bits and managed identities. The header's
allocation anticipates the backing-buffer cost; allocation/reclamation counters
include both. A fresh empty result retains normal growable Array storage.
No source spelling, iteration plan or guessed bounds proof is an emission input.

Dedicated controls erase frontend data, reorder MIR block storage, force
collection before copy allocation, retain nested managed values after source
replacement, grow copied buffers, and check final allocation accounting. Shared
ordinary and REPL cases cover mutation capabilities, optional calls, transfers,
scalar/managed representations and failure boundaries. The canonical compiler
closure has 110 modules, with independent recovery inventories and mutations.

The exact reference advances to the accepted Array receiver-evaluation fix in
TypeRB PR #737. Generic library calls using an outer type parameter and Function
results expose separate reference checker gaps; these stay explicit in the
shared contract. Remaining collection APIs, declaration families, diagnostic
parity and final performance qualification remain open.
