# Safe collection blocks through nullable MIR

The accepted reference preserves safe navigation when parsing a member into a
collection block. Native follows that contract for `each`, sequential transforms
and Array key-based sorting. This extends the existing nullable and iteration
owners; it introduces no new backend instruction or source-dependent execution.

## Evaluation and types

Retain the receiver once before testing absence. The absent edge skips operation
arguments and the block. The present edge extracts the payload, evaluates initial
values where applicable and uses the existing traversal. Arrays inspect live
storage, Hashes retain entry snapshots, and Ranges stream captured endpoints.
Receiver rebinding does not redirect that traversal. Mutation and readonly rules
are unchanged.

Value-producing blocks join the present result with absence. Already-nullable
results do not gain another nullable layer. Non-nullable receivers retain their
ordinary result type. Statement iteration has no result and preserves its
lexical `return`, `break` and `next` owners. Invalid blocks still receive static
checking even when the receiver is absent. Indexed calls place the safe operator
before the operation: `values&.each.with_index`; a safe modifier is rejected.

The reference correction is [TypeRB PR #775](https://github.com/type-rb/type-rb/pull/775).
The exact reference advances to its accepted merge commit. Its AST adds the safe
flag to iteration; no AST node family disappears from the shared inventory.
The earlier safe-sort rejection/panic probe becomes a supported ordinary case.

## MIR and binding ownership

Checking resolves the same immutable block regions and concrete traversal types
used by ordinary iteration. Nullable tests and payload extraction dominate the
present traversal, and ordinary typed joins carry results and updated locals.
QBE consumes those verified operations and edges. REPL execution skips an absent
receiver before visiting initial values and preserves the checked result type.

Combination controls also exposed a lexical-identity defect: Array queries,
sorting and Range materialization introduced MIR scratch bindings only during
lowering. Those slots must not advance the declaration identities selected during
analysis. Scratch slots now have no lexical identity, while authored declarations
retain their analyzed identities and mutable capture cells. Their value types and
GC roots still cross ordinary MIR control edges.

An optional collection literal first uses the present container's contextual key
and element types, then applies the usual nullable conversion. This preserves
empty collections and nullable elements without inferring a different union.

## Verification and remaining scope

Shared check/build/execution/REPL cases cover absent and present receivers, every
supported safe transform, lazy receiver/initial effects, live mutation, indexed
iteration, nested nullable collections, closures, generic calls and rejection
boundaries. Four fixtures execute after erasing source and checked projections,
reordering blocks and forcing collection. CLI controls retain managed Unicode/NUL
values across repeated collections and exercise failed edits, type remapping and
replay. Existing independent MIR and root-plan verification remains required.

Sliced iteration, structured concurrent transforms and remaining safe collection
APIs are still open. The shared registry includes explicit presentation and
reference gaps; these probes do not establish full language coverage or final
performance qualification. No release or immutable seed changes here.
