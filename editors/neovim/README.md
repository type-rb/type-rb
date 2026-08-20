# TypeRB for Neovim

The TypeRB repository is a minimal Neovim plugin for configured projects and
config-free files. It registers `.trb` files, applies basic buffer options,
and connects Neovim's native LSP client to `trb lsp`. Parsing, semantic
highlighting, diagnostics, completion, navigation, rename, and formatting
remain owned by the TypeRB compiler.

## Requirements

- Neovim 0.12 or newer
- `trb` available on `PATH`

Install TypeRB first if necessary:

```sh
brew install type-rb/tap/trb
trb version
```

## Install

Install this repository as an ordinary Neovim plugin. No `setup()` call or
manual `vim.lsp.enable()` call is required. Load the plugin during startup so
its `.trb` file detection is available.

### Built-in package manager

With Neovim 0.12's built-in `vim.pack`, use the example matching your
configuration file.

For `init.vim`:

```vim
lua vim.pack.add({ { src = 'https://github.com/type-rb/type-rb' } }, { load = true })
```

For `init.lua`:

```lua
vim.pack.add({
	{ src = "https://github.com/type-rb/type-rb" },
}, { load = true })
```

### Other plugin managers

Add `type-rb/type-rb` with an existing plugin manager and load it during
startup. For example, with `lazy.nvim`:

```lua
{
	"type-rb/type-rb",
	lazy = false,
}
```

Filetype-only lazy loading is not recommended because this plugin registers the
`.trb` filetype itself.

## Default behavior

The plugin enables its native `typerb` LSP configuration when it loads and
formats `.trb` buffers before saving by default. No additional configuration
is required.

The defaults can be disabled before loading the plugin:

| Behavior | `init.vim` | `init.lua` |
| --- | --- | --- |
| Disable format on save | `let g:typerb_format_on_save = v:false` | `vim.g.typerb_format_on_save = false` |
| Disable automatic LSP startup | `let g:typerb_auto_start = v:false` | `vim.g.typerb_auto_start = false` |

Disable automatic LSP startup only when another configuration should own the
TypeRB language-server lifecycle.

Open any `.trb` file. Below a `trbconfig.jsonc`, Neovim starts one `typerb`
client for the nearest project root and reuses it for other files in that
project. Without a project configuration, it starts an independent
`typerb-standalone` client with the equivalent of `trb lsp FILE.trb`. The
standalone session uses Go mode by default, matching the CLI.

Check activation with:

```vim
:set filetype?
:checkhealth vim.lsp
```

The expected file type is `trb`, and the health report should list either the
`typerb` or `typerb-standalone` client as attached. Neovim's standard LSP
mappings provide hover, navigation, references, and rename. Invoke completion
with `<C-x><C-o>` and format the current buffer with:

```vim
:lua vim.lsp.buf.format()
```

Formatting is skipped when no TypeRB language server with formatting support
is attached. If the formatter fails, Neovim reports a warning and continues
the save.

## Update

Update the TypeRB compiler installed by Homebrew with:

```sh
brew update
brew upgrade type-rb/tap/trb
```

For a plugin managed by `vim.pack`, run:

```vim
:lua vim.pack.update({ "type-rb" })
```

Review the proposed plugin update, write the confirmation buffer with
`:write`, and restart Neovim. Use the corresponding update command when the
plugin is owned by another manager.

## Scope

The initial plugin intentionally does not include a Vim syntax file, a
Tree-sitter parser, snippets, run or test commands, or debugging. Semantic
tokens and formatting come from `trb lsp`, so language changes do not require
a parallel Neovim grammar.

Each config-free file is treated as an independent entry. Its own open-buffer
edits receive diagnostics, semantic highlighting, completion, hover,
navigation, rename, and formatting. The language server can read explicit
local imports from disk, but the plugin does not route unsaved edits from a
separately opened imported buffer back to every entry that imports it. It also
does not arbitrate overlapping diagnostics when multiple standalone entries
import the same helper.
