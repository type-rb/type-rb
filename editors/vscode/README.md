# TypeRB for Visual Studio Code

Language support for [TypeRB](https://github.com/type-rb/type-rb).

> TypeRB and this extension are in preview. Language and tooling behavior may
> change before the first stable release.

## Features

- Syntax highlighting and declaration snippets
- Project-wide diagnostics and quick fixes
- Checked completion for declarations, members, and call arguments
- Explicit import insertion for compiler-owned standard types such as `Result`
- Hover information and signature help
- Go to definition, references, symbol rename, and project-wide symbol search
- Checked symbol occurrence highlighting within the current document
- Syntax-aware expanding and shrinking selections
- Document outline for types, fields, functions, and methods
- Structural code folding for declarations and expression blocks
- Compiler-aware semantic highlighting
- Deterministic, comment-preserving document formatting
- Run, stop, and restart a project or standalone file's top-level `main()`
  through Visual Studio Code's Run and Debug interface
- Source breakpoints, stepping, stack frames, and variable inspection for
  `mode: go`
- Native Test Explorer discovery and execution for `*_test.trb` suites
- Test debugging with TypeRB breakpoints and variables for `mode: go`

The extension discovers every `trbconfig.jsonc` in the opened workspace and
keeps each project in an independent language-server session. A repository may
therefore contain Go, Ruby, and TypeScript applications with overlapping type
names. Project diagnostics and symbols update when TypeRB files are created,
changed, or deleted outside the active editor.

An open `.trb` file that is not owned by a discovered project receives its own
single-file language-server session. No `trbconfig.jsonc` is created, sibling
files are not compiled, and Go is the default target mode.

## Requirements

Use Visual Studio Code 1.130 or newer.

Install the TypeRB compiler and make `trb` available on `PATH`:

```sh
brew install type-rb/tap/trb
trb version
```

## Getting started

Open or create a `.trb` file. The extension starts a standalone TypeRB language
server automatically. Opening a folder containing `trbconfig.jsonc` enables
project-wide analysis instead.

Select **▶ Run** above a top-level `main()` to run without debugging. Startup,
program output, and exit status appear in the Debug Console. Use Visual Studio
Code's standard Restart and Stop controls, or press `Shift+F5` to stop the
process. **TypeRB: Stop Project** remains available from the Command Palette.

For a `mode: go` project, install [Delve](https://github.com/go-delve/delve),
set breakpoints in `.trb` files, and press `F5` or select **Debug TypeRB** in
Run and Debug. TypeRB emits Go debug information that refers directly to the
original TypeRB source, so Visual Studio Code's standard stepping, call stack,
variables, watches, and debug console are available. Ruby and TypeScript
projects currently support Run Without Debugging while their source-debugger
adapters are developed.

Open the Testing view to run every TypeRB test, a nested `describe` suite, or
one `test` case. The extension also shows **▶ Run Test** above individual test
declarations. For Go projects, choose the Testing view's **Debug Test** action
or the CodeLens action to stop at `.trb` breakpoints and inspect variables.
Failures link to the original assertion in the `.trb` source. The project must
use a process runtime; TypeScript browser test hosting is not yet available.

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
- `typerb.standalone.mode` selects `go`, `ruby`, or `typescript` for files
  outside discovered projects. The default is `go`.
- `typerb.standalone.typescript.runtime` selects `node` or `bun` for a
  standalone TypeScript-mode file. The default is `node`.
- `typerb.debug.go.path` selects the Delve executable for Go source debugging.
  The default is `dlv` from `PATH`.

Run and Debug selects the active TypeRB project automatically. A `launch.json`
configuration may set `config`, `args`, and `env` when explicit control is
needed:

```json
{
	"type": "typerb",
	"request": "launch",
	"name": "Debug API",
	"config": "${workspaceFolder}/apps/api/trbconfig.jsonc",
	"args": ["serve"],
	"env": {
		"PORT": "4000"
	}
}
```

If Visual Studio Code cannot find a Homebrew installation, set
`typerb.server.path` to the result of `command -v trb`.

## Local development

The integration suite builds the extension and compiler from the current
checkout, then starts an isolated Extension Development Host. It does not
install or update the Marketplace extension.

```sh
npm ci --prefix editors/vscode
npm test --prefix editors/vscode
npm run test:integration --prefix editors/vscode
npm run test:integration:insiders --prefix editors/vscode
npm run package --prefix editors/vscode
```

The Stable and Insiders suites cover standalone language service activation,
unsaved-file diagnostics, and execution through the real debug adapter
lifecycle.

## Documentation

Read the [TypeRB documentation](https://github.com/type-rb/type-rb/tree/main/docs)
for the language, CLI, packages, and target guides.
