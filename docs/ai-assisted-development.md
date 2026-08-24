# AI-assisted TypeRB development

AI agents can work effectively with TypeRB before the language appears in
general model training data. The reliable boundary is the checked project and
its machine-readable diagnostics, not the model's memory of Ruby, Go, or
TypeScript.

## Repository setup

Keep `AGENTS.md`, `.agents/skills`, TypeRB source, and `trbconfig.jsonc` in the
repository presented to the agent. This repository provides the `use-typerb`
skill for application work. Claude-compatible setups can expose the same skill
directory through `.claude/skills`.

For another TypeRB repository, copy or install an equivalent concise skill and
point it at the documentation for the compiler version used by that project.
Do not copy the complete language specification into every prompt.

## Source-of-truth order

Give the agent this lookup order:

1. existing project source and `trbconfig.jsonc`;
2. the relevant TypeRB guide or reference;
3. `docs/specification.md` for exact language semantics; and
4. package declarations or compiler implementation only when public
   documentation is incomplete.

This keeps application code aligned with the selected TypeRB version and makes
missing documentation visible instead of hiding it behind a guessed API.

## Machine feedback loop

Use the formatter before checking so later locations match canonical source:

```sh
trb fmt
trb check --diagnostic-format json
trb lint --diagnostic-format json
```

The JSON report contains a schema version, stable `TRBxxxx` diagnostic codes,
source spans, related locations, and atomic fixes when available. Agents should
consume those fields directly. Diagnostic message wording may improve without
changing the code. Lint reports additionally contain `toolVersion`, and use the
documented rule ID as the diagnostic code.

After the project checks, run the narrowest executable boundary that proves the
change: a focused command, `trb run`, or `trb build`. Never repair generated Go,
Ruby, or TypeScript directly; change the `.trb` source or report a compiler
defect.

## Useful prompts

For application work:

```text
Use $use-typerb. Inspect this project's configuration and existing modules,
implement the requested feature in TypeRB, then run the formatter and JSON
diagnostics. Do not invent APIs or edit generated target files.
```

For guided learning:

```text
Use $use-typerb as an interactive tutor. Give me one executable exercise at a
time, let me attempt it, and explain the compiler diagnostics before showing a
complete solution.
```

For a compiler or language change, use the repository's `develop-typerb` skill
instead. It carries the additional AST, checker, typed IR, backend, formatter,
and conformance requirements that application work should not load by default.

## Current limits

The compiler service and LSP already expose structured diagnostics,
completion, hover, signatures, navigation, rename, formatting, and quick fixes.
Incremental compilation, a language-level test runner, runtime source maps, and
an agent-specific protocol beyond LSP and JSON diagnostics remain roadmap work.
The current workflow deliberately reuses compiler-owned interfaces so those
future tools do not create a second semantic implementation.
