"use strict";

const { readFile } = require("node:fs/promises");
const { execFile } = require("node:child_process");
const { promisify } = require("node:util");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");
const { TypeRBDebugSession } = require("./debug-session");
const { DlvDAPProcess } = require("./go-debug-adapter");
const { excludeGeneratedProjects, projectForPath, projectPaths } = require("./project-options");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("./server-options");

let projectManager;
let runCodeLensProvider;
const debugSessions = new Map();
const executeFile = promisify(execFile);

async function activate(context) {
	const debugAdapterFactory = new TypeRBDebugAdapterFactory();
	context.subscriptions.push(
		vscode.commands.registerCommand("typerb.runProject", runProject),
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
		const running = debugSessions.has(project.configPath);
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
		await running.customRequest("restart");
		return;
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
			const program = await buildDebugExecutable(project);
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
