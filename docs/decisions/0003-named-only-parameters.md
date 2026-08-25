# 0003: Named-only parameters

## Context

TypeRB originally used `name:: Type` for a typed keyword parameter. The double
colon also denotes namespace access, is unfamiliar in a parameter list, and
does not create a visible boundary between positional and labelled calling
conventions. The implementation consequently inferred too much from token
shape, lost labels across project imports, and let the three backends bind the
same call differently.

TypeRB has no compatibility obligation for this pre-release syntax. The
replacement must make the public calling convention explicit, preserve source
evaluation order, and remain representable by Ruby, Go, TypeScript, the REPL,
interfaces, and imported package contracts.

## Decision

Portable TypeRB has exactly two parameter regions:

```trb
positional-only | * | named-only
```

A bare `*` starts the named-only region. It is a separator, not a rest
parameter:

```trb
def request(
	url: URL,
	*,
	timeout: Duration = Duration.seconds(30),
	retry_count: Integer = 2,
): Response
```

Named-only parameters may be required:

```trb
def connect(*, host: String, port: Integer = 443): Connection
```

Calls bind named arguments only to named-only parameters. Named arguments may
appear in any order, but all positional arguments must appear first:

```trb
connect(host: "example.com")
connect(port: 8443, host: "example.com")
```

The following calls are errors for
`def request(url: URL, *, timeout: Duration): Response`:

```trb
request(timeout: duration, url)       # positional after named
request(url: url, timeout: duration)  # url is positional-only
```

Declarations use one canonical order:

1. required positional parameters;
2. omittable positional parameters;
3. bare `*`;
4. required named-only parameters;
5. omittable named-only parameters.

Trailing positional defaults remain supported. The `name:: Type` spelling is
removed and receives a migration diagnostic.

### Binding and evaluation

Unknown labels, duplicate arguments, missing required arguments, and a
positional argument after a named argument are compile-time errors.

Explicit argument expressions are evaluated from left to right in the order
authored at the call site. Their values are then associated with parameters by
position or label. Omitted default expressions are evaluated at callee entry,
in declaration order, and only when their argument is absent. A default may
refer only to parameters declared before it.

When dispatch selects an overriding method, the dynamically selected
implementation owns omission handling. Its default expression is evaluated at
callee entry; neither the caller nor the receiver's static type materializes a
source default. A native declaration's omittable parameter is different: the
TypeRB caller preserves omission and the native callee handles it.

### Call-signature equivalence

One shared parameter descriptor carries these contracts through local source,
project imports, interfaces, official packages, native declarations, typed IR,
and editor tooling. It contains:

- parameter kind: positional or named-only;
- the label for a named-only parameter;
- the parameter type;
- presence: required or omittable.

For positional parameters, order, type, and presence are part of call-signature
equivalence; their source names are not. For named-only parameters, label,
type, and presence are part of equivalence. Named-only declaration order and
default expressions are not.

Interface implementation and inherited instance-method override initially
require exact call-signature equivalence and an equivalent return type. This
deliberately includes exact required/omittable presence. Default expressions
may differ. Allowing an omittable implementation where a contract requires an
argument is a possible later, backward-compatible relaxation.

Interfaces cannot declare positional or named-only defaults. An interface has
no body that can own their evaluation, and calls through an interface cannot
omit a required parameter. Native Declaration Protocol `Optional` metadata
remains valid because it describes omission accepted by the native callee.

### Portable boundary and lowering

Portable method parameters reject `*args` and `**kwargs`, and portable calls
reject `*values` and `**values`. Only bare `*` is accepted as the named-only
separator. Explicit Ruby-native source retains Ruby compatibility syntax behind
its platform import, except for the removed `name:: Type` spelling.

Ruby lowers named-only parameters to native keyword parameters. TypeScript
uses a compiler-owned labelled options object. Go uses a compiler-owned
labelled map. Calls pass omission through those representations, and generated
callee-entry code evaluates source defaults. These representations do not
depend on named-only declaration order.

Record construction such as `User.new(id: id, name: name)` remains a separate
field-labelled construction feature. It does not declare or infer named-only
method parameters.

## Consequences

- Public APIs visibly distinguish positional-only and named-only parameters.
- Reordering named arguments and named-only declarations cannot change
  binding, backend ABI, or signature equivalence.
- Default evaluation has one target-independent owner and order.
- Import and tooling metadata retain labels instead of reconstructing them
  from positional names.
- Existing `name:: Type` declarations must add a bare `*` and replace `::`
  with `:`.

## Deferred work

- Named parameters on `fn` values and labels in first-class function types.
- Rest parameters, argument splats, named metadata values, `**` forwarding,
  and implicit forwarding.
- Separate external labels and local parameter names.
- Variance or relaxed presence rules for callable contracts.

TypeScript parameter names in an ordinary function type do not affect type
compatibility. TypeRB named-only labels are intentionally different because
they participate in source binding and are therefore part of the callable
contract.
