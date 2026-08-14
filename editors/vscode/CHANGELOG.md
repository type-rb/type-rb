# Changelog

## Unreleased

- Add explicit standard-type imports when accepting compiler completion.
- Debug Go-targeted TypeRB source with breakpoints, stepping, call stacks, and
  variable inspection through Delve.

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
