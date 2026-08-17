"use strict";

const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const test = require("node:test");
const { TypeRBDebugSession, TypeRBProcess, formatCommand } = require("../debug-session");

class FakeRunner extends EventEmitter {
	constructor(specification) {
		super();
		this.specification = specification;
		this.started = false;
		this.stopped = false;
	}

	start() {
		this.started = true;
		this.emit("started", 4242);
	}

	async stop() {
		this.stopped = true;
		this.emit("exit", 0, "SIGINT");
	}

	exit(code) {
		this.emit("exit", code, null);
	}
}

function request(sequence, command, args = {}) {
	return { seq: sequence, type: "request", command, arguments: args };
}

function tick() {
	return new Promise((resolve) => setImmediate(resolve));
}

test("launches a TypeRB process through a DAP session and publishes its lifecycle", async () => {
	const runners = [];
	const messages = [];
	const session = new TypeRBDebugSession({
		processFactory(specification) {
			const runner = new FakeRunner(specification);
			runners.push(runner);
			return runner;
		},
	});
	session.onDidSendMessage((message) => messages.push(message));
	session.handleMessage(request(1, "initialize", {
		adapterID: "typerb",
		linesStartAt1: true,
		columnsStartAt1: true,
		pathFormat: "path",
	}));
	assert.equal(messages[0].type, "response");
	assert.equal(messages[0].body.supportsRestartRequest, true);
	assert.equal(messages[0].body.supportsTerminateRequest, true);
	assert.equal(messages[1].event, "initialized");

	session.handleMessage(request(2, "launch", {
		command: "/workspace/bin/trb",
		commandArgs: ["run", "--config", "/workspace/trbconfig.jsonc"],
		cwd: "/workspace",
		projectName: "todo-api",
		env: { PORT: "4000" },
	}));
	assert.equal(runners.length, 0, "launch waits for configurationDone");
	session.handleMessage(request(3, "configurationDone"));
	assert.equal(runners.length, 1);
	assert.equal(runners[0].started, true);
	assert.deepEqual(
		{
			command: runners[0].specification.command,
			args: runners[0].specification.args,
			cwd: runners[0].specification.cwd,
			port: runners[0].specification.env.PORT,
		},
		{
			command: "/workspace/bin/trb",
			args: ["run", "--config", "/workspace/trbconfig.jsonc"],
			cwd: "/workspace",
			port: "4000",
		}
	);
	assert.ok(messages.some((message) => message.event === "process" && message.body.systemProcessId === 4242));
	assert.ok(messages.some((message) => message.event === "output" && message.body.output.includes("starting todo-api")));
	assert.ok(messages.some((message) => message.event === "output" && message.body.output.includes("Use Stop or Shift+F5")));

	runners[0].emit("output", "stdout", "ready\n");
	assert.ok(messages.some((message) => message.event === "output" && message.body.category === "stdout" && message.body.output === "ready\n"));
	runners[0].exit(0);
	assert.ok(messages.some((message) => message.event === "exited" && message.body.exitCode === 0));
	assert.ok(messages.some((message) => message.event === "terminated"));
	await tick();
});

test("restarts a TypeRB process without terminating the debug session", async () => {
	const runners = [];
	const messages = [];
	const session = new TypeRBDebugSession({
		processFactory(specification) {
			const runner = new FakeRunner(specification);
			runners.push(runner);
			return runner;
		},
	});
	session.onDidSendMessage((message) => messages.push(message));
	session.handleMessage(request(1, "initialize", { linesStartAt1: true, columnsStartAt1: true, pathFormat: "path" }));
	session.handleMessage(request(2, "launch", {
		command: "trb",
		commandArgs: ["run"],
		cwd: "/workspace",
		projectName: "app",
	}));
	session.handleMessage(request(3, "configurationDone"));
	messages.length = 0;
	session.handleMessage(request(4, "restart"));
	await tick();
	assert.equal(runners.length, 2);
	assert.equal(runners[0].stopped, true);
	assert.equal(runners[1].started, true);
	assert.ok(messages.some((message) => message.type === "response" && message.command === "restart" && message.success));
	assert.equal(messages.some((message) => message.event === "terminated"), false);
});

test("stops a TypeRB process tree and forwards output", async () => {
	const child = new EventEmitter();
	child.pid = 77;
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	const signals = [];
	let spawnCall;
	const processRunner = new TypeRBProcess(
		{ command: "trb", args: ["run"], cwd: "/workspace", env: { MODE: "test" } },
		{
			spawnProcess(command, args, options) {
				spawnCall = { command, args, options };
				return child;
			},
			signalProcess(processValue, signal) {
				signals.push({ processValue, signal });
				processValue.emit("close", 0, signal);
			},
		}
	);
	const output = [];
	processRunner.on("output", (category, text) => output.push({ category, text }));
	processRunner.start();
	child.emit("spawn");
	child.stdout.emit("data", Buffer.from("hello\n"));
	child.stderr.emit("data", Buffer.from("warning\n"));
	await processRunner.stop();
	assert.equal(spawnCall.command, "trb");
	assert.deepEqual(spawnCall.args, ["run"]);
	assert.equal(spawnCall.options.cwd, "/workspace");
	assert.deepEqual(output, [
		{ category: "stdout", text: "hello\n" },
		{ category: "stderr", text: "warning\n" },
	]);
	assert.deepEqual(signals, [{ processValue: child, signal: "SIGINT" }]);
});

test("waits for process creation before stopping an immediate launch", async () => {
	const child = new EventEmitter();
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	const signals = [];
	const processRunner = new TypeRBProcess(
		{ command: "trb", args: ["run"], cwd: "/workspace", env: {} },
		{
			spawnProcess() {
				return child;
			},
			signalProcess(processValue, signal) {
				signals.push(signal);
				processValue.emit("close", 0, signal);
			},
		}
	);
	processRunner.start();
	const stopped = processRunner.stop();
	await tick();
	assert.deepEqual(signals, []);
	child.pid = 88;
	child.emit("spawn");
	await stopped;
	assert.deepEqual(signals, ["SIGINT"]);
});

test("bounds process-tree shutdown when a child never exits", async () => {
	const child = new EventEmitter();
	child.pid = 99;
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	const signals = [];
	const processRunner = new TypeRBProcess(
		{ command: "trb", args: ["build"], cwd: "/workspace", env: {} },
		{
			spawnProcess: () => child,
			signalProcess(_processValue, signal) {
				signals.push(signal);
			},
			stopTimeoutMilliseconds: 5,
		}
	);
	processRunner.start();
	child.emit("spawn");
	await assert.rejects(processRunner.stop(), /Timed out stopping the TypeRB process tree/);
	assert.deepEqual(signals, ["SIGINT", "SIGTERM", "SIGKILL"]);
});

test("formats the displayed command without invoking a shell", () => {
	assert.equal(formatCommand("/path with space/trb", ["run", "--config", "/project/a b.jsonc"]),
		'"/path with space/trb" run --config "/project/a b.jsonc"');
});
