# Editor Support

TypeRB keeps editor integrations thin by sharing the compiler's language
services through `trb lsp`. Install TypeRB and make `trb` available on `PATH`
before configuring an editor.

| Editor | Integration | Setup and scope |
| --- | --- | --- |
| Visual Studio Code | Official extension with language services, run and debug support, and test integration | [Visual Studio Code guide](../editors/vscode/README.md) |
| Neovim | Official plugin using Neovim's native LSP client, with format on save by default | [Neovim guide](../editors/neovim/README.md) |
| JetBrains IDEs | Canonical TextMate grammar combined with LSP4IJ | [JetBrains guide](../editors/jetbrains/README.md) |

The editor-specific guides are the authoritative source for installation,
configuration, supported features, limitations, and updates.

## Portable syntax highlighting

TypeRB also publishes a canonical TextMate grammar for compatible editors and
documentation tools. It provides immediate lexical highlighting while
compiler-backed integrations provide semantic behavior. See the
[TextMate grammar documentation](../syntaxes/README.md) for metadata,
accuracy, and maintenance details.
