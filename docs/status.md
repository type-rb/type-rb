# TypeRB Status

Last updated: 2026-08-06

TypeRB is an alpha compiler implemented in Go. The language, standard library,
generated output, and command-line interface may change before beta.

## Tooling

Implemented:

- `trb fmt` with deterministic tab indentation and preservation of comments,
  interpolation, percent literals, and heredoc contents.
- `trb build` with Go, Ruby, and TypeScript backends selected by
  `trbconfig.jsonc`.
- `trb build --compile` for producing a Go executable through temporary
  generated source, using `bin/<project-name>` or an explicit `--outfile`.
- `trb run` for temporary compilation and execution of the conventional
  top-level `main()`.
- A project-aware typed-IR REPL with persistent state, multiline editing,
  history, reverse search, checked completion and suggestions, live syntax
  highlighting, interrupts, imports, and type inspection. Completion and token
  classification use reusable presentation-independent language services.
- A local browser playground with TypeRB syntax highlighting, formatting,
  typed-IR execution, and generated-source views for all three backends.
- A guided browser tour whose lessons can be validated across every mode with
  `trb tour --check`.
- Project initialization, dependency configuration, target-manifest
  generation, and target package installation.
- Managed and external package modes, allowing generated source to be embedded
  without modifying a host project's manifest.
- Project-tree builds that preserve non-TypeRB files when requested.
- Tag-driven binary releases and Homebrew distribution.

## Compiler

The implemented compiler pipeline is:

```text
lossless tokens -> syntax AST -> resolver/type checker -> typed IR -> backend
```

The compiler currently provides:

- byte, line, and column source spans;
- a handwritten recursive-descent and Pratt parser;
- deterministic project import graphs and cross-file symbol checking;
- import, inheritance, and duplicate-entrypoint diagnostics;
- a target-independent Declaration IR for compiler-owned library types;
- compiler-owned standard packages lowered through typed intrinsics;
- separate Go, Ruby, and TypeScript code generators; and
- an initial self-host tree with deterministic bootstrap checking.

Backends consume typed IR. They do not inspect parser state or rewrite source
text.

## Portable language

Implemented portable behavior includes:

- functions, classes, inheritance, interfaces, modules, records, and constants,
  including portable function and method names ending in `?` or `!`;
- typed fields, `readonly`, private members, class members, and checked
  initialization;
- immutable `:=` bindings and explicit `mut` for reassignment or destructive
  collection operations;
- mode-independent diagnostics for unused ordinary imports, method-local
  bindings, iterator parameters, and enum-pattern bindings, with `_` discard
  and readable `_name` opt-out forms;
- non-nullable Boolean conditions and portable operator semantics;
- `if`, `elsif`, `else`, `while`, integer ranges, structured Array/range
  iteration, typed Hash key/value iteration, `break`, `next`, and `return`;
- typed Arrays and `Hash<K, V>` values with strict and safe lookup operations;
- normalized union types, collection union inference, and exhaustive scalar
  type-pattern narrowing;
- payload enums, exhaustive pattern matching, and explicit initial generics for
  payload enums and top-level functions;
- `Result<T, E>` and `Unit`;
- value-producing `map`, `select`, and `reduce` expressions;
- portable scalar receiver operations, strings, bytes, Unicode classification,
  text building, collections, logical paths, filesystem access, process
  execution, and JSON/JSONC codecs;
- local packages shared across projects with different output modes; and
- explicit mode-checked platform imports without grammar variants.

Ruby target interoperability can preserve explicitly imported native syntax in
dedicated AST and IR nodes. Other backends reject those nodes.

## Verification

The repository test suite covers the lexer, parser, resolver, checker, typed IR,
formatter, REPL, project builds, standard packages, type providers, and all
three backends. Generated programs are parsed or executed with their target
toolchains where available.

## Current limitations

The current alpha does not yet provide:

- a complete everyday receiver API beyond the initial scalar baseline for
  strings, Arrays, and Hashes, or structured error types for every safe parse
  and lookup operation;
- position-typed tuples or type-pattern narrowing for nullable, collection,
  enum, record, and class union alternatives;
- inferred type arguments or generic records, classes, and methods;
- complete superclass construction and override semantics;
- first-class blocks or multi-statement collection transformations;
- concise Result propagation syntax;
- stable source maps and runtime stack mapping;
- incremental builds and a persistent build cache;
- a language-level test runner;
- an LSP or editor extension; or
- compatibility guarantees for production use.

The broader path to practical production use is tracked in the
[roadmap](roadmap.md).
