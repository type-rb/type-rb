# Nominal newtypes and representation erasure

Follow the reference's `newtype Name = Representation` contract. Construction
and explicit `.value()` projection retain a distinct semantic identity without
allocating an extra wrapper in compiled programs. Ordinary methods, class
methods, defaults, named arguments, lexical privacy and captured `self` use the
existing checked function and call pipeline. Transparent aliases and imports
preserve the declaration owner.

An optional `do` body accepts methods and a single `private new` before them.
That directive restricts construction to the declaration's lexical body, including
its nested functions, and requires a recursively immutable representation.
Open newtypes can contain mutable values; projection preserves their existing
aliases and ordinary mutation rules. Construction does not grant implicit
conversion or method forwarding. Equality follows the reference's recursively
expanded scalar and payloadless-enum cases. Generic newtype declarations and
nominal Hash keys remain rejected by the reference contract.

`NewtypeDefinition` owns authored identity, origin and unresolved representation.
Representation resolution detects duplicate declarations, invalid or nullable
outer types, and expansion cycles through collections, unions and callables.
Records and enums retain their separate stored identity; their fields participate
in the closed constructor's recursive immutability check.

Verified MIR owns `MirNewtype` declarations and distinct nominal type IDs.
Construction and projection instructions validate their owner, operand, result,
availability and source origin. The independent verifier checks representation
cycles and closed mutability without source declarations. Both operations have
no allocation or other runtime effect. Scalar functions enter ordinary control
MIR before using them; no alternate direct emitter implements newtypes.

MIR storage queries erase only the physical wrapper. QBE signatures, indirect
calls, nullable payloads, fields and Array/Hash slots choose Float versus word
storage from those verified queries. GC derives managedness from the same
representation while liveness retains each semantic value's identity. This
avoids both unnecessary scalar boxes and treating wrapped Float bits as pointers.

The REPL retains a nominal value around its evaluated representation, remaps
declaration IDs when its catalog changes, and checks retained state using typed
witnesses that do not reopen a closed constructor. Display and explicit projection
follow reference behavior; failed edits leave retained values available.

The feature belongs to the existing basic-language completion milestone, #454.
Validation combines paired check/build/execute/REPL cases, source-erased and
reordered MIR, forged identities/storage/cycles/root plans, Unicode/NUL values,
forced GC, exact reclamation and retained session replay. All seven new canonical
modules participate in recovery input and source-mutation checks. Remaining
class/interface representations, generic method parameters and general output
conversions follow their owning incomplete families; this change does not claim
full basic-language completion.
