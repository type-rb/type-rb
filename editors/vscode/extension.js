"use strict";

const { readFile } = require("node:fs/promises");
const { execFile } = require("node:child_process");
const { promisify } = require("node:util");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");
const { TypeRBDebugSession, TypeRBProcess } = require("./debug-session");
const { DlvDAPProcess } = require("./go-debug-adapter");
const { excludeGeneratedProjects, projectForPath, projectPaths } = require("./project-options");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("./server-options");
const { decodeTestEvent } = require("./test-events");

let projectManager;
let runCodeLensProvider;
let typeRBTests;
const debugSessions = new Map();
const executeFile = promisify(execFile);

async function activate(context) {
	const debugAdapterFactory = new TypeRBDebugAdapterFactory();
	context.subscriptions.push(
		vscode.commands.registerCommand("typerb.runProject", runProject),
		vscode.commands.registerCommand("typerb.runTest", (uri, fullName) => typeRBTests?.runByName(uri, fullName)),
		vscode.commands.registerCommand("typerb.debugTest", (uri, fullName) => typeRBTests?.debugByName(uri, fullName)),
		vscode.commands.registerCommand("typerb.stopProject", stopProject),
		vscode.debug.onDidStartDebugSession((session) => {
			if (session.type === "typerb" && typeof session.configuration.configPath === "string") {
				debugSessions.set(path.resolve(session.configuration.configPath), session);
				runCodeLensProvider?.refresh();
			}
		}),
		vscode.debug.onDidTerminateDebugSession((session) => {
			if (session.type === "typerb" && typeof session.configuration.configPath === "string") {
				const configPath = path.resolve(session.configuration.configPath);
				if (debugSessions.get(configPath) === session) {
					debugSessions.delete(configPath);
					runCodeLensProvider?.refresh();
				}
			}
		})
	);
	projectManager = new ProjectManager();
	context.subscriptions.push({
		dispose() {
			void projectManager?.stop();
		}
	});
	await projectManager.start();
	typeRBTests = new TypeRBTestController(projectManager);
	context.subscriptions.push(typeRBTests);
	await typeRBTests.refresh();
	context.subscriptions.push(vscode.workspace.onDidSaveTextDocument((document) => {
		if (document.languageId === "trb") {
			void typeRBTests?.refreshProject(projectManager?.projectForURI(document.uri));
		}
	}));
	context.subscriptions.push(
		vscode.debug.registerDebugConfigurationProvider(
			"typerb",
			new TypeRBDebugConfigurationProvider(projectManager),
			vscode.DebugConfigurationProviderTriggerKind.Dynamic
		),
		vscode.debug.registerDebugAdapterDescriptorFactory("typerb", {
			createDebugAdapterDescriptor(session) {
				return debugAdapterFactory.createDebugAdapterDescriptor(session);
			}
		}),
		debugAdapterFactory,
		vscode.debug.onDidTerminateDebugSession((session) => debugAdapterFactory.terminate(session))
	);
	runCodeLensProvider = new RunCodeLensProvider(projectManager);
	context.subscriptions.push(
		runCodeLensProvider,
		vscode.languages.registerCodeLensProvider(
			[{ scheme: "file", language: "trb" }],
			runCodeLensProvider
		)
	);
}

class ProjectManager {
	constructor() {
		this.projects = [];
	}

	async start() {
		this.projects = await discoverProjects();
		if (this.projects.length === 0) {
			void vscode.window.showErrorMessage("TypeRB could not find trbconfig.jsonc in the opened workspace.");
			return;
		}
		await Promise.all(this.projects.map(async (project, index) => {
			try {
				await this.startProject(project, index);
			} catch (error) {
				project.watcher?.dispose();
				project.watcher = undefined;
				project.client = undefined;
				void vscode.window.showErrorMessage(`Cannot start TypeRB project ${project.configPath}: ${error.message}`);
			}
		}));
	}

	async startProject(project, index) {
		const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
		const workspaceRoot = project.workspaceFolder?.uri.fsPath;
		const server = resolveServerOptions(
			{
				path: settings.get("server.path", "trb"),
				config: project.configPath
			},
			workspaceRoot
		);
		project.watcher = vscode.workspace.createFileSystemWatcher(
			new vscode.RelativePattern(project.sourceRoot, "**/*.trb")
		);
		project.client = new LanguageClient(
			`typerb-${index}`,
			`TypeRB Language Server (${project.label})`,
			{
				command: server.command,
				args: server.args,
				options: { cwd: project.root }
			},
			{
				documentSelector: [{
					scheme: "file",
					language: "trb",
					pattern: new vscode.RelativePattern(project.sourceRoot, "**/*.trb")
				}],
				synchronize: { fileEvents: project.watcher },
				middleware: {
					provideCodeLenses() {
						return [];
					}
				}
			}
		);
		project.testCommand = server.command;
		await project.client.start();
	}

