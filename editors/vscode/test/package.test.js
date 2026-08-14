"use strict";

const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const path = require("node:path");
const test = require("node:test");
const { excludeGeneratedProjects, projectForPath, projectPaths } = require("../project-options");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("../server-options");

const extensionRoot = path.resolve(__dirname, "..");
const repositoryRoot = path.resolve(extensionRoot, "../..");

test("registers the canonical TypeRB language, grammar, and debugger", async () => {
	const manifest = JSON.parse(await readFile(path.join(extensionRoot, "package.json"), "utf8"));
	assert.deepEqual(manifest.activationEvents, ["onLanguage:trb", "onDebug:typerb"]);
	assert.deepEqual(manifest.contributes.languages[0].extensions, [".trb"]);
	assert.equal(manifest.contributes.grammars[0].scopeName, "source.trb");
	assert.deepEqual(
		manifest.contributes.commands.map((command) => command.command),
		["typerb.runProject", "typerb.stopProject"]
	);
	assert.equal(manifest.contributes.debuggers[0].type, "typerb");
	assert.deepEqual(manifest.contributes.debuggers[0].languages, ["trb"]);

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
	assert.deepEqual(
		resolveRunOptions({ path: "trb", config: "/workspace/trbconfig.jsonc" }, "/workspace", ["serve", "4000"]),
		{
			command: "trb",
			args: ["run", "--config", "/workspace/trbconfig.jsonc", "--", "serve", "4000"]
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

test("derives independent project roots from JSONC configuration", () => {
	const api = {
		configPath: path.resolve("/workspace/apps/api/trbconfig.jsonc"),
		...projectPaths(
			"/workspace/apps/api/trbconfig.jsonc",
			'{\n  // API source\n  "name": "todo-api",\n  "sourceDir": "src",\n  "outDir": "generated"\n}\n'
		)
	};
	const web = {
		configPath: path.resolve("/workspace/apps/web/trbconfig.jsonc"),
		...projectPaths(
			"/workspace/apps/web/trbconfig.jsonc",
			'{\n  "sourceDir": "src",\n  "homepage": "https://type-rb.github.io/"\n}\n'
		)
	};
	assert.equal(api.sourceRoot, path.resolve("/workspace/apps/api/src"));
	assert.equal(api.name, "todo-api");
	assert.equal(api.mode, "");
	assert.equal(api.outputRoot, path.resolve("/workspace/apps/api/generated"));
	assert.equal(api.runnable, true);
	assert.equal(web.outputRoot, path.resolve("/workspace/apps/web/build"));
	assert.equal(web.runnable, true);
	assert.equal(projectForPath([api, web], "/workspace/apps/api/src/models/todo.trb"), api);
	assert.equal(projectForPath([api, web], "/workspace/apps/web/src/models/todo.trb"), web);
	assert.equal(projectForPath([api, web], "/workspace/shared/todo.trb"), undefined);
});

test("marks TypeScript browser projects as non-runnable", () => {
	const project = projectPaths(
		"/workspace/apps/web/trbconfig.jsonc",
		'{"mode":"typescript","typescript":{"runtime":"browser"}}'
	);
	assert.equal(project.runnable, false);
	assert.equal(project.mode, "typescript");
});

test("does not start language servers for copied build projects", () => {
	const project = {
		configPath: path.resolve("/workspace/apps/api/trbconfig.jsonc"),
		sourceRoot: path.resolve("/workspace/apps/api/src"),
		outputRoot: path.resolve("/workspace/apps/api/build")
	};
	const generated = {
		configPath: path.resolve("/workspace/apps/api/build/trbconfig.jsonc"),
		sourceRoot: path.resolve("/workspace/apps/api/build/src"),
		outputRoot: path.resolve("/workspace/apps/api/build/build")
	};
	assert.deepEqual(excludeGeneratedProjects([project, generated]), [project]);
});
