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

With Neovim's built-in package manager, add the following to `init.lua`:

```lua
vim.pack.add({
	{ src = "https://github.com/type-rb/type-rb" },
}, { load = true })
```

With `lazy.nvim`:

```lua
{
	"type-rb/type-rb",
	lazy = false,
}
```

The plugin enables its native `typerb` LSP configuration when it loads. Set
`vim.g.typerb_auto_start = false` before loading the plugin only when another
configuration should own TypeRB language-server startup.

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

To format `.trb` buffers before each save, enable the opt-in setting before or
after loading the plugin:

```lua
vim.g.typerb_format_on_save = true
```

Formatting is skipped when no TypeRB language server with formatting support
is attached or when the current source cannot be formatted.

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
