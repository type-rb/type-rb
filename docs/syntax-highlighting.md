# Syntax Highlighting

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
and type checker remain authoritative. The REPL, playground, and future LSP can
use compiler-aware language services when semantic context is available.

Target-specific packages do not change the portable TypeRB grammar. They expose
target capabilities through explicit imports.

## Maintenance

When portable syntax changes, update the grammar together with representative
fixtures and scope assertions in `tools/textmate/test`. Verify it through the
standard `vscode-textmate` consumer:

```sh
npm ci --prefix tools/textmate
npm test --prefix tools/textmate
```
