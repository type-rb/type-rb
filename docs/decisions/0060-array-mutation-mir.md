# Array insertion and removal through MIR

Status: implemented subset under the basic-language completion milestone.

`unshift`, `pop` and `shift` require a mutable Array binding in ordinary files
and the REPL. `unshift` captures the receiver before evaluating its one element
argument, then inserts into that Array's current storage. Argument-side rebinding
does not redirect the operation; growth or removal through another alias remains
visible. Empty removal fails. Removed values preserve their element types and
managed identities, and can outlive the Array that contained them.

Internal MIR operation 48 has closed modes 0 (unshift), 1 (pop) and 2 (shift).
Insertion has an available Array and an exact element operand, no result, and
allocation/mutation/failure effects. Removal has one Array operand, an exact
element result, and mutation/failure effects without allocation. Malformed types,
modes, extra operands and resumable failure edges are rejected independently.
The root plan retains both managed insertion operands through possible growth.

The existing loop analysis admits only explicitly safe operations. Instruction
48 therefore invalidates retained Array storage/length plans without a special
backend exception. A load after removal still checks the Array's current size.
Dedicated controls establish that ordinary invariant traversal remains optimized
while insertion, pop and shift within the loop preserve checked access.

The runtime keeps the shared header stable, reloads backing storage after growth,
and uses overlap-safe moves. Removal clears the vacated slot, shortens the live
length and returns the selected raw slot. Float values preserve their bits;
managed results use ordinary MIR liveness at subsequent allocation points.
The retained REPL mutates the selected pool identity even when rebuilding its
internal child-index storage with seed-supported operations.

These methods exercise the existing stable assignment-position contract:
normalize/check the target before the RHS, retain any old compound value, and
recheck the retained nonnegative position immediately before storing. Shortening
can invalidate a pending write. Insertion can replace the value at a still-valid
position. Removing and rebuilding storage does not add element-generation rules.
Earlier side effects remain when the final write fails.

Shared cases cover live each/map/select/reduce/find traversal, readonly aliases,
receiver replacement, optional calls, lexical transfers and assignment failures.
Source-erased/reordered MIR and forced-collection fixtures cover scalar, nullable,
record, nested Array and captured Function elements. Recovery inventories include
the new bounded runtime owner; compiler implementation syntax remains compatible
with the existing immutable seed. Remaining collection APIs and final performance
qualification are separate work within the larger milestones.
