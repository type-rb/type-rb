# TypeRB Status

Last updated: 2026-08-25

TypeRB is an alpha compiler implemented in Go. The language, standard library,
generated output, and command-line interface may change before beta.

## Current capability

TypeRB uses one portable grammar and typed IR for Go, Ruby, and TypeScript
output. Projects have deterministic formatting, package configuration,
project-wide checking, configurable built-in linting, source generation,
temporary build-and-run, and Go executable compilation. Target-specific
behavior remains behind explicit `trb/platform/<mode>/*` imports.

Short two-value choices use the typed conditional expression
`condition ? value : alternative`. Simple early exits use conditional
control-transfer statements such as `return value if condition`, `next if
condition`, and `break if condition`; trailing `if` is not a general statement
modifier. Both forms normalize into the existing control-flow IR and therefore
share strict Boolean conditions, narrowing, lazy branches, and backend or REPL
semantics.

`trb lint` runs project checking before a small configurable built-in ruleset.
The initial recommended rule, `trb/prefer-conditional-transfer`, reports and
safely fixes simple one-transfer guard blocks. Built-in rules share the TypeRB
version, while JSON reports carry independent `schemaVersion` and
`toolVersion` fields. Third-party rule execution remains deferred until a
deterministic, resource-bounded extension protocol is designed.

The CLI can also run one self-contained `.trb` file without creating project
configuration. It defaults to Go, can explicitly select Ruby or TypeScript,
and isolates generated target source in an operating-system temporary
directory. A discoverable `trbconfig.jsonc` always restores project-wide
compilation and owns mode and runtime selection.

The initial distributed package system resolves TypeRB source directly from
Git repositories or explicit local paths. Short imports default to GitHub but
lock to canonical manifest identities. A deterministic `trb.lock` pins the
transitive graph, commit IDs, and SHA-256 content checksums; frozen and offline
installation are available for CI and disconnected builds. `trb update` can
re-resolve the complete graph or selected direct aliases and their transitive
dependencies while leaving other direct graphs pinned. Package source uses
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
Installed TypeRB packages can supply a versioned, mode-independent declaration
adapter catalog with generic functions, classes whose instance and class
members remain distinct, non-constructible interfaces, records, and transparent
type aliases. The TypeScript adapter is the first consumer and generated code
continues to import the original npm package. The bridge preserves
discriminated generic results and emits transitive native type-only imports
without making their names source-visible. Adapter-declared Promise callback
boundaries can map a Result-returning TypeRB function to native resolution and
rejection without exposing Promise semantics in TypeRB source.
An installed package can instead pair its semantic declaration catalog with a
mode-specific Native Runtime Adapter Protocol file in Go, Ruby, or TypeScript
mode. The initial protocol maps package-owned wire exports to top-level native
shim functions and accepts only `(String) -> String`; package source and the
shim own JSON envelopes, SDK error normalization, and domain `Result`
conversion. Two declarative effect flags propagate TypeScript suspension and
the hidden Go, Ruby, or TypeScript execution scope through the ordinary call
graph. Direct Go module and Ruby gem declaration import, structural native
values, lifecycle hooks, and arbitrary native error mappers are not included.
Package authors can run `trb adapter check` to validate the manifest and every
catalog and runtime mapping through the selected ecosystem consumer before
publishing. Its optional versioned JSON report exposes deterministic adapter
and runtime-binding counts and diagnostics for CI and AI agents.
Packages can also declare a mode-specific conformance project for explicit
`trb adapter test` execution. The command validates the adapter, builds the
installed self-referencing fixture, and invokes its structured native check;
its versioned JSON report exposes stable phase states without implicitly
installing dependencies or running package code during ordinary builds.
The Amplify Auth dogfood fixture additionally verifies that generated native
configuration can remain in an application-owned TypeScript bootstrap while
TypeRB uses the existing String rejection bridge. Rich native error records
remain an explicit future mapper capability rather than falling back to `Any`.

The experimental TypeScript browser path accepts structured JSX in TypeRB
source and emits ordinary TSX for React tooling. Function components use typed
record props, JSX component calls are checked across project modules, and only
modules containing JSX use the `.tsx` extension. The explicit
`trb/platform/typescript/react` package supplies the React boundary, including
purpose-specific mouse, change, form, and keyboard event types. Its typed
`use_state(initial)` wrapper infers `ReactState<T>` and exposes checked `value`
and `set(value)` members while generated TSX uses React `useState`.

