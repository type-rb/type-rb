"use strict";

const { readFile } = require("node:fs/promises");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient } = require("vscode-languageclient/node");
const { TypeRBDebugSession } = require("./debug-session");
const { excludeGeneratedProjects, projectForPath, projectPaths } = require("./project-options");
const { resolveRunOptions, resolveServerOptions, runCodeLensTitle } = require("./server-options");

let projectManager;
let runCodeLensProvider;
const debugSessions = new Map();

async function activate(context) {
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
			createDebugAdapterDescriptor() {
				return new vscode.DebugAdapterInlineImplementation(new TypeRBDebugSession());
			}
		})
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
	const configuration = debugConfigurationForProject(project);
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

	resolveDebugConfiguration(folder, configuration) {
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
		return {
			...configuration,
			...debugConfigurationForProject(project, programArgs),
			name: configuration.name || `TypeRB: ${project.label}`,
			env: configuration.env ?? {},
		};
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

function debugConfigurationForProject(project, programArgs = []) {
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
		internalConsoleOptions: "openOnSessionStart",
	};
}

module.exports = { activate, deactivate };
