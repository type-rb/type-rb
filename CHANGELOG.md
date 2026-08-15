# Changelog

This file records user-visible changes in stable TypeRB releases.

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
