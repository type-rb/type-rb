# MIR consolidation milestone

Status: active development direction under [issue #439](https://github.com/type-rb/type-rb-native/issues/439).
This policy supersedes per-slice cost acceptance during the migration phase;
historical decisions and measurements retain their original meanings.

## Release destination

The accepted [v0.5.0 transition contract](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md) makes this implementation the
canonical compiler in `type-rb` only after it covers the complete reviewed Go
feature contract. The Go implementation moves to `type-rb-go`, retaining three
source backends and matching release versions; Native adds executable output.
[Initiative #1164](https://github.com/type-rb/type-rb-native/issues/1164) joins the
language, package, CLI, source-backend and migration evidence. It does not
change this milestone's criteria or establish current support. Prioritize
usable application paths and previews within the complete release scope.
Archive this repository only after verified migration and release handoff.

## Long-term objective and current scope

The long-term goal is a TypeRB-authored compiler/toolchain covering the reference
language, Go/Ruby/TypeScript emission and execution, and Native executable
output. Coverage includes the standard library and official packages as their
platform prerequisites are implemented. The reference Go implementation already
provides the three source backends; parity in the TypeRB-authored implementation
is future work. Reference semantics remain independently owned and Native must
not acquire its own dialect.

Production use is the intended destination. This includes official application
packages such as `trb/web`, `trb/orm` and `trb/jobs`, a stable user-facing CLI,
documented ABI compatibility where exposed, supported release targets and a
release/security maintenance policy. Incomplete coverage, internal unstable
interfaces and exact development pins describe the current implementation;
they do not exclude these goals. Support and compatibility claims require the
corresponding implementation, validation and published policies.

Native execution time, generated application size and end-to-end application
build time should outperform equivalent Pure Go programs across representative
workloads. Compare equivalent inputs, outputs, algorithms, concurrency, build
settings and toolchain versions. These are workload-specific measured goals,
not a promise that every possible program or metric already wins. Keep Pure Go
controls separate from TypeRB's generated Go output and from compiler self-build
cost. Include assembly/linking in application builds and account for required
external tools in distribution costs.

Current work covers basic ordinary language features, Native output, verified
MIR, source organization and the optimizations already needed by those programs.
The first consolidation inventory includes scalar expressions, calls, records,
Strings, Arrays, Hash, Range, conditionals, loops and lexical transfers.
[The basic-language contract](native-language-coverage.md), tracked in
[issue #454](https://github.com/type-rb/type-rb-native/issues/454), broadens that
inventory from pinned reference syntax and semantic probes. Complete useful
families together with their MIR dependencies; do not wait for all existing MIR
migration before closing basic language gaps. The shared tests generate the
public Capabilities
detail view. A completed MIR milestone is not 100% TypeRB language coverage.

Keep checked-program type, origin and high-level semantic information available
for the later source backends. Native-specific layout, ABI and root lowering
belong after that shared semantic boundary. Basic syntax proceeds with MIR
consolidation. Three-language backend parity, wider standard-library and
official-package coverage follow their dependencies; completing one category
list does not establish those capabilities. Official web, ORM and jobs packages
follow the gated milestones in
[Decision 0086](decisions/0086-production-web-application-path.md); package
slices may start before this milestone's performance assessment only through
those gates.

## Completion conditions

- Every supported ordinary function follows the verified MIR route. Checked
  operations, traps, origins, values,
  control edges and lexical ownership remain explicit.
- Portable range/index/loop relationships, allocation and mutation effects,
  Array-header stability and root-safety decisions have shared MIR owners.
  Backend adaptation consumes those decisions. The existing optimizations can
  be represented without rediscovering semantics from source/QBE text.
- Superseded ordinary paths and duplicated semantic analysis are removed.
  Retained implementation names describe responsibilities; oversized modules are resolved with their consumers. Independently
  required recovery/snapshot adapters and immutable historical evidence remain.
- Differential behavior, diagnostics, GC safety, REPL execution, ordinary
  self-hosting/fixed points, recovery and supported-target correctness pass.
- A milestone performance assessment records application runtime/build/size,
  compiler build/size/RSS and toolchain distribution. It identifies and pursues
  performance recovery before claiming qualification or refreshing formal Pages.

## Current implementation

Every accepted ordinary function requires a verified MIR body. The direct body
emitter is retired; numeric selection, managed roots and container operations
have MIR owners. The [ownership status](native-mir-optimization-status.md)
describes the current representation and remaining optimization work. The
[reference-derived inventory](native-language-feature-inventory.md) and
[Capabilities](capabilities/) record language coverage and known gaps.

Earlier feature checkpoints, acceptance counts and implementation histories
remain in the [immutable milestone history](https://github.com/type-rb/type-rb-native/blob/f864151fa53a99a9491ca0268198d5ec914124fd/docs/mir-consolidation.md#current-integration-sole-mir-emission).

## Development loop

Use cohesive changes that remove an ownership boundary or complete a useful
family of operations. Within a PR, run focused units, differential cases, ordinary
fixed points and relevant recovery checks as the implementation changes. Run
complete hosted correctness at integration. Keep commits reviewable and preserve
bisectability, but do not require a separate PR and full performance assessment
for each helper extraction, rename or small syntax adoption.

PR and main integration explicitly select `mir-migration` cost mode. They retain
all applicable correctness, target identity, process, recovery, fixed-point,
ASan/LSan, Valgrind, GC-lifetime and cleanup authorities. Failed or missing required
jobs still reject integration. Compiler size is recorded without a ceiling,
and smoke-time ceilings produce retained `within-limit` or `exceeded`
observations; an overrun cannot stop those correctness checks. Invalid modes or malformed observations
are errors. Existing workload timeouts and memory-lifecycle checks remain.

Migration PRs omit the separate interleaved arm64 cost matrix. Linux
amd64 integration retains its compiler generations, binary-format/dependency
checks, application outputs and failure-order controls but omits its repeated
formal timing series. CI workflow/routing and cost-policy changes retain their
synthetic controller checks and all applicable target/recovery/runtime correctness,
without restoring old performance ceilings as a naming-cleanup prerequisite.
Standalone tools/workflows
default to `strict`; their original thresholds and full measurement series are
available and failures remain visible. No admin bypass or blanket failed-job
allowance is used. Green migration CI proves integration correctness, not
performance qualification or a passing cost comparison that did not run.

Existing daily/weekly measurement schedules remain the trend feedback loop.
Their compiler preparation also observes historical byte overruns so that
diagnostic workloads can run; measured programs and repetitions are unchanged.
Use their evidence to detect large regressions and choose investigations, without
repeatedly tuning an unchanged small overrun. A coherent architecture checkpoint
or an unexpected material change warrants a focused measurement; each small
commit does not need a new numeric allowance or investigation registration.
A concrete crash, wrong result or unsafe lifetime is still fixed immediately.

## Baselines and milestone assessment

Freeze the migration-start Native source at
`d568d9e712452f66fc9c48d6025bf73b79659661`. Preserve the earlier cumulative
baseline `ac633935a7f248470c59d22666da14c819a131fa`, existing daily/formal controls,
exact toolchain identities and all previous failed cohorts. They answer different
questions and must not be reset to hide growth. Record feature differences when
comparing compiler self-builds; use same-feature/identical-source controls where
needed. Unsupported new programs cannot be attributed to the old compiler.

At consolidation completion, run the full comparative cost and representative
runtime suites on a frozen candidate and compare with both the pre-migration
Native and equivalent Pure Go controls. Retain all observations and report which
metrics regress or win. Restore or improve existing application performance and
pursue the Pure Go goals with optimizations on the consolidated MIR. Historical
compiler-byte ceilings are reference observations, not an obligation to erase
all cost of useful language coverage. Review the complete trade-off at the
milestone and establish the subsequent acceptance contract explicitly.

The migration policy expires when issue #439's structural conditions and
performance assessment are complete. Closing it requires updating integration
routing and the documented next-phase contract; this mode must not silently
become permanent release qualification. Stable releases and final benchmark
claims cannot rely on migration CI alone. Formal Pages changes require complete
accepted formal measurements; no old result is rewritten as a new observation.
