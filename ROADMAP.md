# TypeRB Roadmap

This roadmap records the capabilities required to move TypeRB from the current
alpha compiler to a language that can be used for maintained production
applications. Ordering may change as real Go, Ruby/Rails, and TypeScript/React
projects expose missing language features.

For implemented behavior, see `STATUS.md`. Language decisions belong in
`SPEC.md`; this document tracks outcomes rather than detailed syntax.

## 1. Complete the portable language core

- Extend the implemented typed `map`/`select`/`reduce` expressions from their
  single-result-expression v0.1 blocks to structured multi-statement blocks,
  then add first-class blocks/lambdas. Portable `Result<T, E>` with explicit
  exhaustive handling and safe conversion/lookup APIs is implemented; concise
  propagation syntax remains a later ergonomics decision.
- Add union types, position-typed Tuples, and precise nullable/optional
  semantics. Payload-bearing enums, exhaustive payload pattern narrowing, and
  explicit generics for payload enums/top-level functions are implemented;
  inference and generic records/classes/methods remain.
- Define mutation effects for methods and parameters so immutable values remain
  safe across calls, not only assignments and known collection operations.
- Complete the portable class model with explicit superclass construction and
  `super` semantics, initialization order, override rules, generic classes, and
  a backend-safe decision for field/method name collisions.
- Specify module visibility, initialization order, constant evaluation, and
  cross-file semantics completely.
- Expand the self-host tree until the lexer, parser, checker, and formatter can
  be compiled from TypeRB sources.

## 2. Make testing a language-level workflow

- Add `trb/std/test` with suites, test cases, assertions, setup/teardown, and
  expected-error helpers.
- Add `trb test` with project discovery, name/location filters, deterministic
  output, non-zero failure status, and target-specific runtime adapters.
- Preserve `.trb` locations in failures and stack traces.
- Add coverage reporting after generated-code locations can be mapped back to
  TypeRB source.
- Dogfood the test API in the compiler self-host tree and example applications.

## 3. Production diagnostics and compiler reliability

- Add stable diagnostic codes, recovery after multiple errors, related spans,
  and actionable suggestions.
- Add source maps from generated Ruby, Go, and TypeScript back to `.trb` spans.
- Add incremental project compilation, dependency-aware invalidation, and a
  persistent build cache.
- Maintain target-conformance suites that run equivalent TypeRB programs in all
  three modes.
- Define compatibility and deprecation rules before the first beta release.

## 4. Editor tooling

- Implement a reusable language-server package and `trb lsp`.
- Support live diagnostics, formatting, hover types, completion, go to
  definition, references, rename, document symbols, semantic tokens, and code
  actions.
- Ship a VS Code extension as a thin LSP client with syntax highlighting,
  snippets, formatter integration, and test discovery.
- Add debugger/DAP support later, after source maps and runtime stack mapping
  are reliable.

## 5. Standard library and package ecosystem

- Expand the compiler-owned standard library beyond the current Result,
  Unit, collection, Unicode, path, filesystem, and typed JSON/JSONC baseline
  with time, environment, process, regular-expression, HTTP, and encoding
  packages where semantics can be made consistent across targets.
- Keep target-only functionality in explicit `trb/platform/<mode>/*` packages.
- Stabilize compiler-owned library type providers so gems, Go modules, and npm
  packages do not require application-authored signature files.
- Add reproducible dependency locking, checksums, offline builds, package
  publishing, version constraints, and vulnerability/audit integration.

## 6. Application-level proof

- Extend the generated JSON codecs and client/server contracts from shared
  records with versioned wire fields, validation policy, schema export, and
  compatibility checking.
- Continue the Go API, Rails, and React vertical slices with authentication,
  validation, transactions, background work, and realistic failure paths.
- Separate one-shot `trb run` from framework development servers and add a
  coherent `trb dev`/runtime-adapter model for Rails, Vite, and future TypeRB
  frameworks.
- Define database migration/schema workflows without making ordinary compiler
  commands perform surprising destructive operations.

## 7. Distribution and long-term operation

- Keep Homebrew and direct binary installation reproducible and signed.
- Test supported OS/architecture and current target-toolchain combinations in
  CI.
- Publish a searchable language/standard-library reference, tutorials, and a
  migration guide for every breaking release.
- Add performance benchmarks, crash reporting guidance, security policy, and a
  predictable release cadence.

## Exploratory: native TypeRB runtime

A future `mode: trb` may execute checked TypeRB through typed IR or cached
bytecode without emitting Ruby, Go, or TypeScript source. The existing REPL
evaluator is an early implementation seed, not a production runtime.

This is intentionally only a possibility, not a committed target or release
item. Promoting it to a public mode would require a runtime object model,
module loading, source-mapped failures, portable filesystem/process/network
adapters, dependency and distribution rules, and a clear story for code that
currently imports another mode's platform packages. Internal evaluator work
for REPL, tests, self-hosting, and backend conformance may proceed without
committing TypeRB to shipping this mode.

## Practical-readiness gates

An initial beta should require all of the following:

- A representative API and frontend can be built, tested, edited through LSP,
  and deployed without editing generated code.
- Compiler errors and runtime failures point back to TypeRB source.
- Builds and dependency resolution are reproducible in CI.
- The formatter, compiler, test runner, and language server are stable enough
  for daily use on a multi-package project.
- Each supported mode has an end-to-end conformance suite and documented escape
  hatch for platform-specific functionality.
