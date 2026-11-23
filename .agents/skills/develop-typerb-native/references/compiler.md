# Compiler, MIR, and target changes

Use for source loading, checking/lowering, MIR, runtime, driver, or target work.
Paths below are repository-relative. Use `docs/architecture.md` for the boundary
being changed; unrelated documentation does not require an architecture review.

- Keep ordinary compiler input file- or project-oriented once that boundary is
  available. Source-content argv adapters must be explicitly hidden, limited
  to recovery or differential tests, and kept distinct from ordinary command
  shapes. Test stdout, stderr, exit status, unreadable input, and inputs beyond
  conservative argv limits.
- For file-root compilation, follow the pinned reference compiler's explicit
  import closure: root imports at the entry directory, load only reachable
  declaration imports, retain canonical module and declaration identity, and
  require `main` from the entry module. Resolve a unique directory `index`
  through either equivalent authored path, but reject a resolved graph that
  contains both `name.trb` and `name/index.trb`; never restore direct-file
  precedence between two loaded identities. Test named and bare aliases,
  declaration identity, the ASCII root-key rule, unrelated invalid siblings,
  diamonds, cycles, duplicate and unused bindings, missing exports, path
  escape, optional suffixes, and paths containing spaces. Package, namespace
  and public CLI expansion must preserve the reference contracts; an internal
  source-loading shortcut must not silently define those user-facing semantics.
- Allow external code generators, assemblers, linkers, SDKs, and system
  libraries only behind explicit boundaries whose time and distribution cost
  can be measured.
- For every target addition, use an internal versioned profile and keep the
  frontend, Native MIR semantics, runtime behavior, and backend IL shared.
  Record the QBE/backend target, ABI, linker policy, external dependencies,
  executable format and imports, deterministic-output policy, and recovery
  provenance. Reject unknown profiles before source or tool access, and keep
  all Native target and experimental terminology out of the reference repository.
- When the Native compiler owns external-tool orchestration, execute explicit
  tool paths directly rather than assembling a shell command. Preserve child
  diagnostics, decode child completion deterministically, publish output only
  after every phase succeeds, and remove intermediates after success and every
  failure. Test paths containing spaces, existing-output replacement, each
  phase failure, and that compiler diagnostics launch no external tool.
- Preserve source origins and exact TypeRB semantics through every lowering.
- Use the reference-derived language contract in `docs/native-language-coverage.md`
  to expose accepted, rejected and untested behavior across check/build/run/REPL.
  Keep its generated Capabilities view synchronized; snapshot support alone
  does not complete an ordinary language feature.
- Keep semantic analysis above backend emission. Represent proven Integer
  ranges, index properties, loop structure, call effects, Array-header
  stability, and GC safety as verified Native MIR facts or analysis results;
  target-independent passes consume those facts, and backend adapters consume
  the resulting MIR. Do not add new non-trivial source-pattern analysis to the
  QBE emitter. An already registered narrow emitter experiment may be completed
  as migration evidence, but follow-on generalization belongs in MIR.
- Follow the active MIR consolidation milestone: complete coherent families of
  typed operations and control flow, verify them, and move their semantic owners
  above QBE adaptation. Remove superseded direct-emitter paths with their consumers.
  Detailed performance qualification follows the coherent milestone; correctness,
  ordinary fixed points, recovery and lifetime safety remain required throughout.
- Treat the portable Integer range and its failure classes as correctness
  constraints. Backend optimization may inline or outline checks under a
  deterministic code-size policy, but it must not substitute machine-word
  overflow or omit division and range failures to meet a measurement bound.
- Reject unknown, malformed, unsupported, or unverifiable input with stable,
  deterministic diagnostics. Never add a semantic fallback or `Any` escape
  hatch to improve a benchmark.
- Keep bootstrap snapshots, Native MIR, ABI profiles, and runtime interfaces
  internal and unstable until a decision explicitly promotes them.
- Implement cohesive feature and ownership families within the active milestone,
  including dependencies needed for full language and package coverage. Record a
  decision for changes to language semantics, architecture, self-hosting criteria
  or backend selection policy; implementing an already accepted goal does not
  require reopening whether production use is in scope.
- Remove duplicate and superseded direct-emitter ownership as MIR slices migrate.
  Useful verified optimization code need not be cost-free: propose its retained
  cost explicitly. Before another fact family, account for outstanding migration
  debt and cumulative cost; do not add a second semantic owner or broaden LLVM
  work merely because an investigation budget exists.

Do not add LLVM merely to continue local optimization work. First cover scalar,
Array, allocation, and I/O behavior through the shared MIR and benchmark corpus;
then use a bounded LLVM adapter as an optimization-ceiling comparison unless a
different measured backend question has become more important.
