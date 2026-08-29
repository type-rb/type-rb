# TypeRB TextMate Grammar

This directory contains TypeRB's canonical TextMate grammar for editors and
documentation tools that support TextMate scopes.

## Metadata

- Language ID: `trb`
- Scope name: `source.trb`
- File extension: `.trb`
- Manifest: [`manifest.json`](manifest.json)
- Grammar: [`typerb.tmLanguage.json`](typerb.tmLanguage.json)

The grammar uses conventional scopes for comments, strings, numbers,
declarations, types, constants, variables, methods, operators, and
punctuation. Consumers may use the manifest as the stable entry point instead
of duplicating this metadata.

## Accuracy

TextMate highlighting is a lexical approximation. The compiler lexer, parser,
and type checker remain authoritative. Compiler-aware clients such as the
Visual Studio Code extension combine this grammar's immediate highlighting
with semantic tokens from `trb lsp`.

Target-specific packages do not change the portable TypeRB grammar. They
expose target capabilities through declaration imports or explicit top-level
`activate` statements.

## Maintenance

When portable syntax changes, update the grammar together with representative
fixtures and scope assertions in [`tools/textmate/test`](../tools/textmate/test).
Then mirror the grammar into the
[Visual Studio Code package](../editors/vscode/syntaxes/typerb.tmLanguage.json),
which is also the bundle imported by JetBrains IDEs, and rebuild the
[GitHub Chrome extension](../editors/github/README.md). The editor tests reject
stale generated copies, while the GitHub extension bundles the canonical
grammar directly at build time. Neovim consumes compiler semantic tokens
instead of carrying a second lexical grammar.

Run these checks from the repository root:

```sh
npm ci --prefix tools/textmate
npm test --prefix tools/textmate
npm ci --prefix editors/vscode
npm test --prefix editors/vscode
npm ci --prefix editors/github
npm run build --prefix editors/github
npm test --prefix editors/github
```
