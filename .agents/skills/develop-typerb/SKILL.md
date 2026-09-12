---
name: develop-typerb
description: Implement or review changes to TypeRB language behavior, compiler, backends, or packages.
---

# Develop TypeRB

Read the relevant sections of `docs/specification.md` and the affected
implementation when changing behavior. Use this skill for compiler work, not
for an unrelated application or documentation edit.

## Replace alpha behavior directly

TypeRB is in alpha until the public project documentation explicitly says
otherwise. Treat no existing syntax, semantic rule, standard-library or
official-package API, tooling surface, generated form, or compiler-owned
protocol as settled. Evaluate the desired target design without backward-
compatibility or migration constraints.

Do not preserve replaced behavior with deprecated forms, compatibility aliases,
shims, dual paths, transitional diagnostics, or staged migrations. Remove the
old form and update all affected first-party code, tests, examples, documents,
and protocol fixtures in the same change. Repository consistency is required;
support for the previous alpha behavior is not.

Preserve these invariants:

- Keep one grammar and portable semantics across every mode. Let `mode` select only the backend, toolchain, and package ecosystem.
- For new syntax and APIs, identify one canonical user-facing spelling. Avoid
  synonymous aliases and overlapping overloads unless they have a distinct
  semantic role or concrete application evidence. When alternatives are
  necessary, document which spelling public examples should prefer.
- Gate target-specific APIs and native compatibility behind an explicit `trb/platform/<mode>/*` import. Never relax checking merely because a project uses that mode.
- Carry behavior through syntax AST, checked types, typed IR, and each affected backend; do not bypass the pipeline with source-text rewrites.
- Keep `trb fmt` deterministic and preserve comments.
- Keep `trb check` non-suppressible and correctness-only. Built-in `trb lint`
  rules are optional maintainability policy, use stable rule IDs, and require
  one dedicated documentation page per rule.
- Treat compiler-owned package declarations as the source of external types; do not require application authors to maintain signature files.
- Keep recoverable failure on the compiler-owned `Result<T, E>` model. Prefix
  `try` propagates compatible `Err` values, postfix `catch` handles them
  locally, and exhaustive `case` remains the general inspection form. Do not
  reintroduce a second checked-effect channel or map native exceptions and
  Promise rejections implicitly outside an explicit Result bridge.

For new APIs or compiler integration, choose the narrowest sufficient
[package boundary](references/package-boundaries.md).

Test intended behavior rather than incidental representation:

- Prefer compiler and CLI integration tests for portable semantics and diagnostics across modes.
- Add focused package unit tests when a boundary needs faster or more precise feedback.
- For a behavior-preserving refactor, retain existing coverage and add
  characterization tests where an affected contract has an uncovered risk.
  For an intentional alpha redesign, replace old expectations with coverage of
  the selected contract.
  Avoid full AST, IR, or generated-file snapshots unless that exact
  representation is the contract.

## Validate the affected contract

Add positive and diagnostic coverage for changed behavior; use Ruby, Go, and
TypeScript where the feature is portable. Exercise the typed-IR REPL when runtime
expression semantics change. Cover Result success, propagation, recovery, and
must-use diagnostics where a changed block or native boundary affects them.

Use focused tests during implementation. Before completing compiler/runtime
changes, run `GOCACHE=/tmp/type-rb-go-cache go test ./...` and
`./trb fmt --check .`, and satisfy required CI. Use the checkout launcher so an
older installed formatter cannot validate current syntax. Documentation-only
work needs the checks relevant to its changed text plus required CI, not a new
set of compiler tests. Reuse passing results for the unchanged candidate rather
than repeating them without a new concern.

Update specification, status, or roadmap only where behavior or status changed.
Use the toolchain declared in `go.mod` and the supported target versions; legacy
target compatibility is outside the project scope. Complete the requested work
unit without treating it as permission for a new feature, merge, or release.
