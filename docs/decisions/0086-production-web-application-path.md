# Path to production web applications

Status: accepted. Tracked by [issue #619](https://github.com/type-rb/type-rb-native/issues/619)
and its follow-up initiatives.

## Context

Production use of the official `trb/web`, `trb/orm` and `trb/jobs` packages is a
project goal ([MIR consolidation](../mir-consolidation.md)). The end state is one
unchanged application source that works in every mode of the TypeRB-authored
toolchain: `mode: trb` for Native executables, and the `go`, `ruby` and
`typescript` modes. This decision covers the path to `mode: trb`. The
three-language emission track reuses the package semantics decided here. Native
now loads official package projects and implements package management, portable
HTTP/Web values, typed route input and in-process route dispatch. Real networking,
database adapters and job execution require separately qualified platform
services. Its collector still has one managed-heap mutator.

The [memory evidence](../runtime-memory-stability.md) does not cover sockets,
foreign handles, supervision or real job queues.

The reference implementation (pinned revision `977da4f4`) sets these contracts:

- **Package semantics are compiler-integrated.** The ORM consumes data-only
  project and schema inputs. Jobs generate portable TypeRB wrappers and dispatch
  source.
- **Every backend delegates database wire protocols to host libraries.** Go uses
  `database/sql` drivers; Ruby uses Sequel.
- **`concurrent_map` sets an upper bound only.**
  [Its decision](https://github.com/type-rb/type-rb/blob/977da4f4/docs/decisions/0004-bounded-structured-concurrent-map.md)
  bounds the number of active blocks, leaves start and completion order
  unspecified, and promises no CPU parallelism. Cancellation and deadlines
  travel through a hidden execution scope.
- **Jobs need leases, heartbeats and graceful shutdown.** The
  [jobs guide](https://github.com/type-rb/type-rb/blob/977da4f4/docs/guides/jobs.md)
  limits SQLite to small single-worker use.
- **The `web-orm-jobs` tutorial crosses all three packages.** Its request test
  dispatches a route that opens an ORM transaction and enqueues a job.

## Decision

### Concurrency

Version 1 runs multiple blocking processes. Each process has one managed-heap
mutator.

- `concurrent_map` keeps its capture and borrow checks, nested-group semantics,
  cancellation and joining even when it evaluates admitted blocks serially.
- Deadlines are observed through polling at generated checkpoints.
- Job heartbeats progress independently of handler work.
- Shutdown is graceful, and cancellation stays correct around committed
  enqueues.

Driver timeouts alone do not satisfy these contracts.

Cooperative scheduling inside a process is deferred. It starts only when the
blocking baseline misses frozen throughput, p99 latency or memory targets.

### HTTP transport

Use libuv for TCP/event delivery and llhttp for HTTP/1.1 framing, behind narrow
TypeRB-owned adapters. Pin their source revisions, build configuration, licenses
and ABI, and qualify the actual library binaries on each target. The
[network transport contract](../native-network.md) describes the implementation
and its qualification boundary.

The event loop buffers incomplete requests and writes while the TypeRB handler
loop remains single-threaded. Foreign callbacks cannot enter managed code or
retain managed pointers. This transport loop does not introduce a managed task
scheduler, replace the blocking-process baseline, or qualify process supervision.
Handler cancellation/checkpoints and independently progressing job heartbeats
remain required follow-up work. Using a mature transport first does not preclude
a later replacement supported by measured benefit and equivalent correctness.

### Database access

1. SQLite comes first and establishes the public ORM contract.
2. MySQL follows.

Both adapters use external libraries behind TypeRB-owned adapters. The libraries
own execution, protocol and cryptography. TypeRB owns portable values, errors,
transactions, resource lifetime and package semantics.

Dynamically linked MariaDB Connector/C is the provisional MySQL provider. One
bounded spike qualifies it on both supported targets. The spike covers:

- full and cached `caching_sha2_password` authentication
- TLS certificate rejection
- prepared queries, cancellation and cleanup
- packaging

The build configuration is pinned along with the version. A failed spike returns
a concrete provider decision rather than an open search. A TypeRB-authored wire
client is reconsidered only after socket and TLS services exist and measured
dependency costs justify it. PostgreSQL is deferred.

### Package semantics

The compiler owns TypeRB-authored providers that follow the pinned reference
contracts. They sit at the shared semantic boundary, before Native-specific
lowering, so that one provider serves every mode. Runtime adapters are per mode:
external libraries and TypeRB runtime services for `mode: trb`, and each host
ecosystem's libraries for the source modes. Providers produce checked
declarations and package plans that feed verified MIR, and they preserve:

- file-route precedence and middleware composition
- schema-derived model members and typed queries
- job directives and configuration lifetime

Providers reuse the reference data boundaries and portable package source where
possible. Declarations, rejections and execution are compared with the pinned
reference.

### Shared implementation with the reference

The aim is one TypeRB implementation of the official packages, shared by the
reference and Native, so that package behavior is maintained once.

The reference already bundles portable TypeRB sources for the official packages:
roughly 1,600 lines, including the `trb/web` core, `trb/http` and the web
middleware. The remaining behavior is compiler-integrated Go: roughly 9,000
lines of semantics plus per-backend code generation. The reference keeps these
bundled packages behind a manifest boundary so that they can move to an external
package source.

- Native consumes the reference's portable package sources unchanged at the
  pinned revision. It never keeps a fork.
- When behavior can be expressed in portable TypeRB, moving it from compiler
  integration into the shared package source is preferred over porting it into
  Native. The reference benefits directly because it removes per-backend
  duplication across Go, Ruby and TypeScript.
- Native providers are written in portable TypeRB along the reference's
  protocol boundaries: declarations, project declaration input, generated
  source and the runtime adapter protocol. They can then move into the
  reference if the reference adopts TypeRB-authored providers.
- Shared conformance cases, meaning TypeRB programs with expected outcomes,
  belong with the shared sources and run in both implementations.
- Reference changes follow that repository's own workflow. They must be
  justified by reference semantics and maintenance, without Native terminology,
  and each upstream proposal needs a maintainer decision.

These rules hold for every provider:

- Ordinary builds never invoke the Go reference.
- Builds from a schema lock stay offline.
- Unsupported package behavior fails explicitly.
- A project has one configuration, and its mode selects the target. `mode: trb`
  builds Native executables; `go`, `ruby` and `typescript` select source
  output. Native accepts all four modes. Until it emits a source mode, commands
  that need that backend fail explicitly, and a project is never built for a
  mode other than its configured one. The reference accepts `mode: trb` for
  analysis, formatting, linting, editor support and TypeRB package installation,
  and rejects commands that would build it. Package manifests that omit `modes`
  support every mode, including `trb`. Application source stays unchanged
  across modes.
- No Native-only syntax or MIR package API is introduced.

### Milestones

| Milestone | Scope | First measure |
| --- | --- | --- |
| M1 | Project configuration, explicit mode selection, local path packages and the reference's bundled official package sources | Frozen configuration and import matrix, including invalid options, mode selection and conflicting identities |
| M2 | `trbn test` and `trb/std/test` | Discovery, assertions, failure origins and exit status match the reference |
| M3a | Bytes, JSON and time services | Service conformance cases |
| M3b | Blocking sockets, deadlines and deterministic resource cleanup | Service cases; no leaked owned handles |
| M3c | Process supervision, signals, job lease and heartbeat lifecycle | Lifecycle cases, including shutdown |
| M4 | `trb/web` | Independent synthetic route suite: routing, middleware, malformed requests, limits and shutdown |
| M5 | `trb/orm` on SQLite | Locked-schema checking, CRUD, nullable and time values, transactions and rollback; no database needed to build |
| M6 | `trb/orm` on MySQL | The M5 suite plus authentication, TLS, disconnect, cancellation and contention |
| M7 | Durable `trb/jobs` and the `trb/http` client | Tutorial end to end; no lost acknowledged enqueue across registered crash and restart cases |
| M8 | Production qualification | Matched-load soak, throughput, p99 latency, memory, build time and size against Pure Go and TypeRB-generated Go controls |

Each milestone's first task names an owner. It also names a manifest of at
most 20 initial scenarios, taken from existing public tests and reviewed before
activation. The manifest records exact commands, expected outcomes and
dependencies. Passing it establishes that slice, not full package parity.
Broader conformance stays in the backlog.

A missing baseline is recorded as not runnable, never as zero. M8 freezes its
targets before activation. Supported-target, compatibility and
security-maintenance policies remain separate release gates.

### Scheduling

- **M1 through M3** may proceed alongside basic-language completion
  ([#454](https://github.com/type-rb/type-rb-native/issues/454)) and MIR
  consolidation ([#439](https://github.com/type-rb/type-rb-native/issues/439)).
- **M4 through M7** may start during #439. Each package execution slice first
  requires:
  1. Its required operations, call effects and cleanup and root lifetimes have
     verified MIR ownership.
  2. Frozen same-source check, build and execution cases pass.
  3. The applicable lifetime, fixed-point, recovery and target checks pass.
- **M8** requires #439's milestone performance assessment. That assessment
  remains mandatory for closing #439.
- **Active slots.** This path holds at most one Active initiative by default.
  Web and the SQLite ORM may run as two Active initiatives only when two slots
  are free.

## Consequences

- One heap per process costs memory relative to Go's shared runtime. M8 measures
  that cost against matched controls.
- A dynamic database library adds a deployment prerequisite and a license to
  track (LGPL-2.1).
- Providers may drift from the reference until they are shared. Pinned
  differential cases limit the drift, and shared sources remove it. Shared
  protocol and source changes go through the reference repository's own
  workflow.
- The tutorial is an integration target. It cannot qualify web alone.

Revisit this decision when any of these happens:

- the blocking baseline misses its frozen targets;
- the connector spike fails;
- the reference publishes a portable provider implementation with adequate
  conformance evidence.
