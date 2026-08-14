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
- Document outline for types, fields, functions, and methods
- Structural code folding for declarations and expression blocks
- Compiler-aware semantic highlighting
- Deterministic, comment-preserving document formatting

The extension works with ordinary `.trb` files and uses the project's
`trbconfig.jsonc` to select the target mode and package configuration.

## Requirements

Install the TypeRB compiler and make `trb` available on `PATH`:

```sh
brew install type-rb/tap/trb
trb version
```

## Getting started

Open a folder containing `trbconfig.jsonc`, then open or create a `.trb` file.
The extension starts the TypeRB language server automatically.

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
- `typerb.server.config` selects a specific `trbconfig.jsonc`. Relative paths
  are resolved from the workspace folder.

If Visual Studio Code cannot find a Homebrew installation, set
`typerb.server.path` to the result of `command -v trb`.

## Documentation

Read the [TypeRB documentation](https://github.com/type-rb/type-rb/tree/main/docs)
for the language, CLI, packages, and target guides.
