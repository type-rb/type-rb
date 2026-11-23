# Typed Web context values

The pinned `trb/web` package exposes `ContextKey<T>`, `Context.with`,
`Context.fetch`, and `Context.with_request`. Native supplies their typed
semantics while retaining the complete pinned package source bytes.

```trb
key := ContextKey<Integer>.new("count")
first := context.with(key, 1)
second := first.with(key, 2)
```

Each key instance identifies its own entry. Two keys with the same name remain
independent, including keys with different value types. The key determines the
type accepted by `with` and the `Result<T, ContextValueError>` returned by
`fetch`. A missing entry preserves the key's name in the error. Receiver, key,
and value expressions execute once in source order.

Updating a context creates a new context and a new entry list. Existing contexts
and branches retain their entries. Stored values retain their ordinary aliasing
semantics: storing an Array does not deep-copy that Array. `with_request`
preserves path parameters and attached values while replacing the request.
Ordinary middleware and `Next` calls propagate the resulting context.

## Checked implementation

Loader-owned declarations add the two generic methods. Their complete
signatures and nominal method edges authorize specialization. The checker
infers the value type from the canonical `ContextKey<T>` argument, then checks
the remaining arguments against that type. Specialization produces ordinary
TypeRB bodies; speculative forks do not modify shared source tokens or controls.

The private `ContextStorage` helper implements entry lookup and replacement in
TypeRB. Its typed boxing, type-test and checked extraction declarations lower
to existing union MIR operations. Boxes retain semantic type identity, including
nullable and aggregate values. The private helper is admitted only from the
loader-owned Web source, and primitive selection validates declaration ownership,
the complete generic signature and each concrete instantiation.

Two representation operations complete the implementation:

- MIR 74 copies initialized concrete class storage and replaces one field with
  a value of that field's exact type. It creates fresh storage and preserves the
  source object. Independent verification checks both operands, result identity,
  field bounds, field type and the absence of inherited storage.
- MIR 75 compares the identity of two instances of the same concrete class and
  returns Boolean. Its shape and nominal operands are independently verified.

Readonly field stores remain restricted to verified initializers, and an
uninitialized receiver cannot escape through either operation. QBE uses only
verified field layouts and managed-value metadata. Copy operands remain rooted
across allocation. The REPL executes the same checked helper bodies and keeps
typed boxes and object identities across retained program catalogs.

Current entry lookup is linear in the number of attached keys, and replacement
copies that entry list. This implementation establishes behavior and managed
lifetimes; it does not establish an application performance result.

## Verification and remaining work

The shared registry covers scalar, Bytes, time, enum, union, Any, nullable,
record, generic record, class, interface and container values; key aliases;
missing entries; independent branches; generic callers; evaluation order;
middleware; and invalid types and imports. Callable-body cases verify REPL
value operations with deterministic output. They do not establish the separate
binding-echo contract: raw observations of that display retain the difference
described below, and the registry lists it as an uncovered contract.

Provider units verify source-erased emission, nominal ownership, complete
signatures, speculative isolation, corrupted MIR fields and understated
allocation effects. `tools/native-web-context-test.py` exercises cross-module
values, retained REPL catalogs and forced collection. Terminal collection drops
program-global roots as well as temporary and compiler roots; readonly global
keys are included in the shutdown regression.

The pinned Go REPL echoes compiler-owned context state as a host object dump
containing addresses and internal type information. Native's typed storage has
a different representation. These display differences remain unresolved under
[#1192](https://github.com/type-rb/type-rb-native/issues/1192); no new
portable inspection contract or reference pin is adopted here.

File routing and [endpoint contracts](native-web-endpoints.md) are separate
checked providers. Networking, streaming and server shutdown remain work under
the [production Web application path](decisions/0086-production-web-application-path.md).
