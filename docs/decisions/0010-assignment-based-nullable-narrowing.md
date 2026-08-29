# 0010: Assignment-based nullable narrowing

## Context

TypeRB narrows a nullable lexical binding after a direct `nil` comparison.
Before this decision, assigning to that binding always restored its declared
nullable type, even when the assigned expression was known to be non-null:

```trb
def display_name(mut name: String?): String
	if name == nil
		return "anonymous"
	end
	name = name.strip().downcase()
	return name
end
```

The assignment is valid because `String` is assignable to `String?`, but the
following return previously treated `name` as `String?`. That made a precise
assignment lose information and made editor type display disagree with the
later diagnostic.

Ruby has no separate declared static type to preserve. Go keeps the declared
type unchanged and requires explicit handling. TypeScript and Kotlin both use
control-flow information about local assignments, although their broader
union, smart-cast, and mutation rules differ. TypeRB needs one smaller rule
that works through its explicit nullable conversions in Go, Ruby, TypeScript,
and the REPL.

## Decision

A mutable nullable lexical binding has two types:

- its declared type, which is the permanent assignment contract; and
- its flow type, which describes the value known on the current path.

A plain `=` assignment is checked against the declared type. After a valid
assignment, subsequent statements in the same control-flow path use the most
precise supported assigned type:

- assigning the non-nullable base type makes the binding non-nullable;
- assigning `nil` makes the binding's path type `Nil`; and
- assigning a nullable value keeps the declared nullable flow type.

An implicit `Integer` to nullable `Float` assignment has `Float` as its flow
type because the stored value has already undergone the portable numeric
conversion.

An assignment inside a conditional branch, loop, function value, iteration,
or call block does not narrow the parent path. The construct may not execute,
may execute more than once, or may execute later. The assigned flow type is
available to the remaining statements inside that path only. Existing
returning-guard promotion remains limited to facts proved by the guard.

Assignment invalidates nullable field facts derived from the assigned receiver.
Compound assignment keeps its existing operator and declared-target checks and
does not establish a new nullable flow fact.

Typed IR retains an explicit nullable-to-non-nullable conversion at each use
that relies on the assignment fact. Backends therefore do not rely on their
target language's native flow analysis.

## Consequences

- Normalization pipelines can reassign a nullable parameter and immediately
  use the non-null result without a redundant second `nil` comparison.
- Diagnostics and editor type information use the same flow type after the
  assignment.
- A later assignment of `nil` prevents non-null member access immediately.
- Branch-local assignments cannot make code after a non-exhaustive construct
  unsound.
- The source syntax and runtime representation do not change.

## Alternatives considered

- Always restoring the declared nullable type was rejected because it loses a
  fact established by the assignment itself.
- Changing the binding's declared type was rejected because a later value that
  is valid for the original annotation must remain assignable.
- Exporting assignment facts from every branch was rejected because TypeRB
  does not yet join mutable flow types across complete control-flow constructs.
- Adding assignment narrowing for `Any`, interfaces, and arbitrary non-nullable
  unions in the same change was rejected. Those types need a general,
  representation-aware narrowing conversion rather than the existing portable
  nullable unwrap.

## Deferred work

- Flow-type joins for complete `if` and exhaustive `case` assignments.
- Representation-aware assignment narrowing for non-nullable unions and
  interface bindings.
- Compound-assignment narrowing where the target representation and operator
  result can be retained explicitly in typed IR.
