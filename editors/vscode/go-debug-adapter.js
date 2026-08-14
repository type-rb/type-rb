"use strict";

const { spawn } = require("node:child_process");

const listenPattern = /DAP server listening at:\s+127\.0\.0\.1:(\d+)/;

class DlvDAPProcess {
	constructor(toolPath, options = {}) {
		this.toolPath = toolPath;
		this.spawnProcess = options.spawnProcess ?? spawn;
		this.timeoutMilliseconds = options.timeoutMilliseconds ?? 10000;
		this.child = undefined;
	}

	start() {
		if (this.child !== undefined) {
			throw new Error("Delve DAP is already running");
		}
		return new Promise((resolve, reject) => {
			const child = this.spawnProcess(this.toolPath, ["dap", "--listen=127.0.0.1:0"], {
				stdio: ["ignore", "pipe", "pipe"],
			});
			this.child = child;
			let output = "";
			let settled = false;
			const finish = (callback, value) => {
				if (settled) return;
				settled = true;
				clearTimeout(timer);
				callback(value);
			};
			const inspect = (chunk) => {
				output += chunk.toString();
				const match = listenPattern.exec(output);
				if (match !== null) {
					finish(resolve, Number(match[1]));
				}
			};
			child.stdout?.on("data", inspect);
			child.stderr?.on("data", inspect);
			child.once("error", (error) => finish(reject, new Error(`Cannot start Delve (${this.toolPath}): ${error.message}`)));
			child.once("exit", (code) => {
				finish(reject, new Error(`Delve exited before opening its debug adapter (code ${code ?? 1}). ${output.trim()}`.trim()));
			});
			const timer = setTimeout(() => {
				this.stop();
				finish(reject, new Error(`Delve did not open its debug adapter within ${this.timeoutMilliseconds}ms. ${output.trim()}`.trim()));
			}, this.timeoutMilliseconds);
			timer.unref?.();
		});
	}

	stop() {
		if (this.child !== undefined && this.child.exitCode === null && this.child.signalCode === null) {
			this.child.kill("SIGTERM");
		}
	}
}

module.exports = { DlvDAPProcess, listenPattern };
