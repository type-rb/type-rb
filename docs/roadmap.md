# TypeRB Roadmap

This roadmap records the capabilities required to move TypeRB from the current
alpha compiler to a language that can be used for maintained production
applications. Ordering may change as real Go, Ruby, and TypeScript projects
expose missing language features.

For implemented behavior, see [status.md](status.md). Language decisions belong
in [specification.md](specification.md); this document tracks outcomes rather
than detailed syntax.

## 1. Complete the portable language core

- Extend Result-returning higher-order functions across third-party package
  declarations, native callback boundaries, and concurrent composition. Keep
  prefix `try` and postfix `catch` as the sole propagation and recovery syntax
  until application usage establishes another distinct need.
- Extend union type-pattern narrowing to nullable, collection, enum, record,
  and class alternatives. Add position-typed Tuples and precise optional-value
  semantics.
- Add type-argument inference, generic interface methods, and generic class
  methods.
- Define mutation effects for methods and parameters so immutable values remain
  safe across calls, not only assignments and known collection operations.
- Complete the portable class model with explicit superclass construction and
  `super` semantics, initialization order, override rules, and a backend-safe
  decision for field/method name collisions.
- Specify module visibility, initialization order, constant evaluation, and
  cross-file semantics completely.

## 2. Make testing a language-level workflow

- Evolve the initial portable `trb/std/test`, `trb test`, source-location
  failures, and VS Code Test Explorer from application evidence.
- Add lifecycle and expected-error helpers only after their semantics remain
  explicit and portable; keep data-driven tests in ordinary TypeRB meanwhile.
- Add a browser test host without adding async syntax to TypeRB.
- Extend assertion locations into complete cross-backend runtime stack traces.
- Add coverage reporting after generated-code locations can be mapped back to
  TypeRB source.

## 3. Production diagnostics and compiler reliability

- Expand the initial stable diagnostic catalog and source-edit suggestions as
  application and editor usage exposes useful semantic-specific actions.
- Emit target-standard source maps from the shared generated-range-to-`.trb`
  mapping model, then use them for runtime stack traces and coverage.
- Extend the initial versioned compiler service and dependency-aware
  single-module analysis with finer phase-level invalidation, incremental
  lowering, multi-file change sets, and a persistent build cache.
- Maintain target-conformance suites that run equivalent TypeRB programs in all
  three modes.
- Define compatibility and deprecation rules before the first beta release.

## 4. Editor tooling

- Extend the reusable `trb lsp` diagnostics, formatting, completion, hover,
  signature help, checked navigation, rename, document symbols, and quick fixes
  with richer code actions and additional editor workflows.
- Add preview VS Code workflows for REPL evaluation, package and schema status,
  and coordinated development servers after their shared compiler and CLI
  contracts are stable.
- Extend Go source debugging through Delve with Ruby and TypeScript adapters
  after their target-standard source maps and runtime stack mapping are
  reliable.

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
  signatures and harden the declarative Result/suspending callback bridge.
  Then provide equivalent package-owned type discovery for Go modules and
  gems without application-authored signatures.
- Evolve the initial distributed Git package system with semantic version
  constraints, selective updates, publishing conventions, shared caches,
  vulnerability/audit integration, and namespace-stable type identities.
- Evolve the experimental bundled-package call-specialization data contract and
  declarative native providers into a stable, sandboxed extension protocol
  before external packages can request compiler integration. Use the
  Declaration Protocol and read-only Project Declaration Input Protocol now
  exercised by both ORM and Jobs to shape reusable declaration capabilities.
  Evolve Project Generated Source Protocol v1, first exercised by portable Jobs
  worker dispatch, only when another package identifies a shared capability.
  Characterize fragment-level incremental invalidation and namespace-stable
  imports before external use. Design the remaining ORM and Jobs runtime
  integration as a separate minimal runtime operation ABI. Do not expose
  compiler internals as a public plugin API.

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