	projectForURI(uri) {
		if (uri?.scheme !== "file") {
			return undefined;
		}
		return projectForPath(this.projects, uri.fsPath);
	}

	firstProject() {
		return this.projects.find((project) => project.runnable);
	}

	projectForConfigPath(configPath) {
		const resolved = path.resolve(configPath);
		return this.projects.find((project) => project.configPath === resolved);
	}

	async stop() {
		const projects = this.projects;
		this.projects = [];
		await Promise.all(projects.map(async (project) => {
			project.watcher?.dispose();
			if (project.client !== undefined) {
				await project.client.stop();
			}
		}));
	}
}

async function discoverProjects() {
	const candidates = new Map();
	const found = await vscode.workspace.findFiles(
		"**/trbconfig.jsonc",
		"**/{.git,.trb,node_modules}/**"
	);
	for (const configUri of found) {
		const workspaceFolder = vscode.workspace.getWorkspaceFolder(configUri);
		candidates.set(path.resolve(configUri.fsPath), { configUri, workspaceFolder });
	}
	for (const workspaceFolder of vscode.workspace.workspaceFolders ?? []) {
		const settings = vscode.workspace.getConfiguration("typerb", workspaceFolder.uri);
		const configured = settings.get("server.config", "").trim();
		if (configured === "") {
			continue;
		}
		const configPath = path.isAbsolute(configured)
			? configured
			: path.resolve(workspaceFolder.uri.fsPath, configured);
		candidates.set(path.resolve(configPath), {
			configUri: vscode.Uri.file(configPath),
			workspaceFolder
		});
	}
	const projects = [];
	for (const [configPath, candidate] of candidates) {
		try {
			const source = await readFile(configPath, "utf8");
			const paths = projectPaths(configPath, source);
			const relative = candidate.workspaceFolder === undefined
				? configPath
				: path.relative(candidate.workspaceFolder.uri.fsPath, paths.root) || ".";
			projects.push({
				...candidate,
				...paths,
				configPath,
				label: paths.name || relative
			});
		} catch (error) {
			void vscode.window.showErrorMessage(`Cannot load TypeRB project ${configPath}: ${error.message}`);
		}
	}
	return excludeGeneratedProjects(projects).sort((left, right) => left.configPath.localeCompare(right.configPath));
}

class RunCodeLensProvider {
	constructor(manager) {
		this.manager = manager;
		this.changed = new vscode.EventEmitter();
		this.onDidChangeCodeLenses = this.changed.event;
	}

	refresh() {
		this.changed.fire();
	}

