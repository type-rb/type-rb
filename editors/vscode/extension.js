"use strict";

const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("./server-options");

let client;
let runTerminal;
let runCodeLensProvider;

async function activate(context) {
	const settings = vscode.workspace.getConfiguration("typerb");
	const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
	const workspaceRoot = workspaceFolder?.uri.fsPath;
	const fileWatcher = vscode.workspace.createFileSystemWatcher("**/*.trb");
	context.subscriptions.push(fileWatcher);
	context.subscriptions.push(
		vscode.commands.registerCommand("typerb.runProject", runProject),
		vscode.commands.registerCommand("typerb.stopProject", stopProject),
		vscode.window.onDidCloseTerminal((terminal) => {
			if (terminal === runTerminal) {
				runTerminal = undefined;
				runCodeLensProvider?.setRunning(undefined);
			}
		})
	);
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
			documentSelector: [{ scheme: "file", language: "trb" }],
			synchronize: { fileEvents: fileWatcher },
			middleware: {
				provideCodeLenses() {
					return [];
				}
			}
		}
	);

	context.subscriptions.push({
		dispose() {
			void stopClient();
		}
	});
	await client.start();
	runCodeLensProvider = new RunCodeLensProvider(client);
	context.subscriptions.push(
		runCodeLensProvider,
		vscode.languages.registerCodeLensProvider(
			[{ scheme: "file", language: "trb" }],
			runCodeLensProvider
		)
	);
}

class RunCodeLensProvider {
	constructor(languageClient) {
		this.client = languageClient;
		this.runningRoot = undefined;
		this.changed = new vscode.EventEmitter();
		this.onDidChangeCodeLenses = this.changed.event;
	}

	setRunning(workspaceRoot) {
		if (workspaceRoot === this.runningRoot) {
			return;
		}
		this.runningRoot = workspaceRoot;
		this.changed.fire();
	}

	async provideCodeLenses(document, token) {
		let items;
		try {
			items = await this.client.sendRequest(
				"textDocument/codeLens",
				{ textDocument: { uri: document.uri.toString() } },
				token
			);
		} catch {
			return [];
		}
		const workspaceRoot = vscode.workspace.getWorkspaceFolder(document.uri)?.uri.toString();
		const running = workspaceRoot !== undefined && workspaceRoot === this.runningRoot;
		return items.map((item) => {
			const range = new vscode.Range(
				item.range.start.line,
				item.range.start.character,
				item.range.end.line,
				item.range.end.character
			);
			if (item.command?.command !== "typerb.runProject") {
				return new vscode.CodeLens(range, item.command);
			}
			return new vscode.CodeLens(range, {
				title: runCodeLensTitle(running),
				command: item.command.command,
				arguments: item.command.arguments
			});
		});
	}

	dispose() {
		this.changed.dispose();
	}
}

async function runProject(uriValue) {
	const uri = runURI(uriValue);
	const workspaceFolder = uri === undefined
		? vscode.workspace.workspaceFolders?.[0]
		: vscode.workspace.getWorkspaceFolder(uri) ?? vscode.workspace.workspaceFolders?.[0];
	if (workspaceFolder === undefined) {
		void vscode.window.showErrorMessage("Open a TypeRB project folder before running main().");
		return;
	}
	if (!(await saveTypeRBDocuments(workspaceFolder))) {
		void vscode.window.showErrorMessage("Save TypeRB project files before running main().");
		return;
	}
	const settings = vscode.workspace.getConfiguration("typerb", uri);
	const run = resolveRunOptions(
		{
			path: settings.get("server.path", "trb"),
			config: settings.get("server.config", "")
		},
		workspaceFolder.uri.fsPath
	);
	await stopProject();
	runTerminal = vscode.window.createTerminal({
		name: "TypeRB: Run",
		cwd: workspaceFolder.uri.fsPath,
		shellPath: run.command,
		shellArgs: run.args,
		iconPath: new vscode.ThemeIcon("play")
	});
	runCodeLensProvider?.setRunning(workspaceFolder.uri.toString());
	runTerminal.show(true);
}

function runURI(value) {
	if (typeof value === "string" && value !== "") {
		return vscode.Uri.parse(value);
	}
	const editor = vscode.window.activeTextEditor;
	if (editor?.document.languageId === "trb") {
		return editor.document.uri;
	}
	return undefined;
}

async function saveTypeRBDocuments(workspaceFolder) {
	const documents = vscode.workspace.textDocuments.filter((document) =>
		document.languageId === "trb" &&
		document.isDirty &&
		vscode.workspace.getWorkspaceFolder(document.uri) === workspaceFolder
	);
	const saved = await Promise.all(documents.map((document) => document.save()));
	return saved.every(Boolean);
}

async function stopProject() {
	if (runTerminal === undefined) {
		return;
	}
	const terminal = runTerminal;
	runTerminal = undefined;
	runCodeLensProvider?.setRunning(undefined);
	await new Promise((resolve) => {
		let finished = false;
		const complete = () => {
			if (finished) {
				return;
			}
			finished = true;
			listener.dispose();
			clearTimeout(timeout);
			resolve();
		};
		const listener = vscode.window.onDidCloseTerminal((closed) => {
			if (closed === terminal) {
				complete();
			}
		});
		const timeout = setTimeout(complete, 1000);
		terminal.dispose();
	});
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
	await stopProject();
	await stopClient();
	runCodeLensProvider = undefined;
}

module.exports = { activate, deactivate };
