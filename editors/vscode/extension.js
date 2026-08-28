"use strict";

const { readFile } = require("node:fs/promises");
const { execFile } = require("node:child_process");
const { promisify } = require("node:util");
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient, State } = require("vscode-languageclient/node");
const { TypeRBDebugSession, TypeRBProcess } = require("./debug-session");
const { DlvDAPProcess } = require("./go-debug-adapter");
const { containsPath, excludeGeneratedProjects, literalGlobPattern, projectForPath, projectPaths } = require("./project-options");
const { DebugArtifactSessions, DebugArtifactStore, reserveDebugArtifact } = require("./debug-artifacts");
const { resolveRunOptions, resolveServerOptions, resolveStandaloneDebugBuildOptions, runCodeLensTitle } = require("./server-options");
const { transitionStandaloneClient } = require("./standalone-client-state");
const { decodeTestEvent } = require("./test-events");

let projectManager;
let runCodeLensProvider;
let typeRBTests;
let debugArtifacts;
let debugAdapterFactory;
const debugSessions = new Map();
const executeFile = promisify(execFile);
const clearDebugConsoleCommand = "workbench.debug.panel.action.clearReplAction";

async function activate(context) {
	debugArtifacts = new DebugArtifactStore();
	debugAdapterFactory = new TypeRBDebugAdapterFactory(debugArtifacts);
	const factory = debugAdapterFactory;
	context.subscriptions.push(
		vscode.commands.registerCommand("typerb.runProject", runProject),
		vscode.commands.registerCommand("typerb.debugFile", debugFile),
		vscode.commands.registerCommand("typerb.runTest", (uri, fullName) => typeRBTests?.runByName(uri, fullName)),
		vscode.commands.registerCommand("typerb.debugTest", (uri, fullName) => typeRBTests?.debugByName(uri, fullName)),
		vscode.commands.registerCommand("typerb.stopProject", stopProject),
		vscode.debug.onDidStartDebugSession((session) => {
			const key = debugSessionKey(session);
			if (key !== undefined) {
				debugSessions.set(key, session);
				runCodeLensProvider?.refresh();
			}
		}),
		vscode.debug.onDidTerminateDebugSession((session) => {
			const key = debugSessionKey(session);
			if (key !== undefined) {
				if (debugSessions.get(key) === session) {
					debugSessions.delete(key);
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
		vscode.workspace.onDidOpenTextDocument((document) => void projectManager?.openDocument(document)),
		vscode.workspace.onDidChangeTextDocument((event) => void projectManager?.changeDocument(event)),
		vscode.workspace.onDidSaveTextDocument((document) => void projectManager?.saveDocument(document)),
		vscode.workspace.onDidCloseTextDocument((document) => void projectManager?.closeDocument(document))
	);
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
				return factory.createDebugAdapterDescriptor(session);
			}
		}),
		factory,
		vscode.debug.onDidTerminateDebugSession((session) => {
			void factory.terminate(session).catch((error) => {
				console.error("Cannot clean up TypeRB debug session:", error);
			});
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
		this.standalones = new Map();
		this.nextStandaloneID = 0;
		this.documentQueue = Promise.resolve();
		this.standaloneDiagnostics = vscode.languages.createDiagnosticCollection("typerb-standalone");
	}

	async start() {
		this.projects = await discoverProjects();
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
		await Promise.all(vscode.workspace.textDocuments.map((document) => this.openDocument(document)));
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
					pattern: languageClientRelativePattern(project.sourceRoot, "**/*.trb")
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

	openDocument(document) {
		const queued = this.documentQueue.then(() => this.openDocumentNow(document));
		this.documentQueue = queued.catch(() => {});
		return queued;
	}

	async openDocumentNow(document) {
		if (document.languageId !== "trb" || document.uri.scheme !== "file") {
			return;
		}
		const filename = path.resolve(document.uri.fsPath);
		if (
			projectForPath(this.projects, filename) !== undefined ||
			this.projects.some((project) => containsPath(project.root, filename)) ||
			this.standalones.has(filename)
		) {
			return;
		}
		await this.reconcileStandaloneDocuments();
		const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
		const settings = vscode.workspace.getConfiguration("typerb", document.uri);
		const mode = settings.get("standalone.mode", "go");
		const runtime = settings.get("standalone.typescript.runtime", "node");
		const root = path.dirname(filename);
		const project = {
			standalone: true,
			filename,
			configUri: document.uri,
			workspaceFolder,
			root,
			sourceRoot: root,
			outputRoot: path.join(root, "build"),
			label: path.basename(filename),
			mode,
			runtime,
			runnable: true,
			files: new Set([filename]),
			forwardedDocuments: new Set(),
			diagnostics: new Map(),
		};
		this.standalones.set(filename, project);
		this.refreshStandaloneDiagnostics();
		try {
			const workspaceRoot = workspaceFolder?.uri.fsPath;
			const server = resolveServerOptions({
				path: settings.get("server.path", "trb"),
				config: "",
				file: filename,
				mode,
				runtime,
			}, workspaceRoot);
			project.watcher = vscode.workspace.createFileSystemWatcher(
				new vscode.RelativePattern(root, "**/*.trb")
			);
			project.client = new LanguageClient(
				`typerb-standalone-${this.nextStandaloneID++}`,
				`TypeRB Language Server (${project.label})`,
				{
					command: server.command,
					args: server.args,
					options: { cwd: root }
				},
				{
					documentSelector: [{
						scheme: "file",
						language: "trb",
						pattern: languageClientRelativePattern(root, literalGlobPattern(path.basename(filename)))
					}],
					synchronize: { fileEvents: project.watcher },
					middleware: {
						handleDiagnostics: (uri, diagnostics, next) => {
							next(uri, []);
							this.cacheStandaloneDiagnostics(project, uri, diagnostics);
						},
						provideCodeLenses() {
							return [];
						}
					}
				}
			);
			project.testCommand = server.command;
			project.clientStarted = false;
			project.clientRunning = false;
			project.clientState = project.client.onDidChangeState((event) => {
				const running = event.newState === State.Running;
				const replay = transitionStandaloneClient(project, running);
				if (!running) {
					project.diagnostics.clear();
					this.refreshStandaloneDiagnostics();
				}
				if (!replay) {
					return;
				}
				void (async () => {
					await this.refreshStandaloneFiles(project);
					await this.reconcileStandaloneDocuments();
				})().catch((error) => {
					console.error(`Cannot replay TypeRB helper documents for ${project.filename}:`, error);
				});
			});
			project.fileRootNotification = project.client.onNotification(
				"typerb/fileRootFilesChanged",
				(params) => void this.updateStandaloneFiles(project, params?.files)
			);
			await project.client.start();
			await this.refreshStandaloneFiles(project);
			await this.reconcileStandaloneDocuments();
		} catch (error) {
			this.standalones.delete(filename);
			this.refreshStandaloneDiagnostics();
			project.clientState?.dispose();
			project.fileRootNotification?.dispose();
			project.watcher?.dispose();
			project.client = undefined;
			void vscode.window.showErrorMessage(`Cannot start TypeRB standalone file ${filename}: ${error.message}`);
		}
	}

	async changeDocument(event) {
		const document = event.document;
		if (document.languageId !== "trb" || document.uri.scheme !== "file") {
			return;
		}
		const filename = path.resolve(document.uri.fsPath);
		await Promise.all([...this.standalones.values()].map(async (project) => {
			if (project.filename === filename || !project.forwardedDocuments.has(filename)) {
				return;
			}
			await project.client?.sendNotification("textDocument/didChange", {
				textDocument: { uri: document.uri.toString(), version: document.version },
				contentChanges: event.contentChanges.map(textDocumentContentChange)
			});
		}));
	}

	async saveDocument(document) {
		if (document.languageId !== "trb" || document.uri.scheme !== "file") {
			return;
		}
		const filename = path.resolve(document.uri.fsPath);
		await Promise.all([...this.standalones.values()].map(async (project) => {
			if (project.filename === filename || !project.forwardedDocuments.has(filename)) {
				return;
			}
			await project.client?.sendNotification("textDocument/didSave", {
				textDocument: { uri: document.uri.toString() },
				text: document.getText()
			});
		}));
	}

	closeDocument(document) {
		const queued = this.documentQueue.then(() => this.closeDocumentNow(document));
		this.documentQueue = queued.catch(() => {});
		return queued;
	}

	async closeDocumentNow(document) {
		if (document.uri.scheme !== "file") {
			return;
		}
		const filename = path.resolve(document.uri.fsPath);
		const project = this.standalones.get(filename);
		if (project !== undefined) {
			this.standalones.delete(filename);
			this.refreshStandaloneDiagnostics();
		}
		await Promise.all([...this.standalones.values()].map((candidate) =>
			this.closeForwardedDocument(candidate, document)
		));
		if (project === undefined) {
			return;
		}
		project.fileRootNotification?.dispose();
		project.clientState?.dispose();
		project.watcher?.dispose();
		if (project.client !== undefined) {
			await project.client.stop();
		}
		await this.reconcileStandaloneDocuments();
		for (const candidate of vscode.workspace.textDocuments) {
			await this.openDocumentNow(candidate);
		}
	}

	projectForURI(uri) {
		if (uri?.scheme !== "file") {
			return undefined;
		}
		const filename = path.resolve(uri.fsPath);
		return projectForPath(this.projects, filename)
			?? this.standalones.get(filename)
			?? this.standaloneProjectsForPath(filename)[0];
	}

	diagnosticProjectForURI(uri) {
		return this.projectForURI(uri);
	}

	cacheStandaloneDiagnostics(project, uri, diagnostics) {
		if (this.standalones.get(project.filename) !== project) {
			return;
		}
		project.diagnostics.set(uri.toString(), { uri, diagnostics });
		this.publishStandaloneDiagnostics(uri);
	}

	publishStandaloneDiagnostics(uri) {
		const owner = this.diagnosticProjectForURI(uri);
		const diagnostics = owner?.standalone
			? owner.diagnostics.get(uri.toString())?.diagnostics
			: undefined;
		if (diagnostics === undefined || diagnostics.length === 0) {
			this.standaloneDiagnostics.delete(uri);
		} else {
			this.standaloneDiagnostics.set(uri, diagnostics);
		}
	}

	refreshStandaloneDiagnostics() {
		const uris = new Map();
		this.standaloneDiagnostics.forEach((uri) => uris.set(uri.toString(), uri));
		for (const project of this.standalones.values()) {
			for (const cached of project.diagnostics.values()) {
				uris.set(cached.uri.toString(), cached.uri);
			}
		}
		for (const uri of uris.values()) {
			this.publishStandaloneDiagnostics(uri);
		}
	}

	standaloneProjectsForPath(filename) {
		const resolved = path.resolve(filename);
		return [...this.standalones.values()].filter((project) => project.files.has(resolved));
	}

	async refreshStandaloneFiles(project) {
		if (project.client === undefined) {
			return;
		}
		try {
			await this.updateStandaloneFiles(project, await project.client.sendRequest("typerb/fileRootFiles", {}));
		} catch {
			// Older language servers only report the standalone entry file.
		}
	}

	async updateStandaloneFiles(project, values) {
		if (!Array.isArray(values) || this.standalones.get(project.filename) !== project) {
			return;
		}
		project.files = new Set([
			project.filename,
			...values.filter((value) => typeof value === "string").map((value) => path.resolve(value))
		]);
		this.refreshStandaloneDiagnostics();
		await this.reconcileStandaloneDocuments();
	}

	async reconcileStandaloneDocuments() {
		const documents = vscode.workspace.textDocuments.filter((document) =>
			document.languageId === "trb" && document.uri.scheme === "file"
		);
		await Promise.all([...this.standalones.values()].flatMap((project) => documents.map(async (document) => {
			const filename = path.resolve(document.uri.fsPath);
			const shouldForward = project.filename !== filename && containsPath(project.root, filename);
			if (shouldForward && !project.forwardedDocuments.has(filename) && project.clientRunning) {
				project.forwardedDocuments.add(filename);
				try {
					await project.client.sendNotification("textDocument/didOpen", {
						textDocument: {
							uri: document.uri.toString(),
							languageId: document.languageId,
							version: document.version,
							text: document.getText()
						}
					});
				} catch (error) {
					project.forwardedDocuments.delete(filename);
					throw error;
				}
			} else if (!shouldForward && project.forwardedDocuments.has(filename)) {
				await this.closeForwardedDocument(project, document);
			}
		})));
	}

	async closeForwardedDocument(project, document) {
		const filename = path.resolve(document.uri.fsPath);
		if (!project.forwardedDocuments.delete(filename)) {
			return;
		}
		await project.client?.sendNotification("textDocument/didClose", {
			textDocument: { uri: document.uri.toString() }
		});
	}

	firstProject() {
		return this.allProjects().find((project) => project.runnable);
	}

	projectForConfigPath(configPath) {
		const resolved = path.resolve(configPath);
		return this.projects.find((project) => project.configPath === resolved);
	}

	allProjects() {
		return [...this.projects, ...this.standalones.values()];
	}

	async stop() {
		await this.documentQueue;
		const projects = this.allProjects();
		this.projects = [];
		this.standalones.clear();
		await Promise.all(projects.map(async (project) => {
			project.clientState?.dispose();
			project.fileRootNotification?.dispose();
			project.watcher?.dispose();
			if (project.client !== undefined) {
				await project.client.stop();
			}
		}));
		this.standaloneDiagnostics.dispose();
	}
}

function languageClientRelativePattern(root, pattern) {
	// LanguageClient converts protocol selectors back to VS Code selectors.
	// Supplying the protocol shape preserves the base URI; a vscode.RelativePattern
	// instance is otherwise reduced to an unscoped language selector.
	return { baseUri: vscode.Uri.file(root).toString(), pattern };
}

function textDocumentContentChange(change) {
	if (change.range === undefined) {
		return { text: change.text };
	}
	return {
		range: {
			start: { line: change.range.start.line, character: change.range.start.character },
			end: { line: change.range.end.line, character: change.range.end.character }
		},
		rangeLength: change.rangeLength,
		text: change.text
	};
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
		if (project.standalone && path.resolve(document.uri.fsPath) !== project.filename) {
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
		const session = debugSessions.get(projectSessionKey(project));
		const running = session !== undefined && typeof session.configuration.testFilter !== "string";
		return items.flatMap((item) => {
			const range = new vscode.Range(
				item.range.start.line,
				item.range.start.character,
				item.range.end.line,
				item.range.end.character
			);
			if (item.command?.command !== "typerb.runProject") {
				return [new vscode.CodeLens(range, item.command)];
			}
			const run = new vscode.CodeLens(range, {
				title: runCodeLensTitle(running),
				command: item.command.command,
				arguments: item.command.arguments
			});
			if (!project.standalone || project.mode !== "go") {
				return [run];
			}
			return [run, new vscode.CodeLens(range, {
				title: "$(debug-alt) Debug",
				command: "typerb.debugFile",
				arguments: item.command.arguments
			})];
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
		if (project?.client === undefined || project.standalone) {
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
		const names = invocation.tests.map((item) => this.metadata.get(item.id).fullName);
		const configuration = debugConfigurationForTest(invocation.project, invocation.filter, invocation.file, names);
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
			const names = invocation.tests.map((item) => this.metadata.get(item.id).fullName);
			args.push("--test-name-pattern", exactTestNamePattern(names));
		}
		if (invocation.file !== "") {
			args.push(invocation.file);
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
		void vscode.window.showErrorMessage("Open a TypeRB project or standalone file before running main().");
		return;
	}
	if (!(await saveTypeRBDocuments(project))) {
		void vscode.window.showErrorMessage("Save TypeRB files before running main().");
		return;
	}
	const running = debugSessions.get(projectSessionKey(project));
	if (running !== undefined) {
		if (typeof running.configuration.testFilter !== "string") {
			await vscode.commands.executeCommand(clearDebugConsoleCommand);
			await running.customRequest("restart");
			return;
		}
		await vscode.debug.stopDebugging(running);
	}
	const configuration = debugConfigurationForProject(project, [], true);
	const started = await vscode.debug.startDebugging(project.workspaceFolder, configuration);
	if (!started) {
		void vscode.window.showErrorMessage(`Cannot start TypeRB ${project.standalone ? "file" : "project"} ${project.label}.`);
	}
}

async function debugFile(uriValue) {
	const uri = runURI(uriValue);
	const project = uri === undefined ? undefined : projectManager?.projectForURI(uri);
	if (
		project?.standalone !== true ||
		uri.scheme !== "file" ||
		path.resolve(uri.fsPath) !== project.filename
	) {
		void vscode.window.showErrorMessage("Open a standalone TypeRB file with main() before debugging it.");
		return;
	}
	if (project.mode !== "go") {
		void vscode.window.showErrorMessage(`Source debugging is not yet available for mode: ${project.mode}.`);
		return;
	}
	if (!(await saveTypeRBDocuments(project))) {
		void vscode.window.showErrorMessage("Save the standalone TypeRB file before debugging it.");
		return;
	}
	const running = debugSessions.get(projectSessionKey(project));
	if (running !== undefined) {
		await vscode.debug.stopDebugging(running);
	}
	const started = await vscode.debug.startDebugging(
		project.workspaceFolder,
		debugConfigurationForProject(project)
	);
	if (!started) {
		void vscode.window.showErrorMessage(`Cannot debug TypeRB file ${project.label}.`);
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
	if (project.standalone) {
		await projectManager?.refreshStandaloneFiles(project);
	}
	const documents = vscode.workspace.textDocuments.filter((document) =>
		document.languageId === "trb" &&
		document.isDirty &&
		(project.standalone
			? document.uri.scheme === "file" && project.files.has(path.resolve(document.uri.fsPath))
			: projectManager?.projectForURI(document.uri) === project)
	);
	const saved = await Promise.all(documents.map((document) => document.save()));
	return saved.every(Boolean);
}

async function stopProject(uriValue) {
	const uri = runURI(uriValue);
	const project = uri === undefined ? undefined : projectManager?.projectForURI(uri);
	const session = project === undefined
		? vscode.debug.activeDebugSession
		: debugSessions.get(projectSessionKey(project));
	if (session?.type !== "typerb") {
		return;
	}
	await vscode.debug.stopDebugging(session);
}

async function deactivate() {
	await Promise.all([...debugSessions.values()].map((session) => vscode.debug.stopDebugging(session)));
	debugSessions.clear();
	try {
		await debugAdapterFactory?.stop();
	} catch (error) {
		console.error("Cannot fully stop the TypeRB debugger during deactivation:", error);
	}
	try {
		await projectManager?.stop();
	} catch (error) {
		console.error("Cannot fully stop TypeRB language servers during deactivation:", error);
	}
	try {
		await debugArtifacts?.dispose();
	} catch (error) {
		console.error("Cannot fully remove TypeRB debug artifacts during deactivation:", error);
	}
	debugAdapterFactory = undefined;
	projectManager = undefined;
	debugArtifacts = undefined;
	typeRBTests = undefined;
	runCodeLensProvider = undefined;
}

class TypeRBDebugConfigurationProvider {
	constructor(manager) {
		this.manager = manager;
	}

	provideDebugConfigurations(folder) {
		return this.manager.allProjects()
			.filter((project) => project.runnable && (folder === undefined || project.workspaceFolder === folder))
			.map((project) => debugConfigurationForProject(
				project,
				[],
				project.standalone && project.mode !== "go"
			));
	}

	async resolveDebugConfiguration(folder, configuration, token) {
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
		if (!(await saveTypeRBDocuments(project))) {
			void vscode.window.showErrorMessage("Save TypeRB files before starting Run and Debug.");
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
			const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
			if (project.standalone && typeof configuration.testFilter !== "string") {
				const artifact = reserveDebugArtifact(process.platform === "win32" ? "app.exe" : "app");
				return {
					...resolved,
					mode: "exec",
					standaloneDebugBuild: true,
					program: artifact.program,
					debugArtifactDirectory: artifact.directory,
					dlvToolPath: resolveDebugToolPath(settings.get("debug.go.path", "dlv"), project.workspaceFolder?.uri.fsPath),
				};
			}
			const prepared = typeof configuration.testFilter === "string"
				? { program: await buildDebugTestExecutable(project) }
				: { program: await buildDebugExecutable(project) };
			return {
				...resolved,
				...prepared,
				mode: "exec",
				cwd: project.root,
				dlvToolPath: resolveDebugToolPath(settings.get("debug.go.path", "dlv"), project.workspaceFolder?.uri.fsPath),
			};
		} catch (error) {
			if (token?.isCancellationRequested || error.name === "AbortError") {
				return undefined;
			}
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
		if (typeof configuration.file === "string" && configuration.file !== "") {
			return this.manager.projectForURI(vscode.Uri.file(configuration.file));
		}
		const editor = vscode.window.activeTextEditor;
		const active = editor?.document.languageId === "trb"
			? this.manager.projectForURI(editor.document.uri)
			: undefined;
		if (active !== undefined && (folder === undefined || active.workspaceFolder === folder)) {
			return active;
		}
		return this.manager.allProjects().find((project) =>
			project.runnable && (folder === undefined || project.workspaceFolder === folder)
		);
	}
}

function debugConfigurationForProject(project, programArgs = [], noDebug = false) {
	const settings = vscode.workspace.getConfiguration("typerb", project.configUri);
	const workspaceRoot = project.workspaceFolder?.uri.fsPath;
	const run = resolveRunOptions(
		project.standalone ? {
			path: settings.get("server.path", "trb"),
			config: "",
			file: project.filename,
			mode: project.mode,
			runtime: project.runtime,
		} : {
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
		...(project.standalone ? {
			file: project.filename,
			mode: project.mode,
			runtime: project.runtime,
		} : {
			config: project.configPath,
			configPath: project.configPath,
		}),
		projectKey: projectSessionKey(project),
		command: run.command,
		commandArgs: run.args,
		cwd: project.root,
		projectName: project.label,
		args: [...programArgs],
		noDebug,
		internalConsoleOptions: "openOnSessionStart",
	};
}

function projectSessionKey(project) {
	return path.resolve(project.standalone ? project.filename : project.configPath);
}

function debugSessionKey(session) {
	if (session.type !== "typerb") {
		return undefined;
	}
	const value = session.configuration.projectKey ?? session.configuration.configPath ?? session.configuration.file;
	return typeof value === "string" && value !== "" ? path.resolve(value) : undefined;
}

function exactTestNamePattern(names) {
	return `^(${[...new Set(names)].map((name) => name.replace(/[\\.^$|?*+()[\]{}]/g, "\\$&")).join("|")})$`;
}

function debugConfigurationForTest(project, filter, file, names) {
	return {
		...debugConfigurationForProject(project),
		name: filter === "" ? `TypeRB Tests: ${project.label}` : `TypeRB Test: ${filter}`,
		testFilter: filter,
		env: {
			TRB_TEST_NAMES: names.length === 0 ? "" : JSON.stringify(names),
			TRB_TEST_FILE: file,
			TRB_TEST_REPORTER: "human",
		},
	};
}

class TypeRBDebugAdapterFactory {
	constructor(artifacts) {
		this.artifactSessions = new DebugArtifactSessions(artifacts);
		this.processes = new Map();
	}

	async createDebugAdapterDescriptor(session) {
		if (session.configuration.noDebug === true) {
			return new vscode.DebugAdapterInlineImplementation(new TypeRBDebugSession());
		}
		const standaloneBuild = session.configuration.standaloneDebugBuild === true;
		if (
			session.configuration.mode !== "exec" ||
			(!standaloneBuild && typeof session.configuration.program !== "string")
		) {
			throw new Error("TypeRB source debugging requires a prepared Go executable");
		}
		if (standaloneBuild) {
			try {
				const artifact = await this.artifactSessions.prepare(
					session.id,
					process.platform === "win32" ? "app.exe" : "app",
					(program, signal) => buildStandaloneDebugExecutable(
						session.configuration,
						program,
						signal
					),
					{
						directory: session.configuration.debugArtifactDirectory,
						program: session.configuration.program
					}
				);
				session.configuration.program = artifact.program;
				session.configuration.debugArtifactDirectory = artifact.directory;
			} catch (error) {
				if (error.name === "AbortError") {
					throw error;
				}
				throw new Error(`Cannot prepare TypeRB debugger: ${debugBuildError(error)}`, { cause: error });
			}
		}
		const debuggerProcess = new DlvDAPProcess(session.configuration.dlvToolPath || "dlv");
		this.processes.set(session.id, debuggerProcess);
		try {
			const port = await debuggerProcess.start();
			return new vscode.DebugAdapterServer(port, "127.0.0.1");
		} catch (error) {
			this.processes.delete(session.id);
			try {
				await this.releaseAfterProcess(session, debuggerProcess);
			} catch (cleanupError) {
				console.error("Cannot clean up failed TypeRB debugger startup:", cleanupError);
			}
			throw error;
		}
	}

	async terminate(session) {
		const process = this.processes.get(session.id);
		this.processes.delete(session.id);
		await this.releaseAfterProcess(session, process);
	}

	async releaseAfterProcess(session, process) {
		const errors = [];
		try {
			await process?.stop();
		} catch (error) {
			errors.push(error);
		}
		try {
			await this.artifactSessions.release(session.id);
		} catch (error) {
			errors.push(error);
		}
		if (errors.length > 0) {
			throw new AggregateError(errors, "Cannot fully clean up the TypeRB debug session");
		}
	}

	async stop() {
		const processes = [...this.processes.values()];
		this.processes.clear();
		const results = await Promise.allSettled([
			...processes.map((process) => process.stop()),
			this.artifactSessions.dispose()
		]);
		const errors = results.filter((result) => result.status === "rejected").map((result) => result.reason);
		if (errors.length > 0) {
			throw new AggregateError(errors, "Cannot fully stop the TypeRB debugger");
		}
	}

	dispose() {
		void this.stop().catch((error) => {
			console.error("Cannot dispose TypeRB debugger:", error);
		});
	}
}

async function buildStandaloneDebugExecutable(configuration, program, signal) {
	const build = resolveStandaloneDebugBuildOptions({
		path: configuration.command,
		file: configuration.file,
	}, undefined, program);
	await executeDebugBuild(build.command, build.args, {
		cwd: configuration.cwd,
		maxBuffer: 10 * 1024 * 1024,
	}, signal);
}

async function executeDebugBuild(command, args, options, signal) {
	if (signal.aborted) {
		throw debugBuildAbortError();
	}
	const runner = new TypeRBProcess({
		command,
		args,
		cwd: options.cwd,
		env: process.env,
	});
	let stdout = "";
	let stderr = "";
	let failure;
	const maxBuffer = options.maxBuffer ?? 10 * 1024 * 1024;
	const collect = (category, output) => {
		if (category === "stdout") stdout += output;
		else stderr += output;
		if (failure === undefined && Buffer.byteLength(stdout) + Buffer.byteLength(stderr) > maxBuffer) {
			failure = new Error(`TypeRB debug build output exceeded ${maxBuffer} bytes`);
			stopRunner();
		}
	};
	let rejectStopFailure;
	const stopFailure = new Promise((_, reject) => {
		rejectStopFailure = reject;
	});
	const stopRunner = () => {
		void runner.stop().catch(rejectStopFailure);
	};
	const exited = new Promise((resolve) => {
		runner.on("output", collect);
		runner.once("error", (error) => {
			failure = error;
			stopRunner();
		});
		runner.once("exit", (code, exitSignal) => resolve({ code, signal: exitSignal }));
	});
	const completed = Promise.race([exited, stopFailure]);
	const cancel = () => stopRunner();
	signal.addEventListener("abort", cancel, { once: true });
	try {
		runner.start();
		const result = await completed;
		if (signal.aborted) {
			await runner.stop();
			throw debugBuildAbortError();
		}
		if (failure !== undefined) {
			throw failure;
		}
		if (result.code !== 0) {
			throw new Error(`TypeRB debug build exited with status ${result.code}`);
		}
	} catch (error) {
		await runner.stop().catch(() => {});
		error.debugOutput = [stdout, stderr].filter(Boolean).join("\n");
		throw error;
	} finally {
		signal.removeEventListener("abort", cancel);
	}
}

function debugBuildAbortError() {
	const error = new Error("TypeRB debug build was cancelled");
	error.name = "AbortError";
	return error;
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
