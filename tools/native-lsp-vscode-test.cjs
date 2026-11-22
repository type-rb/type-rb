"use strict";

const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const path = require("node:path");
const vscode = require("vscode");

async function waitFor(description, check) {
	const deadline = Date.now() + 20_000;
	do {
		if (await check()) return;
		await new Promise((resolve) => setTimeout(resolve, 100));
	} while (Date.now() < deadline);
	throw new Error("Timed out waiting for " + description);
}

exports.run = async function () {
	const root = vscode.workspace.workspaceFolders[0].uri.fsPath;
	const extension = vscode.extensions.getExtension("type-rb.typerb");
	assert.ok(extension, "official TypeRB extension");
	for (const folder of ["project/src", "standalone"]) {
		const uri = vscode.Uri.file(path.join(root, folder, "main.trb"));
		const helper = vscode.Uri.file(path.join(root, folder, "helper.trb"));
		const original = await readFile(uri.fsPath, "utf8");
		const document = await vscode.workspace.openTextDocument(uri);
		await vscode.window.showTextDocument(document);
		await extension.activate();
		assert.equal(document.languageId, "trb");

		const position = document.positionAt(original.indexOf("Greeting.new"));
		const findTarget = async () => {
			const items = await vscode.commands.executeCommand("vscode.executeDefinitionProvider", uri, position);
			return items?.some((item) => (item.uri ?? item.targetUri).fsPath === helper.fsPath);
		};
		await waitFor(folder + " imported record", findTarget);
		const member = original.indexOf("row.text") + "row.".length;
		await waitFor(folder + " typed member completion", async () => {
			const result = await vscode.commands.executeCommand("vscode.executeCompletionItemProvider",
				uri, document.positionAt(member), ".");
			return result?.items.some((item) => item.label === "text" && item.kind === vscode.CompletionItemKind.Field);
		});
		await waitFor(folder + " typed binding hover", async () => {
			const items = await vscode.commands.executeCommand("vscode.executeHoverProvider",
				uri, document.positionAt(member - 3));
			return items?.some((item) => item.contents.some((entry) => entry.value?.includes("row: Greeting")));
		});
		await waitFor(folder + " imported call signature", async () => {
			const call = original.indexOf("greeting_text(row)") + "greeting_text(".length;
			const help = await vscode.commands.executeCommand("vscode.executeSignatureHelpProvider",
				uri, document.positionAt(call), "(");
			return help?.activeParameter === 0 && help.signatures[0]?.label === "greeting_text(row: Greeting): String";
		});
		const start = original.indexOf("row.text");
		const edit = new vscode.WorkspaceEdit();
		edit.replace(uri, new vscode.Range(document.positionAt(start), document.positionAt(start + 8)), "1");
		assert.ok(await vscode.workspace.applyEdit(edit));
		await waitFor(folder + " unsaved diagnostic", () => vscode.languages.getDiagnostics(uri)
			.some((d) => d.severity === vscode.DiagnosticSeverity.Error && d.range.start.line === 4));
		const repair = new vscode.WorkspaceEdit();
		repair.replace(uri, new vscode.Range(document.positionAt(start), document.positionAt(start + 1)), "row.text");
		assert.ok(await vscode.workspace.applyEdit(repair));
		await waitFor(folder + " diagnostic clearing", () => vscode.languages.getDiagnostics(uri).length === 0);
		await waitFor(folder + " restored definition", findTarget);
		assert.equal(document.getText(), original);
		const binding = original.indexOf("row := ");
		const compact = new vscode.WorkspaceEdit();
		compact.replace(uri, new vscode.Range(document.positionAt(binding), document.positionAt(binding + 7)), "row:=");
		assert.ok(await vscode.workspace.applyEdit(compact));
		await waitFor(folder + " format document", async () => {
			const edits = await vscode.commands.executeCommand("vscode.executeFormatDocumentProvider", uri,
				{ tabSize: 4, insertSpaces: false });
			if (!edits?.length) return false;
			const formatted = new vscode.WorkspaceEdit();
			formatted.set(uri, edits);
			return vscode.workspace.applyEdit(formatted);
		});
		assert.ok(document.getText().includes("row := Greeting.new"));
		assert.equal(await readFile(uri.fsPath, "utf8"), original);
		console.log(folder + " extension-host diagnostics, definitions, formatting, completion, hover and signature help passed");
	}
};
