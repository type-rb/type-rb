"use strict";

const path = require("node:path");

function resolveFromWorkspace(value, workspaceRoot) {
	if (value === "" || workspaceRoot === undefined || path.isAbsolute(value)) {
		return value;
	}
	if (!value.includes("/") && !value.includes("\\")) {
		return value;
	}
	return path.resolve(workspaceRoot, value);
}

function resolveServerOptions(settings, workspaceRoot) {
	const command = resolveFromWorkspace(settings.path.trim() || "trb", workspaceRoot);
	const config = resolveFromWorkspace(settings.config.trim(), workspaceRoot);
	const args = ["lsp"];
	if (config !== "") {
		args.push("--config", config);
	}
	return { command, args };
}

function resolveRunOptions(settings, workspaceRoot, programArgs = []) {
	const command = resolveFromWorkspace(settings.path.trim() || "trb", workspaceRoot);
	const config = resolveFromWorkspace(settings.config.trim(), workspaceRoot);
	const args = ["run"];
	if (config !== "") {
		args.push("--config", config);
	}
	if (programArgs.length > 0) {
		args.push("--", ...programArgs);
	}
	return { command, args };
}

function runCodeLensTitle(running) {
	return running ? "↻ Restart" : "▶ Run";
}

module.exports = { resolveRunOptions, resolveServerOptions, runCodeLensTitle };
