# Literal constraints and common union members in MIR

Follow the pinned reference's explicit Integer/String literal type contract.
An authored literal can satisfy its singleton annotation; computed scalars and
inferred scalar bindings cannot narrow implicitly. Literal values widen to their
ordinary scalar type, and scalar alternatives subsume their literals in unions.
A literal type cannot itself have optional, Array-suffix or generic modifiers.
Containers, declarations and callable signatures can carry literal arguments.

Canonical singleton spellings encode String bytes without syntax delimiters.
They preserve equality across escape spellings and prevent quoted commas, angle
brackets or NUL from changing union/container structure. Compiler byte access has
one source owner shared by the frontend and backend constants.

MIR retains distinct singleton type IDs with a validated scalar representation.
A singleton constant records its exact value and scalar tag; an explicit widening
operation erases its constraint. The independent verifier checks the table,
constant payload, operand availability and result identity after source erasure.
QBE consumes these verified operations as constants and identity copies. No
singleton box, source inspection or runtime type inference is introduced.

Literal unions reuse the existing verified union representation and typed CFG.
Homogeneous alternatives widen through checked edges before numeric operations,
Range construction, ordering and equality. Mixed Integer/String literal cases
compare alternative tags, keeping `0` distinct from `"0"`. Singleton Hash keys
retain their semantic type IDs with the ordinary scalar key representation.
Hash keys formed from literal unions still need a separate verified key layout;
this change does not erase their constraint or accept unverified storage.
Common data members require every nominal alternative to expose the member
through the same storage model. Record alternatives use verified record layouts;
class alternatives use the same typed field loads as ordinary class receivers.
Mixed record/class storage, private class fields, missing members and common methods
remain rejected. Exhaustive
literal cases and branch-local discriminant narrowing belong in semantic
checking and MIR, preserving overlapping alternatives and mutation invalidation.

Common callable fields use indirect calls after the same checked projection;
this does not select ordinary methods from a union. Field readonly flags prevent
replacement, while mutable receivers retain the existing shallow collection
capabilities of their fields.

Class discriminants must be readonly in every alternative. Reading a common
mutable field is valid, but it cannot establish a stable discriminant fact.
Direct lexical receivers retain overlapping alternatives or the unhandled
alternatives of an `else` branch. Rebinding invalidates the fact; ordinary mutable
fields remain writable through a correctly narrowed mutable class receiver.
Generic fields accept validated singleton types without replacing type parameters
with arbitrary concrete types during declaration validation.

Common fields lower to union tests, checked payload extraction, record/class
loads, explicit widening and typed join arguments. No new backend instruction
or source-dependent emitter analysis is needed. Independent verifier controls
reject forged extraction identities and field layouts; source-erased/reordered
MIR, forced collection and retained REPL replay cover managed alternatives.

Common class-union stores select and retain the receiver before evaluating the
right-hand side. Each alternative must expose an equivalent, writable class
field and the receiver binding must be mutable. MIR branches on the union tag,
extracts the verified class payload and emits the existing typed object store
for that alternative. The right-hand side is evaluated once; rebinding the
source binding during that evaluation does not redirect the store. Readonly,
private, missing or differently typed fields reject before MIR publication.

Output of a union whose alternatives are printable scalars follows the same
verified tag branches and payload extraction. Each branch uses the ordinary
scalar conversion and output operation. An unprintable alternative still
rejects during semantic checking; union output does not add a backend opcode.

The pinned reference rejects external private fields through unions
([TypeRB #816](https://github.com/type-rb/type-rb/pull/816)) and executes common
class-union field stores ([#817](https://github.com/type-rb/type-rb/pull/817)).
The shared registry pairs Native and reference check, build, execution and
REPL outcomes for ordinary, imported and side-effecting stores and rejected
alternatives. This does not complete union parity.

The REPL preserves the singleton's semantic type independently of scalar storage.
Retained declarations, failed edits and replay must preserve those constraints.
Validation pairs ordinary check/build/execution/REPL observations with explicit
rejection cases, independently forged MIR and forced-GC lifetime controls.
This is part of basic-language completion in #454, not a performance qualification
or a claim that all union, object or generic features are complete.