	async provideCodeLenses(document, token) {
		const project = this.manager.projectForURI(document.uri);
		if (project?.client === undefined) {
			return [];
		}
		let items;
		try {
			items = await project.client.sendRequest(
				"textDocument/codeLens",
				{ textDocument: { uri: document.uri.toString() } },
				token
			);
		} catch {
			return [];
		}
		const session = debugSessions.get(project.configPath);
		const running = session !== undefined && typeof session.configuration.testFilter !== "string";
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

class TypeRBTestController {
	constructor(manager) {
		this.manager = manager;
		this.controller = vscode.tests.createTestController("typerb", "TypeRB");
		this.metadata = new Map();
		this.debugTag = new vscode.TestTag("typerb-go-debug");
		this.controller.refreshHandler = () => this.refresh();
		this.controller.createRunProfile(
			"Run",
			vscode.TestRunProfileKind.Run,
			(request, token) => this.run(request, token),
			true
		);
		this.controller.createRunProfile(
			"Debug",
			vscode.TestRunProfileKind.Debug,
			(request, token) => this.debug(request, token),
			true,
			this.debugTag
		);
	}

	async refresh() {
		await Promise.all(this.manager.projects.map((project) => this.refreshProject(project)));
	}

	async refreshProject(project) {
		if (project?.client === undefined) {
			return;
		}
		let discovered;
		try {
			discovered = await project.client.sendRequest("typerb/discoverTests", {});
		} catch {
			return;
		}
		const rootID = `project:${project.configPath}`;
		let root = this.controller.items.get(rootID);
		if (root === undefined) {
			root = this.controller.createTestItem(rootID, project.label, project.configUri);
			this.controller.items.add(root);
		}
		root.tags = project.mode === "go" ? [this.debugTag] : [];
		for (const [id, metadata] of this.metadata) {
			if (metadata.project === project) {
				this.metadata.delete(id);
			}
		}
		root.children.replace([]);
		this.metadata.set(root.id, { project, kind: "project", fullName: "" });
		const byID = new Map();
		for (const raw of discovered) {
			const id = `${project.configPath}:${raw.id}`;
			const item = this.controller.createTestItem(id, raw.name, vscode.Uri.parse(raw.uri));
			item.tags = project.mode === "go" ? [this.debugTag] : [];
			item.range = new vscode.Range(raw.range.start.line, raw.range.start.character, raw.range.end.line, raw.range.end.character);
			byID.set(raw.id, item);
			this.metadata.set(item.id, { project, kind: raw.kind, fullName: raw.fullName });
			const parent = raw.parentId === "" ? root : byID.get(raw.parentId);
			(parent ?? root).children.add(item);
		}
	}

	async runByName(uriValue, fullName) {
		return this.runOrDebugByName(uriValue, fullName, false);
	}

	async debugByName(uriValue, fullName) {
		return this.runOrDebugByName(uriValue, fullName, true);
	}

	async runOrDebugByName(uriValue, fullName, debug) {
		const uri = typeof uriValue === "string" ? vscode.Uri.parse(uriValue) : uriValue;
		const project = this.manager.projectForURI(uri);
		if (project === undefined) {
			return;
		}
		await this.refreshProject(project);
		let selected;
		for (const [id, metadata] of this.metadata) {
			if (metadata.project === project && metadata.kind === "test" && metadata.fullName === fullName) {
				const candidate = this.findItem(id);
				if (candidate?.uri.fsPath !== uri.fsPath) {
					continue;
				}
				selected = candidate;
				break;
			}
		}
		if (selected === undefined) {
			void vscode.window.showErrorMessage(`TypeRB could not find test ${fullName}.`);
			return;
		}
		const cancellation = new vscode.CancellationTokenSource();
		try {
			const request = new vscode.TestRunRequest([selected]);
			if (debug) {
				await this.debug(request, cancellation.token);
			} else {
				await this.run(request, cancellation.token);
			}
		} finally {
			cancellation.dispose();
		}
	}

	async debug(request, token) {
		const invocations = this.invocations(request);
		if (invocations.length === 0 || token.isCancellationRequested) {
			return;
		}
		if (invocations.length > 1) {
			void vscode.window.showInformationMessage("TypeRB debugs one selected test group at a time. Start another debug session for the remaining selection.");
		}
		const invocation = invocations[0];
		if (invocation.project.mode !== "go") {
			void vscode.window.showErrorMessage(`Source debugging tests is not yet available for mode: ${invocation.project.mode}. Run the tests normally instead.`);
			return;
		}
		if (!(await saveTypeRBDocuments(invocation.project))) {
			void vscode.window.showErrorMessage("Save TypeRB project files before debugging tests.");
			return;
		}
		const running = debugSessions.get(invocation.project.configPath);
		if (running !== undefined) {
			await vscode.debug.stopDebugging(running);
		}
		const configuration = debugConfigurationForTest(invocation.project, invocation.filter, invocation.file);
		const started = await vscode.debug.startDebugging(invocation.project.workspaceFolder, configuration);
		if (!started) {
			void vscode.window.showErrorMessage(`Cannot debug TypeRB tests in ${invocation.project.label}.`);
		}
	}

	findItem(id) {
		let found;
		const visit = (collection) => collection.forEach((item) => {
			if (item.id === id) {
				found = item;
			} else if (found === undefined) {
				visit(item.children);
			}
		});
		visit(this.controller.items);
		return found;
	}

	leafTests(item, result = []) {
		const metadata = this.metadata.get(item.id);
		if (metadata?.kind === "test") {
			result.push(item);
			return result;
		}
		item.children.forEach((child) => this.leafTests(child, result));
		return result;
	}

	invocations(request) {
		const excluded = new Set();
		for (const item of request.exclude ?? []) {
			for (const leaf of this.leafTests(item)) {
				excluded.add(leaf.id);
			}
		}
		const selected = [];
		if (request.include === undefined) {
			this.controller.items.forEach((item) => selected.push(item));
		} else {
			selected.push(...request.include);
		}
		const result = [];
		const covered = new Set();
		for (const item of selected) {
			const metadata = this.metadata.get(item.id);
			if (metadata === undefined) {
				continue;
			}
			const leaves = this.leafTests(item).filter((leaf) => !excluded.has(leaf.id) && !covered.has(leaf.id));
			if (leaves.length === 0) {
				continue;
			}
			for (const leaf of leaves) {
				covered.add(leaf.id);
			}
			if (excluded.size === 0) {
				const files = new Set(leaves.map((leaf) => leaf.uri.fsPath));
				result.push({ project: metadata.project, filter: metadata.fullName, file: files.size === 1 ? leaves[0].uri.fsPath : "", tests: leaves });
			} else {
				for (const leaf of leaves) {
					const leafMetadata = this.metadata.get(leaf.id);
					result.push({ project: leafMetadata.project, filter: leafMetadata.fullName, file: leaf.uri.fsPath, tests: [leaf] });
				}
			}
		}
		return result;
	}

	async run(request, token) {
		const testRun = this.controller.createTestRun(request);
		try {
			for (const invocation of this.invocations(request)) {
				if (token.isCancellationRequested) {
					break;
				}
				await this.runInvocation(testRun, invocation, token);
			}
		} finally {
			testRun.end();
		}
	}

	async runInvocation(testRun, invocation, token) {
		if (!(await saveTypeRBDocuments(invocation.project))) {
			for (const item of invocation.tests) {
				testRun.errored(item, new vscode.TestMessage("Save TypeRB project files before running tests."));
			}
			return;
		}
		const args = ["test", "--config", invocation.project.configPath, "--reporter", "json"];
		if (invocation.filter !== "") {
			args.push("--filter", invocation.filter);
		}
		if (invocation.file !== "") {
			args.push("--file", invocation.file);
		}
		const runner = new TypeRBProcess({
			command: invocation.project.testCommand || "trb",
			args,
			cwd: invocation.project.root,
			env: process.env,
		});
		const cancellation = token.onCancellationRequested(() => void runner.stop());
		const byIdentity = new Map(invocation.tests.map((item) => [
			testIdentity(this.metadata.get(item.id).fullName, item.uri.fsPath),
			item,
		]));
		const byName = new Map(invocation.tests.map((item) => [this.metadata.get(item.id).fullName, item]));
		let buffer = "";
		let failures = 0;
		const consume = (text) => {
			buffer += text;
			const lines = buffer.split(/\r?\n/);
			buffer = lines.pop() ?? "";
			for (const line of lines) {
				const event = decodeTestEvent(line);
				if (event === undefined) {
					if (line !== "") testRun.appendOutput(line + "\r\n");
					continue;
				}
				const item = typeof event.test_file === "string"
					? byIdentity.get(testIdentity(event.name, event.test_file))
					: byName.get(event.name);
				if (item === undefined) continue;
				if (event.type === "test_started") testRun.started(item);
				if (event.type === "test_passed") testRun.passed(item);
				if (event.type === "test_failed") {
					failures += 1;
					const message = new vscode.TestMessage(event.message || "Test failed");
					if (typeof event.file === "string" && typeof event.line === "number" && typeof event.column === "number") {
						message.location = new vscode.Location(vscode.Uri.file(event.file), new vscode.Position(Math.max(0, event.line - 1), Math.max(0, event.column - 1)));
					}
					testRun.failed(item, message);
				}
			}
		};
		const status = await new Promise((resolve) => {
			runner.on("output", (category, text) => {
				if (category === "stdout") consume(text);
				else testRun.appendOutput(text.replace(/\r?\n/g, "\r\n"));
			});
			runner.once("error", (error) => resolve({ code: -1, error }));
			runner.once("exit", (code) => resolve({ code: code ?? -1 }));
			runner.start();
		});
		cancellation.dispose();
		if (buffer !== "") consume("\n");
		if (status.code !== 0 && failures === 0 && !token.isCancellationRequested) {
			const message = new vscode.TestMessage(status.error?.message || `trb test exited with status ${status.code}`);
			for (const item of invocation.tests) testRun.errored(item, message);
		}
	}

	dispose() {
		this.metadata.clear();
		this.controller.dispose();
	}
}

function testIdentity(name, filename) {
	return `${name}\0${path.resolve(filename)}`;
}

async function runProject(uriValue) {
	const uri = runURI(uriValue);
	const project = uri === undefined
		? projectManager?.firstProject()
		: projectManager?.projectForURI(uri);
	if (project === undefined) {
		void vscode.window.showErrorMessage("Open a TypeRB project folder before running main().");
		return;
	}
	if (!(await saveTypeRBDocuments(project))) {
		void vscode.window.showErrorMessage("Save TypeRB project files before running main().");
		return;
	}
	const running = debugSessions.get(project.configPath);
	if (running !== undefined) {
		if (typeof running.configuration.testFilter !== "string") {
			await running.customRequest("restart");
			return;
		}
		await vscode.debug.stopDebugging(running);
	}
	const configuration = debugConfigurationForProject(project, [], true);
	const started = await vscode.debug.startDebugging(project.workspaceFolder, configuration);
	if (!started) {
		void vscode.window.showErrorMessage(`Cannot start TypeRB project ${project.label}.`);
	}
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

async function saveTypeRBDocuments(project) {
	const documents = vscode.workspace.textDocuments.filter((document) =>
		document.languageId === "trb" &&
		document.isDirty &&
		projectManager?.projectForURI(document.uri) === project
	);
	const saved = await Promise.all(documents.map((document) => document.save()));
	return saved.every(Boolean);
}

async function stopProject(uriValue) {
	const uri = runURI(uriValue);
	const project = uri === undefined ? undefined : projectManager?.projectForURI(uri);
	const session = project === undefined
		? vscode.debug.activeDebugSession
		: debugSessions.get(project.configPath);
	if (session?.type !== "typerb") {
		return;
	}
	await vscode.debug.stopDebugging(session);
}

async function deactivate() {
	await Promise.all([...debugSessions.values()].map((session) => vscode.debug.stopDebugging(session)));
	debugSessions.clear();
	await projectManager?.stop();
	projectManager = undefined;
	typeRBTests = undefined;
	runCodeLensProvider = undefined;
}

class TypeRBDebugConfigurationProvider {
	constructor(manager) {
		this.manager = manager;
	}

	provideDebugConfigurations(folder) {
		return this.manager.projects
			.filter((project) => project.runnable && (folder === undefined || project.workspaceFolder === folder))
			.map((project) => debugConfigurationForProject(project));
	}

	async resolveDebugConfiguration(folder, configuration) {
		const project = this.selectProject(folder, configuration);
		if (project === undefined) {
			void vscode.window.showErrorMessage("TypeRB could not find a runnable project for this debug configuration.");
			return undefined;
		}
		if (!project.runnable) {
			void vscode.window.showErrorMessage("TypeRB browser projects are started by their browser toolchain, not trb run.");
			return undefined;
		}
		const programArgs = Array.isArray(configuration.args) ? configuration.args : [];
		const noDebug = configuration.noDebug === true;
		if (!noDebug && project.mode !== "go") {
			void vscode.window.showErrorMessage(`Source debugging is not yet available for mode: ${project.mode}. Use Run Without Debugging instead.`);
			return undefined;
		}
		if (!noDebug && !(await saveTypeRBDocuments(project))) {
			void vscode.window.showErrorMessage("Save TypeRB project files before starting the debugger.");
			return undefined;
		}
		const resolved = {
			...configuration,
			...debugConfigurationForProject(project, programArgs, noDebug),
			name: configuration.name || `TypeRB: ${project.label}`,
			env: configuration.env ?? {},
		};
		if (noDebug) {
			return resolved;
		}
		try {
			const program = typeof configuration.testFilter === "string"
				? await buildDebugTestExecutable(project)
				: await buildDebugExecutable(project);
			const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
			return {
				...resolved,
				mode: "exec",
				program,
				cwd: project.root,
				dlvToolPath: resolveDebugToolPath(settings.get("debug.go.path", "dlv"), project.workspaceFolder?.uri.fsPath),
			};
		} catch (error) {
			void vscode.window.showErrorMessage(`Cannot prepare TypeRB debugger: ${debugBuildError(error)}`);
			return undefined;
		}
	}

	selectProject(folder, configuration) {
		const configured = configuration.config ?? configuration.configPath;
		if (typeof configured === "string" && configured !== "") {
			const root = folder?.uri.fsPath;
			const workspaceRelative = root === undefined
				? configured
				: configured.replace(/^\$\{workspaceFolder\}[\\/]/, "");
			const configPath = path.isAbsolute(workspaceRelative) || root === undefined
				? workspaceRelative
				: path.resolve(root, workspaceRelative);
			return this.manager.projectForConfigPath(configPath);
		}
		const editor = vscode.window.activeTextEditor;
		const active = editor?.document.languageId === "trb"
			? this.manager.projectForURI(editor.document.uri)
			: undefined;
		if (active !== undefined && (folder === undefined || active.workspaceFolder === folder)) {
			return active;
		}
		return this.manager.projects.find((project) =>
			project.runnable && (folder === undefined || project.workspaceFolder === folder)
		);
	}
}

function debugConfigurationForProject(project, programArgs = [], noDebug = false) {
	const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
	const workspaceRoot = project.workspaceFolder?.uri.fsPath;
	const run = resolveRunOptions(
		{
			path: settings.get("server.path", "trb"),
			config: project.configPath,
		},
		workspaceRoot,
		programArgs
	);
	return {
		type: "typerb",
		request: "launch",
		name: `TypeRB: ${project.label}`,
		config: project.configPath,
		configPath: project.configPath,
		command: run.command,
		commandArgs: run.args,
		cwd: project.root,
		projectName: project.label,
		args: [...programArgs],
		noDebug,
		internalConsoleOptions: "openOnSessionStart",
	};
}

function debugConfigurationForTest(project, filter, file) {
	return {
		...debugConfigurationForProject(project),
		name: filter === "" ? `TypeRB Tests: ${project.label}` : `TypeRB Test: ${filter}`,
		testFilter: filter,
		env: {
			TRB_TEST_FILTER: filter,
			TRB_TEST_FILE: file,
			TRB_TEST_REPORTER: "human",
		},
	};
}

class TypeRBDebugAdapterFactory {
	constructor() {
		this.processes = new Map();
	}

	async createDebugAdapterDescriptor(session) {
		if (session.configuration.noDebug === true) {
			return new vscode.DebugAdapterInlineImplementation(new TypeRBDebugSession());
		}
		if (session.configuration.mode !== "exec" || typeof session.configuration.program !== "string") {
			throw new Error("TypeRB source debugging requires a prepared Go executable");
		}
		const process = new DlvDAPProcess(session.configuration.dlvToolPath || "dlv");
		this.processes.set(session.id, process);
		try {
			const port = await process.start();
			return new vscode.DebugAdapterServer(port, "127.0.0.1");
		} catch (error) {
			this.processes.delete(session.id);
			throw error;
		}
	}

	terminate(session) {
		const process = this.processes.get(session.id);
		process?.stop();
		this.processes.delete(session.id);
	}

	dispose() {
		for (const process of this.processes.values()) {
			process.stop();
		}
		this.processes.clear();
	}
}

async function buildDebugExecutable(project) {
	const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
	const workspaceRoot = project.workspaceFolder?.uri.fsPath;
	const run = resolveRunOptions({ path: settings.get("server.path", "trb"), config: project.configPath }, workspaceRoot);
	const filename = process.platform === "win32" ? "app.exe" : "app";
	const program = path.join(project.root, ".trb", "debug", filename);
	try {
		await executeFile(run.command, ["build", "--config", project.configPath, "--compile", "--debug", "--outfile", program], {
			cwd: project.root,
			maxBuffer: 10 * 1024 * 1024,
		});
	} catch (error) {
		error.debugOutput = [error.stdout, error.stderr].filter(Boolean).join("\n");
		throw error;
	}
	return program;
}

async function buildDebugTestExecutable(project) {
	const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
	const workspaceRoot = project.workspaceFolder?.uri.fsPath;
	const run = resolveRunOptions({ path: settings.get("server.path", "trb"), config: project.configPath }, workspaceRoot);
	const filename = process.platform === "win32" ? "tests.exe" : "tests";
	const program = path.join(project.root, ".trb", "debug", filename);
	try {
		await executeFile(run.command, ["test", "--config", project.configPath, "--compile", "--debug", "--outfile", program], {
			cwd: project.root,
			maxBuffer: 10 * 1024 * 1024,
		});
	} catch (error) {
		error.debugOutput = [error.stdout, error.stderr].filter(Boolean).join("\n");
		throw error;
	}
	return program;
}

function resolveDebugToolPath(value, workspaceRoot) {
	if (value === "" || path.isAbsolute(value) || workspaceRoot === undefined || (!value.includes("/") && !value.includes("\\"))) {
		return value || "dlv";
	}
	return path.resolve(workspaceRoot, value);
}

function debugBuildError(error) {
	const output = typeof error.debugOutput === "string" ? error.debugOutput.trim() : "";
	return output || error.message;
}

module.exports = { activate, deactivate };