The implemented language includes functions, typed first-class function values
with lexical capture and checked Result control flow, positional-only and
bare-`*` named-only parameters with callee-owned defaults, and classes, modules and
generic interfaces, records, ordinary and raw-value enums, payload enums as sum
types, enum instance methods, transparent `alias` declarations, nominal
`newtype` declarations over concrete non-nullable representations, explicit
generics for enums, aliases, records,
classes, top-level functions and instance methods, normalized unions, immutable
and mutable bindings, first-constraint inference for fresh empty mutable Arrays
and Hashes, typed collections and iteration, exhaustive pattern
matching, value-producing `if` and `case` expressions, and explicit Result
propagation and recovery with prefix `try` and postfix `catch`. See the
[language guide](language.md) and [specification](specification.md) for the
current semantics. Generated Go targets Go 1.27 and represents generic class
instance methods directly with native generic methods instead of package-level
helper functions.

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
strict and safe element access with negative indexes, value-based Array index
search, Range materialization, Range-based Array and Unicode String slicing,
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

Runtime Integer arithmetic is checked against the shared exact range
`-9007199254740991..9007199254740991` in generated Go, Ruby, TypeScript, and the
REPL; String, JSON, and ORM ingress use the same boundary. Float remains
binary64 and consistently exposes arithmetic Infinity and NaN while source
literals remain finite. Formatting preserves opaque native syntax and prevents
whitespace removal from fusing separate tokens into another operator.

The typed-IR REPL supports persistent state, auto-formatted multiline editing,
history, completion menus, project declaration auto-import, portable-standard
completion candidates, visible import edits for accepted completion
candidates, suggestions, syntax highlighting, interrupts, type inspection,
and mutable-binding result markers.
The same compiler and evaluator power the local and hosted playground and tour.
The hosted TypeRB website also renders the maintained user documentation,
including the language and package guides, references, and one page for every
built-in lint rule. Maintainer-only development and release material remains
in the source repository.
The repository also publishes a reusable TextMate grammar for lexical editor
highlighting. Its thin Visual Studio Code client packages that grammar and
snippets, starts `trb lsp`, and exposes compiler-backed diagnostics, completion,
formatting, and quick fixes without duplicating semantic logic. A minimal
Neovim plugin automatically activates configured `.trb` projects, starts
best-effort per-file sessions outside them, formats on save by default with an
explicit opt-out, and delegates syntax-aware behavior to the same language
server. JetBrains IDEs can import the packaged
TextMate grammar and connect `trb lsp` through LSP4IJ without a TypeRB-specific
IDE plugin. A Chrome extension applies the same TextMate grammar to GitHub
`.trb` files, pull request diffs, and explicit TypeRB Markdown code blocks while
native GitHub Linguist support is unavailable. In the VS
Code extension, open files outside configured projects
receive isolated file-root language-server
sessions that follow explicit local imports and editor overlays without
including unrelated siblings. Local Extension Development Host tests exercise
standalone language services and execution against current Stable and Insiders
builds without a Marketplace install. Focused unit tests cover private Go debug
artifact creation and cleanup. Command details belong in the
[CLI reference](cli.md).

Explicit tooling can consume the same compiler service through
`trb compiler inspect`. Its experimental version 3 JSON snapshot contains exact
source inputs, modules and authored imports, flattened checked declarations and
semantic types including nominal newtypes, and diagnostics without generating
target source. The command
is read-only and one-shot; mutable AST or typed IR access, backend hooks, and a
long-lived protocol server are not exposed. See the
[compiler tooling protocol guide](guides/compiler-tooling.md).

