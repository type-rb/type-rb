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

function resolveFileFromWorkspace(value, workspaceRoot) {
	if (value === "" || workspaceRoot === undefined || path.isAbsolute(value)) {
		return value;
	}
	return path.resolve(workspaceRoot, value);
}

function resolveServerOptions(settings, workspaceRoot) {
	const command = resolveFromWorkspace(String(settings.path ?? "").trim() || "trb", workspaceRoot);
	const config = resolveFromWorkspace(String(settings.config ?? "").trim(), workspaceRoot);
	const file = resolveFileFromWorkspace(String(settings.file ?? "").trim(), workspaceRoot);
	const args = ["lsp"];
	if (config !== "") {
		args.push("--config", config);
	} else if (file !== "") {
		appendStandaloneOptions(args, settings);
		args.push(file);
	}
	return { command, args };
}

function resolveRunOptions(settings, workspaceRoot, programArgs = []) {
	const command = resolveFromWorkspace(String(settings.path ?? "").trim() || "trb", workspaceRoot);
	const config = resolveFromWorkspace(String(settings.config ?? "").trim(), workspaceRoot);
	const file = resolveFileFromWorkspace(String(settings.file ?? "").trim(), workspaceRoot);
	const args = ["run"];
	if (config !== "") {
		args.push("--config", config);
	} else if (file !== "") {
		appendStandaloneOptions(args, settings);
		args.push(file);
	}
	if (programArgs.length > 0) {
		args.push("--", ...programArgs);
	}
	return { command, args };
}

function resolveDebugBuildOptions(settings, workspaceRoot, outfile) {
	const command = resolveFromWorkspace(String(settings.path ?? "").trim() || "trb", workspaceRoot);
	const config = resolveFromWorkspace(String(settings.config ?? "").trim(), workspaceRoot);
	const file = resolveFileFromWorkspace(String(settings.file ?? "").trim(), workspaceRoot);
	const args = ["build"];
	if (config !== "") {
		args.push("--config", config);
	}
	args.push("--compile", "--debug", "--outfile", outfile);
	if (config === "" && file !== "") {
		appendStandaloneOptions(args, settings);
		args.push(file);
	}
	return { command, args };
}

function appendStandaloneOptions(args, settings) {
	const mode = String(settings.mode ?? "go").trim() || "go";
	args.push("--mode", mode);
	if (mode === "typescript") {
		const runtime = String(settings.runtime ?? "node").trim() || "node";
		args.push("--runtime", runtime);
	}
}

function runCodeLensTitle(running, standalone = false) {
	if (standalone) {
		return running ? "↻ Restart File" : "▶ Run File";
	}
	return running ? "↻ Restart" : "▶ Run";
}

module.exports = { resolveDebugBuildOptions, resolveRunOptions, resolveServerOptions, runCodeLensTitle };
