# 0008: Canonical declaration and type identity

## Context

The parser preserves the spelling written by an author, while each backend
needs a target-language name. Neither is a semantic declaration identity.
Leaf-name reconstruction became ambiguous when declarations shared a name in
different modules or nested namespaces, and when a module, type, class method,
and instance method occupied distinct language namespaces. The ambiguity was
most visible in effect propagation and qualified TypeScript annotations, but
it also affects future typed-IR operations and compiler integrations.

Using a generated Go, Ruby, or TypeScript identifier as the identity would
make semantic analysis depend on one backend. Using only a source leaf name
would continue to merge unrelated declarations. Exposing more nested
declarations through Project Declaration Input would move the ambiguity into
providers rather than resolve it.

## Decision

The compiler uses one canonical declaration identity from resolution through
checking, typed IR, and backend analysis. A declaration identity consists of:

- the canonical compiler module path;
- the source-qualified declaration name, using `::` between nested owners;
- the declaration kind, such as class, record, enum, interface, type alias,
  module, function, or value.

For example, a nested record is identified semantically as
`{module: "services/config", name: "Admin::Options", kind: "record"}`.
The module path remains exact: `models` and `models/index` are different
identities even if an import convenience can resolve either path.

Member dispatch adds the member name and a class/instance discriminator to the
owner declaration identity. Consequently, an instance method never shares an
effect or calling convention merely because a class method has the same owner
and source name.

A checked `Type` retains its portable display name and, when it denotes a
declared type, its canonical declaration identity. Built-in scalar types and
type parameters do not acquire declaration identities. Generic arguments and
collection, union, and function components retain their own identities rather
than inheriting a mapping from an enclosing leaf name.

Transparent aliases keep two facts separate. The authored type remains the
alias for diagnostics and generated annotations, while semantic operations can
follow an explicit alias target identity. An alias name is not silently
replaced by its target's generated name.

Backend names remain backend-owned projections. TypeScript namespace names,
Go flattened constructor names, Ruby constants, private-name mangling, and
compiler-generated helper identifiers are not declaration identities.

This decision does not change source syntax or the Project Declaration Input
v6 exposure contract. Record-construction IR and a future PDI identity schema
consume this identity in separate changes; they do not broaden nested class,
function, Job, or provider visibility as part of this decision.

## Consequences

- Effect analysis resolves local, imported, inherited, and aliased calls by
  canonical owner and class/instance dispatch kind before using compatibility
  fallbacks for compiler-created IR.
- Typed IR can distinguish same-leaf declarations without asking a backend to
  reconstruct their source owner.
- TypeScript can qualify local nested type annotations from checked type
  identity while retaining existing import aliases for external declarations.
- Future record-construction and provider protocols have one semantic identity
  to carry instead of defining package-specific naming rules.
- Display formatting, diagnostics, and generated identifiers remain stable
  concerns separate from semantic equality.

## Alternatives considered

- Module path plus leaf name was rejected because nested declarations and
  class/instance namespaces still collide.
- Fully qualified source text alone was rejected because identical source
  names in different modules remain unrelated.
- Backend-generated identifiers were rejected because they differ by target
  and may change without a language-semantic change.
- Expanding PDI visibility first was rejected because existing providers do
  not yet have a canonical identity or generated-identifier contract for those
  declarations.