The early official `trb/web` package discovers file-based routes at compile
time and runs typed request, path-parameter, JSON decode, and JSON response
handlers through the same dispatcher in all three backends. Applications can
start a generated Go, Ruby, or TypeScript HTTP server with `serve()`. The
`trb init --mode <target> --template web` command creates a buildable portable
API project with file-based routing and an explicit editable middleware stack.
The optional `configure_server` value sets host, port, request-body limit, and
graceful-shutdown timeout through typed named arguments. Every adapter
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
segments and bind their slash-joined value. Static segments take precedence
over parameter segments, and the dispatcher selects the most specific path
before selecting its HTTP method. For the same method, route analysis rejects
overlapping catch-all patterns, sibling parameter patterns, and patterns whose
static specificity reverses between segments; catch-alls outside the final
position are always invalid. Calls to
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
their original typed error inside `EndpointInputError`. Combined binding
remains optional, route parameter compatibility is still verified at build
time, and applications keep control of the corresponding HTTP error response.
The implementation is the first consumer of the experimental bundled-package
call specialization boundary: `trb/web` returns target-independent TypeRB
helper source, and the compiler re-parses, resolves, checks, and lowers that
source instead of carrying a Web-specific bind intrinsic through each backend.
Route modules may separately declare optional `Endpoint` subclasses that bind
an actual handler function, an optional input type, and response types keyed by
literal HTTP status. The handler reference is checked as `(Context) ->
Response`, the class must map to a file-route handler in the same module, and
the declaration calls are omitted from all three runtime outputs. `trb/web`
owns the resulting version 1 endpoint catalog; the generic compiler IR only
transports it as package extension data. The catalog does not infer handler
bodies or perform validation. `trb web openapi` combines that package-owned
catalog with the read-only Project Declaration Input and emits deterministic
OpenAPI 3.1 JSON without invoking a target toolchain. It maps the existing
`Context#bind<T>()` envelope to path, query, and JSON request inputs and emits
status-specific JSON or `Unit` responses. Its initial shared-schema boundary
covers portable scalars and time values, Arrays, `Hash<String, V>`, records,
raw-value enums, transparent aliases, nominal newtypes, nullable values, and
`@json` wire names. Unsupported JSON shapes, generic schemas, recursive
records, and catch-all routes are generation-time diagnostics rather than new
language or `trb check` restrictions. `trb web client` consumes that same
catalog and schema boundary to emit a deterministic TypeRB facade over the
existing TypeScript browser `HttpClient`. It imports client-visible endpoint
types from authored shared modules, maps declared statuses to typed result-enum
variants, and keeps transport, decoding, and unexpected-status failures in the
existing `RequestError` model. It is target-toolchain-independent at generation
time and browser-specific at compilation time. Runtime validation, contract
compatibility policy, authentication descriptions, richer endpoint metadata,
and portable outbound HTTP remain subsequent package work.
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
Each source directory is an ORM model group: separate model files can declare
typed direct and through associations without mutual imports, while ordinary
model references still require explicit imports. Cross-group object navigation
is a located project diagnostic; foreign-key based repository queries remain
available across the boundary. Runnable roots bootstrap Ruby and TypeScript
model registration without adding cyclic imports between model modules.
String- and Integer-backed enum columns preserve nominal enum types throughout
queries and writes, while ordinary enums use a checked lower-snake-case storage
convention. Unknown stored values become structured invalid-data errors.
Date, time-of-day, civil date-time, and instant columns are inferred from each
database schema and retain their portable types through predicates, writes,
projections, and aggregates. Instant storage is normalized through UTC.
The repository runs the same application contract across all nine backend and
database combinations, plus an ORM-backed JSON route across all three backends.
Database terminals, lazy association access, transactions, and streaming
return `DbResult<T>`. Applications propagate compatible errors with `try`,
recover with `catch`, or inspect the Result with exhaustive `case`. The REPL
uses the same schema-backed read and write API. A deterministic portable schema
lock removes the live database requirement from compiler checks and builds.
ORM declaration output uses the experimental bundled Declaration Protocol.
Schema-derived properties, literal-dependent terminals, and
structured transaction and streaming contracts cross a validated,
mode-independent data boundary before the compiler resolves them. Project model
discovery and backend runtime operations remain bundled while the protocol is
validated against another provider.
Optional `trb db`
commands provide plan, guarded apply, export, lock, and drift checks around a
pinned external sqldef executable on SQLite, PostgreSQL, and MySQL. Production
compatibility policy remains future work.

