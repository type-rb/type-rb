# Object types and execution through MIR

Status: implementation in progress within the basic-language milestone.

Classes and interfaces retain nominal identities separate from records. Authored
object declarations preserve their source owner, type parameters, fields,
initializers and method kinds. Concrete instances own resolved type arguments,
parent and interface edges, and field types. Recursive field expansion publishes
a nominal shell before following storage edges; inheritance cycles are rejected
independently. Transparent aliases and imported names select the same declaration,
while generic applications with different arguments keep distinct identities.
Template checking copies the concrete instance catalog and lookup index.

MIR composite kinds 13 and 14 identify classes and interfaces. The MIR object
catalog owns declaration origins, parent and interface type IDs, and field types,
origins and readonly flags. Fields remain attached to their declaring type;
inheritance does not flatten away the owner needed for privacy and initialization.
All nominal shells precede container and field resolution, permitting recursive
storage through records, classes, Arrays, Hashes and nullable values.

The independent verifier rejects missing or mismatched nominal IDs, wrong object
kinds, invalid origins, malformed fields, non-class parents, non-interface
contracts and cyclic inheritance. Source-level repeated `implements` declarations
are accepted and normalized to unique resolved edges. An interface carries no
class fields or inheritance. Neither structural resemblance nor a matching display
name creates nominal conformance. The pinned reference rejects assigning a child
class value to a parent-class binding; the inheritance graph does not authorize
that conversion. An inherited explicit interface edge remains available.

Object methods, including nongeneric methods, retain immutable templates.
Interface templates describe contracts and cannot instantiate executable functions.
Concrete class methods specialize the class arguments followed by method arguments;
ordinary function signatures and calls own argument binding, defaults, results and
receiver identity. Discovery finishes before MIR signatures and layouts are frozen.

MIR instruction 63 allocates zeroed, traceable class storage. A class with fields
must immediately enter its registered initializer, after explicit arguments and
omitted parameter defaults have been evaluated. Instructions 64 and 65 load and
store typed fields. A CFG analysis derives receiver aliases and intersects the
initialized-field sets at joins and backedges. It rejects reads and escapes before
initialization, completing returns with missing fields, and readonly stores outside
the declaring initializer. Declaration defaults execute in order and can read
initialized earlier fields. Zeroed GC storage is not a language-level default.

Interface method signatures use declaration kind 4 without executable source
bodies. The method catalog retains receiver identity, parameter names, positional
and named-only regions, and required/optional presence. Explicit nominal edges
produce witnesses binding every interface method to a concrete implementation.
Named arguments are permuted by label; positional names and implementation-local
mutability do not affect conformance. Missing methods and mismatched signatures
are rejected even for unused declarations and generic templates.

Instruction 66 converts a concrete class value into a managed interface box with
the original object and a verified witness table. Independent MIR checks validate
that table's owners, complete method set, target signatures and argument
permutations. QBE translates the table to call adapters and emits a tracing
descriptor for the retained object; it performs no source-level lookup. Arrays,
Hashes, nullable values, arguments, returns and captured receivers use the same
managed representation. Source-erased emission, forced collection and corrupt
conversion/witness tests exercise the boundary.

Field-free parent methods specialize against the concrete child receiver while
preserving the declaring scope for constants and private access. Inherited
explicit interface edges reuse those concrete targets. Parent constructors and
parent field storage remain guarded until portable initialization is defined.
Overrides remain guarded while the reference dispatch discrepancy is unresolved.

The retained REPL consumes the checked callee identity, preserves nominal values
with their original type catalog, and remaps concrete generic identities when a
later declaration changes catalog order. Typed, never-executed projections carry
stored values into the next check without calling their constructors again.
Class/interface input framing, project imports and explicit replay use the same
ordinary declaration and checking contracts.

The [byte encoding and request ID implementation](../native-encodings.md) keeps
encoding semantics in ordinary TypeRB bodies and gives the OS entropy bridge an
owned signature with independently checked MIR effects and types.

The [typed Web context implementation](../native-web-context.md) adds independently
verified immutable class-copy and instance-identity operations. They preserve
the initialization and readonly-store restrictions described above.

The shared ordinary check/build/execution/REPL registry includes positive and
negative object cases plus qualified record, class and interface contracts.
The ordinary CLI authority also exercises retained fields, heterogeneous interface
values, nested nominal identities, returned captures and replay. Required
hosted integration authorities remain separate from these focused checks; the
remaining guarded contracts prevent a claim of complete object coverage.

Construction and class-method calls through a transparent class alias now resolve
to the canonical target, including generic, qualified and imported classes. The
checked allocator and constructor path owns the resulting MIR; an alias does not
create a second nominal class. The pinned reference incorporates this behavior
([TypeRB #820](https://github.com/type-rb/type-rb/pull/820)), qualified class execution
([TypeRB #811](https://github.com/type-rb/type-rb/pull/811)), nominal interfaces
([#809](https://github.com/type-rb/type-rb/pull/809)), namespaced records
([#812](https://github.com/type-rb/type-rb/pull/812)), private REPL receivers
([#806](https://github.com/type-rb/type-rb/pull/806)), callable interface results
([#807](https://github.com/type-rb/type-rb/pull/807)) and inherited class methods
([#803](https://github.com/type-rb/type-rb/pull/803)). Paired ordinary cases retain
these owners through aliases, captures and imported descendants. Inherited
instance overrides remain a separate contract.

A separate reference discrepancy remains in inherited self-method dispatch:
Go, Ruby and TypeScript do not agree when an inherited method calls an overridden
method. [TypeRB #797](https://github.com/type-rb/type-rb/issues/797) records a
field-free reproduction. Preserve that evidence while completing executable
method contracts; a passing nominal graph must not hide the discrepancy.

The implementation follows the reference class contract without resolving its
explicitly deferred questions: superclass constructor chaining, initialization
order across initialized superclasses, mutating-method receiver requirements,
variance, generic class methods and same-named fields/methods. The pinned
reference includes constructor-path initialization checks
([TypeRB #813](https://github.com/type-rb/type-rb/pull/813)) and rejects
constructor defaults referring to an unavailable receiver
([TypeRB #818](https://github.com/type-rb/type-rb/pull/818)). Native initialization
proofs reject unsafe paths instead of exposing zeroed storage.

Acceptance for the complete family requires paired positive and negative cases,
ordinary check/build/run/REPL behavior, independent MIR corruption and lifetime
controls, compiler self-use where applicable, ordinary fixed points and required
hosted integration authorities. Capabilities and the shared registry advance only
with those executable contracts. Intermediate type-graph checks are not a claim
that the object family or basic-language milestone is complete.
