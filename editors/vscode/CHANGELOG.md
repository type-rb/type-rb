# Changelog

## Unreleased

- Enable the breakpoint gutter for TypeRB source files and verify that
  standalone Go debugging stops on the selected `.trb` line.
- Navigate from JSX component uses directly to imported TypeRB declarations.

## 0.3.0 - 2026-08-19

- Require TypeRB 0.3.0 and support its Result-only control flow with prefix
  `try`, postfix `catch`, Result-aware hover and symbol details, and structured
  catch folding.
- Remove completion, highlighting, symbol, and rename treatment for the
  removed `fails` and `attempt` syntax.

## 0.2.7 - 2026-08-18

- Request TypeRB completions while identifiers are typed without requiring a
  trigger character to be retyped.
- Work with TypeRB 0.2.31 or newer to offer project and standard-library
  imports from completion and the Quick Fix menu, clear stale diagnostics
  immediately, and recalculate project diagnostics faster.

## 0.2.6 - 2026-08-18

- Preserve nested project roots when registering language clients, restoring
  format-on-save and correct language-feature routing from monorepo workspace
  roots.
- Work with TypeRB 0.2.30 or newer to keep completion and formatting
  responsive while project diagnostics are recalculated.

## 0.2.5 - 2026-08-18

- Follow explicit local imports in standalone language-server sessions and
  keep imported diagnostics synchronized with unsaved editor changes.
- Debug Go-mode standalone files and their imported helpers with session-local
  executables that are removed when debugging ends.

## 0.2.4 - 2026-08-17

- Complete members through instantiated generic results, transparent aliases,
  and union-backed native package contracts.
- Navigate from interface types and methods to their concrete class
  implementations.
- Reduce initial language-service readiness for large monorepos by indexing
  project contexts once and avoiding recompilation when unchanged files open.
- Verify completion, auto-import edits, formatting, and navigation in nested
  monorepo projects through Extension Host integration tests.
- Clear prior Debug Console output before restarting a running TypeRB project.
- Require VS Code 1.130 or newer and verify local development builds in Stable
  and Insiders Extension Development Hosts.
- Analyze and run standalone `.trb` files without requiring
  `trbconfig.jsonc`, with configurable Go, Ruby, or TypeScript mode.
- Add explicit standard-type imports when accepting compiler completion.
- Debug Go-targeted TypeRB source with breakpoints, stepping, call stacks, and
  variable inspection through Delve.
- Discover and run portable TypeRB suites through Visual Studio Code Test
  Explorer and test CodeLens actions.
- Debug Go-targeted tests with TypeRB breakpoints, stepping, and variables.

## 0.2.3 - 2026-08-15

- Run TypeRB projects through Visual Studio Code's standard Run and Debug
  lifecycle, including native Stop and Restart controls.
- Show the launch command, process identifier, program output, and exit status
  in the Debug Console from session startup.

## 0.2.2 - 2026-08-15

- Run a project's top-level `main()` from the editor. The CodeLens changes from
  `▶ Run` to `↻ Restart` while its project is running.
- Keep nested TypeRB projects in independent language-server sessions, so API
  and frontend applications may use overlapping declaration names.

## 0.2.1 - 2026-08-14

- Add compiler-aware semantic highlighting, document outlines, and project-wide
  symbol search.
- Add structural folding, checked occurrence highlights, and syntax-aware
  selection ranges.
- Track TypeRB files created, changed, or deleted outside active editor buffers.
- Add quick fixes that replace noncanonical type names with their TypeRB names.
- Use incremental document synchronization with `trb lsp`.

## 0.2.0 - 2026-08-14

- Show checked hover information and call signatures from `trb lsp`.
- Navigate to project and lexical declarations with Go to Definition.
- Find checked references and rename project or lexical symbols.

## 0.1.0

- Add TypeRB syntax highlighting and starter snippets.
- Add compiler-backed diagnostics, completion, formatting, and quick fixes
  through `trb lsp`.
