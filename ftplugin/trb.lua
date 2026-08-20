if vim.b.did_ftplugin then
	return
end
vim.b.did_ftplugin = true

vim.bo.commentstring = "# %s"
vim.bo.expandtab = false
vim.bo.shiftwidth = 0
vim.bo.softtabstop = 0

vim.b.undo_ftplugin = "setlocal commentstring< expandtab< shiftwidth< softtabstop<"

if not vim.lsp.is_enabled("typerb") then
	return
end

local bufnr = vim.api.nvim_get_current_buf()
local filename = vim.fs.normalize(vim.api.nvim_buf_get_name(bufnr))
if filename == "" or vim.fs.root(filename, { "trbconfig.jsonc" }) then
	return
end

-- A config-free buffer is an independent file-root entry. Imported buffers are
-- intentionally not attached to this client.
local root_dir = vim.fs.dirname(filename)
local client_id = vim.lsp.start({
	name = "typerb-standalone",
	cmd = { "trb", "lsp", filename },
	cmd_cwd = root_dir,
	root_dir = root_dir,
}, {
	bufnr = bufnr,
	reuse_client = function(client, config)
		return client.name == config.name
			and not client:is_stopped()
			and vim.deep_equal(client.config.cmd, config.cmd)
	end,
})

if not client_id then
	return
end

vim.api.nvim_create_autocmd("LspDetach", {
	buffer = bufnr,
	desc = "Stop the detached TypeRB standalone language server",
	callback = function(args)
		if not args.data or args.data.client_id ~= client_id then
			return false
		end
		vim.schedule(function()
			local client = vim.lsp.get_client_by_id(client_id)
			if client and not client:is_stopped() and next(client.attached_buffers) == nil then
				client:stop(client.exit_timeout)
			end
		end)
		return true
	end,
})
