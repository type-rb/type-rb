# Union values and typed alternatives

General `A | B` values retain the reference's flattened, deduplicated and
deterministically ordered alternatives. Integer and Float normalize to Float.
Assignment accepts each source alternative only when the target accepts it;
it never implicitly narrows a union. Array and Hash value inference use this
same common-type rule. Generic substitution resolves alternatives before
constructing a concrete union.

Scalar type cases accept exactly one Boolean, Integer, Float or String binding,
including `_` for a discarded payload. Duplicate, foreign and missing alternatives
are errors unless `else` covers the remaining alternatives. Statement and value
cases preserve lexical binding scope and all-path return checks. Composite values
may be retained and passed through compatible unions; composite type patterns,
literal types and discriminated unions remain separate coverage work.

MIR semantic type kind 10 owns a canonical alternative catalog. Instructions 52,
53 and 54 inject, test and checked-extract an exact semantic alternative. The
independent verifier requires valid catalog ownership, canonical ordering,
backward type references, operand availability, exact result types and closed
instruction shapes. Runtime tags are semantic MIR type IDs, not source names or
frontend declaration slots. Widening and optional payload conversion use ordinary
typed control-flow edges; only the selected alternative executes its conversion.

An immutable managed box contains the tag and one word-sized payload after its
GC descriptor. Per-alternative descriptors trace managed payloads and exclude
scalar bits. Injection has allocation/failure effects and publishes its input's
live root. Extraction checks the tag before reading the payload. QBE translates
these verified operations without deciding source assignability or case coverage.
This representation favors complete typed ownership and safe composition; box
elimination and alternative layouts can be measured after the integration
milestone without changing source semantics.

The REPL preserves the checked union type around its selected payload. Captures,
collection storage, retained nominal identities and replay retain that pairing.
Array literals consume their checker-owned element type instead of independently
inferring a potentially different type from evaluated values. Source witnesses
prefer visible aliases for optional union types. Grouped union annotations and
nullable alternatives retain the reference's explicitly recorded boundaries;
they do not silently introduce a different grammar or assignment rule.

`Nil` remains an internal inferred type, including inside collection elements.
Authored annotations cannot name it. Retained REPL bindings may project inferred
types only within their compiler-owned source span; new declarations, aliases,
parameters and imported files receive the same rejection as ordinary files.

Independent tests erase frontend state, reorder MIR blocks and force collection
between allocating instructions. Negative controls corrupt alternative catalogs,
instruction types and GC roots. Ordinary shared probes cover scalar cases,
numeric widening, arrays, hashes, records, enum payloads, generics, aliases,
optional values, Results, defaults, closures and invalid operations. The CLI
authority additionally retains inferred arrays and captured union variables
across new declarations, failed assignment and explicit replay.
