# TypeRB in JetBrains IDEs

JetBrains IDEs can use TypeRB without a dedicated language plugin by combining
the repository's canonical TextMate grammar with the LSP4IJ plugin. This setup
targets one configured TypeRB project per JetBrains project.

## Requirements

- a current IntelliJ-based IDE supported by LSP4IJ
- `trb` available on `PATH`
- a project with `trbconfig.jsonc` at its root

Install TypeRB first if necessary:

```sh
brew install type-rb/tap/trb
trb version
```

## Add syntax highlighting

Download and extract the TypeRB source archive for the installed release. In
the IDE, open **Settings | Editor | TextMate Bundles**, choose **Add**, and
select the extracted `editors/vscode` directory. JetBrains IDEs recognize its
VS Code `package.json` and load the packaged canonical TypeRB grammar.
If the TextMate Bundles page is unavailable, enable the bundled **TextMate
Bundles** plugin under **Settings | Plugins | Installed**.

## Add language services

1. Open **Settings | Plugins | Marketplace**, install **LSP4IJ**, and restart
   the IDE if prompted.
2. Open the directory containing `trbconfig.jsonc` as the JetBrains project.
3. Open **Settings | Languages & Frameworks | Language Servers** and add a
   user-defined language server.
4. On the **Server** tab, use `TypeRB` as the name and enter:

   ```text
   trb lsp --config $PROJECT_DIR$/trbconfig.jsonc
   ```

5. On the **Mappings** tab, add a **File name pattern** mapping for `*.trb`
   and set its language ID to `trb`.
6. Apply the settings and reopen a `.trb` file.

If an IDE launched from the desktop cannot find `trb`, run `command -v trb`
in a terminal and replace `trb` in the command with that absolute path.

LSP4IJ exposes compiler-backed diagnostics, completion, hover information,
navigation, rename, semantic highlighting, formatting, and quick fixes through
the IDE's ordinary actions. Its LSP console shows server startup and protocol
errors when troubleshooting is necessary.

## Scope

The initial integration does not support config-free files, multiple
`trbconfig.jsonc` projects in one IDE project, TypeRB run or test CodeLens
commands, the native Test Runner, or source debugging. Open nested TypeRB
projects in separate IDE windows. The integration does not implement an
IntelliJ lexer, parser, PSI model, formatter, or semantic analysis.
