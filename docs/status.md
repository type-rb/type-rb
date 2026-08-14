# TypeRB Status

Last updated: 2026-08-14

TypeRB is an alpha compiler implemented in Go. The language, standard library,
generated output, and command-line interface may change before beta.

## Current capability

TypeRB uses one portable grammar and typed IR for Go, Ruby, and TypeScript
output. Projects have deterministic formatting, package configuration,
project-wide checking, source generation, temporary build-and-run, and Go
executable compilation. Target-specific behavior remains behind explicit
`trb/platform/<mode>/*` imports.

The initial distributed package system resolves TypeRB source directly from
Git repositories or explicit local paths. Short imports default to GitHub but
lock to canonical manifest identities. A deterministic `trb.lock` pins the
transitive graph, commit IDs, and SHA-256 content checksums; frozen and offline
installation are available for CI and disconnected builds. Package source uses
the same parser, project-wide checker, typed IR, and Go, Ruby, or TypeScript
backend as application source. Target-native modules requested by a TypeRB
package are merged into the generated `go.mod`, `Gemfile`, or `package.json`.

TypeScript projects select a browser, Bun, or Node runtime independently from
their npm or Bun package manager. Node remains the compatibility default, while
Bun can be selected explicitly for server packages.

TypeScript projects can also import supported named functions and React
components directly from configured native packages. `trb install` derives a
cached semantic index from installed `.d.ts` files; ordinary builds, the REPL's
project compiler, and completion consume the cache without invoking TypeScript.
The indexer currently uses the TypeScript 6 compiler API; TypeScript 7 support
is deferred until its replacement programmatic API is stable.
Unsupported declaration shapes are diagnosed instead of becoming `Any`.
Installed TypeRB packages can supply versioned declarative generic functions,
classes, records, and transparent type aliases while generated code continues
to import the original npm package. The bridge preserves discriminated generic
results and emits transitive native type-only imports without making their
names source-visible. Provider-declared Promise callback boundaries can map a
checked fallible TypeRB function to native resolution and rejection without
exposing Promise semantics in TypeRB source.

The experimental TypeScript browser path accepts structured JSX in TypeRB
source and emits ordinary TSX for React tooling. Function components use typed
record props, JSX component calls are checked across project modules, and only
modules containing JSX use the `.tsx` extension. The explicit
`trb/platform/typescript/react` package supplies the React boundary, including
purpose-specific mouse, change, form, and keyboard event types. Its typed
`use_state(initial)` wrapper infers `ReactState<T>` and exposes checked `value`
and `set(value)` members while generated TSX uses React `useState`.

The implemented language includes functions, typed first-class function values
with lexical capture and checked fallible effects, and classes, modules and
interfaces, records, ordinary and raw-value enums, payload enums as sum types,
enum instance methods, explicit generics for enums, aliases, records, classes,
top-level functions and instance methods, normalized unions, immutable and
mutable bindings, typed collections and iteration, exhaustive pattern
matching, value-producing `if` and `case` expressions, and explicit fallible
effects with `fails` and `attempt`. See the
[language guide](language.md) and [specification](specification.md) for the
current semantics.

Array transformations such as `map`, `select`, `reduce`, predicate searches,
and key-based sorting accept ordinary statements followed by one final result
expression. Their structured typed IR runs the same block scope in generated
Go, Ruby, TypeScript, and the REPL. TypeScript lowers a transformation that
reaches a suspending platform operation to a sequential async loop without
adding `async` or `await` to TypeRB source.

Integer and String literal types support status- and kind-indexed data.
Exhaustive `case` over a readonly literal field narrows the complete record or
class union in every backend and in the REPL. Ordinary scalar `case` remains
available for untyped external values that have no endpoint or data contract.

The compiler-owned portable library covers scalar and collection foundations,
including stable Array sorting, typed short-circuit predicates and nullable
searches, stable deduplication and non-destructive concatenation, canonical
strict and safe element access, Range-based Array and Unicode String slicing,
code-point substring search, `Result`,
bytes, hexadecimal and Base64 encoding, legacy MD5/SHA-1 checksums,
SHA-256/SHA-512 hashing and HMAC, non-cryptographic and secure randomness,
constant-time byte comparison, Unicode text, logical paths,
URL component and query handling, filesystem and process access, JSON/JSONC,
and immutable date/time values. The portable time package separates `Date`,
`TimeOfDay`, civil `DateTime`, exact `Instant`, fixed `Duration`, and named
`TimeZone`; its canonical JSON codecs and DST error behavior run across all
three backends.
Raw-value enums use the same checked String or Integer representation for
conversion, JSON codecs, generated applications, and the REPL. Its
public contracts are listed in the
[standard-library reference](standard-library.md).

