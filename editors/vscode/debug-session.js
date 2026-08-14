"use strict";

const { spawn } = require("node:child_process");
const { EventEmitter } = require("node:events");
const {
	DebugSession,
	Event,
	ExitedEvent,
	InitializedEvent,
	OutputEvent,
	TerminatedEvent,
} = require("@vscode/debugadapter");

const stopTimeout = 750;

class TypeRBProcess extends EventEmitter {
	constructor(specification, options = {}) {
		super();
		this.specification = specification;
		this.spawnProcess = options.spawnProcess ?? spawn;
		this.signalProcess = options.signalProcess ?? signalProcessTree;
		this.child = undefined;
		this.closed = false;
		this.stopPromise = undefined;
	}

	start() {
		if (this.child !== undefined) {
			throw new Error("TypeRB process is already running");
		}
		const child = this.spawnProcess(
			this.specification.command,
			this.specification.args,
			{
				cwd: this.specification.cwd,
				env: this.specification.env,
				detached: process.platform !== "win32",
				stdio: ["ignore", "pipe", "pipe"],
			}
		);
		this.child = child;
		child.stdout?.on("data", (data) => this.emit("output", "stdout", data.toString()));
		child.stderr?.on("data", (data) => this.emit("output", "stderr", data.toString()));
		child.once("spawn", () => this.emit("started", child.pid));
		child.once("error", (error) => this.emit("error", error));
		child.once("close", (code, signal) => this.finish(code, signal));
	}

	stop() {
		this.stopPromise ??= this.stopOnce();
		return this.stopPromise;
	}

	async stopOnce() {
		const child = this.child;
		if (child === undefined || this.closed) {
			return;
		}
		const closed = new Promise((resolve) => this.once("closed", resolve));
		if (child.pid === undefined) {
			await Promise.race([
				new Promise((resolve) => this.once("started", resolve)),
				closed,
			]);
		}
		if (this.closed) {
			return;
		}
		this.signalProcess(child, "SIGINT");
		const terminate = setTimeout(() => {
			if (!this.closed) {
				this.signalProcess(child, "SIGTERM");
			}
		}, stopTimeout);
		terminate.unref?.();
		const force = setTimeout(() => {
			if (!this.closed) {
				this.signalProcess(child, "SIGKILL");
			}
		}, stopTimeout * 2);
		force.unref?.();
		await closed;
		clearTimeout(terminate);
		clearTimeout(force);
	}

	finish(code, signal) {
		if (this.closed) {
			return;
		}
		this.closed = true;
		this.emit("exit", code ?? 1, signal);
		this.emit("closed");
	}
}

function signalProcessTree(child, signal) {
	if (child.pid === undefined) {
		return;
	}
	try {
		if (process.platform === "win32") {
			child.kill(signal);
		} else {
			process.kill(-child.pid, signal);
		}
	} catch (error) {
		if (error?.code !== "ESRCH") {
			try {
				child.kill(signal);
			} catch (fallbackError) {
				if (fallbackError?.code !== "ESRCH") {
					throw fallbackError;
				}
			}
		}
	}
}

class TypeRBDebugSession extends DebugSession {
	constructor(options = {}) {
		super();
		this.processFactory = options.processFactory ?? ((specification) => new TypeRBProcess(specification));
		this.configurationDone = false;
		this.pendingLaunch = undefined;
		this.launchArguments = undefined;
		this.runningProcess = undefined;
		this.restarting = false;
	}

	initializeRequest(response) {
		response.body = {
			supportsConfigurationDoneRequest: true,
			supportsRestartRequest: true,
			supportsTerminateRequest: true,
			supportTerminateDebuggee: true,
		};
		this.sendResponse(response);
		this.sendEvent(new InitializedEvent());
	}

	launchRequest(response, args) {
		try {
			this.launchArguments = validateLaunchArguments(args);
			this.pendingLaunch = response;
			this.startPendingLaunch();
		} catch (error) {
			this.sendErrorResponse(response, 2001, error.message);
		}
	}

	configurationDoneRequest(response) {
		this.configurationDone = true;
		this.sendResponse(response);
		this.startPendingLaunch();
	}

	terminateRequest(response) {
		this.sendResponse(response);
		void this.terminate("TypeRB: stopping process...\n");
	}

	disconnectRequest(response) {
		this.sendResponse(response);
		void this.terminate();
	}

	restartRequest(response) {
		if (this.launchArguments === undefined) {
			this.sendErrorResponse(response, 2002, "TypeRB project has not been launched");
			return;
		}
		void this.restart(response);
	}

	threadsRequest(response) {
		response.body = { threads: [] };
		this.sendResponse(response);
	}

