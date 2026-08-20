# Editor Support and Syntax Highlighting

## Visual Studio Code

The repository ships a thin Visual Studio Code extension in
[`editors/vscode`](../editors/vscode). It registers `.trb` files, packages the
canonical TextMate grammar and snippets, and starts `trb lsp` from `PATH`.
Diagnostics, completion with explicit import insertion, hover information,
signature help, definition and
reference navigation, symbol rename and search, occurrence highlighting,
document outlines, folding, expanding selections, semantic highlighting,
formatting, quick fixes, and runnable `main()` locations therefore use the same
compiler and language services as the CLI instead of editor-specific language
logic. Project symbols and diagnostics also follow `.trb` files created,
changed, or deleted outside the active editor. The client discovers nested
`trbconfig.jsonc` files and gives each project an independent language-server
session, including repositories that contain API and frontend applications in
different target modes. A config-free `.trb` entry receives a file-root session
that follows its explicit local imports and shares unsaved helper buffers
without compiling unrelated siblings. The VS Code client owns the Debug
Adapter Protocol run, restart, and Debug Console lifecycle.

Install [TypeRB from the Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=type-rb.typerb),
or run:

```sh
code --install-extension type-rb.typerb
```

To test an unpublished extension change, build and install a local VSIX with:

```sh
npm ci --prefix editors/vscode
npm run package --prefix editors/vscode
code --install-extension editors/vscode/dist/typerb.vsix
```

Set `typerb.server.path` when `trb` is not on `PATH`. Automatic discovery uses
ordinary `trbconfig.jsonc` files. Set `typerb.server.config` only to add a
configuration that discovery cannot find; relative paths are resolved from the
workspace folder. Go projects and config-free Go entries may use TypeRB source
breakpoints, stepping, stack frames, and variables through Delve;
`typerb.debug.go.path` selects its executable when `dlv` is not on `PATH`.
Each standalone debug session builds a private temporary executable and removes
it after the session ends.

## Neovim

The repository is also a minimal Neovim 0.12 plugin for configured TypeRB
projects and config-free files. Install it as an ordinary Neovim plugin; no
`setup()` or manual LSP enable call is required. With the built-in package
manager, add this to `init.vim`:

```vim
lua vim.pack.add({ { src = 'https://github.com/type-rb/type-rb' } }, { load = true })
```

The equivalent `init.lua` configuration is:

```lua
vim.pack.add({
	{ src = "https://github.com/type-rb/type-rb" },
}, { load = true })
```

The plugin registers `.trb`, uses the nearest `trbconfig.jsonc` as the project
root, and starts `trb lsp` from `PATH`. Without a project configuration it
starts `trb lsp FILE.trb` for that file, using Go mode by default. It
deliberately relies on compiler semantic tokens and formatting instead of
carrying a separate Vim syntax, Tree-sitter, indentation, or formatting
implementation. Formatting on save is enabled by default and can be disabled
explicitly. Unsaved imported-buffer routing, run and test commands, and
debugging are outside its initial scope. See the
[Neovim guide](../editors/neovim/README.md) for optional settings, other plugin
managers, updates, verification, and ordinary editor commands.

## JetBrains IDEs

JetBrains users can combine the canonical TextMate grammar with LSP4IJ without
a TypeRB-specific IDE plugin. Import `editors/vscode` under **Settings | Editor
| TextMate Bundles**, then register `trb lsp --config
$PROJECT_DIR$/trbconfig.jsonc` as a user-defined language server mapped to
`*.trb` with language ID `trb`.

This initial setup supports one configured TypeRB project per JetBrains
project. Config-free files, nested TypeRB projects in one IDE project, run and
test CodeLens commands, Test Runner integration, and debugging are deferred.
See the [JetBrains guide](../editors/jetbrains/README.md) for the complete setup
and troubleshooting steps.

## Portable grammar

TypeRB publishes a portable TextMate grammar for editors and documentation
tools that support TextMate scopes.

## Grammar metadata

- Language ID: `trb`
- Scope name: `source.trb`
- File extension: `.trb`
- Manifest: [`syntaxes/manifest.json`](../syntaxes/manifest.json)
- Grammar: [`syntaxes/typerb.tmLanguage.json`](../syntaxes/typerb.tmLanguage.json)

The grammar uses conventional scopes for comments, strings, numbers,
declarations, types, constants, variables, methods, operators, and punctuation.
Consumers may use the manifest as the stable entry point instead of duplicating
this metadata.

## Accuracy

TextMate highlighting is a lexical approximation. The compiler lexer, parser,
and type checker remain authoritative. The REPL, playground, language server,
and editor clients use compiler-aware language services when semantic context
is available. Visual Studio Code combines the TextMate grammar with semantic
tokens from `trb lsp`; the lexical grammar still provides immediate colors
while the server starts.

Target-specific packages do not change the portable TypeRB grammar. They expose
target capabilities through explicit imports.

## Maintenance

When portable syntax changes, update the grammar together with representative
fixtures and scope assertions in `tools/textmate/test`, then mirror it into the
VS Code package consumed by both VS Code and JetBrains IDEs. The extension test
rejects a stale mirror. Neovim consumes compiler semantic tokens instead of a
second lexical grammar. Verify the grammar and packaged consumer with:

```sh
npm ci --prefix tools/textmate
npm test --prefix tools/textmate
npm ci --prefix editors/vscode
npm test --prefix editors/vscode
```
