"use strict";

const assert = require("node:assert/strict");
const { access, readFile, rm } = require("node:fs/promises");
const { spawnSync } = require("node:child_process");
const path = require("node:path");
const vscode = require("vscode");

const originalSource = `import { greet } from "./helper.trb"

def main()
\tputs(greet("Extension Host"))
\treturn
end
`;
const originalHelper = `def greet(name: String): String
\treturn "Hello, " + name
end
`;

suite("TypeRB Extension Host", () => {
	let document;
	let extension;
	let helperURI;
	let sourceURI;
	let siblingURI;
	let workspaceRoot;

	suiteSetup(async () => {
		const folder = vscode.workspace.workspaceFolders?.[0];
		assert.ok(folder, "the standalone fixture workspace must be open");
		workspaceRoot = folder.uri.fsPath;
		sourceURI = vscode.Uri.file(path.join(workspaceRoot, "hello.trb"));
		helperURI = vscode.Uri.file(path.join(workspaceRoot, "helper.trb"));
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
			new vscode.Position(3, 7)
		);
		assert.ok(hovers?.length > 0, "the standalone LSP should provide checked hover information");
		assert.deepEqual(vscode.languages.getDiagnostics(siblingURI), [], "a sibling file must not enter the standalone session");
		const lenses = await vscode.commands.executeCommand("vscode.executeCodeLensProvider", sourceURI, 10);
		assert.ok(lenses?.some((lens) => lens.command?.command === "typerb.debugFile"), "a Go standalone entry should offer Debug File");
	});

	test("hands an imported helper from its graph owner to an exact feature client", async () => {
		const invalidHelper = "def greet(name: String): String\n\treturn missing(name)\nend\ngre\n";
		let helper;
		await vscode.workspace.fs.writeFile(helperURI, Buffer.from(invalidHelper));
		try {
			await waitFor(
				() => vscode.languages.getDiagnostics(helperURI).some((item) => item.severity === vscode.DiagnosticSeverity.Error),
				"closed imported helper diagnostics"
			);
			helper = await vscode.workspace.openTextDocument(helperURI);
			await waitFor(async () => {
				const hovers = await vscode.commands.executeCommand(
					"vscode.executeHoverProvider",
					helperURI,
					new vscode.Position(0, 5)
				);
				return hovers?.length > 0;
			}, "exact helper hover provider");
			const completions = await vscode.commands.executeCommand(
				"vscode.executeCompletionItemProvider",
				helperURI,
				new vscode.Position(3, 3)
			);
			assert.ok(
				completions?.items?.some((item) => item.label === "greet" || item.label?.label === "greet"),
				"the exact helper client should provide completion"
			);
			const definitions = await vscode.commands.executeCommand(
				"vscode.executeDefinitionProvider",
				sourceURI,
				new vscode.Position(3, 7)
			);
			assert.ok(definitions?.some((item) => item.uri.toString() === helperURI.toString()), "entry definition should resolve to its helper");
			await waitFor(() => vscode.languages.getDiagnostics(helperURI).length > 0, "exact helper diagnostics");
			const diagnostics = vscode.languages.getDiagnostics(helperURI);
			assert.equal(new Set(diagnosticIdentities(diagnostics)).size, diagnostics.length, "diagnostic ownership handoff must not duplicate errors");
			await replaceDocument(helper, originalHelper);
			await waitFor(() => vscode.languages.getDiagnostics(helperURI).length === 0, "cleared exact helper diagnostics");
			assert.equal(await helper.save(), true);
		} finally {
			if (helper === undefined) {
				await vscode.workspace.fs.writeFile(helperURI, Buffer.from(originalHelper));
			} else if (helper.getText() !== originalHelper || helper.isDirty) {
				await replaceDocument(helper, originalHelper);
				await helper.save();
			}
		}
	});

	test("hands closed-helper diagnostics to the remaining importing graph", async () => {
		const firstURI = vscode.Uri.file(path.join(workspaceRoot, "handoff-first.trb"));
		const secondURI = vscode.Uri.file(path.join(workspaceRoot, "handoff-second.trb"));
		const sharedURI = vscode.Uri.file(path.join(workspaceRoot, "handoff-shared.trb"));
		const importingSource = `import { shared_value } from "./handoff-shared.trb"

def main()
\tputs(shared_value())
\treturn
end
`;
		const simpleSource = "def main()\n\treturn\nend\n";
		const missingImportSource = `import { absent } from "./handoff-missing.trb"

def main()
\tabsent()
\treturn
end
`;
		let first;
		let second;
		await vscode.workspace.fs.writeFile(firstURI, Buffer.from(importingSource));
		await vscode.workspace.fs.writeFile(secondURI, Buffer.from(importingSource));
		await vscode.workspace.fs.writeFile(sharedURI, Buffer.from("def shared_value(): MissingType\n\treturn missing\nend\n"));
		try {
			first = await vscode.workspace.openTextDocument(firstURI);
			second = await vscode.workspace.openTextDocument(secondURI);
			await waitFor(() => vscode.languages.getDiagnostics(sharedURI).length > 0, "shared helper diagnostics from the first graph");
			await replaceDocument(first, missingImportSource);
			await waitFor(() => vscode.languages.getDiagnostics(firstURI).length > 0, "first graph import error");
			await replaceDocument(first, simpleSource);
			await waitFor(() => vscode.languages.getDiagnostics(firstURI).length === 0, "first graph import removal");
			await vscode.workspace.fs.writeFile(sharedURI, Buffer.from("def shared_value(): OtherMissingType\n\treturn other_missing\nend\n"));
			await waitFor(
				() => vscode.languages.getDiagnostics(sharedURI).some((item) => item.message.includes("OtherMissingType")),
				"shared helper diagnostics from the remaining graph"
			);
			const diagnostics = vscode.languages.getDiagnostics(sharedURI);
			assert.equal(new Set(diagnosticIdentities(diagnostics)).size, diagnostics.length, "graph ownership handoff must not duplicate errors");
		} finally {
			await closeDocumentAndSave(first);
			await closeDocumentAndSave(second);
			await Promise.all([
				rm(firstURI.fsPath, { force: true }),
				rm(secondURI.fsPath, { force: true }),
				rm(sharedURI.fsPath, { force: true })
			]);
		}
	});

	test("discovers an imported helper from an open overlay after its file is removed", async () => {
		const dynamicURI = vscode.Uri.file(path.join(workspaceRoot, "dynamic-helper.trb"));
		const numberSource = "def dynamic_value(): Number\n\treturn 41\nend\n";
		const stringSource = "def dynamic_value(): String\n\treturn \"overlay\"\nend\n";
		const importingSource = `import { dynamic_value } from "./dynamic-helper.trb"

def main()
\tputs(dynamic_value() + 1)
\treturn
end
`;
		await vscode.workspace.fs.writeFile(dynamicURI, Buffer.from(numberSource));
		const dynamic = await vscode.workspace.openTextDocument(dynamicURI);
		await waitFor(async () => {
			const symbols = await vscode.commands.executeCommand("vscode.executeDocumentSymbolProvider", dynamicURI);
			return symbols?.some((symbol) => symbol.name === "dynamic_value");
		}, "candidate helper standalone client");
		await replaceDocument(dynamic, stringSource);
		await vscode.workspace.fs.delete(dynamicURI);
		try {
			await replaceDocument(document, importingSource);
			await waitFor(
				() => vscode.languages.getDiagnostics(sourceURI).some((item) => item.severity === vscode.DiagnosticSeverity.Error),
				"entry diagnostics from a missing-on-disk helper overlay"
			);
			await replaceDocument(dynamic, numberSource);
			await waitFor(() => vscode.languages.getDiagnostics(sourceURI).length === 0, "entry diagnostics refreshed from helper overlay");
			const baselineHelperDiagnostics = diagnosticIdentities(vscode.languages.getDiagnostics(dynamicURI));
			await replaceDocument(dynamic, "def dynamic_value(): MissingType\n\treturn missing\nend\n");
			await waitFor(
				() => JSON.stringify(diagnosticIdentities(vscode.languages.getDiagnostics(dynamicURI))) !== JSON.stringify(baselineHelperDiagnostics),
				"helper diagnostics from overlapping graphs"
			);
			const diagnostics = vscode.languages.getDiagnostics(dynamicURI);
			const identities = diagnosticIdentities(diagnostics);
			assert.equal(new Set(identities).size, diagnostics.length, "only the deterministic standalone owner should publish helper diagnostics");
			await replaceDocument(dynamic, numberSource);
			await waitFor(
				() => JSON.stringify(diagnosticIdentities(vscode.languages.getDiagnostics(dynamicURI))) === JSON.stringify(baselineHelperDiagnostics),
				"restored helper diagnostics from its owner"
			);
		} finally {
			await replaceDocument(document, originalSource);
			await vscode.workspace.fs.writeFile(dynamicURI, Buffer.from(numberSource));
			await replaceDocument(dynamic, numberSource);
			await dynamic.save();
			await closeDocumentAndSave(dynamic);
			await rm(dynamicURI.fsPath, { force: true });
		}
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
		const original = apiDocument.getText();
		const workerDocument = await vscode.workspace.openTextDocument(workerURI);
		await vscode.workspace.openTextDocument(interfaceURI);
		await vscode.window.showTextDocument(apiDocument, { preview: false });
		await waitFor(async () => {
			const symbols = await vscode.commands.executeCommand("vscode.executeDocumentSymbolProvider", apiURI);
			return symbols?.some((symbol) => symbol.name === "ApiMessage");
		}, "nested API project symbols");
		assert.deepEqual(vscode.languages.getDiagnostics(apiURI), []);
		assert.deepEqual(vscode.languages.getDiagnostics(workerURI), []);

		await waitFor(async () => {
			const definition = await vscode.commands.executeCommand(
				"vscode.executeDefinitionProvider",
				apiURI,
				new vscode.Position(5, 15)
			);
			return definition?.some((item) => item.uri.toString() === apiURI.toString());
		}, "nested API checked definition");
		const completions = await vscode.commands.executeCommand(
			"vscode.executeCompletionItemProvider",
			apiURI,
			new vscode.Position(6, 14),
			"."
		);
		assert.ok(completions?.items?.some((item) => item.label === "text"), "nested API member completion should be routed to its project");

		const quickSuggestions = vscode.workspace.getConfiguration("editor", apiDocument).get("quickSuggestions");
		assert.equal(quickSuggestions.other, "on", "TypeRB should request completions while identifiers are typed");
		const incompleteImportSource = `def render(renderer: MessageRende)
	return
end
`;
		try {
			await replaceDocument(apiDocument, incompleteImportSource);
			const editor = await vscode.window.showTextDocument(apiDocument, { preview: false });
			const offset = incompleteImportSource.indexOf("MessageRende") + "MessageRende".length;
			const cursor = apiDocument.positionAt(offset);
			editor.selection = new vscode.Selection(cursor, cursor);
			const importCompletions = await vscode.commands.executeCommand(
				"vscode.executeCompletionItemProvider",
				apiURI,
				cursor
			);
			const imported = importCompletions?.items?.find((item) => item.label === "MessageRenderer");
			assert.ok(imported, "a partial type prefix should offer the project type");
			assert.equal(imported.textEdit?.newText, "MessageRenderer");
			assert.ok(
				imported.additionalTextEdits?.some((edit) => edit.newText === "import { MessageRenderer } from ports\n"),
				"the project type completion should add its import"
			);
		} finally {
			await replaceDocument(apiDocument, original);
		}

		const implementations = await vscode.commands.executeCommand(
			"vscode.executeImplementationProvider",
			interfaceURI,
			new vscode.Position(1, 7)
		);
		assert.ok(implementations?.some((item) => item.uri.fsPath.endsWith(path.join("apps", "api", "src", "adapter.trb"))), "interface methods should navigate to nested project implementations");

		const editorSettings = vscode.workspace.getConfiguration("editor", {
			uri: apiURI,
			languageId: "trb",
		});
		const previousFormatOnSave = editorSettings.inspect("formatOnSave")?.globalLanguageValue;
		const previousDefaultFormatter = editorSettings.inspect("defaultFormatter")?.globalLanguageValue;
		try {
			await editorSettings.update("formatOnSave", true, vscode.ConfigurationTarget.Global, true);
			await editorSettings.update("defaultFormatter", "type-rb.typerb", vscode.ConfigurationTarget.Global, true);
			const activeEditorSettings = vscode.workspace.getConfiguration("editor", apiDocument);
			assert.equal(activeEditorSettings.get("formatOnSave"), true, "format-on-save must be enabled for TypeRB");
			assert.equal(activeEditorSettings.get("defaultFormatter"), "type-rb.typerb", "TypeRB must be the selected formatter");
			assert.equal(vscode.window.activeTextEditor?.document.uri.toString(), apiURI.toString(), "the nested API editor must remain active");
			const malformed = original.replace("\tmessage :=", "  message :=");
			assert.notEqual(malformed, original, "the format-on-save fixture must change indentation");
			await replaceDocument(apiDocument, malformed);
			await vscode.commands.executeCommand("editor.action.formatDocument");
			assert.equal(apiDocument.getText(), original, "the configured default formatter should format the active nested API document");
			await replaceDocument(apiDocument, malformed);
			assert.equal(await apiDocument.save(), true, "the nested API document should save successfully");
			assert.equal(apiDocument.getText(), original, "format-on-save should restore the canonical buffer");
			assert.equal(await readFile(apiURI.fsPath, "utf8"), original, "format-on-save should write canonical source to disk");
		} finally {
			try {
				if (apiDocument.getText() !== original) {
					await replaceDocument(apiDocument, original);
				}
				if (apiDocument.isDirty) {
					await apiDocument.save();
				}
				if (await readFile(apiURI.fsPath, "utf8") !== original) {
					await vscode.workspace.fs.writeFile(apiURI, Buffer.from(original));
				}
			} finally {
				try {
					await editorSettings.update("defaultFormatter", previousDefaultFormatter, vscode.ConfigurationTarget.Global, true);
				} finally {
					await editorSettings.update("formatOnSave", previousFormatOnSave, vscode.ConfigurationTarget.Global, true);
				}
			}
		}
	});

	test("runs the standalone file through the real debug adapter lifecycle", async () => {
		const commands = await vscode.commands.getCommands(true);
		assert.ok(commands.includes("workbench.debug.panel.action.clearReplAction"), "the supported VS Code host should expose Debug Console clearing");
		const marker = path.join(workspaceRoot, "extension-host-run.txt");
		const runSource = `import { write_text } from trb/std/filesystem

def main()
\twrite_text(${JSON.stringify(marker)}, "extension-host-ok") catch |_error|
\t\treturn
\tend
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

	test("debugs the standalone file with a session-private executable", async function() {
		if (spawnSync("dlv", ["version"], { stdio: "ignore" }).status !== 0) {
			this.skip();
			return;
		}
		const started = eventPromise(
			vscode.debug.onDidStartDebugSession,
			(session) => session.type === "typerb" && session.configuration.standaloneDebugBuild === true,
			"standalone source debug session start"
		);
		const terminated = eventPromise(
			vscode.debug.onDidTerminateDebugSession,
			(session) => session.type === "typerb" && session.configuration.standaloneDebugBuild === true,
			"standalone source debug session termination",
			30000
		);
		await vscode.commands.executeCommand("typerb.debugFile", sourceURI.toString());
		const session = await started;
		const program = session.configuration.program;
		assert.equal(typeof program, "string");
		await waitFor(
			async () => {
				try {
					await access(program);
					return true;
				} catch {
					return false;
				}
			},
			"standalone debug executable",
			30000
		);
		assert.notEqual(path.dirname(program), workspaceRoot);
		assert.ok(path.basename(path.dirname(program)).startsWith("typerb-vscode-debug-"));
		await terminated;
		await waitFor(async () => {
			try {
				await access(path.dirname(program));
				return false;
			} catch (error) {
				return error.code === "ENOENT";
			}
		}, "standalone debug artifact cleanup");
	});

});

async function replaceDocument(document, source) {
	const edit = new vscode.WorkspaceEdit();
	const lastLine = document.lineAt(document.lineCount - 1);
	edit.replace(document.uri, new vscode.Range(new vscode.Position(0, 0), lastLine.range.end), source);
	assert.equal(await vscode.workspace.applyEdit(edit), true);
}

async function closeDocumentAndSave(document) {
	if (document === undefined || document.isClosed) return;
	if (document.isDirty) {
		await document.save();
	}
	await vscode.window.showTextDocument(document, { preview: false });
	await vscode.commands.executeCommand("workbench.action.closeActiveEditor");
}

function diagnosticIdentities(diagnostics) {
	return diagnostics
		.map((item) => `${item.range.start.line}:${item.range.start.character}:${item.message}`)
		.sort();
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
