# TypeRB for Neovim

The TypeRB repository is a minimal Neovim plugin for configured projects. It
registers `.trb` files, applies basic buffer options, and provides a native
Neovim configuration for `trb lsp`. Parsing, semantic highlighting,
diagnostics, completion, navigation, rename, and formatting remain owned by
the TypeRB compiler.

## Requirements

- Neovim 0.12 or newer
- `trb` available on `PATH`
- a project containing `trbconfig.jsonc`

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

vim.lsp.enable("typerb")
```

With `lazy.nvim`:

```lua
{
	"type-rb/type-rb",
	lazy = false,
	config = function()
		vim.lsp.enable("typerb")
	end,
}
```

Open a `.trb` file below the project configuration. Neovim starts one client
for the nearest `trbconfig.jsonc` root and reuses it for other files in that
project.

Check activation with:

```vim
:set filetype?
:checkhealth vim.lsp
```

The expected file type is `trb`, and the health report should list the
`typerb` client as attached. Neovim's standard LSP mappings provide hover,
navigation, references, and rename. Invoke completion with `<C-x><C-o>` and
format the current buffer with:

```vim
:lua vim.lsp.buf.format()
```

## Scope

The initial plugin intentionally does not include a Vim syntax file, a
Tree-sitter parser, snippets, run or test commands, debugging, or support for
config-free files. Semantic tokens and formatting come from `trb lsp`, so
language changes do not require a parallel Neovim grammar.

Config-free sessions are deferred because each entry needs its own language
server and imported helper buffers must be routed to the correct entry. The
plugin avoids a partial implementation that would analyze saved imports but
miss unsaved helper edits.