	dispose() {
		void this.stopProcess().catch(() => {});
		super.dispose();
	}

	startPendingLaunch() {
		if (!this.configurationDone || this.pendingLaunch === undefined || this.launchArguments === undefined) {
			return;
		}
		const response = this.pendingLaunch;
		this.pendingLaunch = undefined;
		try {
			this.startProcess(this.launchArguments);
			this.sendResponse(response);
		} catch (error) {
			this.sendErrorResponse(response, 2003, error.message);
		}
	}

	startProcess(args) {
		const specification = {
			command: args.command,
			args: args.commandArgs,
			cwd: args.cwd,
			env: { ...process.env, ...args.env },
		};
		const runner = this.processFactory(specification);
		this.runningProcess = runner;
		this.sendEvent(new OutputEvent(`TypeRB: starting ${args.projectName}...\n`, "console"));
		this.sendEvent(new OutputEvent(`TypeRB: ${formatCommand(specification.command, specification.args)}\n`, "console"));
		runner.on("output", (category, output) => this.sendEvent(new OutputEvent(output, category)));
		runner.once("started", (pid) => {
			const event = new Event("process");
			event.body = {
				name: args.projectName,
				systemProcessId: pid,
				isLocalProcess: true,
				startMethod: "launch",
			};
			this.sendEvent(event);
			this.sendEvent(new OutputEvent(`TypeRB: process started (PID ${pid}). Use Stop or Shift+F5 to stop.\n`, "console"));
		});
		runner.once("error", (error) => {
			this.sendEvent(new OutputEvent(`TypeRB: failed to start: ${error.message}\n`, "stderr"));
		});
		runner.once("exit", (code, signal) => this.processExited(runner, code, signal));
		runner.start();
	}

	processExited(runner, code, signal) {
		if (this.runningProcess !== runner) {
			return;
		}
		this.runningProcess = undefined;
		if (this.restarting) {
			return;
		}
		const suffix = signal === null || signal === undefined ? `code ${code}` : `signal ${signal}`;
		this.sendEvent(new OutputEvent(`TypeRB: process exited with ${suffix}.\n`, "console"));
		this.sendEvent(new ExitedEvent(code));
		this.sendEvent(new TerminatedEvent());
	}

	async restart(response) {
		this.restarting = true;
		this.sendEvent(new OutputEvent("TypeRB: restarting process...\n", "console"));
		try {
			await this.stopProcess();
			this.restarting = false;
			this.startProcess(this.launchArguments);
			this.sendResponse(response);
		} catch (error) {
			this.restarting = false;
			this.sendErrorResponse(response, 2004, error.message);
		}
	}

	async terminate(message) {
		if (message !== undefined && this.runningProcess !== undefined) {
			this.sendEvent(new OutputEvent(message, "console"));
		}
		if (this.runningProcess === undefined) {
			this.sendEvent(new TerminatedEvent());
			return;
		}
		try {
			await this.stopProcess();
		} catch (error) {
			this.sendEvent(new OutputEvent(`TypeRB: failed to stop process: ${error.message}\n`, "stderr"));
			this.sendEvent(new TerminatedEvent());
		}
	}

	async stopProcess() {
		const runner = this.runningProcess;
		if (runner !== undefined) {
			await runner.stop();
		}
	}
}

function validateLaunchArguments(args) {
	if (args === null || typeof args !== "object") {
		throw new Error("TypeRB launch configuration is missing");
	}
	if (typeof args.command !== "string" || args.command === "") {
		throw new Error("TypeRB compiler path is missing");
	}
	if (!Array.isArray(args.commandArgs) || !args.commandArgs.every((argument) => typeof argument === "string")) {
		throw new Error("TypeRB command arguments must be strings");
	}
	if (typeof args.cwd !== "string" || args.cwd === "") {
		throw new Error("TypeRB project directory is missing");
	}
	if (args.env !== undefined && (args.env === null || typeof args.env !== "object" || Array.isArray(args.env))) {
		throw new Error("TypeRB environment must be an object");
	}
	return {
		command: args.command,
		commandArgs: [...args.commandArgs],
		cwd: args.cwd,
		env: args.env ?? {},
		projectName: typeof args.projectName === "string" && args.projectName !== "" ? args.projectName : "TypeRB",
	};
}

function formatCommand(command, args) {
	return [command, ...args].map((argument) => /[\s"'\\]/.test(argument) ? JSON.stringify(argument) : argument).join(" ");
}

module.exports = {
	TypeRBDebugSession,
	TypeRBProcess,
	formatCommand,
	validateLaunchArguments,
};
