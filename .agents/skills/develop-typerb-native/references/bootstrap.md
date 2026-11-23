# Compiler recovery and bootstrap changes

Read for changes to ordinary compiler source (including moves), recovery,
self-hosting, or seed distribution. Paths below are repository-relative.
Use `docs/compiler-project-layout.md` when source paths or cross-revision
consumers move. Use `docs/bootstrap-seed-updates.md` for a seed refresh or before
compiler self-use adopts syntax beyond the verified checkout seed.

- For self-hosting checks, require runtime-supplied source to pass through the
  checked-in lexer, parser, resolver, checker, and emitter. Reject embedded
  compiler artifacts, source-specific output paths, quines, and hidden host
  fallbacks as bootstrap evidence.
- Treat the canonical compiler as a real file-root closure. Keep extracted
  declarations in one TypeRB source only, require explicit imports, and run
  every ordinary replacement generation from the entry path. A temporary
  flattened equivalent is recovery-only: derive it deterministically from the
  canonical modules, verify and record its inputs, never commit it, and never
  substitute it for an ordinary self-hosted build.
- Record B0, B1, and B2 roles explicitly, plus B3 when a fixed-point check is
  required. Verify the ordinary regeneration process graph rather than
  inferring Go independence from the output binary. Keep recovery compilers and
  measurement orchestrators out of the ordinary semantic chain, and inspect
  executable imports and external-tool subprocesses when recording the graph.
- When claiming Native bootstrap closure, use each produced compiler as the
  executable seed of the following full build. Record the initial seed
  provenance separately, compare same-basename output bytes across repeated
  generations, and never count recovery seed creation as part of the ordinary
  Go-free chain.
- When distributing a development bootstrap seed, keep compiler binaries out
  of Git history. Record one-time root provenance separately, publish raw
  target compilers with a strict versioned SHA-256 manifest and artifact
  attestations, make the completed release immutable, and verify the actual
  published assets from fresh target runners before calling the handoff
  durable. Later ordinary chains start only from a previous Native compiler;
  they must not quietly recreate recovery through Go or the reference
  compiler. Do not infer stable version, compatibility, installation, signing,
  or support promises from a bootstrap seed tag alone. Supported release
  distribution is a separate goal with explicit compatibility and security policies.

## Required compiler-source verification

Compiler-source integration requires the recovery and QBE-backed tests in CI.
During development, use focused local proof and ordinary Native regeneration;
do not duplicate the full recovery suite for every intermediate language edit.
Keep compiler sources unchanged during each multi-generation regeneration; a
mid-build edit makes the fixed-point comparison meaningless. In a fresh task
worktree, point `TRBN_QBE` and `TRBN_BOOTSTRAP_SEED` at already verified local
assets when available so the fast loop does not wait on network downloads.
For CLI tests against a temporary `trbn` binary, put a verified `qbe` beside
that binary (a symlink is sufficient). Several test scripts invoke the adjacent
executable directly, even when `TRBN_QBE` is set. Shared language-case runs
also need `TRBN_QBE` when the binary has no adjacent QBE.
When compiler-module imports change, or a module is added, renamed or removed,
run `python3 tools/recovery_layout_sync.py --write` and `--check`. It keeps the
recovery layout, the per-module recovery mutations and the own-frontend module
list in `compiler/src/tests/compiler/source_loading_test.trb` equal to the import closure of
`compiler/src/compiler.trb`, and generates a default mutation for a new module
or a stale needle. Review generated mutations, then run the focused
`Compiler recovery source closure` root test before pushing.
Run `tools/check-bootstrap-snapshot.sh /path/to/pinned/trb` on the canonical
compiler closure before publishing each compiler-source batch. It checks
snapshot-v4 compatibility and the byte, source, type and function bounds read
from the recovery decoder; it does not replace recovery-enabled suites. If a
larger compiler snapshot reaches a bound, measure the affected size or count
and retain exact-limit tests when adjusting the compiler-only limit. Keep the
ordinary snapshot limits separate.
When introducing compiler-source constructs or data structures, run this short
check before expensive unit suites and ordinary regeneration. Repeat it after
compatibility repairs so a successful ordinary build does not postpone discovery
of recovery-subset restrictions.
Bootstrap and recovery-execution changes run the complete lanes before merge
through the planner's `complete` output; full local recovery is for diagnosing
recovery or platform failures, including a red `Main validation`. For a full run,
set `TYPE_RB_NATIVE_REFERENCE_TRB` to the absolute pinned compiler executable
and `TYPE_RB_NATIVE_QBE` to QBE 1.3, together with `TYPE_RB_NATIVE_ROOT`, when
running the root and `compiler` suites. Without those variables, optional
tests report success without exercising recovery; do not count that as complete
bootstrap verification. The recovery snapshot supports a narrower source subset
than the ordinary Native compiler, so ordinary fixed points alone are not a
substitute. See the environment in `.github/workflows/native-validation.yml`.

Treat a published bootstrap tag, release, and asset set as immutable. Keep a
release-integrity re-verification pinned to the release source and fixtures,
and record the exact verifier revision separately. For a later source
compatibility revalidation, verify that immutable seed and its provenance
before execution, then record the newer compiler-source revision and resulting
fixed-point identity separately from the seed's source-era root and compiler
identity. Never replace or relabel the seed merely to align revisions.
