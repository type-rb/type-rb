# Changelog

This file records user-visible changes in stable TypeRB releases.

## 0.4.1 - 2026-08-30

### Breaking changes

- `trb/web` advances to package version 0.2.0 and adopts owner-qualified APIs.
  Replace `serve()` and `configure_server(...)` with `Web.serve()` and
  `Web::ServerConfig.new(...)`; call the response builders as `Response.text`,
  `Response.bytes`, `Response.json`, `Response.empty`, and
  `Response.redirect`; and replace `new_response_cookie(...)` with
  `ResponseCookie.new(...)`. `ResponseCookie.attributes` now defaults to an
  empty array. ([#611](https://github.com/type-rb/type-rb/pull/611))

## 0.4.0 - 2026-08-30

### Breaking changes

- Imports now bind exact top-level declarations or one unique declaration root
  selected from a package's exports. Replace legacy package-namespace imports
  with bare root imports, use named imports for peer declarations and top-level
  functions, and use `activate` only for bindingless compiler capabilities.
  `trb fmt` and editor auto-imports emit the deterministic canonical form.
  ([#603](https://github.com/type-rb/type-rb/pull/603))
- Standard-library support declarations now belong to their public roots.
  Replace peer-style imports and references with owner-qualified forms such as
  `JSON::Error`, `FileSystem::Error`, `Process::Output`, and
  `Hex::DecodeError`.
  ([#604](https://github.com/type-rb/type-rb/pull/604))

### Language and compiler

- Exact named imports support aliases, declaration-root matching preserves
  acronym spelling, and authored capability activation is isolated from
  compiler-generated imports. Declaration ownership is preserved through
  checking, lowering, completion, the REPL, and all generated backends.
  ([#603](https://github.com/type-rb/type-rb/pull/603),
  [#604](https://github.com/type-rb/type-rb/pull/604))
- Go now emits payload enum constructor calls with the complete owner name for
  nested declarations.
  ([#606](https://github.com/type-rb/type-rb/pull/606))

### Tooling

- Bootstrap snapshot version 4 accepts registered arrays as array elements,
  including nested literals, indexing, and mutation.
  ([#601](https://github.com/type-rb/type-rb/pull/601))
- Generated browser clients no longer emit a redundant terminal `return` in
  their initializer, so recommended lint passes without editing generated
  source. ([#605](https://github.com/type-rb/type-rb/pull/605))
- The VS Code extension source is version 0.4.0, requires TypeRB 0.4.0, and
  highlights declaration-root imports and explicit capability activation. The
  extension is published separately from the compiler release.
  ([#607](https://github.com/type-rb/type-rb/pull/607))

## 0.3.48 - 2026-08-28

### Language and compiler

- Mutable nullable bindings now retain the precise flow type of their latest
  assignment while all later assignments continue to be checked against the
  binding's declared nullable type. Editor hover information reflects the
  narrowed flow type. ([#595](https://github.com/type-rb/type-rb/pull/595))
- Postfix calls and portable iteration blocks chained after `do...end` or
  brace collection blocks are now parsed, type-checked, formatted, and
  compiled instead of being silently discarded or diagnosed against the
  wrong receiver. ([#598](https://github.com/type-rb/type-rb/pull/598))

### Tooling

- The recommended `trb/omit-terminal-void-return` lint rule now reports and
  safely removes redundant final bare `return` statements from Void functions.
  The language and formatter continue to accept and preserve this syntax.
  ([#597](https://github.com/type-rb/type-rb/pull/597))
- `trb compiler bootstrap-snapshot --snapshot-version 4` now represents
  strings, supported arrays, first-class functions, lexical captures, and
  managed record and tagged fields while retaining the version 2 and 3
  envelopes. ([#594](https://github.com/type-rb/type-rb/pull/594))

## 0.3.47 - 2026-08-28

### Compiler

- Go generation now retains the standard `Result` runtime when source type
  annotations expand through imported aliases. This fixes undefined `Result`
  build failures for nullable cross-module aliases and typed Result test
  helpers. ([#591](https://github.com/type-rb/type-rb/pull/591))

## 0.3.46 - 2026-08-28

### Breaking changes

- `trb test` now accepts one or more `_test.trb` files or directories as
  positional paths. Directories are searched recursively, multiple paths form
  a union, and shell-expanded globs work as separate paths. Replace `--file`
  with a positional path and replace `--filter` with `-t` or
  `--test-name-pattern`, which matches full test names with a Go regular
  expression. Focused runs compile all production files and only the selected
  test files. ([#584](https://github.com/type-rb/type-rb/pull/584))

### Standard library

- `trb/std/test` adds typed `expect_ok(Result<T, E>) -> T` and
  `expect_err(Result<T, E>) -> E` assertions. They accept standard Result type
  aliases, return the expected payload, and report the helper call site when
  the opposite variant is returned. Complete Result values remain directly
  comparable with `expect(...).to_equal(...)`.
  ([#586](https://github.com/type-rb/type-rb/pull/586))

## 0.3.45 - 2026-08-28

### Breaking changes

- Project Declaration Input advances from version 6 to 7. Strict consumers
  must accept version 7 and read canonical declaration identities separately
  from source display names. Nested records and enums now expose structured
  owner identities, while the set of visible declaration categories remains
  unchanged. ([#581](https://github.com/type-rb/type-rb/pull/581))

### CLI

- Single-binary CLI applications now use `trb/cli` as their canonical import.
  The `trb/platform/go/cli` spelling introduced in 0.3.44 remains a compatible
  alias. Building currently requires the Go toolchain, but TypeRB application
  source does not expose Go APIs.
  ([#580](https://github.com/type-rb/type-rb/pull/580))

## 0.3.44 - 2026-08-28

### Breaking changes

- Function and method parameters are now immutable bindings by default. Add
  `mut` before a parameter that its implementation reassigns or uses for a
  destructive operation. This marker does not change calls, interfaces,
  overrides, or caller bindings. ([#572](https://github.com/type-rb/type-rb/pull/572))
- Portable logical expressions now use only `&&`, `||`, and `!`. Replace the
  removed `and`, `or`, and prefix `not` spellings with their symbolic forms.
  ([#569](https://github.com/type-rb/type-rb/pull/569))
- String interpolation now requires each embedded expression to already be a
  non-nullable `String`. Convert numbers, Booleans, nullable values, and other
  types explicitly, for example with `value.to_s()`.
  ([#570](https://github.com/type-rb/type-rb/pull/570))
- The `trb/web` `ParameterError::Missing`, `Duplicate`, and `Invalid` payloads
  now use named-only fields. Add labels such as `source:` and `name:` when
  constructing or matching these variants.
  ([#571](https://github.com/type-rb/type-rb/pull/571))
- Project Declaration Input advances from version 5 to 6. Strict consumers
  must accept version 6; record fields now expose `hasDefault`, enum members
  expose attributes, and nested records and enums retain qualified identities.
  ([#567](https://github.com/type-rb/type-rb/pull/567))

### Language and compiler

- Record fields can declare per-construction defaults with
  `field: Type = expression`. Defaults may reference earlier fields, distinguish
  omission from explicit `nil`, and behave consistently across Go, Ruby,
  TypeScript, imports, and the REPL. JSON, ORM, and web decoding continue to
  apply their own missing-field policies.
  ([#567](https://github.com/type-rb/type-rb/pull/567))
- Payload enum variants can place a bare `*` between positional and named-only
  fields. Constructors and exhaustive patterns bind named-only payload fields
  by label across every backend.
  ([#571](https://github.com/type-rb/type-rb/pull/571))
- Nested, imported, aliased, and same-leaf declarations now retain canonical
  identity through checking and generation, preventing unrelated declarations
  from sharing effect behavior and preserving qualified TypeScript types.
  ([#573](https://github.com/type-rb/type-rb/pull/573))
- TypeScript generation no longer lets compiler temporaries shadow source
  bindings named `value` in Float conversions, Array removal operations, or
  raw-value enum lookup.
  ([#568](https://github.com/type-rb/type-rb/pull/568))

### CLI

- Go projects can build typed command-line applications as one native binary
  with `trb/platform/go/cli`. Records define positional arguments and options,
  payload enums define subcommands, and static `@cli` metadata controls names
  and help. The generated standard-library parser supports defaults, nullable
  scalar fields, aliases, Unicode option names, help, version output, and
  source-located schema diagnostics without runtime reflection.
  ([#575](https://github.com/type-rb/type-rb/pull/575))

## 0.3.43 - 2026-08-26

### Web

- Go projects can now run `_test.trb` files colocated with web routes when the
  tests call ordinary functions in the route package. Generated test runners
  retain the route-package import while also building the project dispatcher.
  ([#564](https://github.com/type-rb/type-rb/pull/564))

## 0.3.42 - 2026-08-26

### Standard library

- `Array#concurrent_map` now rejects mutation of mutable containers borrowed
  from enclosing scope, including through aliases, nested iteration, derived
  containers, and control-flow values. Read-only access and task-owned local
  mutation remain valid. ([#557](https://github.com/type-rb/type-rb/pull/557))

### Web

- File-route discovery now ignores colocated files ending in `_test.trb` under
  `src/routes`, so project checks and tests no longer require those test files
  to declare HTTP handlers.
  ([#561](https://github.com/type-rb/type-rb/pull/561))

## 0.3.41 - 2026-08-26

### Tooling

- `trb test` now reports failures to start the selected target runtime, such as
  a missing executable, instead of exiting without an explanation. Test
  programs that start and report their own failures retain concise output.
  ([#556](https://github.com/type-rb/type-rb/pull/556))

## 0.3.40 - 2026-08-26

### Standard library

- `Array#concurrent_map` now provides import-free, bounded I/O concurrency with
  ordered results, a portable default limit of 8, explicit named limits, and
  shared capacity across nested maps. Go, Ruby, TypeScript, and the REPL use
  the same structured cancellation and joining contract, while compile-time
  checks reject unsafe shared captures and mutation across authored calls and
  constructors. ([#546](https://github.com/type-rb/type-rb/pull/546))

## 0.3.39 - 2026-08-26

### Web

- `trb web client` now generates deterministic typed TypeRB browser clients
  from explicit endpoint contracts. Generated clients reuse shared contract
  modules and the browser HTTP transport, serialize path, query, and JSON body
  input, and expose declared response statuses as endpoint-specific result
  enums. Contract types imported from TypeRB packages, including package-root
  imports, are resolved across Go, Ruby, and TypeScript projects.
  ([#545](https://github.com/type-rb/type-rb/pull/545),
  [#550](https://github.com/type-rb/type-rb/pull/550))

## 0.3.38 - 2026-08-26

### Language and compiler

- JSX component props now contextually type fresh `Array` and `Hash` literals
  against their declared collection types. Native props such as
  `Array<"compact" | "expanded">` accept supported literal values without
  weakening collection invariance, while unsupported values remain compile-time
  errors. ([#547](https://github.com/type-rb/type-rb/pull/547))

## 0.3.37 - 2026-08-26

### ORM

- Live MySQL schema introspection now accepts the documented portable
  `mysql://` URL, including `allowPublicKeyRetrieval`. Commands such as
  `trb db apply` and `trb db check --from-db` can use the same database source
  as generated runtimes. ([#542](https://github.com/type-rb/type-rb/pull/542))

### Language and compiler

- Go output now imports project types inferred for `Array` and `Hash` literals,
  preventing valid cross-package collection code from generating undefined
  type references. ([#541](https://github.com/type-rb/type-rb/pull/541))

## 0.3.36 - 2026-08-26

### Language and compiler

- Imported class values now satisfy interface parameters when the interface is
  exposed transitively through another imported function. Valid programs no
  longer need a redundant direct import of that interface solely for
  assignability checking. ([#538](https://github.com/type-rb/type-rb/pull/538))

## 0.3.35 - 2026-08-26

### Web

- `trb web openapi` now generates deterministic OpenAPI 3.1 JSON from typed
  `trb/web` endpoint contracts. Path and query parameters, JSON request bodies,
  status-specific responses, shared records, raw enums, aliases, newtypes,
  nullable values, collections, and portable time types retain their checked
  TypeRB contracts without starting a target toolchain.
  ([#534](https://github.com/type-rb/type-rb/pull/534))

### Language and compiler

- Ruby output now keeps same-named top-level functions from different TypeRB
  modules isolated. Local calls and named or namespace imports resolve to the
  correct declaration instead of whichever Ruby method was loaded last.
  ([#535](https://github.com/type-rb/type-rb/pull/535))
- Legacy syntax diagnostics now link to the current language guide instead of
  unpublished pre-release migration pages.
  ([#529](https://github.com/type-rb/type-rb/pull/529))

## 0.3.34 - 2026-08-26

### Breaking changes

- Fixed Ruby declaration catalogs must set `protocolVersion` to `3`. Their
  catalog fields are otherwise unchanged, and project-aware class-body rules
  remain available only to bundled providers.
  ([#528](https://github.com/type-rb/type-rb/pull/528))

### Web

- `trb/web` file routes can publish optional typed `Endpoint` contracts that
  connect a checked handler to one input type and status-specific response
  types. The `handles`, `input`, and `response` declarations are omitted from
  Go, Ruby, and TypeScript runtime output and retained in a versioned catalog
  for downstream tooling; routes without contracts continue to work.
  ([#530](https://github.com/type-rb/type-rb/pull/530))

### Packages

- Bundled package providers can designate exact direct class-body calls as
  checked declaration metadata. The ordinary call signature is still
  validated, while every backend omits the declaration-only call from runtime
  output. ([#528](https://github.com/type-rb/type-rb/pull/528))

### Language and compiler

- Values returned through imported project functions now retain member types
  from transitively exposed record contracts. Portable receiver methods such
  as `Array#size` compile correctly without a redundant direct import of the
  record type. ([#531](https://github.com/type-rb/type-rb/pull/531))

## 0.3.33 - 2026-08-26

### Web

- Go projects now generate valid package paths for routes nested below dynamic
  and catch-all segments, so paths such as `routes/accounts/[id]/preferences`
  build and run in application and test targets. TypeRB route identities and
  non-Go backends remain unchanged.
  ([#525](https://github.com/type-rb/type-rb/pull/525))

## 0.3.32 - 2026-08-26

### Packages

- Bundled project-aware package providers can now inspect record declarations,
  top-level function signatures, and generic declarative calls through Project
  Declaration Input v5 without receiving executable source bodies or compiler
  internals. ([#521](https://github.com/type-rb/type-rb/pull/521))

### ORM

- The typed three-argument `where(column, operator, value)` form now accepts
  `LIKE` for String columns across Go, Ruby, TypeScript, and the REPL. Patterns
  remain bound query values, and non-String columns reject the operator during
  type checking. ([#522](https://github.com/type-rb/type-rb/pull/522))

## 0.3.31 - 2026-08-26

### Language and compiler

- JSON codecs now follow project record, raw-enum, alias, and newtype contracts
  returned through imported functions. Go, Ruby, and TypeScript builds no
  longer require redundant type imports or annotations at the call site.
  ([#518](https://github.com/type-rb/type-rb/pull/518))

### Tooling

- TypeScript native declaration indexes now pool repeated types, fields, and
  diagnostics, substantially reducing generated `.trb/native-types.json` size
  for component libraries with inherited props. Run `trb install` after
  upgrading when an older generated cache must be refreshed.
  ([#515](https://github.com/type-rb/type-rb/pull/515))

## 0.3.30 - 2026-08-25

### Language and compiler

- Ruby generation now resolves namespace-imported portable functions to their
  loaded definitions and preserves module-scoped private helper names inside
  conditional and `Result` control flow. Valid calls no longer dispatch to
  unrelated Ruby methods or fail because a private helper is undefined.
  ([#513](https://github.com/type-rb/type-rb/pull/513))
- TypeScript generation reserves its internal slice-operation variables, so
  source variables such as `start` no longer trigger temporal-dead-zone errors
  at runtime.
  ([#513](https://github.com/type-rb/type-rb/pull/513))

## 0.3.29 - 2026-08-25

### Language and compiler

- Go generation gives array-index helpers source-module-specific names and
  reserves internal standard-library import aliases. Valid applications with
  multiple generated files in one Go package no longer fail with duplicate
  helper declarations or import-name collisions.
  ([#508](https://github.com/type-rb/type-rb/pull/508))
- TypeScript generation now imports project types introduced by inferred local
  variable annotations, so cross-module return values no longer produce
  generated code with missing type names.
  ([#509](https://github.com/type-rb/type-rb/pull/509))
- `trb check` and `trb build` now diagnose a nested Go package importing the
  runnable root module at the TypeRB source location instead of deferring the
  failure to the Go toolchain.
  ([#507](https://github.com/type-rb/type-rb/pull/507))

## 0.3.28 - 2026-08-25

### Breaking changes

- Typed keyword parameter declarations written as `name:: Type` have been
  removed. Place a bare `*` after positional-only parameters and declare
  named-only parameters after it with `name: Type`. Parameters before `*`
  cannot be supplied by name. TypeRB source and affected official package APIs
  must use the new declaration form.
  ([#504](https://github.com/type-rb/type-rb/pull/504))
- Tooling Protocol advances to version 3 and Project Declaration Input
  Protocol advances to version 4 so exported signatures can identify
  named-only parameters. Strict protocol consumers must accept the new
  versions and honor the `namedOnly` field.
  ([#504](https://github.com/type-rb/type-rb/pull/504))

### Language and compiler

- A bare `*` now separates positional-only parameters from named-only
  parameters. Named-only parameters may be required or have defaults, and
  callers may reorder or omit them by label. Binding diagnostics, left-to-right
  argument evaluation, and callee-owned default evaluation are consistent
  across Ruby, Go, TypeScript, and the REPL. Interfaces, overrides, imports,
  and native declarations preserve the same call signature.
  ([#504](https://github.com/type-rb/type-rb/pull/504))

## 0.3.27 - 2026-08-25

### Language and compiler

- Ruby generation gives private top-level functions source-module-specific
  runtime names, preventing same-named helpers from overwriting one another
  when an application loads multiple generated files.
  ([#501](https://github.com/type-rb/type-rb/pull/501))

## 0.3.26 - 2026-08-25

### Breaking changes

- `trb/std/arrays` and `trb/std/hashes` are no longer public packages. Remove
  those imports and use the corresponding `Array` and `Hash` receiver methods,
  such as `values.reverse()`, `values.count(item)`, `labels.keys()`, and
  `labels.delete(key)`. ([#495](https://github.com/type-rb/type-rb/pull/495))

### Language and compiler

- Go generation preserves imported transparent aliases when a local helper
  result is lifted into a nullable return, preventing unqualified underlying
  type names in the generated source.
  ([#496](https://github.com/type-rb/type-rb/pull/496))

### REPL

- Completion offers portable standard packages and functions, keeps ambiguous
  origins available for explicit selection, inserts selected imports visibly,
  and supports cancellable and expression-continuation workflows.
  ([#498](https://github.com/type-rb/type-rb/pull/498))

### Tooling and documentation

- TypeRB now publishes one documentation website for installation, guides,
  lint rules, the language reference, Tour, and Playground. Code examples use
  build-time highlighting, provide accessible copy buttons, and distinguish
  runnable programs from browser scratch evaluation.
  ([#492](https://github.com/type-rb/type-rb/pull/492),
  [#493](https://github.com/type-rb/type-rb/pull/493),
  [#494](https://github.com/type-rb/type-rb/pull/494))
- Tagged `go install` builds report their release version. Getting Started and
  application guides provide executable target-specific workflows, and
  managed React projects install the TypeScript declarations they require.
  ([#497](https://github.com/type-rb/type-rb/pull/497))

## 0.3.25 - 2026-08-24

### Breaking changes

- `return if condition` is now the bare conditional-return statement. Source
  that directly returned a complete value-producing `if` block must assign the
  complete expression before returning it or replace it with the new
  conditional expression. ([#489](https://github.com/type-rb/type-rb/pull/489))

### Language and compiler

- `condition ? value : alternative` provides a lazy, strictly Boolean
  conditional expression, while `return`, `next`, and `break` accept a trailing
  `if` for concise control transfer. Both forms share narrowing, typed IR, and
  portable behavior across Go, Ruby, TypeScript, and the REPL.
  ([#489](https://github.com/type-rb/type-rb/pull/489))
- Formatting preserves native syntax and token boundaries, and portable
  Integer range checks and Float edge behavior are consistent across every
  backend and the REPL.
  ([#486](https://github.com/type-rb/type-rb/pull/486))

### Tooling

- `trb lint` runs correctness checking before a configurable built-in ruleset,
  supports human or versioned JSON diagnostics, and applies explicitly safe
  fixes. The initial recommended rule converts simple single-transfer guard
  blocks into conditional transfers without discarding comments or producing
  lines over 120 rendered columns.
  ([#489](https://github.com/type-rb/type-rb/pull/489))
- REPL compile-time and runtime diagnostics report stable source locations,
  label the interactive session as `(trb)`, and use project-relative paths for
  project source.
  ([#487](https://github.com/type-rb/type-rb/pull/487))

### Packages

- `trb update PACKAGE...` selectively re-resolves the requested direct package
  graphs while keeping other direct dependency graphs pinned.
  ([#488](https://github.com/type-rb/type-rb/pull/488))
- The adapter conformance examples now include Amplify Auth sign-in with
  checked failures and application-owned runtime configuration. Managed
  TypeScript package manifests also keep shell operators readable.
  ([#485](https://github.com/type-rb/type-rb/pull/485))

## 0.3.24 - 2026-08-24

### Breaking changes

- The built-in Rails provider now exposes only framework-owned
  `ActionController::API`, `ActionController::Base`,
  `ActionController::Parameters`, and generic `ActiveRecord::Relation<T>`
  contracts. Projects that referenced the prototype-only
  `ApplicationController`, `Api::ApplicationController`, `PaginationHelper`,
  or `Pagination` declarations must move those application or gem contracts
  into their own source or package declarations.
  ([#480](https://github.com/type-rb/type-rb/pull/480))

### Packages and Ruby interop

- Ruby TypeRB packages can publish fixed, data-only declarations for
  gem-owned classes, modules, mixins, generic methods, overloads, and
  literal-dependent signatures through `declarationProviders.ruby`. An
  explicit package-root import activates the catalog and preserves the
  package-owned native `require`. The strict host rejects compiler hooks,
  project-aware metadata, unsafe types, representation privileges, declaration
  conflicts, and package-path escapes.
  ([#481](https://github.com/type-rb/type-rb/pull/481))

## 0.3.23 - 2026-08-24

### Breaking changes

- The machine-readable `trb adapter check` and `trb adapter test` report
  protocol advances to version 2 and includes native runtime adapter paths,
  protocol versions, and binding counts. Strict report consumers must accept
  protocol version 2 and the new runtime fields.
  ([#477](https://github.com/type-rb/type-rb/pull/477))

### Packages

- TypeRB packages can pair semantic declaration adapters with validated,
  mode-specific native runtime mappings for package-owned Go, Ruby, and
  TypeScript shims. Protocol version 1 deliberately accepts only top-level
  `(String) -> String` wire functions, while explicit effect flags propagate
  TypeScript suspension and the hidden backend execution scope. Package source
  continues to own JSON validation and conversion to domain `Result` values.
  ([#477](https://github.com/type-rb/type-rb/pull/477))

## 0.3.22 - 2026-08-24

### Packages and TypeScript interop

- Declaration adapters can expose readonly native properties on
  non-constructible interfaces. This supports checked projections of opaque
  objects such as authentication context values without making them
  constructible in TypeRB.
  ([#473](https://github.com/type-rb/type-rb/pull/473))
- Native Promise-returning functions and instance members can expose checked
  `Result<T, String>` values. Promise resolution produces `Ok`, synchronous
  throws and rejections produce `Err`, and `Promise<void>` maps to
  `Result<Unit, String>` while preserving TypeScript suspension propagation
  and strict native Promise conformance.
  ([#474](https://github.com/type-rb/type-rb/pull/474))

## 0.3.21 - 2026-08-23

### Go backend

- Go ORM and SQL Jobs runtimes validate and remove Bun's
  `allowPublicKeyRetrieval` URL parameter before opening a MySQL DSN. Projects
  can now share the same explicit local-development MySQL URL across Go, Ruby,
  and TypeScript modes without the Go driver forwarding the unknown parameter
  as a server system variable.
  ([#469](https://github.com/type-rb/type-rb/pull/469))

## 0.3.20 - 2026-08-23

### TypeScript backend

- TypeScript/Bun ORM and SQL Jobs runtimes honor an explicit
  `allowPublicKeyRetrieval=true` MySQL URL parameter for trusted local MySQL 8
  connections without TLS. The option remains disabled unless the application
  opts in, and the database guide documents the security tradeoff and recommends
  TLS for deployed databases.
  ([#466](https://github.com/type-rb/type-rb/pull/466))

## 0.3.19 - 2026-08-23

### Go backend

- Generated Go prunes package imports that become unused when imported
  newtype construction and unwrapping are lowered to the representation type.
  This prevents otherwise valid newtype-based applications from failing the
  Go build with an unused-import error.
  ([#463](https://github.com/type-rb/type-rb/pull/463))

## 0.3.18 - 2026-08-23

### Breaking changes

- Transparent type aliases now use `alias Name = Target`. The former
  `type Name = Target` spelling reports a migration diagnostic so each
  declaration can be classified as a transparent `alias` or a nominal
  `newtype`.
  ([#460](https://github.com/type-rb/type-rb/pull/460))
- The bundled Declaration Protocol advances to version 2, call-specialization
  protocol to version 2, and Project Declaration Input Protocol to version 3
  to carry nominal representation boundaries. The read-only
  `trb compiler inspect` protocol advances to version 2 and reports authored
  newtype declarations. ([#460](https://github.com/type-rb/type-rb/pull/460))

### Language and compiler

- `newtype Name = Representation` declares a strict nominal type over any
  concrete, fully instantiated, non-nullable representation, including
  collections such as `Array<ProductId>`. Newtypes use `.new(value)` and
  `.value()`, preserve nominal checking in ordinary source, and expose their
  concrete shape only through typed JSON, Web, Jobs, ORM, and explicitly
  declared package boundaries. Go, Ruby, TypeScript, and the REPL share the
  same behavior. ([#460](https://github.com/type-rb/type-rb/pull/460))

### REPL and tooling

- Multiline editing displays two-space indentation consistently, preserves the
  accepted formatted source on screen, and keeps accepted indentation stable
  after submission. ([#456](https://github.com/type-rb/type-rb/pull/456),
  [#458](https://github.com/type-rb/type-rb/pull/458),
  [#459](https://github.com/type-rb/type-rb/pull/459))
- The TextMate grammar, Playground highlighter, completion, document symbols,
  compiler inspection, and Visual Studio Code snippets recognize `alias` and
  `newtype`. ([#460](https://github.com/type-rb/type-rb/pull/460))

## 0.3.17 - 2026-08-23

### Breaking changes

- Declaration/Adapter Protocol v2 distinguishes class instance members from
  class members. Adapter authors must set `protocolVersion` to `2`, replace
  each class's `members` with `instanceMembers` or `classMembers` according to
  how callers access it, and run `trb install` to regenerate the native-type
  cache. ([#448](https://github.com/type-rb/type-rb/pull/448))

### REPL

- Interactive multiline input is automatically indented while it is open and
  formatted before evaluation and history storage. Results directly associated
  with mutable bindings are marked with `[mut]`.
  ([#451](https://github.com/type-rb/type-rb/pull/451))

### Packages and tooling

- Declaration adapters can model non-constructible interfaces for opaque
  native objects with checked instance members. A pinned TanStack Router
  example verifies this boundary and documents why route-tree-derived types
  require a future project provider.
  ([#449](https://github.com/type-rb/type-rb/pull/449),
  [#450](https://github.com/type-rb/type-rb/pull/450))

## 0.3.16 - 2026-08-23

### Breaking changes

- Package-owned native declarations now use the mode-independent
  Declaration/Adapter Protocol v1. Rename `nativeTypeProviders` to
  `declarationAdapters`, replace provider `formatVersion: 2` with
  `protocolVersion: 1`, rename nested semantic type `args` fields to
  `arguments`, and run `trb install` to regenerate the native-type cache.
  ([#439](https://github.com/type-rb/type-rb/pull/439))
- Jobs configuration now defines one application-scoped
  `JOBS_ADAPTER: JobAdapter := SQLAdapter.new(...)` constant instead of a
  `configure_jobs()` factory. Job definitions and enqueue call sites are
  unchanged. ([#435](https://github.com/type-rb/type-rb/pull/435))

### Packages and tooling

- `trb adapter check` validates declaration adapter manifests, semantic
  catalogs, and ecosystem-specific constraints with human or versioned JSON
  output. `trb adapter test` additionally builds an explicitly installed
  conformance project and invokes its structured native check without adding
  implicit installation or network access.
  ([#441](https://github.com/type-rb/type-rb/pull/441),
  [#444](https://github.com/type-rb/type-rb/pull/444))
- The declaration adapter host rejects malformed semantic shapes and name
  conflicts before native package resolution. Compiler-generated TypeRB
  helpers also reuse matching imports written through package aliases.
  ([#440](https://github.com/type-rb/type-rb/pull/440),
  [#445](https://github.com/type-rb/type-rb/pull/445))
- A pinned TanStack Query example demonstrates generic declarations,
  discriminated query results, Result-to-Promise bridging, and strict native
  TypeScript conformance.
  ([#443](https://github.com/type-rb/type-rb/pull/443))
- `trb compiler inspect` emits a versioned, read-only JSON snapshot of project
  sources, modules, authored imports, checked declarations and types, and
  diagnostics without exposing mutable compiler internals.
  ([#438](https://github.com/type-rb/type-rb/pull/438))

### Compiler and runtimes

- Generated TypeScript browser requests and React components pass strict
  native checking when optional request inputs and the internal execution
  scope are absent. ORM transactions are treated as asynchronous even when
  their block contains no other database terminal operation.
  ([#434](https://github.com/type-rb/type-rb/pull/434),
  [#442](https://github.com/type-rb/type-rb/pull/442))
- Ruby and TypeScript Jobs adapters return a successful Job reference after
  storage confirms enqueue instead of changing that known success to a
  cancellation error. ([#436](https://github.com/type-rb/type-rb/pull/436))

### Tooling distribution

- GitHub syntax highlighting is now distributed exclusively through the
  published Chrome Web Store extension; the temporary Tampermonkey fallback
  is no longer shipped. ([#437](https://github.com/type-rb/type-rb/pull/437))

## 0.3.15 - 2026-08-22

### Jobs

- Jobs now exposes a small portable enqueue adapter contract. Derived Job
  methods share serialization, scheduling validation, and error behavior
  across generated Go, Ruby, and Bun applications.
  ([#430](https://github.com/type-rb/type-rb/pull/430))

### Tooling

- `trb fmt` now recursively indents structured JSX while preserving
  text-bearing JSX whitespace and grouping parentheses in embedded
  expressions. ([#431](https://github.com/type-rb/type-rb/pull/431))
- TypeRB source files, pull request diffs, and explicit TypeRB Markdown blocks
  can now be syntax-highlighted on github.com with the Chrome extension.
  ([#422](https://github.com/type-rb/type-rb/pull/422))
- TypeRB syntax highlighting on GitHub now preserves blank lines in repository
  file views. ([#428](https://github.com/type-rb/type-rb/pull/428))

## 0.3.14 - 2026-08-22

### Language and collections

- Fresh unannotated mutable Arrays and Hashes now infer their types from all
  statically checked writes in the first constraining statement, including
  branches and literal callback bodies. Later statements are checked against
  the fixed type, and Hash keys remain homogeneous.
  ([#424](https://github.com/type-rb/type-rb/pull/424))

### REPL

- Configured REPL sessions now complete unique project declarations and their
  class members immediately after startup, before the first expression is
  evaluated. ([#421](https://github.com/type-rb/type-rb/pull/421))

## 0.3.13 - 2026-08-22

### Compiler and REPL performance

- Incremental project analysis reuses unchanged type-provider declarations and
  lowered IR, including semantically unchanged TypeScript native package
  catalogs loaded into new instances. This substantially reduces repeated
  compiler work in larger, schema-backed projects.
  ([#412](https://github.com/type-rb/type-rb/pull/412),
  [#413](https://github.com/type-rb/type-rb/pull/413),
  [#415](https://github.com/type-rb/type-rb/pull/415))
- The REPL batches project definition loading, refreshes only changed modules,
  and reuses unchanged project language metadata between submissions. In the
  measured 274-file application, repeated input fell from about 305 to 28
  milliseconds in Ruby mode, 303 to 28 milliseconds in Go mode, and 313 to 35
  milliseconds in TypeScript mode.
  ([#414](https://github.com/type-rb/type-rb/pull/414),
  [#416](https://github.com/type-rb/type-rb/pull/416))

## 0.3.12 - 2026-08-22

### Breaking changes

- Go projects now require Go 1.27. Generated generic class instance methods use
  native Go generic methods, compiler-owned Unicode behavior follows Unicode
  17, and the reference Bun container uses Bun 1.4. Projects using Go 1.26 must
  upgrade their Go toolchain and set `go.version` to 1.27 or later.
  ([#409](https://github.com/type-rb/type-rb/pull/409))

### Compiler performance

- Project compilation now indexes transparent type aliases, substantially
  reducing semantic analysis time in type-alias-heavy projects. In the measured
  application, cold analysis fell from about 693 milliseconds to 246
  milliseconds, while a 512-module lookup benchmark improved from about 7.66
  microseconds to 18.8 nanoseconds with no lookup allocations.
  ([#408](https://github.com/type-rb/type-rb/pull/408))

### Packages and web

- Bundled compiler-integrated packages can use an experimental, versioned,
  data-only call-specialization protocol. `trb/web` now implements generic
  endpoint input binding through target-independent TypeRB helper source for
  consistent Go, Ruby, and TypeScript behavior.
  ([#407](https://github.com/type-rb/type-rb/pull/407))

## 0.3.11 - 2026-08-21

### Compiler and tooling

- Long-lived compiler clients reuse parsed source and checked semantic results
  across single-module edits. Body-only changes recheck that module, while
  public catalog changes invalidate its downstream importers and provider
  declaration changes conservatively invalidate the whole project. In a
  64-module benchmark, an independent edit fell from about 1.17 milliseconds
  to 0.17 milliseconds and allocations fell from about 1.02 MB to 243 KB.
  ([#402](https://github.com/type-rb/type-rb/pull/402),
  [#404](https://github.com/type-rb/type-rb/pull/404))

### REPL

- Project-aware REPL submissions reuse resolution and type-checking results for
  unchanged project modules while preserving the last successful state after
  invalid input. Configuration and compiler-owned dependency changes still
  fall back to complete analysis. In a 64-module benchmark, repeated input fell
  from about 1.16 milliseconds to 0.16 milliseconds and allocations fell from
  about 1.03 MB to 238 KB.
  ([#403](https://github.com/type-rb/type-rb/pull/403))

## 0.3.10 - 2026-08-21

### REPL

- Project-aware REPL sessions reuse their initial semantic analysis and no
  longer generate unused backend source for every project module. In a
  274-file application, startup fell from 6.09 seconds to 1.05 seconds and
  startup plus the first expression fell from 9.18 seconds to 2.02 seconds.
  ([#397](https://github.com/type-rb/type-rb/pull/397),
  [#398](https://github.com/type-rb/type-rb/pull/398))

### Web

- TypeScript projects can use `trb/web/testing.dispatch` from source-root
  `*_test.trb` files without an undefined dispatcher at runtime.
  ([#395](https://github.com/type-rb/type-rb/pull/395))

## 0.3.9 - 2026-08-21

### Compiler and CLI

- `trb check FILE.trb` validates a config-free file and its transitive explicit
  local imports in Ruby, Go, or TypeScript mode without requiring `main()` or a
  `trbconfig.jsonc` project. ([#387](https://github.com/type-rb/type-rb/pull/387))
- Invalid UTF-8 source now produces a precise syntax diagnostic instead of
  reaching the parser, formatter, or language service with malformed bytes.
  ([#388](https://github.com/type-rb/type-rb/pull/388))

### Tooling

- Project-aware CLI and language-server formatting remove a redundant terminal
  `/index` from imports while retaining it whenever shortening would select a
  different or unresolved module.
  ([#392](https://github.com/type-rb/type-rb/pull/392))
- Contributors can launch the current compiler and Visual Studio Code
  extension together in an isolated local QA profile without replacing their
  regular extension or installed compiler.
  ([#391](https://github.com/type-rb/type-rb/pull/391))
- The packaged Visual Studio Code extension is minified, reducing its bundle
  by about 53% and its compressed VSIX by about 26%.
  ([#389](https://github.com/type-rb/type-rb/pull/389))

## 0.3.8 - 2026-08-21

### Language and compiler

- Integer literals outside TypeRB's portable exact range now produce a
  compile-time diagnostic instead of rounding differently between targets.
  ([#378](https://github.com/type-rb/type-rb/pull/378))
- Malformed call expressions no longer cause a compiler, formatter, or
  language-service panic. ([#379](https://github.com/type-rb/type-rb/pull/379))

### Web

- Static file routes take precedence over parameter routes, so paths such as
  `/todos/new` can coexist with `/todos/[id]` consistently in Go, Ruby, and
  TypeScript modes. ([#383](https://github.com/type-rb/type-rb/pull/383))

### ORM and database tooling

- Database commands accept the same portable `mysql://` URLs as generated
  runtimes while retaining Go driver DSN support.
  ([#381](https://github.com/type-rb/type-rb/pull/381))
- Generated TypeScript applications initialize MySQL ORM sessions to UTC,
  matching Go and Ruby and keeping database-evaluated time defaults portable.
  ([#382](https://github.com/type-rb/type-rb/pull/382))

### Tooling

- The formatter now produces stable one-pass output for chained multiline
  tokens and symbols following operators.
  ([#380](https://github.com/type-rb/type-rb/pull/380))

## 0.3.7 - 2026-08-20

### Tooling

- `trb install --config PATH` installs TypeRB packages and native dependencies
  for an explicitly selected project configuration. This supports projects
  that compile one source tree with separate Go, Ruby, or TypeScript configs.
  The container guide now also shows how to check or transpile source with the
  compiler image without installing TypeRB on the host.
  ([#375](https://github.com/type-rb/type-rb/pull/375))

## 0.3.6 - 2026-08-20

### Tooling

- The Neovim plugin now formats `.trb` buffers before save by default. Set
  `vim.g.typerb_format_on_save = false` to opt out.
  ([#370](https://github.com/type-rb/type-rb/pull/370))
- TypeRB releases now publish the Linux compiler as a public multi-platform
  OCI image at `ghcr.io/type-rb/trb`. Projects can copy the compiler into
  their own Go, Ruby, Node, or Bun toolchain images without taking on an
  official combined runtime support matrix. The image is built for Arm64 and
  x86-64 and includes an SBOM and provenance attestation.
  ([#371](https://github.com/type-rb/type-rb/pull/371))

## 0.3.5 - 2026-08-20

### Tooling

- The Neovim plugin now enables its native TypeRB language-server
  configuration when loaded, so installation no longer requires a separate
  `vim.lsp.enable("typerb")` call. Set
  `vim.g.typerb_format_on_save = true` to opt in to formatting before save, or
  `vim.g.typerb_auto_start = false` when another configuration owns startup.
  ([#367](https://github.com/type-rb/type-rb/pull/367))

## 0.3.4 - 2026-08-20

### Compiler and runtimes

- Incomplete method parameter lists now produce parser diagnostics instead of
  aborting compilation, formatting, or language-service analysis.
  ([#364](https://github.com/type-rb/type-rb/pull/364))
- Ruby code generation preserves receiver grouping for calls on compound
  expressions. Generated Go and Ruby ORM runtimes accept portable `mysql://`
  database URLs, so the same server configuration can run across Go, Ruby,
  and TypeScript modes. ([#363](https://github.com/type-rb/type-rb/pull/363))

### Tooling

- TypeRB projects can use the repository as a minimal Neovim plugin with
  filetype detection, canonical indentation, and native language-server
  configuration. JetBrains IDE setup with TextMate and LSP4IJ is documented.
  ([#361](https://github.com/type-rb/type-rb/pull/361))
- Neovim starts an independent compiler-backed language-server client for a
  config-free TypeRB file, providing diagnostics, semantic highlighting,
  completion, hover, navigation, rename, and formatting without requiring a
  `trbconfig.jsonc` project. ([#362](https://github.com/type-rb/type-rb/pull/362))

## 0.3.3 - 2026-08-20

### Tooling

- Go to Definition treats the complete path in a project import as one link
  to its resolved TypeRB source. Project auto-imports omit a redundant
  terminal `/index` while retaining it when the shorter path would select a
  different module. ([#358](https://github.com/type-rb/type-rb/pull/358))
- Parenthesized integer Range literals such as `(1..10)` now offer `to_a()`
  through member completion in editors and the REPL.
  ([#357](https://github.com/type-rb/type-rb/pull/357))

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

### Tooling

- Visual Studio Code extension 0.3.0 understands `try` and `catch`, removes the
  retired failure syntax from language features, and requires a TypeRB 0.3
  compiler.

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
