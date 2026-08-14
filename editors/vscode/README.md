# TypeRB for Visual Studio Code

Language support for [TypeRB](https://github.com/type-rb/type-rb).

> TypeRB and this extension are in preview. Language and tooling behavior may
> change before the first stable release.

## Features

- Syntax highlighting and declaration snippets
- Project-wide diagnostics and quick fixes
- Checked completion for declarations, members, and call arguments
- Hover information and signature help
- Go to definition, references, symbol rename, and project-wide symbol search
- Checked symbol occurrence highlighting within the current document
- Syntax-aware expanding and shrinking selections
- Document outline for types, fields, functions, and methods
- Structural code folding for declarations and expression blocks
- Compiler-aware semantic highlighting
- Deterministic, comment-preserving document formatting
- Run and restart a project's top-level `main()` from the editor

The extension discovers every `trbconfig.jsonc` in the opened workspace and
keeps each project in an independent language-server session. A repository may
therefore contain Go, Ruby, and TypeScript applications with overlapping type
names. Project diagnostics and symbols update when TypeRB files are created,
changed, or deleted outside the active editor.

## Requirements

Install the TypeRB compiler and make `trb` available on `PATH`:

```sh
brew install type-rb/tap/trb
trb version
```

## Getting started

Open a folder containing `trbconfig.jsonc`, then open or create a `.trb` file.
The extension starts the TypeRB language server automatically.

Select **▶ Run** above a top-level `main()` to save the project's TypeRB files
and start `trb run` in an integrated terminal. The action changes to
**↻ Restart** while the project is running; selecting it stops the active
process before starting it again. Use **TypeRB: Stop Project** from the Command
Palette to stop it without restarting.

Create a small Go-targeted project from a terminal with:

```sh
trb init --mode go --module example.com/hello hello
code hello
```

See [A Tour of TypeRB](https://type-rb.github.io/tour/) for an introduction to
the language.

## Configuration

- `typerb.server.path` selects the `trb` executable. The default is `trb` from
  `PATH`; relative paths are resolved from the workspace folder.
- `typerb.server.config` adds an explicit `trbconfig.jsonc` when automatic
  workspace discovery is not sufficient. Relative paths are resolved from the
  workspace folder.

If Visual Studio Code cannot find a Homebrew installation, set
`typerb.server.path` to the result of `command -v trb`.

## Documentation

Read the [TypeRB documentation](https://github.com/type-rb/type-rb/tree/main/docs)
for the language, CLI, packages, and target guides.
