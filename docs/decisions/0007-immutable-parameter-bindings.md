# 0007: Immutable parameter bindings

## Context

TypeRB local bindings are immutable unless their declaration uses `mut`, but
function and method parameters were implicitly mutable. That exception made a
binding's mutability depend on how it entered a scope rather than on visible
source syntax.

Ruby, Go, and TypeScript ordinarily permit parameter reassignment. Copying
that shared backend behavior would preserve the exception rather than the
TypeRB binding model. Rust demonstrates the useful alternative: a parameter
binding can be immutable by default and explicitly mutable without making it
an `inout` parameter.

## Decision

Parameters of `def` declarations and `fn` values are immutable bindings by
default. `mut` before the parameter name makes only that implementation
binding mutable:

```trb
def advance(mut value: Integer, amount: Integer): Integer
	value += amount
	return value
end

increment := fn(mut value: Integer): Integer
	value += 1
	return value
end
```

The marker is valid in either positional region. For example, a mutable
named-only binding is written `def retry(*, mut count: Integer = 1)`.

Parameter `mut` is not an `inout` or reference-passing mode. Reassigning the
binding never assigns a new value to the caller's binding. It is an
implementation detail and therefore does not participate in function types,
call signatures, overload selection, interface conformance, inherited method
override matching, or Project Declaration Input. An implementation may add or
remove `mut` without changing how callers invoke it.

Interfaces reject `mut` because an interface declaration creates no
implementation binding. Enum payload declarations reject it because payload
fields are data, not parameter bindings. Record fields continue to use record
field syntax.

Existing binding rules continue to apply inside the implementation: a mutable
parameter may be reassigned and may receive destructive operations supported
for mutable bindings. Array identity is preserved across calls, so destructive
Array operations are visible to aliases while parameter reassignment remains
local. This decision still does not define a general mutation-effect system or
caller-side ownership model for every shared reference type.

Iterator and call-block bindings are not changed by this decision. Adding an
explicit `|mut value|` pattern requires a separate decision together with the
ownership rules for collection elements and concurrent blocks.

## Consequences

- Reassignment and destructive operations on an unmarked `def` or `fn`
  parameter produce the same `declare it with mut` diagnostic as immutable
  local bindings.
- Generated Go, Ruby, and TypeScript signatures do not gain a target-level
  marker or hidden argument.
- Existing implementations that intentionally rebind or mutate a parameter
  must add `mut`; callers and interface declarations do not change.
- The typed IR retains parameter mutability for analysis and tooling without
  adding it to semantic callable identity.

## Alternatives considered

- Keeping parameters mutable by default was rejected because it preserves the
  inconsistency with every ordinary local binding.
- Treating `mut` as `inout` was rejected because it would add caller-visible
  aliasing, writeback, and backend ABI semantics unrelated to binding
  mutability.
- Including `mut` in function types or interface signatures was rejected
  because local reassignment is not a caller contract.
- Changing block bindings in the same decision was rejected because a usable
  design also needs explicit block-pattern syntax and element ownership rules.
