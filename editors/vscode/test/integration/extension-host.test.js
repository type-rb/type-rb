"use strict";

const assert = require("node:assert/strict");
const { readFile, rm } = require("node:fs/promises");
const path = require("node:path");
const vscode = require("vscode");

const originalSource = `def greet(name: String): String
\treturn "Hello, " + name
end

def main()
\tputs(greet("Extension Host"))
\treturn
end
`;

suite("TypeRB Extension Host", () => {
	let document;
	let extension;
	let sourceURI;
	let siblingURI;
	let workspaceRoot;

	suiteSetup(async () => {
		const folder = vscode.workspace.workspaceFolders?.[0];
		assert.ok(folder, "the standalone fixture workspace must be open");
		workspaceRoot = folder.uri.fsPath;
		sourceURI = vscode.Uri.file(path.join(workspaceRoot, "hello.trb"));
		siblingURI = vscode.Uri.file(path.join(workspaceRoot, "sibling.trb"));
		extension = vscode.extensions.getExtension("type-rb.typerb");
		assert.ok(extension, "the local TypeRB development extension must be available");
		const binary = path.join(
			extension.extensionPath,
			".vscode-test",
			"bin",
			process.platform === "win32" ? "trb.exe" : "trb"
		);
		await vscode.workspace.getConfiguration("typerb").update(
			"server.path",
			binary,
			vscode.ConfigurationTarget.Global
		);
		document = await vscode.workspace.openTextDocument(sourceURI);
		await vscode.window.showTextDocument(document);
		await extension.activate();
		await waitFor(async () => {
			const lenses = await vscode.commands.executeCommand("vscode.executeCodeLensProvider", sourceURI, 10);
			return lenses?.some((lens) => lens.command?.command === "typerb.runProject");
		}, "standalone Run CodeLens");
	});

	suiteTeardown(async () => {
		await vscode.workspace.getConfiguration("typerb").update(
			"server.path",
			undefined,
			vscode.ConfigurationTarget.Global
		);
		await vscode.commands.executeCommand("workbench.action.closeAllEditors");
	});

	test("activates the local extension and serves one standalone file", async () => {
		assert.equal(extension.isActive, true);
		assert.equal(document.languageId, "trb");
		const hovers = await vscode.commands.executeCommand(
			"vscode.executeHoverProvider",
			sourceURI,
			new vscode.Position(5, 7)
		);
		assert.ok(hovers?.length > 0, "the standalone LSP should provide checked hover information");
		assert.deepEqual(vscode.languages.getDiagnostics(siblingURI), [], "a sibling file must not enter the standalone session");
	});

	test("publishes and clears diagnostics for unsaved standalone edits", async () => {
		await replaceDocument(document, "def main()\n\tmissing()\n\treturn\nend\n");
		await waitFor(
			() => vscode.languages.getDiagnostics(sourceURI).some((item) => item.severity === vscode.DiagnosticSeverity.Error),
			"standalone diagnostics"
		);
		await replaceDocument(document, originalSource);
		await waitFor(() => vscode.languages.getDiagnostics(sourceURI).length === 0, "cleared standalone diagnostics");
	});

	test("routes language features to nested monorepo projects from the workspace root", async () => {
		const apiURI = vscode.Uri.file(path.join(workspaceRoot, "apps", "api", "src", "main.trb"));
		const workerURI = vscode.Uri.file(path.join(workspaceRoot, "apps", "worker", "src", "main.trb"));
		const interfaceURI = vscode.Uri.file(path.join(workspaceRoot, "apps", "api", "src", "ports.trb"));
		const apiDocument = await vscode.workspace.openTextDocument(apiURI);
		const workerDocument = await vscode.workspace.openTextDocument(workerURI);
		await vscode.workspace.openTextDocument(interfaceURI);
		await vscode.window.showTextDocument(apiDocument, { preview: false });
		await waitFor(async () => {
			const symbols = await vscode.commands.executeCommand("vscode.executeDocumentSymbolProvider", apiURI);
			return symbols?.some((symbol) => symbol.name === "ApiMessage");
		}, "nested API project symbols");
		assert.deepEqual(vscode.languages.getDiagnostics(apiURI), []);
		assert.deepEqual(vscode.languages.getDiagnostics(workerURI), []);

		const definition = await vscode.commands.executeCommand(
			"vscode.executeDefinitionProvider",
			apiURI,
			new vscode.Position(5, 15)
		);
		assert.ok(definition?.some((item) => item.uri.toString() === apiURI.toString()), "ApiMessage should resolve within the nested API project");
		const completions = await vscode.commands.executeCommand(
			"vscode.executeCompletionItemProvider",
			apiURI,
			new vscode.Position(6, 14),
			"."
		);
		assert.ok(completions?.items?.some((item) => item.label === "text"), "nested API member completion should be routed to its project");
		const implementations = await vscode.commands.executeCommand(
			"vscode.executeImplementationProvider",
			interfaceURI,
			new vscode.Position(1, 7)
		);
		assert.ok(implementations?.some((item) => item.uri.fsPath.endsWith(path.join("apps", "api", "src", "adapter.trb"))), "interface methods should navigate to nested project implementations");

		const original = apiDocument.getText();
		await replaceDocument(apiDocument, original.replace("\tmessage :=", "  message :="));
		try {
			const edits = await vscode.commands.executeCommand("vscode.executeFormatDocumentProvider", apiURI, {
				tabSize: 4,
				insertSpaces: false,
			});
			assert.ok(edits?.length > 0, "the nested API formatter should repair indentation");
		} finally {
			await replaceDocument(apiDocument, original);
		}
	});

	test("runs the standalone file through the real debug adapter lifecycle", async () => {
		const commands = await vscode.commands.getCommands(true);
		assert.ok(commands.includes("workbench.debug.panel.action.clearReplAction"), "the supported VS Code host should expose Debug Console clearing");
		const marker = path.join(workspaceRoot, "extension-host-run.txt");
		const runSource = `import { write_text } from trb/std/filesystem

def main()
\twrite_text(${JSON.stringify(marker)}, "extension-host-ok")
\treturn
end
`;
		await replaceDocument(document, runSource);
		assert.equal(await document.save(), true);
		const started = eventPromise(vscode.debug.onDidStartDebugSession, (session) => session.type === "typerb", "debug session start");
		const terminated = eventPromise(vscode.debug.onDidTerminateDebugSession, (session) => session.type === "typerb", "debug session termination");
		try {
			await vscode.commands.executeCommand("typerb.runProject", sourceURI.toString());
			const session = await started;
			assert.deepEqual(session.configuration.commandArgs, ["run", "--mode", "go", sourceURI.fsPath]);
			await waitFor(async () => (await readFile(marker, "utf8")) === "extension-host-ok", "standalone program output marker");
			await terminated;
		} finally {
			await rm(marker, { force: true });
			await replaceDocument(document, originalSource);
			await document.save();
		}
	});
});

async function replaceDocument(document, source) {
	const edit = new vscode.WorkspaceEdit();
	const lastLine = document.lineAt(document.lineCount - 1);
	edit.replace(document.uri, new vscode.Range(new vscode.Position(0, 0), lastLine.range.end), source);
	assert.equal(await vscode.workspace.applyEdit(edit), true);
}

async function waitFor(predicate, description, timeout = 15000) {
	const deadline = Date.now() + timeout;
	let lastError;
	while (Date.now() < deadline) {
		try {
			if (await predicate()) return;
		} catch (error) {
			lastError = error;
		}
		await new Promise((resolve) => setTimeout(resolve, 100));
	}
	throw new Error(`Timed out waiting for ${description}${lastError === undefined ? "" : `: ${lastError.message}`}`);
}

function eventPromise(event, predicate, description, timeout = 15000) {
	return new Promise((resolve, reject) => {
		const timer = setTimeout(() => {
			disposable.dispose();
			reject(new Error(`Timed out waiting for ${description}`));
		}, timeout);
		const disposable = event((value) => {
			if (!predicate(value)) return;
			clearTimeout(timer);
			disposable.dispose();
			resolve(value);
		});
	});
}
