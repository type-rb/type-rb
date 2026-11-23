# Standard Result control flow through ordinary MIR

## Context

Concrete generic enums already have canonical instances, checked payloads,
verified MIR operations and traced runtime layouts. Standard Result needs the
same representation plus ownership, propagation and required-use rules from
the reference language. It must not introduce a parallel exception mechanism
or a backend that recognizes source spellings.

## Decision

Load `trb/std/result` and `trb/std/unit` as compiler-owned TypeRB declaration
sources. They pass through the ordinary lexer, parser, resolver and nominal
catalogs. A loader-assigned module tag identifies the standard declaration;
an application enum named `Result` does not acquire standard behavior.
Bare and named imports retain their normal declaration identity rules.

Prefix `try` checks one postfix operand against the enclosing function's Result
contract. It evaluates that operand once, tests the variant, extracts the Ok
payload, or converts the Err payload with the ordinary safe assignment rules
and returns an Err of the enclosing Result instance. Ordinary `each` does not
introduce another propagation boundary.

Statement-value `catch` checks the Result once and creates an Ok path plus a
handler path with an immutable error binding. The handler contributes a value
assignable to the success type, or transfers to its lexical return/loop owner.
Arguments and collection elements do not acquire a new catch grammar.

Both forms lower to existing enum construction/test/extraction, value joins,
branches and return/loop terminators. MIR verification and managed-value
liveness therefore cover the same operations as authored exhaustive cases.
The QBE adapter receives no Result syntax, template, source-token or handler
interpretation responsibility.

Required-use tracking belongs to checked lexical bindings and statement-value
contexts. Scope checks run before branch/loop locals are removed. Bare standard
Results and unused local Result bindings, including underscore-prefixed names,
are rejected; handling, passing, returning and storing remain ordinary uses.
The rule is shallow and does not recursively inspect containers or establish
path-sensitive ownership. Interactive entry values may be displayed and kept
for later submissions; authored functions retain the ordinary checks.

The REPL consumes explicit checked Result operation projections. It uses the
existing lexical transfer state for early return, break and next; target
exceptions and runtime panics are not caught as Result errors. Projections
remain optional compiler metadata and are unnecessary for Native emission.

## Validation and remaining boundaries

Controls cover Unit, success and error paths, exactly-once/lazy effects, each
propagation, handler transfers, numeric and nullable error conversion, nested
Results, managed payloads, invalid propagation boundaries, user-defined
homonyms and required-use rejection at function/branch/loop scopes. Source and
checked projections are erased before repeated emission; forced collection
exercises the ordinary root plans. Shared cases keep check/build/execution/REPL
outcomes separate and generate the Capabilities view.

Transparent aliases, callable values, collection transformations, general
unions and compiler-declared structured package boundaries still depend on
their corresponding language/package families. This change does not claim
those forms, a complete standard library, a seed refresh or final performance
qualification. The ordinary compiler continues to use syntax supported by its
unchanged immutable seed; recovery maintains the exact expanded source closure.