Portable `trb/http` owns the open HTTP method, ordered case-insensitive
headers, and buffered body value types shared by server and client packages.
TypeScript browser applications can import the official
`trb/platform/typescript/browser` package. Its single request primitive accepts
those shared methods and headers, repeated query parameters, text, bytes, form,
JSON, and native browser `File` bodies, and timeouts. Indexed native component
callbacks can pass a DOM `File` through the platform package's checked metadata
boundary and into a request without losing the underlying browser object.
`File#read()` and `File#read_text()` expose their buffered contents as Result
values from generated-TypeScript-only browser operations.
Fetch responses retain status, headers, final URL, and buffered bytes; explicit
JSON decoding produces `Response<T>` and preserves
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
independent from storage. The module exposes one explicitly typed
`JOBS_ADAPTER` constant, initialized once and reused by every enqueue wrapper,
so adapter lifetime is application-scoped rather than per-enqueue.
Jobs-derived `perform_later`, `perform_in`, and `perform_at` declarations cross
the same validated, mode-independent Declaration Protocol as ORM. Their typed
arguments, scheduling parameters, enqueue Results, and runtime operation names
use the generic protocol fields. Worker dispatch crosses the versioned Project
Generated Source Protocol as an ordinary TypeRB fragment:
one portable dispatcher validates payload versions, decodes scalar arguments,
calls the typed Job, and returns `JobResult` before all three backends lower it.
The same protocol emits a stable fragment per Job for payload serialization,
negative-delay validation, relative-to-absolute schedule normalization, and
dispatch through the ordinary TypeRB `JobAdapter#enqueue`/`enqueue_at`
contract. `EnqueueRequest` carries serialized payload and queue policy; the SQL
adapter owns ID generation, persistence, and native error mapping.
Generated-source responses contain stable fragment and module identity,
required named imports, authored origin spans, and located issues, but no AST,
typed IR, backend source, filesystem handles, or arbitrary provider data.
The compiler removes stale fragments, includes them in cache identity, and maps
their diagnostics and source locations to authored source. Project edits
currently use conservative full analysis while such fragments are active.
Retry policy, SQL worker lifecycle, and the SQL adapter's final native
persistence primitives remain a separate bundled runtime boundary. Jobs and
ORM declaration discovery also
consume versioned, JSON-serializable Project Declaration Input snapshots.
Version 5 contains canonical module/import identity, aliases, newtypes and
their concrete boundary representations, record declarations and field
attributes, enum and class declarations, top-level function and class method
signatures, authored and resolved types, resolved generic directive arguments,
declarative call values, structural block summaries, and source spans. The ORM
host combines that project snapshot with a separate versioned ORM schema snapshot containing
only the adapter and table, column, foreign-key, and unique-constraint facts
needed to derive its catalog. Database credentials, parser nodes, function,
method, and block bodies, default expressions, resolver/checker state,
filesystem access, and backend objects do
not cross the declaration-provider input boundary. The ORM runtime manifest
remains a separate bundled integration boundary.
Their direct class-body DSL calls cross one generic Declaration Protocol
version 3 rule. The rule matches an exact provider package function and exact
project class, preserves ordinary signature checking, and marks the typed-IR
call as declaration-only so Go, Ruby, and TypeScript omit it from runtime
output. The generic rule keeps package-specific function-name checks out of
the ORM and Jobs backend generators.
Workers support queues, priorities, retry and failed state, heartbeats, stale
claim recovery, graceful stop, listing, manual retry, and discard. PostgreSQL
and MySQL use short locking claims for multiple workers. SQLite is deliberately
limited to one worker. Delivery is at least once, so Jobs remain responsible
for idempotence. A fallible job returns `JobResult`; `Err` enters the same retry
and failure policy as a runtime execution failure. Recurring schedules,
transactional outbox delivery, parallel
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
model. Virtual TypeRB helpers returned by bundled package call specializers map
back to their originating call rather than exposing synthetic source lines.
Emitting target-standard map files and translating runtime stack traces remain
follow-up work.

`trb check` validates a configured project without writing generated source or
starting a target toolchain. Its human diagnostics and versioned JSON report
share stable `TRBxxxx` codes, one-based source locations, related locations,
and atomic source-edit suggestions. Parser, resolver, checker, and project
integration errors use the same model, and independent errors across project
files are reported in one run. This model is the diagnostic boundary for the
language server and other machine clients.

The reusable compiler service owns versioned project snapshots and unsaved
document overlays above the ordinary compiler pipeline. Successful snapshots
carry checked artifacts and per-module completion contexts. When an incomplete
edit fails, the service publishes current diagnostics while retaining the last
successful context, so editor completion does not disappear during typing.
Concurrent edits invalidate obsolete analysis before it can become current.
Long-lived compiler-service and REPL sessions reuse prepared syntax trees for
compiler-identical source units. When one ordinary module changes, the analyzer
reuses unchanged resolution and checking results, and invalidates downstream
importers when its public catalog changes. Provider declaration changes
invalidate the whole project; compiler-owned dependency, configuration, and
multi-file changes conservatively fall back to complete analysis. `trb check`
already consumes this service. Finer phase-level invalidation, incremental
lowering, and a persistent build cache remain future work.

