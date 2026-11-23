# Declaration-scoped default initializers

Status: implementation decision within the ordinary language and MIR milestone.

## Behavior and representation

The pinned reference evaluates every explicit call/constructor argument in
authored order before evaluating omitted defaults in declaration order. A default
sees its declaration module and preceding parameter/field bindings. It cannot
capture caller locals or a later slot. Every default must type-check even when
unused by the currently observed calls; managed values are fresh per omission.

The parser retains each default's token span, owner and slot. After resolving
authored declaration types, `default_arguments.trb` assigns a private function
identity with the preceding slots as typed parameters and the declared slot type
as its result. Source functions keep their original identities. Private names do
not enter the source name index or REPL completion list. Record initializers
share one typed parameter prefix per record, avoiding duplicated source or a
quadratic parameter metadata table.

The ordinary resolver and function checker validate these bodies in declaration
scope. Call checking first captures all explicit expressions, then emits normal
MIR calls for omitted defaults and finally the full-arity source call or record
construction. Presence belongs to the checked binding, independently of the
value; final MIR never carries an absent argument, invalid managed placeholder
or fixed-width presence mask. Ordinary signature, control-flow, effect and root
verification apply to initializer calls and bodies. QBE consumes the resulting
verified functions and operands without consulting default source metadata.

The REPL uses the same initializer identities and parameter prefixes, evaluated
in fresh declaration environments. Its expression evaluator remains separate
from Native execution and is checked by paired ordinary/REPL cases.

## Trade-offs and boundaries

Private initializer calls keep source scope and effects explicit without changing
the ordinary callable ABI. They can increase calls and live arguments; shared MIR
inlining, liveness and unused-parameter optimization can improve them later.
Migration acceptance observes costs and does not claim performance qualification.

Copying default source into callers would require repeated scope/origin handling
and risk capturing caller names. A callee-entry presence convention is another
possible lowering, but would require typed optional carriers, initialized-value
proofs and an ABI policy. Neither is needed for these statically resolved calls.
Future methods/function values must preserve the same declaration identities and
evaluation contract; this decision does not claim those callable forms or nullable
values are already implemented.

Acceptance requires reference and Native check/build/run/REPL comparisons,
negative declaration/type/scope controls, fresh Array/Hash/record defaults,
source-erased and reordered MIR under forced collection, ordinary fixed points
and the full recovery/target/lifetime authorities. Compiler implementation source
does not yet use authored default syntax because the immutable checkout seed
cannot parse it. An accepted seed refresh must precede that self-use.
