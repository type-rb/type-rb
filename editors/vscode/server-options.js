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

function resolveStandaloneDebugBuildOptions(settings, workspaceRoot, outfile) {
	const command = resolveFromWorkspace(String(settings.path ?? "").trim() || "trb", workspaceRoot);
	const file = resolveFileFromWorkspace(String(settings.file ?? "").trim(), workspaceRoot);
	if (file === "") {
		throw new TypeError("A standalone TypeRB file is required for a debug build");
	}
	return {
		command,
		args: ["build", "--compile", "--debug", "--outfile", outfile, file]
	};
}

function appendStandaloneOptions(args, settings) {
	const mode = String(settings.mode ?? "go").trim() || "go";
	args.push("--mode", mode);
	if (mode === "typescript") {
		const runtime = String(settings.runtime ?? "node").trim() || "node";
		args.push("--runtime", runtime);
	}
}

function runCodeLensTitle(running) {
	return running ? "↻ Restart" : "▶ Run";
}

module.exports = { resolveRunOptions, resolveServerOptions, resolveStandaloneDebugBuildOptions, runCodeLensTitle };