The typed-IR REPL supports persistent state, multiline editing, history,
completion menus, project declaration auto-import, suggestions, syntax
highlighting, interrupts, and type inspection.
The same compiler and evaluator power the local and hosted playground and tour.
The repository also publishes a reusable TextMate grammar for lexical editor
highlighting. Its thin Visual Studio Code client packages that grammar and
snippets, starts `trb lsp`, and exposes compiler-backed diagnostics, completion,
formatting, and quick fixes without duplicating semantic logic. Command details
belong in the [CLI reference](cli.md).

The early official `trb/web` package discovers file-based routes at compile
time and runs typed request, path-parameter, JSON decode, and JSON response
handlers through the same dispatcher in all three backends. Applications can
start a generated Go, Ruby, or TypeScript HTTP server with `serve()`. The
`trb init --template web` command creates a buildable portable API project with
file-based routing and an explicit editable middleware stack. The
optional `configure_server` value sets host, port, request-body limit, and
graceful-shutdown timeout through typed keyword arguments. Every adapter
validates that configuration before binding, handles SIGINT and SIGTERM, stops
accepting new work, and gives active requests the configured time to finish.
Unhandled handler failures become a portable JSON 500 response. Before
middleware runs, request methods are uppercased and header names are normalized
to lowercase while insertion order and repeated values are preserved. Request
paths are decoded exactly once as UTF-8 at the
dispatcher boundary. Malformed escapes, encoded separators, backslashes, and
dot segments receive a portable JSON 400 response. Repeated and trailing
slashes remain distinct paths instead of being silently collapsed. Terminal
catch-all files such as `[...path].trb` match one or more decoded path
segments and bind their slash-joined value. Route analysis rejects catch-alls
outside the final position and any static, parameter, or catch-all pattern that
could ambiguously match the same method and request path. Calls to
`Context#path_value`
inside route files require a string literal naming a parameter declared by that
file's route pattern, so misspelled and dynamic names fail during the build.
`Context#params<T>()` binds all route parameters to a checked record and
verifies that the record fields exactly match the file-based route.
`Request#query<T>()` binds decoded query parameters to a record: scalar fields
are strict single values, nullable fields represent omission, and Array fields
preserve repeated keys. Boolean, portable numeric, raw-value enum, and
date/time conversions share the same semantics in every backend. Binding
failures remain explicit `ParameterError` results so applications choose their
own validation response policy.
`Context#bind<T>()` optionally combines path, query, and JSON body binding into
one ordinary endpoint input record. The reserved record fields `params`,
`query`, and `body` reuse the individual checked codecs, and failures retain
their original typed error inside `EndpointInputError`. Contracts remain
optional, route parameter compatibility is still verified at build time, and
applications keep control of the corresponding HTTP error response.
Typed `ContextKey<T>` values let middleware pass authentication principals,
request identifiers, and other request-scoped state to handlers without casts
or string-key collisions. `Context#with` returns a new context and
`Context#fetch` infers `Result<T, ContextValueError>` from the key; the same
identity-based behavior runs in generated applications and the typed-IR REPL.
The experimental `trb/auth/oidc` and `trb/web/auth/bearer` packages build on
that context boundary with the same OIDC bearer-token profile in all three
backends. They discover provider metadata, cache JWKS documents, verify RS256
signatures and standard issuer, audience, time, and subject claims, refresh an
unknown signing key through a rate-limited path, and attach a typed
`OidcPrincipal` to authenticated requests. Server-managed browser sessions and
authorization-code flows remain future package work.
Typed `Request#json<T>()` accepts `application/json` and `application/*+json`,
rejects ambiguous content types and invalid UTF-8, and reports each failure as
a `RequestError` without exposing backend parser behavior. Root and nested
`_middleware.trb` files form the same outer-to-inner onion chain in every
backend. A single middleware file can build an explicit `Array<Middleware>`
and pass it to `compose`; the first item is the outermost layer, and `Next` can
still be called only once. Root middleware surrounds the complete dispatch
boundary, including generated 400, 404, 405, 413, and recovered 500 responses;
nested middleware remains scoped to matched routes. Packaged middleware expose
`middleware()` factories with specific option types such as `LoggerOptions` and
`CORSOptions`. The
logger emits JSONL access logs and supports typed output-selection and
path-exclusion options. It records the normalized routing path for valid
requests but omits the raw query string by default so secrets in URLs are not
copied into logs. A portable secure-headers middleware adds a conservative
browser-security preset and accepts an explicit typed header map. An opt-in
CORS middleware handles actual and preflight requests, explicit origin
policies, credentials, exposed and allowed headers, and typed preflight cache
duration. A request-ID middleware preserves bounded safe
incoming IDs or generates cryptographically random IDs and exposes the chosen
value to downstream handlers and the response. Opt-in response-compression
middleware negotiates gzip with `Accept-Encoding`, uses a typed minimum-size
option, and consistently excludes unsafe or unsuitable responses across all
three backends. An opt-in timeout middleware adds a portable request deadline,
returns a JSON 504 response, and propagates a compiler-owned cancellation scope
through handlers, ORM work, and browser HTTP without adding a source-level
`async` or context parameter. Additional middleware is still under development.
Routing distinguishes missing paths from unsupported methods and returns a
portable JSON 405 response with an `Allow` header. Request bodies use a
configurable limit of 1 MiB by default before dispatch, and oversized requests
receive the same JSON 413 response in every backend. Query parameters use the
portable URL decoder and preserve repeated keys and source order instead of
collapsing them into a hash. `Request#query_values` returns all repeated values,
while strict `Request#query_value` reports malformed, missing, and duplicate
values through a typed error. HEAD requests prefer an explicit handler,
otherwise reuse the matching GET handler and middleware chain, and never expose
a response body. OPTIONS requests likewise prefer explicit handlers; otherwise
a middleware-aware 204 response advertises the available methods through
`Allow`.
Request header lookup is case-insensitive; `Request#header_value` rejects
missing and duplicate values instead of choosing one implicitly. Request
headers can also be replaced, appended, or removed without mutating the
original request.
Portable cookie parsing preserves header order, duplicate names, and opaque
values without delegating
semantics to the target runtime. `Request#cookie_values` returns all matching
values, while strict `Request#cookie_value` reports missing and duplicate names
through a typed error.
`Request`, `Response`, and `Context` are immutable classes. Their method APIs
replace, append, remove, or inspect values by returning new instances where
state changes. Responses support case-insensitive header operations
without mutating the original response. Strict response lookup rejects missing
and duplicate values instead of selecting one. `vary` composes cache keys
without duplicating an existing field.
Typed response cookies support ordered `Domain`, `Path`, `Max-Age`, `Secure`,
`HttpOnly`, and `SameSite` attributes while preserving multiple `Set-Cookie`
header values. Cookie names, values, domains, paths, attribute uniqueness,
`SameSite=None`, and the `__Secure-` and `__Host-` prefixes are validated before
serialization. Invalid cookie construction reaches the same portable JSON 500
boundary as any other invalid response.
Portable `text`, `bytes`, `empty`, and `redirect` builders create common
responses with consistent default statuses and content types.
`Response#with_status` returns a copy with a different status.
Before a response leaves the portable dispatcher, every backend rejects invalid
status codes, header names, and CR/LF-bearing header values through the same
JSON 500 boundary.

