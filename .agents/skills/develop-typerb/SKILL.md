---
name: develop-typerb
description: Evolve the TypeRB language and toolchain. Use when changing TypeRB syntax or semantics, AST/typed IR, checker rules, formatter behavior, standard or platform packages, Ruby/Go/TypeScript backends, REPL behavior, or TypeRB examples.
---

# Develop TypeRB

Read `SPEC.md` and the relevant implementation before changing behavior.

Preserve these invariants:

- Keep one grammar and portable semantics across every mode. Let `mode` select only the backend, toolchain, and package ecosystem.
- Gate target-specific APIs and native compatibility behind an explicit `trb/platform/<mode>/*` import. Never relax checking merely because a project uses that mode.
- Carry behavior through syntax AST, checked types, typed IR, and each affected backend; do not bypass the pipeline with source-text rewrites.
- Keep `trb fmt` deterministic and preserve comments.
- Treat compiler-owned package declarations as the source of external types; do not require application authors to maintain signature files.

For each coherent change:

1. Add positive and diagnostic tests, covering Ruby, Go, and TypeScript when the feature is portable.
2. Exercise the typed-IR REPL when runtime expression semantics change.
3. Run `GOCACHE=/tmp/type-rb-go-cache go test ./...` and `trb fmt --check .`.
4. Check affected examples with `trb build --check`.
5. Update `SPEC.md`, `STATUS.md`, or `ROADMAP.md` only where behavior or status changed.
6. Commit the completed work unit. Use Go 1.26 and current target language features; legacy target versions are out of scope.
