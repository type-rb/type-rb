# TypeRB Status

Last updated: 2026-08-08

TypeRB is an alpha compiler implemented in Go. The language, standard library,
generated output, and command-line interface may change before beta.

## Current capability

TypeRB uses one portable grammar and typed IR for Go, Ruby, and TypeScript
output. Projects have deterministic formatting, package configuration,
project-wide checking, source generation, temporary build-and-run, and Go
executable compilation. Target-specific behavior remains behind explicit
`trb/platform/<mode>/*` imports.

The implemented language includes functions and classes, modules and
interfaces, records, payload enums, initial generics, normalized unions,
immutable and mutable bindings, typed collections and iteration, exhaustive
pattern matching, and value-producing `if` and `case` expressions. See the
[language guide](language.md) and [specification](specification.md) for the
current semantics.

The compiler-owned portable library covers scalar and collection foundations,
`Result`, bytes, hexadecimal and Base64 encoding, SHA-256/SHA-512 hashing and
HMAC, non-cryptographic and secure randomness, Unicode text, logical paths,
URL component and query handling, filesystem and process access, and JSON/JSONC. Its
public contracts are listed in the
[standard-library reference](standard-library.md).

The typed-IR REPL supports persistent state, multiline editing, history,
completion, suggestions, syntax highlighting, interrupts, and type inspection.
The same compiler and evaluator power the local and hosted playground and tour.
The repository also publishes a reusable TextMate grammar for lexical editor
highlighting. Command details belong in the [CLI reference](cli.md).

The compiler pipeline is:

```text
lossless tokens -> syntax AST -> resolver/type checker -> typed IR -> backend
```

Backends consume typed IR and do not inspect parser state or rewrite source
text. The repository suite covers compiler phases, formatter, language
services, REPL, project builds, standard packages, type providers, generated
target code, and browser tools.

## Current limitations

The current alpha does not yet provide:

- a complete everyday receiver API;
- position-typed tuples or comprehensive narrowing for nullable and structured
  union alternatives;
- inferred type arguments or generic records, classes, and methods;
- complete superclass construction, override, and mutation-effect semantics;
- first-class blocks or multi-statement collection transformations;
- concise `Result` propagation syntax;
- stable source maps, runtime stack mapping, incremental builds, or a
  persistent build cache;
- a language-level test runner, LSP, or editor extension; or
- compatibility guarantees for production use.

Future outcomes are tracked in the [roadmap](roadmap.md); executable scoped work
is tracked as GitHub issues.
