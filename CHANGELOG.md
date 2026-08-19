# Changelog

This file records user-visible changes in stable TypeRB releases.

## 0.3.2 - 2026-08-19

### Tooling

- Language-server definition navigation now resolves JSX component uses such
  as `<InsurerPage>` directly to their imported TypeRB declarations. The
  matching Visual Studio Code extension 0.3.2 also enables breakpoint controls
  for `.trb` files and stops Go debugging on selected TypeRB source lines.
  ([#351](https://github.com/type-rb/type-rb/pull/351))

## 0.3.1 - 2026-08-19

### Language and collections

- Array and Unicode String element access now accepts negative indexes counted
  from the end, including safe `try_fetch` lookup and mutable Array element
  assignment. `-1` names the last element, and out-of-bounds errors retain the
  originally requested index.
- Arrays add `index(value)`, which returns the first matching position as
  `Integer?` using portable equality. Integer Ranges add `to_a()`, preserving
  inclusive and exclusive ends while producing an empty Array for reversed
  ranges. ([#350](https://github.com/type-rb/type-rb/pull/350))

## 0.3.0 - 2026-08-19

### Language

- **Breaking:** Recoverable failures now use the compiler-owned
  `Result<T, E>` type as a single explicit model. Prefix `try` unwraps success
  and propagates a compatible `Err`, while postfix
  `catch |error| ... end` unwraps success and handles failure locally. The
  previous authored `fails` and `attempt` syntax has been removed; the
  compiler reports focused migration diagnostics for both forms.
- Standard Result values must be handled rather than silently discarded.
  Returning, passing, storing, exhaustively matching, using `try`, or using
  `catch` counts as handling the value, while an unused `_result` binding is
  still diagnosed.
- `case/when` remains the value and enum branching construct, including enum
  payload binding. TypeRB 0.3 does not add a separate `case/in` or `match`
  syntax.

### Official packages and runtime boundaries

- **Breaking:** ORM terminal operations, lazy associations, writes, and
  deletes return `DbResult<T>`. Transactions and streaming query blocks are
  structured Result boundaries: propagated errors stop the block and complete
  rollback or cleanup before an outer `catch` runs.
- **Breaking:** Fallible jobs return `JobResult`, enqueue operations return
  `Result<JobReference, EnqueueError>`, and browser request, response decoding,
  and file APIs return Result values directly. A job `Err` follows the existing
  retry and exhaustion policy with its portable `error.message`.
- **Breaking:** Native TypeScript provider and cache format version 2 removes
  the legacy `fails` and `effectBridge` fields. Promise rejection adapters use
  the explicit `resultBridge` contract instead.
- The affected bundled packages `trb/orm`, `trb/jobs`, `trb/jobs/sql`, and
  `trb/platform/typescript/browser` are version 0.2.0.

### Tooling and migration

- Visual Studio Code extension 0.3.0 understands `try` and `catch`, removes the
  retired failure syntax from language features, and requires a TypeRB 0.3
  compiler.
- The [TypeRB 0.3 Result-control migration guide](docs/migrations/0.3-result-control.md)
  explains function signatures, call-site rewrites, structured blocks, jobs,
  browser APIs, native providers, and intentional uses of `case/when`.
  ([#347](https://github.com/type-rb/type-rb/pull/347))

## 0.2.31 - 2026-08-18

### Editor tooling

- Visual Studio Code now requests TypeRB completion while identifiers are
  typed, and unresolved project or standard-library names can add their import
  from completion or the Quick Fix menu.
  ([#344](https://github.com/type-rb/type-rb/pull/344))
- The language server now clears stale diagnostics immediately, and semantic
  project analysis no longer generates backend source. This substantially
  reduces diagnostic latency in large workspaces while preserving backend
  validation.
  ([#344](https://github.com/type-rb/type-rb/pull/344))

## 0.2.30 - 2026-08-18

### Editor tooling

- Completion and formatting now remain responsive while `trb lsp` recalculates
  full-project diagnostics. Rapid edits coalesce into one analysis, obsolete
  results are discarded, and project import candidates are indexed once per
  snapshot. ([#341](https://github.com/type-rb/type-rb/pull/341))
- Visual Studio Code now preserves discovered project roots when a monorepo is
  opened above its `trbconfig.jsonc` files, so nested completion,
  format-on-save, and navigation use the correct language-server session.
  ([#341](https://github.com/type-rb/type-rb/pull/341))

## 0.2.29 - 2026-08-18

### CLI and standalone workflows

- Config-free `.trb` entries now resolve explicit local imports transitively
  when running in Go, Ruby, or TypeScript mode, without compiling unrelated
  sibling or imported `*_test.trb` files.
  ([#338](https://github.com/type-rb/type-rb/pull/338))
- Config-free Go entries can now build standalone executables with
  `trb build --compile FILE.trb`. Output defaults to
  `<entry-directory>/bin/<source-stem>`, while `--debug` retains TypeRB source
  mappings for debuggers.
  ([#336](https://github.com/type-rb/type-rb/pull/336))

### Editor tooling

- Standalone language-server sessions now keep explicit local import closures
  synchronized with unsaved editor changes and filesystem events, so imported
  helpers participate in diagnostics and language features.
  ([#337](https://github.com/type-rb/type-rb/pull/337))
- Visual Studio Code can now debug config-free Go entries and their imported
  TypeRB helpers through Delve, using private session executables that are
  removed when debugging ends.
  ([#336](https://github.com/type-rb/type-rb/pull/336))

## 0.2.28 - 2026-08-17

### ORM

- Models in separate files within one source directory can now declare typed
  `belongs_to`, `has_many`, and `has_one` associations without mutual source
  imports. Cross-directory associations and runtime bootstrap cycles produce
  located compiler diagnostics, while completion and navigation understand
  association model references without inserting imports.
  ([#333](https://github.com/type-rb/type-rb/pull/333))

## 0.2.27 - 2026-08-17

### ORM

- `enum_column` now accepts enums imported through TypeRB package aliases,
  including enums shared from local application packages.
  ([#329](https://github.com/type-rb/type-rb/pull/329))

## 0.2.26 - 2026-08-17

### CLI and standalone workflows

- A self-contained `.trb` file can now run without `trbconfig.jsonc` by using
  `trb FILE.trb`. Standalone execution defaults to Go and supports explicit
  Ruby or TypeScript selection, while a discovered project configuration
  remains authoritative. ([#322](https://github.com/type-rb/type-rb/pull/322))

### Editor tooling

- Visual Studio Code can analyze and run standalone TypeRB files with
  configurable Go, Ruby, or TypeScript mode. Configured and standalone files
  keep independent language-server sessions.
  ([#323](https://github.com/type-rb/type-rb/pull/323))
- Completion now follows instantiated generic results, transparent aliases,
  and native package contracts. Interface types and methods support Go to
  Implementation, large monorepos become ready substantially faster, and
  restarting a project clears prior Debug Console output.
  ([#326](https://github.com/type-rb/type-rb/pull/326))
- **Breaking:** Extension 0.2.4 requires Visual Studio Code 1.130 or newer.
  Upgrade Visual Studio Code before installing this extension version.
  ([#324](https://github.com/type-rb/type-rb/pull/324))

## 0.2.25 - 2026-08-16

### CLI

- `trb run` and `trb test` now keep generated target trees below the
  compiler-owned `.trb` directory, forward shutdown signals, and recover
  abandoned workspaces without disturbing concurrent commands.
- `trb run --keep-generated` retains generated source for inspection, while
  `trb clean` can remove abandoned workspaces and optionally clean retained
  source, build output, or compiler caches.
  ([#319](https://github.com/type-rb/type-rb/pull/319))

## 0.2.24 - 2026-08-16

### Language and compiler reliability

- Unsupported primitive constructors such as `Bytes.new()` are now rejected
  during type checking instead of producing invalid backend code. Use the
  standard conversion APIs, such as `String#to_bytes()`, for primitive values.
  ([#315](https://github.com/type-rb/type-rb/pull/315))

## 0.2.23 - 2026-08-16

### Language and compiler reliability

- Transparent aliases now remain transparent when used as standard-library
  collection element types, including `push()` and `include?()`, and imported
  result aliases produce valid nullable Go values.
- Backend effect analysis now visits every child of structured control flow.
  Fallible helpers called from ORM transaction blocks therefore receive the
  compiler-owned execution scope correctly.
  ([#312](https://github.com/type-rb/type-rb/pull/312))

## 0.2.22 - 2026-08-16

### Go backend

- Member access on a nullable value now compiles correctly after a successful
  non-`nil` narrowing. ([#309](https://github.com/type-rb/type-rb/pull/309))

## 0.2.21 - 2026-08-16

### Package integration

- Native TypeScript package indexing now handles synthetic object properties
  without declarations. Libraries built from mapped or inferred prop types can
  be installed and checked without crashing the compiler.
  ([#306](https://github.com/type-rb/type-rb/pull/306))

## 0.2.20 - 2026-08-16

### Frontend and package integration

- Native React component props typed as `ReactNode` now accept directly
  renderable strings, numbers, booleans, and other supported values. Ordinary
  props such as `label="Name"` no longer require a wrapper element.
  ([#303](https://github.com/type-rb/type-rb/pull/303))

## 0.2.19 - 2026-08-16

### Language

- Fallible operations inside `map`, `select`, `reduce`, and predicate-based
  collection transformations now propagate through a typed transformation
  boundary across Go, Ruby, and TypeScript. This fixes invalid generated code
  when application mapping logic performs database or other fallible work.
  ([#300](https://github.com/type-rb/type-rb/pull/300))

## 0.2.18 - 2026-08-16

### Go backend

- Explicitly typed nullable local variables initialized with `nil` now emit a
  typed Go nil, so declarations such as `mut value: String? := nil` compile in
  generated Go applications. ([#297](https://github.com/type-rb/type-rb/pull/297))

## 0.2.17 - 2026-08-16

### Language

- Transparent aliases now behave like their underlying scalar types in unary
  and binary expressions and as JSX children. Imported aliases can therefore
  be rendered and combined without explicit conversion.
  ([#294](https://github.com/type-rb/type-rb/pull/294))

## 0.2.16 - 2026-08-16

### ORM

- Go applications can create ORM records whose primary key is supplied by the
  application, including string identifiers such as email addresses, without
  generating invalid Go code. ([#291](https://github.com/type-rb/type-rb/pull/291))

## 0.2.15 - 2026-08-16

### Compiler and package integration

- Go applications can combine `trb/web` and `trb/jobs` in one entrypoint while
  using imported transparent scalar aliases as job payloads. Generated worker
  imports and standard-library aliases now remain consistent in this setup.
  ([#288](https://github.com/type-rb/type-rb/pull/288))

## 0.2.14 - 2026-08-16

### Compiler and package integration

- Transparent scalar aliases now work as typed `trb/web` request parameters
  and `trb/jobs` payloads. Job-enabled projects avoid unused runtime imports in
  unrelated Go modules, `trb test` remains an application test runner instead
  of starting a worker, and named-unused ORM transaction results compile
  correctly. ([#285](https://github.com/type-rb/type-rb/pull/285))

## 0.2.13 - 2026-08-15

### Frontend and package integration

- TypeScript browser applications can read the contents of a native `File` as
  `Bytes` or `String` through fallible `read()` and `read_text()` operations.
  The compiler lowers both operations to the asynchronous browser File API
  without exposing target-specific `async` syntax. ([#282](https://github.com/type-rb/type-rb/pull/282))

## 0.2.12 - 2026-08-15

### Frontend and package integration

- TypeScript browser applications can receive a checked DOM `File` from an
  indexed native component callback and send the original file as an HTTP
  request body. The browser platform type exposes file metadata, supplies the
  media type by default, and preserves explicit request headers.
  ([#279](https://github.com/type-rb/type-rb/pull/279))

## 0.2.11 - 2026-08-15

### Language

- A `case` branch can now match multiple comma-separated Integer or String
  literals consistently across Go, Ruby, TypeScript, and the REPL. Previously,
  only the first value was retained. ([#276](https://github.com/type-rb/type-rb/pull/276))

## 0.2.10 - 2026-08-15

### Frontend and package integration

- Native TypeScript package bridges can expose typed compound JSX components
  such as `Table.Row` and `Sidebar.Item`. TypeRB checks their props and reports
  undeclared component members while preserving the native TSX API.
  ([#273](https://github.com/type-rb/type-rb/pull/273))
- React components with browser effects now keep compiler-owned cancellation
  scope internal, and browser response conversion, native component indexing,
  and TypeScript HTTP request bindings avoid generated-name collisions.
  ([#265](https://github.com/type-rb/type-rb/pull/265),
  [#266](https://github.com/type-rb/type-rb/pull/266),
  [#268](https://github.com/type-rb/type-rb/pull/268))

### Language and compiler reliability

- Nil checks now narrow nested readonly nullable fields, including fields
  reached through discriminated native package result types. Transparent aliases
  also inherit portable receiver methods from their underlying types.
  ([#269](https://github.com/type-rb/type-rb/pull/269),
  [#271](https://github.com/type-rb/type-rb/pull/271))
- TypeScript JSON decoding now retains runtime imports for raw-value enums inside
  imported records. Full builds also remove generated target files whose TypeRB
  sources were deleted.
  ([#270](https://github.com/type-rb/type-rb/pull/270),
  [#272](https://github.com/type-rb/type-rb/pull/272))

### ORM

- Go applications can place multiple TypeRB ORM models in the same package
  without generated runtime helper declarations colliding.
  ([#267](https://github.com/type-rb/type-rb/pull/267))

## 0.2.9 - 2026-08-15

### Language

- TypeRB now supports invariant generic interfaces and explicit specialized
  implementations across Go, Ruby, TypeScript, and the REPL.
  ([#259](https://github.com/type-rb/type-rb/pull/259))

### Testing and editor tooling

- Projects can define portable test suites with `describe`, `test`, and
  `expect` from `trb/std/test`, run them with `trb test`, and inspect them in
  VS Code Test Explorer. Go-mode tests additionally support source debugging
  from `.trb` files. ([#260](https://github.com/type-rb/type-rb/pull/260))
- VS Code completion can add explicit imports for unambiguous types, and Go
  applications support `.trb` source breakpoints, stepping, stack frames,
  variables, watches, and evaluation through the standard debugger UI.
  ([#258](https://github.com/type-rb/type-rb/pull/258))

### Compiler reliability

- Transparent aliases now behave like their target types in JSON, path, and
  query codecs and in interface signatures. Formatting multiline named
  imports also preserves slash-separated package paths.
  ([#261](https://github.com/type-rb/type-rb/pull/261))
- Generated Go applications now preserve ORM execution scope through imported
  interfaces and avoid collisions between `trb/http` and user packages named
  `http`. JSON codecs also handle aliases declared inside imported records.
  ([#262](https://github.com/type-rb/type-rb/pull/262))

## 0.2.8 - 2026-08-15

### Editor tooling

- The Visual Studio Code extension now runs TypeRB projects through the
  standard Run and Debug interface. Native restart and stop controls manage the
  complete process tree, while the Debug Console immediately reports the
  launch command, process identifier, program output, and exit status.
  ([#255](https://github.com/type-rb/type-rb/pull/255))

## 0.2.7 - 2026-08-15

### Editor tooling

- The Visual Studio Code extension can run, restart, and stop non-browser
  projects directly from a top-level `main()`, saving dirty TypeRB source before
  launching `trb run` in an integrated terminal.
  ([#251](https://github.com/type-rb/type-rb/pull/251))
- Workspaces may contain independent TypeRB API and frontend projects with
  overlapping declaration names. The extension starts one language server per
  `trbconfig.jsonc`, routes editor actions to the owning project, and prevents
  files outside that project's source tree from contaminating diagnostics.
  ([#252](https://github.com/type-rb/type-rb/pull/252))

## 0.2.6 - 2026-08-14

### Language

- Type annotations now require canonical, case-sensitive TypeRB names such as
  `Integer`, `Boolean`, and `Hash`. Target-language aliases such as `Int`,
  `int`, `bool`, and `Map` are no longer accepted, and unknown type names are
  reported by the compiler and editors. Existing source that uses an alias
  must replace it with the canonical TypeRB name.
  ([#235](https://github.com/type-rb/type-rb/pull/235),
  [#246](https://github.com/type-rb/type-rb/pull/246))

### Editor tooling

- TypeRB editors now provide hierarchical document outlines, project-wide
  symbol search, and compiler-aware semantic highlighting for declarations,
  types, constants, calls, literals, comments, and keywords.
  ([#238](https://github.com/type-rb/type-rb/pull/238),
  [#239](https://github.com/type-rb/type-rb/pull/239),
  [#240](https://github.com/type-rb/type-rb/pull/240))
- Editors can fold structural declarations and expressions, highlight checked
  occurrences of a symbol, and expand or shrink selections through token,
  line, and enclosing syntax ranges. These syntax-backed features remain
  available while source is incomplete.
  ([#242](https://github.com/type-rb/type-rb/pull/242),
  [#243](https://github.com/type-rb/type-rb/pull/243),
  [#244](https://github.com/type-rb/type-rb/pull/244))
- The language server now accepts incremental document updates and tracks
  project files created, changed, or deleted outside active editor buffers
  while preserving unsaved editor contents.
  ([#241](https://github.com/type-rb/type-rb/pull/241),
  [#245](https://github.com/type-rb/type-rb/pull/245))

### Documentation discovery

- The TypeRB website now publishes an AI-readable documentation index that
  points agents to the current learning path, language and package references,
  application guides, and compiler documentation.
  ([#237](https://github.com/type-rb/type-rb/pull/237))

## 0.2.5 - 2026-08-14

### Editor tooling

- TypeRB editors can now show checked hover information and call signatures,
  navigate to project and lexical definitions, find references, and rename
  declarations and uses across project files. Symbol identity preserves
  receiver types and lexical scope instead of relying on matching text.
  ([#229](https://github.com/type-rb/type-rb/pull/229),
  [#230](https://github.com/type-rb/type-rb/pull/230),
  [#231](https://github.com/type-rb/type-rb/pull/231))

### CLI

- `trb version`, `trb --version`, and `trb -v` now print only the semantic
  version. Scripts that expected a `trb ` prefix must consume the bare version
  instead. ([#228](https://github.com/type-rb/type-rb/pull/228))

## 0.2.4 - 2026-08-14

### Editor and compiler tooling

- TypeRB now includes `trb lsp` and a Visual Studio Code extension with syntax
  highlighting, snippets, live project diagnostics, completion, formatting,
  and compiler-backed quick fixes. The shared compiler service analyzes unsaved
  documents while preserving useful completion context during incomplete edits.
  ([#221](https://github.com/type-rb/type-rb/pull/221),
  [#222](https://github.com/type-rb/type-rb/pull/222),
  [#223](https://github.com/type-rb/type-rb/pull/223),
  [#224](https://github.com/type-rb/type-rb/pull/224))
- `trb check` is now the canonical project validation command and supports
  versioned JSON diagnostics with stable codes, precise source locations,
  related information, and suggested fixes. Replace `trb build --check` with
  `trb check`; ordinary `trb build` continues to generate target source.
  ([#221](https://github.com/type-rb/type-rb/pull/221))
- Compiler artifacts now retain shared source-location mappings for future
  runtime stack traces, coverage, and target-native source maps.
  ([#220](https://github.com/type-rb/type-rb/pull/220))

### Language and collections

- Nullable lexical bindings, record fields, and readonly class fields now
  narrow through nil checks, short-circuit expressions, loops, and returning
  guards. ([#206](https://github.com/type-rb/type-rb/pull/206),
  [#220](https://github.com/type-rb/type-rb/pull/220))
- Collection transformations can contain ordinary statements before their
  final result expression across Go, Ruby, TypeScript, and the REPL.
  ([#219](https://github.com/type-rb/type-rb/pull/219))
- Arrays now provide short-circuit predicates, nullable searches, stable
  natural and key-based sorting, stable `uniq()`, and non-destructive
  `concat(other)`. ([#205](https://github.com/type-rb/type-rb/pull/205),
  [#207](https://github.com/type-rb/type-rb/pull/207),
  [#209](https://github.com/type-rb/type-rb/pull/209),
  [#211](https://github.com/type-rb/type-rb/pull/211))
- Arrays and Unicode strings now support canonical Range-based slicing and
  safe element access, and strings provide code-point-based `index()` and
  `rindex()`. Strict `fetch()` was removed; use `value[index]` for strict
  element access and `try_fetch()` for safe access.
  ([#213](https://github.com/type-rb/type-rb/pull/213))

### Application development

- Portable OIDC bearer authentication now provides discovery, cached JWKS key
  rotation, RS256 verification, typed request principals, and standard JSON
  authentication failures in Go, Ruby, and TypeScript server applications.
  ([#218](https://github.com/type-rb/type-rb/pull/218))
- `trb build` and `trb run` now retain locked Git package output for projects
  that use the default root source directory.
  ([#214](https://github.com/type-rb/type-rb/pull/214))
- Generated Go code now avoids collisions between lexical bindings and import
  identifiers. ([#217](https://github.com/type-rb/type-rb/pull/217))
