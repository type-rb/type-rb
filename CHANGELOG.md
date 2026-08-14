# Changelog

This file records user-visible changes in stable TypeRB releases.

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