The experimental official [`trb/orm`](guides/orm.md) package targets generated
Go, Ruby, and TypeScript; TypeScript server applications currently select Bun.
It reads SQLite, PostgreSQL, or MySQL schema metadata directly and exposes typed
models, immutable queries, associations and preload, aggregates, transactions,
batching, writes, conflict handling, and destroy lifecycles.
String- and Integer-backed enum columns preserve nominal enum types throughout
queries and writes, while ordinary enums use a checked lower-snake-case storage
convention. Unknown stored values become structured invalid-data errors.
Date, time-of-day, civil date-time, and instant columns are inferred from each
database schema and retain their portable types through predicates, writes,
projections, and aggregates. Instant storage is normalized through UTC.
The repository runs the same application contract across all nine backend and
database combinations, plus an ORM-backed JSON route across all three backends.
Database terminals and lazy association access use `fails DbError`; `attempt`
captures them as ordinary `Result` values. The REPL uses the same schema-backed
read and write API. A deterministic portable schema lock now removes the live
database requirement from compiler checks and builds. Optional `trb db`
commands provide plan, guarded apply, export, lock, and drift checks around a
pinned external sqldef executable on SQLite, PostgreSQL, and MySQL. Production
compatibility policy remains future work.

Portable `trb/http` owns the open HTTP method, ordered case-insensitive
headers, and buffered body value types shared by server and client packages.
TypeScript browser applications can import the official
`trb/platform/typescript/browser` package. Its single request primitive accepts
those shared methods and headers, repeated query parameters, text/bytes/form/
JSON bodies, and timeouts. Fetch responses retain status, headers, final URL,
and buffered bytes; explicit JSON decoding produces `Response<T>` and preserves
the raw response in a classified `RequestError` when the contract is invalid.
Non-2xx statuses remain ordinary responses. The backend inserts suspension
only in generated TypeScript, so TypeRB source does not add target-specific
`async` syntax.

