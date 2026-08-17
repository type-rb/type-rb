"use strict";

const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const test = require("node:test");
const { DlvDAPProcess } = require("../go-debug-adapter");

test("starts Delve on an ephemeral DAP port and stops it", async () => {
	const child = new EventEmitter();
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	child.exitCode = null;
	child.signalCode = null;
	let invocation;
	let signal;
	child.kill = (value) => {
		signal = value;
		queueMicrotask(() => {
			child.signalCode = value;
			child.emit("exit", null, value);
		});
	};
	const process = new DlvDAPProcess("/tools/dlv", {
		spawnProcess(command, args, options) {
			invocation = { command, args, options };
			return child;
		},
	});
	const started = process.start();
	child.stdout.emit("data", Buffer.from("DAP server listening at: 127.0.0.1:43127\n"));
	assert.equal(await started, 43127);
	assert.deepEqual(invocation, {
		command: "/tools/dlv",
		args: ["dap", "--listen=127.0.0.1:0"],
		options: { stdio: ["ignore", "pipe", "pipe"] },
	});
	await process.stop();
	assert.equal(signal, "SIGTERM");
});

test("reports an actionable error when Delve is missing", async () => {
	const child = new EventEmitter();
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	child.exitCode = null;
	child.signalCode = null;
	child.kill = () => {};
	const process = new DlvDAPProcess("dlv", { spawnProcess: () => child });
	const started = process.start();
	child.emit("error", Object.assign(new Error("spawn dlv ENOENT"), { code: "ENOENT" }));
	await assert.rejects(started, /Cannot start Delve \(dlv\): spawn dlv ENOENT/);
});

test("bounds Delve shutdown when the process does not exit", async () => {
	const child = new EventEmitter();
	child.stdout = new EventEmitter();
	child.stderr = new EventEmitter();
	child.exitCode = null;
	child.signalCode = null;
	const signals = [];
	child.kill = (signal) => signals.push(signal);
	const process = new DlvDAPProcess("dlv", {
		spawnProcess: () => child,
		shutdownTimeoutMilliseconds: 5,
	});
	const started = process.start();
	child.stdout.emit("data", Buffer.from("DAP server listening at: 127.0.0.1:43127\n"));
	await started;
	await assert.rejects(process.stop(), /Timed out stopping Delve \(dlv\)/);
	assert.deepEqual(signals, ["SIGTERM", "SIGKILL"]);
});
