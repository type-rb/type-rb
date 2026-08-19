"use strict";

const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const path = require("node:path");
const test = require("node:test");
const { excludeGeneratedProjects, literalGlobPattern, projectForPath, projectPaths } = require("../project-options");
const { resolveRunOptions, resolveServerOptions, resolveStandaloneDebugBuildOptions, runCodeLensTitle } = require("../server-options");
const { transitionStandaloneClient } = require("../standalone-client-state");

const extensionRoot = path.resolve(__dirname, "..");
const repositoryRoot = path.resolve(extensionRoot, "../..");

test("registers the canonical TypeRB language, grammar, and debugger", async () => {
	const manifest = JSON.parse(await readFile(path.join(extensionRoot, "package.json"), "utf8"));
	assert.equal(manifest.version, "0.3.1");
	assert.deepEqual(manifest.activationEvents, ["onLanguage:trb", "onDebug:typerb", "workspaceContains:trbconfig.jsonc"]);
	assert.deepEqual(manifest.contributes.languages[0].extensions, [".trb"]);
	assert.equal(manifest.contributes.grammars[0].scopeName, "source.trb");
	assert.deepEqual(
		manifest.contributes.commands.map((command) => command.command),
		["typerb.runProject", "typerb.debugFile", "typerb.runTest", "typerb.debugTest", "typerb.stopProject"]
	);
	assert.equal(manifest.contributes.debuggers[0].type, "typerb");
	assert.deepEqual(manifest.contributes.debuggers[0].languages, ["trb"]);
	assert.deepEqual(manifest.contributes.breakpoints, [{ language: "trb" }]);
	assert.equal(manifest.contributes.configuration.properties["typerb.standalone.mode"].default, "go");
	assert.deepEqual(manifest.contributes.configuration.properties["typerb.standalone.mode"].enum, ["go", "ruby", "typescript"]);
	assert.equal(manifest.contributes.configuration.properties["typerb.standalone.typescript.runtime"].default, "node");
	assert.deepEqual(manifest.contributes.configurationDefaults["[trb]"], {
		"editor.quickSuggestions": { other: "on", comments: "off", strings: "off" },
		"editor.suggestOnTriggerCharacters": true,
	});

	const canonical = await readFile(path.join(repositoryRoot, "syntaxes/typerb.tmLanguage.json"));
	const packaged = await readFile(path.join(extensionRoot, "syntaxes/typerb.tmLanguage.json"));
	assert.deepEqual(packaged, canonical, "the packaged grammar must match the repository grammar");
	const grammar = JSON.parse(canonical);
	const controlPattern = grammar.repository.keywords.patterns.find((pattern) => pattern.name === "keyword.control.trb");
	assert.ok(controlPattern, "the grammar must define control-flow keywords");
	const controlKeywords = new RegExp(controlPattern.match);
	for (const keyword of ["try", "catch"]) assert.equal(controlKeywords.test(keyword), true, `${keyword} must be highlighted`);
	for (const legacy of ["attempt", "fails"]) assert.equal(controlKeywords.test(legacy), false, `${legacy} must not be highlighted`);

	const snippets = JSON.parse(await readFile(path.join(extensionRoot, "snippets/typerb.json"), "utf8"));
	const prefixes = Object.values(snippets).map((snippet) => snippet.prefix);
	assert.ok(prefixes.includes("try"));
	assert.ok(prefixes.includes("catch"));
	assert.ok(!prefixes.includes("attempt"));
	assert.ok(!prefixes.includes("fails"));

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

test("runs a standalone file with its configured mode and runtime", () => {
	assert.deepEqual(
		resolveRunOptions(
			{ path: "trb", config: "", file: "/workspace/hello.trb", mode: "go" },
			"/workspace"
		),
		{ command: "trb", args: ["run", "--mode", "go", "/workspace/hello.trb"] }
	);
	assert.deepEqual(
		resolveRunOptions(
			{ path: "trb", config: "", file: "hello.trb", mode: "typescript", runtime: "bun" },
			"/workspace",
			["Ada"]
		),
		{
			command: "trb",
			args: ["run", "--mode", "typescript", "--runtime", "bun", "/workspace/hello.trb", "--", "Ada"]
		}
	);
});

test("builds a standalone Go debug executable at the session-private path", () => {
	assert.deepEqual(
		resolveStandaloneDebugBuildOptions(
			{ path: "./bin/trb", file: "src/hello.trb" },
			"/workspace",
			"/private/session-42/app"
		),
		{
			command: path.resolve("/workspace/bin/trb"),
			args: [
				"build",
				"--compile",
				"--debug",
				"--outfile",
				"/private/session-42/app",
				path.resolve("/workspace/src/hello.trb")
			]
		}
	);
});

test("starts trb lsp from PATH by default", () => {
	assert.deepEqual(resolveServerOptions({ path: "trb", config: "" }, "/workspace"), {
		command: "trb",
		args: ["lsp"]
	});
});

test("starts a standalone language server for one file", () => {
	assert.deepEqual(
		resolveServerOptions(
			{ path: "trb", config: "", file: "/workspace/hello.trb", mode: "ruby" },
			"/workspace"
		),
		{ command: "trb", args: ["lsp", "--mode", "ruby", "/workspace/hello.trb"] }
	);
	assert.deepEqual(
		resolveServerOptions(
			{ path: "trb", config: "", file: "hello.trb", mode: "typescript", runtime: "node" },
			"/workspace"
		),
		{
			command: "trb",
			args: ["lsp", "--mode", "typescript", "--runtime", "node", "/workspace/hello.trb"]
		}
	);
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

test("matches standalone filenames literally in language client selectors", () => {
	assert.equal(literalGlobPattern("[slug] {draft}*.trb"), "[[]slug[]] [{]draft[}][*].trb");
});

test("replays manually forwarded helpers after a standalone language server restart", () => {
	const project = {
		clientStarted: false,
		clientRunning: false,
		forwardedDocuments: new Set()
	};
	assert.equal(transitionStandaloneClient(project, true), false, "initial startup is reconciled by activation");
	project.forwardedDocuments.add("/workspace/helper.trb");
	assert.equal(transitionStandaloneClient(project, false), false);
	assert.deepEqual([...project.forwardedDocuments], []);
	assert.equal(project.clientRunning, false);
	assert.equal(transitionStandaloneClient(project, true), true, "a restarted client needs full didOpen replay");
	assert.equal(project.clientRunning, true);
});
