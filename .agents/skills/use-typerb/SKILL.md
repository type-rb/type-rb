---
name: use-typerb
description: Build, debug, explain, or teach TypeRB applications. Compiler changes use develop-typerb.
---

# Use TypeRB

For application edits, work from the nearest `trbconfig.jsonc` and existing
source. Use a scratch Go-mode REPL when an executable example is useful and no
project exists; explanation-only requests do not require creating a project.

## Find the authoritative API

Consult the documentation for the question at hand; these are entry points,
not a reading list required before every task:

- Read `docs/language.md` for implemented syntax and
  `docs/specification.md` for exact semantics.
- Read `docs/standard-library.md` for portable APIs and the relevant file in
  `docs/guides/` for Web, ORM, Jobs, React, browser HTTP, authentication, or
  packages.
- Read `docs/cli.md` and `docs/configuration.md` for commands and project
  fields.
- Search declarations or implementation only when the public documentation is
  incomplete. Report the documentation gap instead of inventing an API.

Keep these application invariants:

- Use canonical, case-sensitive TypeRB types such as `Integer`, `Boolean`, and
  `String`; do not substitute target-language aliases.
- Keep source grammar portable across modes. Import target APIs explicitly
  through `trb/platform/<mode>/*`.
- Keep ordinary imports explicit. Do not rely on REPL-only hidden imports in
  project source.
- Treat recoverable failure as `Result<T, E>`. Use prefix `try` only inside a
  compatible Result-returning function, postfix `catch` for local recovery or
  early control transfer, and exhaustive `case` when both variants are data to
  inspect. Do not discard a standard Result value.
- Keep `case`/`when` for the current value and enum-payload matching surface.
  Do not invent `case`/`in`, `match`, or implicit payload accessors.
- Use `condition ? value : alternative` only for a short two-value choice.
  Use `return`, `next`, or `break` with trailing `if` for a simple conditional
  transfer; do not invent a general modifier `if` or `unless`.
- Edit `.trb` source and `trbconfig.jsonc`, not generated target files or a
  managed native manifest.

## Use the compiler feedback loop

For source changes, format the affected files and run
`trb check --diagnostic-format json`. Use diagnostic codes, spans, related
locations, and fixes rather than parsing message text alone. Run the narrowest application
test or command that exercises the change; build or run the executable when
that boundary is affected.

Use `trb lint --diagnostic-format json` for requested lint work or the project's
checks, applying only fixes declared safe. Use `--deny-warnings` when required
by project policy. Retain required CI and expand validation for failures or
unresolved concerns; an unchanged successful check need not be repeated.

Inside a TypeRB compiler checkout, use `./trb` so the source compiler validates
the current language. In an application repository, use the installed `trb`.
Never weaken types or add native escape syntax merely to silence a diagnostic.

## Teaching requests

For an interactive lesson, use [teaching guidance](references/teaching.md).
For a focused question, answer it directly and link the relevant rule.
