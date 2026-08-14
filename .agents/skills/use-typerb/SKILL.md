---
name: use-typerb
description: Build, modify, debug, explain, or teach TypeRB applications. Use for .trb source, trbconfig.jsonc projects, TypeRB package APIs, compiler diagnostics encountered by application authors, and interactive TypeRB learning. Do not use for changing the TypeRB compiler or language semantics; use develop-typerb for those tasks.
---

# Use TypeRB

Work from the nearest `trbconfig.jsonc`, or use a scratch Go-mode REPL when no
project exists. Inspect existing source and configuration before proposing a
new layout.

## Find the authoritative API

Use documentation as an index rather than guessing from Go, Ruby, or
TypeScript:

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
- Edit `.trb` source and `trbconfig.jsonc`, not generated target files or a
  managed native manifest.

## Use the compiler feedback loop

After each coherent edit:

1. Run `trb fmt` on the affected source.
2. Run `trb check --diagnostic-format json` and use diagnostic codes, spans,
   related locations, and fixes rather than parsing message text alone.
3. Run the narrowest relevant application command, then `trb run` or
   `trb build` when the project has an executable boundary.

Inside a TypeRB compiler checkout, use `./trb` so the source compiler validates
the current language. In an application repository, use the installed `trb`.
Never weaken types or add native escape syntax merely to silence a diagnostic.

## Teach interactively

When the user asks to learn TypeRB:

1. Establish the intended path: language basics, API/backend, database, Jobs,
   or browser UI.
2. Start with [A Tour of TypeRB](https://type-rb.github.io/tour/) or one small
   local `.trb` exercise that runs immediately.
3. Introduce one new concept at a time and let the learner write or modify the
   code before showing a complete answer unless they request it.
4. Validate the learner's actual code with `trb fmt` and `trb check`; explain
   the TypeRB rule behind each diagnostic.
5. End each step with one observable result and one suggested next exercise.

Prefer executable feedback over a long lecture. Link the relevant reference
section when the learner wants the full rule.