`trb lsp` exposes that service over standard LSP framing. Its capabilities are
project-wide live diagnostics, completion with explicit import insertion for
unambiguous standard and project types, checked hover information, signature
help, deterministic formatting, and quick fixes from structured diagnostic
edits. Ordered incremental updates and UTF-16 protocol positions are translated
at the adapter boundary; the compiler and formatter continue to use complete
UTF-8 source snapshots and offsets. The preview Visual Studio Code extension is
a published thin client over this boundary. Definition, reference, document
highlight, and rename queries follow stable source declaration identities
across project imports, receiver types, and common lexical bindings. Go to
Definition treats a complete project-import path as one module reference and
opens its resolved TypeRB source; project auto-imports omit a redundant
terminal `/index`. CLI and language-server formatting apply the same
resolution-aware canonicalization to existing imports without changing an
ambiguous or unresolved module selection.
Association model references supplied by declarative package providers also
participate in completion, hover, definition, references, and rename without
adding ordinary source imports. Document
symbols expose the structural outline from the lossless syntax tree even while
a file has type errors, and workspace symbol queries search the same
declarations across project files. Structural folding ranges cover declarations
and expression blocks from that syntax tree. Selection ranges expand from tokens
through source lines and enclosing structural blocks.
Full-document semantic tokens reuse the compiler-aware highlighting service
and translate UTF-8 byte spans to the UTF-16 positions required by editors.
The VS Code client discovers nested `trbconfig.jsonc` files, starts one language
server per project, and watches each project's `.trb` files independently. The
language server rejects editor overlays outside its configured source root and
updates the saved project snapshot beneath active overlays.
Files outside discovered projects receive isolated, configless language-server
sessions with configurable Go, Ruby, or TypeScript mode. Each session follows
the selected entry's explicit local import closure and rebuilds it from saved
files and unsaved editor overlays. Its top-level `main()` CodeLens runs the
file-root program through the same Debug Adapter Protocol process lifecycle
used by projects. A Go-mode standalone entry can also build a private,
session-scoped executable for source debugging through Delve.
Canonical type-name diagnostics carry structured fixes, so editors can replace
aliases such as `Int` with `Integer` without reconstructing source text.
Top-level `main()` declarations expose a compiler-owned run CodeLens in
non-browser projects. The VS Code client saves dirty TypeRB project files and
starts `trb run` through a Debug Adapter Protocol session when invoked without
debugging. Visual
Studio Code owns the standard start, stop, and restart lifecycle, while the
Debug Console reports the launch command, process identifier, program output,
and exit status immediately. Invoking the CodeLens again requests a DAP
restart. Go debug builds project the shared source map into Go line directives,
and the VS Code client connects directly to Delve DAP for TypeRB breakpoints,
stepping, stack frames, variables, watches, and evaluation. Ruby and TypeScript
source-debugger adapters remain staged; both modes retain Run Without Debugging.
Portable colocated tests use `*_test.trb`, nested `describe` suites, explicit
`test` cases, and typed expectations from `trb/std/test`. `trb test` executes
the same source through Go, Ruby, and TypeScript process backends, supports
file/name filtering and JSON Lines events, preserves assertion locations, and
returns a nonzero failure status. The VS Code extension consumes compiler-owned
LSP discovery through the native Test Explorer and test CodeLens. Go test
selections can use the same Delve-backed TypeRB source debugger as application
entrypoints. Browser-hosted TypeScript execution, lifecycle hooks, and
higher-level testing packages remain staged.

## Current limitations

The current alpha does not yet provide:

- a complete everyday receiver API;
- position-typed tuples or type-pattern narrowing for nullable, collection,
  and non-discriminated structured union alternatives;
- inferred type arguments, generic interface methods, or generic class methods;
- complete superclass construction, override, and mutation-effect semantics;
- general first-class call blocks;
- complete target-standard source maps and runtime stack mapping across Ruby
  and TypeScript, incremental builds, or a persistent build cache;
- semantic package version constraints, publishing or audit services, or a
  stable external compiler-extension protocol;
- namespace-stable public type identities across independent packages;
- compatibility guarantees for production use.

Future outcomes are tracked in the [roadmap](roadmap.md); executable scoped work
is tracked as GitHub issues.
