"use strict";

const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");
const { resolveServerOptions } = require("./server-options");

let client;

async function activate(context) {
	const settings = vscode.workspace.getConfiguration("typerb");
	const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
	const workspaceRoot = workspaceFolder?.uri.fsPath;
	const server = resolveServerOptions(
		{
			path: settings.get("server.path", "trb"),
			config: settings.get("server.config", "")
		},
		workspaceRoot
	);

	client = new LanguageClient(
		"typerb",
		"TypeRB Language Server",
		{
			command: server.command,
			args: server.args,
			options: workspaceRoot === undefined ? {} : { cwd: workspaceRoot }
		},
		{
			documentSelector: [{ scheme: "file", language: "trb" }]
		}
	);

	context.subscriptions.push({
		dispose() {
			void stopClient();
		}
	});
	await client.start();
}

async function stopClient() {
	if (client === undefined) {
		return;
	}
	const activeClient = client;
	client = undefined;
	await activeClient.stop();
}

async function deactivate() {
	await stopClient();
}

module.exports = { activate, deactivate };
