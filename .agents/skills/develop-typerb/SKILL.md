---
name: develop-typerb
description: Evolve the TypeRB language and toolchain. Use when changing TypeRB syntax or semantics, AST/typed IR, checker rules, formatter behavior, standard or platform packages, Ruby/Go/TypeScript backends, or REPL behavior.
---

# Develop TypeRB

Read `docs/specification.md` and the relevant implementation before changing
behavior.

Preserve these invariants:

- Keep one grammar and portable semantics across every mode. Let `mode` select only the backend, toolchain, and package ecosystem.
- For new syntax and APIs, identify one canonical user-facing spelling. Avoid
  synonymous aliases and overlapping overloads unless they have a distinct
  semantic role or concrete application evidence. When alternatives are
  necessary, document which spelling public examples should prefer.
- Gate target-specific APIs and native compatibility behind an explicit `trb/platform/<mode>/*` import. Never relax checking merely because a project uses that mode.
- Carry behavior through syntax AST, checked types, typed IR, and each affected backend; do not bypass the pipeline with source-text rewrites.
- Keep `trb fmt` deterministic and preserve comments.
- Treat compiler-owned package declarations as the source of external types; do not require application authors to maintain signature files.
- Keep recoverable failure on the compiler-owned `Result<T, E>` model. Prefix
  `try` propagates compatible `Err` values, postfix `catch` handles them
  locally, and exhaustive `case` remains the general inspection form. Do not
  reintroduce a second checked-effect channel or map native exceptions and
  Promise rejections implicitly outside an explicit Result bridge.

Choose the narrowest package boundary that can implement a feature:

1. Use an ordinary TypeRB package by default.
2. Use an explicit platform package for backend- or ecosystem-specific
   behavior.
3. Add an API to `trb/std/*` only when it is foundational, portable, broadly
   applicable, and stable enough to version with the compiler.
4. Treat bundling as a distribution decision. It never grants compiler
   privileges or makes a dependency implicit.
5. Use compiler integration only when TypeRB source, native dependencies, and
   the current extension protocol cannot express the required behavior. Do not
   choose it merely because a package is official or convenient to ship.
6. When compiler integration is unavoidable, document the missing extension
   capability and preserve a path toward an ordinary package.

Test stable behavior rather than incidental representation:

- Prefer compiler and CLI integration tests for portable semantics and diagnostics across modes.
- Add focused package unit tests when a boundary needs faster or more precise feedback.
- Before refactoring, add characterization coverage for the behavior being preserved. Avoid full AST, IR, or generated-file snapshots unless that exact representation is the contract.

For each coherent change:

1. Add positive and diagnostic tests, covering Ruby, Go, and TypeScript when the feature is portable.
2. Exercise the typed-IR REPL when runtime expression semantics change.
3. Exercise Result success, propagation, recovery, and must-use diagnostics
   across every affected structured block and native boundary.
4. Run `GOCACHE=/tmp/type-rb-go-cache go test ./...` and
   `./trb fmt --check .`. Use the source-checkout launcher so an older released
   `trb` on `PATH` cannot validate current syntax with a stale formatter.
5. Update `docs/specification.md`, `docs/status.md`, or `docs/roadmap.md` only
   where behavior or status changed.
6. Commit the completed work unit. Use Go 1.26 and current target language features; legacy target versions are out of scope.
