if vim.b.did_ftplugin then
	return
end
vim.b.did_ftplugin = true

vim.bo.commentstring = "# %s"
vim.bo.expandtab = false
vim.bo.shiftwidth = 0
vim.bo.softtabstop = 0

vim.b.undo_ftplugin = "setlocal commentstring< expandtab< shiftwidth< softtabstop<"
