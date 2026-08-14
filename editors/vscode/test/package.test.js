"use strict";

const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const path = require("node:path");
const test = require("node:test");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("../server-options");

const extensionRoot = path.resolve(__dirname, "..");
const repositoryRoot = path.resolve(extensionRoot, "../..");

test("registers the canonical TypeRB language and grammar", async () => {
	const manifest = JSON.parse(await readFile(path.join(extensionRoot, "package.json"), "utf8"));
	assert.deepEqual(manifest.activationEvents, ["onLanguage:trb"]);
	assert.deepEqual(manifest.contributes.languages[0].extensions, [".trb"]);
	assert.equal(manifest.contributes.grammars[0].scopeName, "source.trb");
	assert.deepEqual(
		manifest.contributes.commands.map((command) => command.command),
		["typerb.runProject", "typerb.stopProject"]
	);

	const canonical = await readFile(path.join(repositoryRoot, "syntaxes/typerb.tmLanguage.json"));
	const packaged = await readFile(path.join(extensionRoot, "syntaxes/typerb.tmLanguage.json"));
	assert.deepEqual(packaged, canonical, "the packaged grammar must match the repository grammar");

	const repositoryLicense = await readFile(path.join(repositoryRoot, "LICENSE"));
	const extensionLicense = await readFile(path.join(extensionRoot, "LICENSE"));
	assert.deepEqual(extensionLicense, repositoryLicense, "the packaged license must match the repository license");
});

test("runs the project with the configured TypeRB compiler", () => {
	assert.equal(runCodeLensTitle(false), "▶ Run");
	assert.equal(runCodeLensTitle(true), "↻ Restart");
	assert.deepEqual(resolveRunOptions({ path: "trb", config: "" }, "/workspace"), {
		command: "trb",
		args: ["run"]
	});
	assert.deepEqual(
		resolveRunOptions(
			{ path: "./bin/trb", config: "./apps/api/trbconfig.jsonc" },
			"/workspace"
		),
		{
			command: path.resolve("/workspace/bin/trb"),
			args: ["run", "--config", path.resolve("/workspace/apps/api/trbconfig.jsonc")]
		}
	);
});

test("starts trb lsp from PATH by default", () => {
	assert.deepEqual(resolveServerOptions({ path: "trb", config: "" }, "/workspace"), {
		command: "trb",
		args: ["lsp"]
	});
});

test("resolves configured paths from the workspace", () => {
	assert.deepEqual(
		resolveServerOptions(
			{ path: "./bin/trb", config: "./apps/api/trbconfig.jsonc" },
			"/workspace"
		),
		{
			command: path.resolve("/workspace/bin/trb"),
			args: ["lsp", "--config", path.resolve("/workspace/apps/api/trbconfig.jsonc")]
		}
	);
});
