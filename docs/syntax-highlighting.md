# Editor Support and Syntax Highlighting

## Visual Studio Code

The repository ships a thin Visual Studio Code extension in
[`editors/vscode`](../editors/vscode). It registers `.trb` files, packages the
canonical TextMate grammar and snippets, and starts `trb lsp` from `PATH`.
Diagnostics, completion, hover information, signature help, definition and
reference navigation, symbol rename and search, occurrence highlighting,
document outlines, folding, expanding selections, semantic highlighting,
formatting, quick fixes, and runnable `main()` locations therefore use the same
compiler and language services as the CLI instead of editor-specific language
logic. Project symbols and diagnostics also follow `.trb` files created,
changed, or deleted outside the active editor. The client discovers nested
`trbconfig.jsonc` files and gives each project an independent language-server
session, including repositories that contain API and frontend applications in
different target modes. The VS Code client owns the integrated-terminal run
and restart lifecycle.

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
workspace folder.

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
VS Code package. The extension test rejects a stale mirror. Verify both
consumers with:

```sh
npm ci --prefix tools/textmate
npm test --prefix tools/textmate
npm ci --prefix editors/vscode
npm test --prefix editors/vscode
```
