if vim.g.loaded_typerb_nvim then
	return
end
vim.g.loaded_typerb_nvim = true

local auto_start = vim.g.typerb_auto_start
if auto_start ~= false and auto_start ~= 0 then
	vim.lsp.enable("typerb")
end

local function is_typerb(client)
	return client.name == "typerb" or client.name == "typerb-standalone"
end

local group = vim.api.nvim_create_augroup("TypeRBNvim", { clear = true })
vim.api.nvim_create_autocmd("BufWritePre", {
	group = group,
	pattern = "*.trb",
	desc = "Format TypeRB before saving",
	callback = function(args)
		local format_on_save = vim.g.typerb_format_on_save
		if format_on_save == false or format_on_save == 0 then
			return
		end

		local available = false
		for _, client in ipairs(vim.lsp.get_clients({ bufnr = args.buf })) do
			if is_typerb(client) and client:supports_method("textDocument/formatting", args.buf) then
				available = true
				break
			end
		end
		if not available then
			return
		end

		local ok, err = pcall(vim.lsp.buf.format, {
			bufnr = args.buf,
			async = false,
			timeout_ms = 2000,
			filter = is_typerb,
		})
		if not ok then
			pcall(vim.notify, "TypeRB format on save failed: " .. tostring(err), vim.log.levels.WARN)
		end
	end,
})
