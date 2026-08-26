# 0004: Bounded structured concurrent Array mapping

## Context

Everyday server applications frequently fan out independent HTTP, database,
or filesystem work over a collection. Requiring users to construct tasks,
spawn workers, and join them makes the common case unnecessarily procedural.
Leaving admission unbounded is easier to spell but can turn a large Array into
the same number of active requests, goroutines, Threads, or Promises. It also
makes nested fan-out multiply capacity unexpectedly.

TypeRB needs one source meaning for Go, Ruby, TypeScript, and the typed-IR
REPL. The language already forwards a compiler-owned cancellation and deadline
scope without exposing `async`, `await`, or a context parameter in source. A
collection operation can build on that scope while retaining the familiar
`map do` shape.

This decision covers I/O-oriented concurrency. CPU parallelism has different
runtime and isolation requirements and must not be implied by the same name.

## Decision

Portable `Array<T>` has an import-free `concurrent_map` transformation:

```trb
pages := urls.concurrent_map do |url|
	fetch_page(url)
end

pages := urls.concurrent_map(limit: 4) do |url|
	fetch_page(url)
end
```

The block maps `T` to `U` and the operation returns `Array<U>` in input order.
The result type is not inspected for special behavior. In particular, a block
returning `Result<U, E>` produces `Array<Result<U, E>>`.

`limit` is a named positive `Integer` and bounds the number of element blocks
that are active at once. Its portable default is 8. A non-positive literal is
a compile-time error; a non-positive dynamic value is a runtime contract
failure. The implementation creates only a bounded number of workers and
does not eagerly create one task per input element.

An outermost call creates a structured task group. An explicit limit chooses
that root group's capacity; otherwise its capacity is 8. A nested
`concurrent_map` invoked by one of its element blocks joins the same group. A
nested explicit limit may reduce its own local admission but cannot expand the
shared capacity. While an element block waits for a nested map, it temporarily
returns its group permit and reacquires it before continuing. This preserves
the global bound without deadlocking when every active element nests.

The call does not return until all admitted children have finished. Parent
cancellation stops further admission, propagates cooperatively to active
children through the hidden execution scope, and joins them before control
leaves the call. Start and completion order are unspecified. Empty input
returns an empty Array. CPU parallel execution is not guaranteed.

The block is a value-producing transformation: it ends in one result
expression and rejects `return`, `break`, `next`, and prefix `try`. Its element
binding is local to one evaluation, but the element value is not uniquely owned
because an Array may contain duplicate references. Values created inside one
element evaluation are task-owned. Mutable reference elements and reference
parameters of transitively reached authored calls are borrowed; they may be
read but not mutated. Direct aliases and containers derived from borrowed
references preserve that borrowed status. Borrowed state is monotonic for one
binding across reassignment and control-flow joins; independently created
storage must use a new local binding to regain task-owned mutation.
Assignment to an outer binding is rejected. Lexical captures are initially
limited to concurrency-safe values: scalars, Bytes, Ranges, and recursively safe
records, enums, newtypes, nullable values, and unions. Mutable containers,
function values, `Any`, StringBuilder, and class or interface instances cannot
be captured. Calls to same-module authored top-level functions, module or class
methods, instance methods, and constructors are followed transitively and
checked for shared lexical mutation. Class field defaults and parameter defaults
are part of that constructor audit, including classes with no explicit
`initialize`. Direct field initialization on a fresh constructor receiver is
allowed. Other instance field mutation is rejected until an ownership contract
can prove that its receiver is unaliased. Calls through interface-typed receivers
or function values are rejected until they can carry an explicit
concurrency-safety contract. A native function used in a concurrent block must
participate in the compiler-owned execution-scope contract; package-owned native
handles need a future explicit safety contract before they can be captured.

## Consequences

- The common API has the same receiver-first shape as `map` and needs no
  package import or task vocabulary.
- Capacity is deterministic across targets and does not depend on CPU count,
  an environment variable, or backend scheduler defaults.
- Nested fan-out cannot multiply the root limit and does not deadlock solely
  because parent blocks wait for their children.
- The operation is useful for I/O concurrency in every mode, while Ruby's GVL
  and each target's scheduler remain free to limit actual CPU parallelism.
- Arrays already contain all input values. Large or streaming data sources
  must batch explicitly and run `concurrent_map` within each batch; batch size
  and active concurrency remain separate controls.
- Conservative capture checking rejects some values that a particular target
  runtime could share safely. This is preferable to silently giving the three
  modes different data-race behavior.

## Rejected alternatives

- `concurrent.map(values)` requires an import and moves the common collection
  operation away from the established receiver API.
- `Concurrent` or `Parallel` looks like a nominal declaration in TypeRB and
  conflates I/O concurrency with CPU parallelism.
- Unbounded admission follows Promise or lightweight-task primitives too
  closely and is unsafe for large collections and external services.
- A CPU-count-derived or environment-required default makes otherwise
  portable source depend on deployment state. Environment and pool sizing can
  still be expressed with an explicit dynamic `limit`.
- Making `spawn` the entry point optimizes the API for heterogeneous task
  graphs instead of the dominant homogeneous map case.
- Automatically aggregating `Result` changes the block-to-element mapping and
  introduces failure, cancellation, and multiple-error policy into the base
  operation.

## Deferred work

- A Result-aggregating convenience operation, if application use demonstrates
  that explicit handling of `Array<Result<U, E>>` is materially inconvenient.
  No name, including `try_concurrent_map`, is reserved by this decision.
- Heterogeneous structured task APIs, race/select, detached tasks, channels,
  actors, and timeout scopes.
- CPU-parallel collection processing and its isolation/transfer rules.
- Concurrent streaming and iterator operations. The initial receiver is
  exactly `Array`.
- Explicit concurrency-safety declarations for package-owned native values and
  cross-package authored call-graph verification beyond the conservative
  initial boundary.