The experimental `trb/jobs` contract discovers typed Job classes and provides
typed immediate, relative-delay, and absolute-`Instant` enqueue operations. A
separate `trb/jobs/sql` adapter persists queue state in SQLite, PostgreSQL, or
MySQL and runs the same Job source in generated Go, Ruby, and Bun applications.
Projects select the adapter through a typed TypeRB composition module rather
than a string adapter name in `packageOptions`; Job definitions remain
independent from storage.
Workers support queues, priorities, retry and failed state, heartbeats, stale
claim recovery, graceful stop, listing, manual retry, and discard. PostgreSQL
and MySQL use short locking claims for multiple workers. SQLite is deliberately
limited to one worker. Delivery is at least once, so Jobs remain responsible
for idempotence. Recurring schedules, transactional outbox delivery, parallel
workers within one process, and forced-shutdown timeouts remain future work.

The compiler pipeline is:

```text
lossless tokens -> syntax AST -> resolver/type checker -> typed IR -> backend
```

Backends consume typed IR and do not inspect parser state or rewrite source
text. The repository suite covers compiler phases, formatter, language
services, REPL, project builds, standard packages, type providers, generated
target code, and browser tools.

Nullable lexical bindings narrow through direct `nil` comparisons in
conditional branches, loops, compatible short-circuit expressions, and
returning guards. Direct record fields and `readonly` class fields narrow when
their receiver is a stable lexical binding. Reassignment invalidates the flow
fact, while typed IR keeps the required unwrap explicit for every backend and
the REPL.

Compiler artifacts carry a versioned, backend-independent mapping from
generated statement ranges to original `.trb` paths and spans. Go mappings are
retained through target formatting; Ruby and TypeScript use the same internal
model. Emitting target-standard map files and translating runtime stack traces
remain follow-up work.

`trb check` validates a configured project without writing generated source or
starting a target toolchain. Its human diagnostics and versioned JSON report
share stable `TRBxxxx` codes, one-based source locations, related locations,
and atomic source-edit suggestions. Parser, resolver, checker, and project
integration errors use the same model, and independent errors across project
files are reported in one run. This model is the diagnostic boundary for the
future language server and other machine clients.

The reusable compiler service owns versioned project snapshots and unsaved
document overlays above the ordinary compiler pipeline. Successful snapshots
carry checked artifacts and per-module completion contexts. When an incomplete
edit fails, the service publishes current diagnostics while retaining the last
successful context, so editor completion does not disappear during typing.
Concurrent edits invalidate obsolete analysis before it can become current.
`trb check` already consumes this service; dependency-aware incremental phase
reuse remains future work.

`trb lsp` exposes that service over standard LSP framing. Its capabilities are
project-wide live diagnostics, completion, checked hover information, signature
help, deterministic formatting, and quick fixes from structured diagnostic
edits. Full-document updates and UTF-16 protocol positions are translated at
the adapter boundary; the compiler and formatter continue to use UTF-8 source
offsets. The preview Visual Studio Code extension is a published thin client
over this boundary. Definition, reference, and rename queries follow stable
source declaration identities across project imports, receiver types, and
common lexical bindings. Semantic tokens remain follow-up work.

## Current limitations

The current alpha does not yet provide:

- a complete everyday receiver API;
- position-typed tuples or type-pattern narrowing for nullable, collection,
  and non-discriminated structured union alternatives;
- inferred type arguments, generic interfaces, or generic class methods;
- complete superclass construction, override, and mutation-effect semantics;
- general first-class call blocks;
- concise `Result` propagation syntax;
- emitted target-standard source maps, runtime stack mapping, incremental
  builds, or a persistent build cache;
- semantic package version constraints, publishing or audit services, or a
  stable external compiler-extension protocol;
- namespace-stable public type identities across independent packages;
- a language-level test runner, published editor marketplace extension, or
  semantic editor navigation; or
- compatibility guarantees for production use.

Future outcomes are tracked in the [roadmap](roadmap.md); executable scoped work
is tracked as GitHub issues.
