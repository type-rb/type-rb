# TypeRB for Visual Studio Code

This extension provides TypeRB syntax highlighting, snippets, diagnostics,
completion, formatting, and quick fixes. Semantic features come from the
compiler-owned language server started as `trb lsp`.

Install `trb` and make it available on `PATH`. The extension discovers the
nearest project through the language server's ordinary `trbconfig.jsonc`
resolution. Use `typerb.server.path` or `typerb.server.config` when the default
process or project is not appropriate for the workspace.

## Local development

```sh
npm ci
npm test
npm run package
code --install-extension dist/typerb.vsix
```

Open this directory in Visual Studio Code and run the `Launch TypeRB Extension`
configuration to start an Extension Development Host without packaging.
