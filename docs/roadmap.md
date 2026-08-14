# TypeRB Roadmap

This roadmap records the capabilities required to move TypeRB from the current
alpha compiler to a language that can be used for maintained production
applications. Ordering may change as real Go, Ruby, and TypeScript projects
expose missing language features.

For implemented behavior, see [status.md](status.md). Language decisions belong
in [specification.md](specification.md); this document tracks outcomes rather
than detailed syntax.

## 1. Complete the portable language core

- Extend higher-order function effects across third-party package declarations,
  native callback boundaries, and concurrent composition. Evaluate concise
  `Result` propagation syntax only after application usage establishes a clear
  ergonomic need.
- Extend union type-pattern narrowing to nullable, collection, enum, record,
  and class alternatives. Add position-typed Tuples and precise optional-value
  semantics.
- Add type-argument inference, generic interfaces, and generic class methods.
- Define mutation effects for methods and parameters so immutable values remain
  safe across calls, not only assignments and known collection operations.
- Complete the portable class model with explicit superclass construction and
  `super` semantics, initialization order, override rules, and a backend-safe
  decision for field/method name collisions.
- Specify module visibility, initialization order, constant evaluation, and
  cross-file semantics completely.

## 2. Make testing a language-level workflow

- Add `trb/std/test` with suites, test cases, assertions, setup/teardown, and
  expected-error helpers.
- Add `trb test` with project discovery, name/location filters, deterministic
  output, non-zero failure status, and target-specific runtime adapters.
- Preserve `.trb` locations in failures and stack traces.
- Add coverage reporting after generated-code locations can be mapped back to
  TypeRB source.

## 3. Production diagnostics and compiler reliability

- Expand the initial stable diagnostic catalog and source-edit suggestions as
  application and editor usage exposes useful semantic-specific actions.
- Emit target-standard source maps from the shared generated-range-to-`.trb`
  mapping model, then use them for runtime stack traces and coverage.
- Extend the initial versioned compiler service with phase-level incremental
  compilation, dependency-aware invalidation, and a persistent build cache.
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

- Complete the everyday receiver surface for Unicode strings, Arrays, and
  Hashes. Keep operations that need first-class blocks staged with the
  language-level block/lambda work.
- Expand the compiler-owned baseline with UUID, regular-expression, and full
  URL packages. Evolve the initial date/time package only from application
  evidence, especially for calendar periods, locale formatting, and clocks.
  Prefer `Bytes` at binary and digest boundaries, and keep legacy hashes
  explicitly limited to compatibility use.
- Use browser compatibility as the baseline for portable TypeScript APIs while
  also supporting Bun and Node server runtimes. Runtime-only APIs and server
  framework integrations remain explicit platform packages.
- Keep target-only functionality in explicit `trb/platform/<mode>/*` packages.
- Expand automatic TypeScript `.d.ts` indexing beyond its initial simple
  signatures and harden the declarative fallible/suspending callback bridge.
  Then provide equivalent package-owned type discovery for Go modules and
  gems without application-authored signatures.
- Evolve the initial distributed Git package system with semantic version
  constraints, selective updates, publishing conventions, shared caches,
  vulnerability/audit integration, and namespace-stable type identities.
- Define a stable extension protocol before external packages can request
  compiler integration; do not expose compiler internals as a public plugin
  API.

## 6. Application-level proof

- Harden the official portable `trb/web` package around its existing
  compile-time file-based routes, typed JSON requests and responses,
  middleware, and consistent HTTP behavior across Go, Ruby, and TypeScript
  runtimes.
- Extend the generated JSON codecs and client/server contracts from shared
  records with versioned wire fields, validation policy, schema export, and
  compatibility checking.
- Harden the experimental `trb/orm` package with schema caching, compatibility
  policy, production diagnostics, and larger end-to-end `trb/web` applications.
- Harden the experimental `trb/jobs` contract with structured payload codecs,
  worker failure-injection tests, recurring schedules, operational metrics,
  and explicit delivery profiles such as transactional outbox adapters.
- Continue representative backend, server-rendered, and browser application
  slices with authentication, validation, transactions, background work, and
  realistic failure paths.
- Harden the initial portable OIDC bearer profile, then add a separately
  reviewed server-session profile with authorization-code PKCE, cookie-key
  rotation, and pluggable session storage.
- Evolve the initial buffered TypeScript browser HTTP client with generated
  endpoint-contract adapters, external contract import, streaming, cancellation
  handles, and established server-state package interop without introducing a
  second request model.
- Separate one-shot `trb run` from framework development servers and add a
  coherent `trb dev` and runtime-adapter model for backend and frontend
  development servers.
- Harden the database schema workflow with compatibility policy, additional
  schema constructs, and supported-platform release validation while keeping
  destructive changes explicit.

## 7. Distribution and long-term operation

- Keep Homebrew and direct binary installation reproducible and signed.
- Keep the hosted `/play/` and `/tour/` editions current with the compiler and
  add shareable source links to their sandboxed browser runtime.
- Test supported OS/architecture and current target-toolchain combinations in
  CI.
- Publish a searchable language/standard-library reference, tutorials, and a
  migration guide for every breaking release.
- Add performance benchmarks, crash reporting guidance, security policy, and a
  predictable release cadence.

## Exploratory: native TypeRB runtime

A future `mode: trb` may execute checked TypeRB through typed IR or cached
bytecode without emitting Go, Ruby, or TypeScript source. The existing REPL
evaluator is an early implementation seed, not a production runtime.

This is intentionally only a possibility, not a committed target or release
item. Promoting it to a public mode would require a runtime object model,
module loading, source-mapped failures, portable filesystem/process/network
adapters, dependency and distribution rules, and a clear story for code that
currently imports another mode's platform packages. Internal evaluator work
for REPL, tests, and backend conformance may proceed without committing TypeRB
to shipping this mode.

Self-hosting is deliberately outside the current roadmap. It will be
reconsidered after the Go compiler has broad real-world use and its public
behavior is stable.

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
