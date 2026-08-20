local plugin_path = vim.fs.joinpath(vim.fn.getcwd(), "plugin", "typerb.lua")

local clients = {}
local format_calls = 0
local format_impl = function() end
local notifications = {}

local original_enable = vim.lsp.enable
local original_get_clients = vim.lsp.get_clients
local original_format = vim.lsp.buf.format
local original_notify = vim.notify

vim.lsp.enable = function() end
vim.lsp.get_clients = function()
	return clients
end
vim.lsp.buf.format = function(opts)
	format_calls = format_calls + 1
	return format_impl(opts)
end
vim.notify = function(message, level)
	table.insert(notifications, { message = message, level = level })
end

local function assert_equal(actual, expected, message)
	if actual ~= expected then
		error(string.format("%s: expected %s, got %s", message, vim.inspect(expected), vim.inspect(actual)))
	end
end

local function assert_true(value, message)
	assert_equal(value, true, message)
end

local function assert_false(value, message)
	assert_equal(value, false, message)
end

local function client(name, supports_formatting)
	return {
		name = name,
		supports_method = function(_, method, bufnr)
			assert_equal(method, "textDocument/formatting", "formatting capability method")
			assert_true(type(bufnr) == "number", "formatting capability buffer")
			return supports_formatting
		end,
	}
end

local buffer = vim.api.nvim_create_buf(false, true)
vim.api.nvim_buf_set_name(buffer, vim.fn.tempname() .. ".trb")

local function load_plugin(format_on_save)
	pcall(vim.api.nvim_del_augroup_by_name, "TypeRBNvim")
	vim.g.loaded_typerb_nvim = nil
	vim.g.typerb_auto_start = false
	vim.g.typerb_format_on_save = format_on_save
	dofile(plugin_path)
end

local function run_write_pre()
	vim.api.nvim_exec_autocmds("BufWritePre", { buffer = buffer })
end

local function reset()
	clients = {}
	format_calls = 0
	format_impl = function() end
	notifications = {}
end

reset()
clients = { client("typerb", true), client("lua_ls", true) }
format_impl = function(opts)
	assert_equal(opts.bufnr, buffer, "format buffer")
	assert_false(opts.async, "synchronous format")
	assert_equal(opts.timeout_ms, 2000, "format timeout")
	assert_true(opts.filter(clients[1]), "TypeRB client filter")
	assert_false(opts.filter(clients[2]), "non-TypeRB client filter")
end
load_plugin(nil)
run_write_pre()
assert_equal(format_calls, 1, "formatting is enabled by default")

for _, disabled in ipairs({ false, 0 }) do
	reset()
	clients = { client("typerb", true) }
	load_plugin(disabled)
	run_write_pre()
	assert_equal(format_calls, 0, "explicit format-on-save opt-out")
end

reset()
clients = { client("lua_ls", true), client("typerb", false) }
load_plugin(nil)
run_write_pre()
assert_equal(format_calls, 0, "formatting requires a capable TypeRB client")

reset()
clients = { client("typerb-standalone", true) }
format_impl = function()
	error("formatter failed")
end
load_plugin(nil)
local ok, err = pcall(run_write_pre)
assert_true(ok, "formatting failure must not abort the write: " .. tostring(err))
assert_equal(format_calls, 1, "standalone TypeRB formatting attempt")
assert_equal(#notifications, 1, "formatting failure warning")
assert_true(notifications[1].message:find("formatter failed", 1, true) ~= nil, "formatting failure message")
assert_equal(notifications[1].level, vim.log.levels.WARN, "formatting failure level")

vim.lsp.enable = original_enable
vim.lsp.get_clients = original_get_clients
vim.lsp.buf.format = original_format
vim.notify = original_notify

print("Neovim plugin tests passed")
