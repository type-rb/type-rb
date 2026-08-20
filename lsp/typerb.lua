return {
	cmd = function(dispatchers, config)
		-- TypeRB discovers trbconfig.jsonc from the language server's working directory.
		return vim.lsp.rpc.start({ "trb", "lsp" }, dispatchers, {
			cwd = config.root_dir,
		})
	end,
	filetypes = { "trb" },
	root_markers = { "trbconfig.jsonc" },
	workspace_required = true,
}
