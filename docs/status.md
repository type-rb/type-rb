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

The early official `trb/web` package discovers file-based routes at compile
time and runs typed request, path-parameter, JSON decode, and JSON response
handlers through the same dispatcher in all three backends. Applications can
start a generated Go, Ruby, or TypeScript HTTP server with `serve()`. Unhandled
handler failures become a portable JSON 500 response. Root and nested
`_middleware.trb` files form the same outer-to-inner onion chain in every
backend. The first packaged middleware emits JSONL access logs and supports
typed output and path-exclusion options. A portable secure-headers middleware
adds a conservative browser-security preset and accepts an explicit typed
header map. An opt-in CORS middleware handles actual and preflight requests,
explicit origin policies, credentials, exposed and allowed headers, and typed
preflight cache duration. A request-ID middleware preserves bounded safe
incoming IDs or generates cryptographically random IDs and exposes the chosen
value to downstream handlers and the response. Production lifecycle controls
and additional middleware are still under development. Routing distinguishes
missing paths from unsupported methods and returns a portable JSON 405 response
with an `Allow` header. Request bodies are limited to 1 MiB before dispatch and
oversized requests receive the same JSON 413 response in every backend. Query
parameters use the portable URL decoder and preserve repeated keys and source
order instead of collapsing them into a hash. `query_values` returns all
repeated values, while strict `query_value` reports malformed, missing, and
duplicate values through a typed error. HEAD requests prefer an explicit
handler, otherwise reuse the matching GET handler and middleware chain, and
never expose a response body. OPTIONS requests likewise prefer explicit
handlers; otherwise a middleware-aware 204 response advertises the available
methods through `Allow`.
Request header lookup is case-insensitive; `header_value` rejects missing and
duplicate values instead of choosing one implicitly. Portable cookie parsing
preserves header order, duplicate names, and opaque values without delegating
semantics to the target runtime.
Responses can replace, append, remove, or inspect case-insensitive header values
without mutating the original response. `vary` composes cache keys without
duplicating an existing field.
Typed response cookies support ordered `Domain`, `Path`, `Max-Age`, `Secure`,
`HttpOnly`, and `SameSite` attributes while preserving multiple `Set-Cookie`
header values.
Portable `text`, `bytes`, `empty`, and `redirect` builders create common
responses with consistent default statuses and content types. `with_status`
returns a copy with a different status.
Before a response leaves the portable dispatcher, every backend rejects invalid
status codes, header names, and CR/LF-bearing header values through the same
JSON 500 boundary.

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
