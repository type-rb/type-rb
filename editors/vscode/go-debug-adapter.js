"use strict";

const { spawn } = require("node:child_process");

const listenPattern = /DAP server listening at:\s+127\.0\.0\.1:(\d+)/;

class DlvDAPProcess {
	constructor(toolPath, options = {}) {
		this.toolPath = toolPath;
		this.spawnProcess = options.spawnProcess ?? spawn;
		this.timeoutMilliseconds = options.timeoutMilliseconds ?? 10000;
		this.shutdownTimeoutMilliseconds = options.shutdownTimeoutMilliseconds ?? 5000;
		this.child = undefined;
		this.exited = false;
		this.stopping = undefined;
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
			child.once("error", (error) => {
				this.exited = true;
				finish(reject, new Error(`Cannot start Delve (${this.toolPath}): ${error.message}`));
			});
			child.once("exit", (code) => {
				this.exited = true;
				finish(reject, new Error(`Delve exited before opening its debug adapter (code ${code ?? 1}). ${output.trim()}`.trim()));
			});
			const timer = setTimeout(() => {
				void this.stop().catch(() => {});
				finish(reject, new Error(`Delve did not open its debug adapter within ${this.timeoutMilliseconds}ms. ${output.trim()}`.trim()));
			}, this.timeoutMilliseconds);
			timer.unref?.();
		});
	}

	stop() {
		this.stopping ??= this.stopOnce();
		return this.stopping;
	}

	async stopOnce() {
		const child = this.child;
		if (child === undefined || this.exited || child.exitCode !== null || child.signalCode !== null) {
			return;
		}
		await new Promise((resolve, reject) => {
			let settled = false;
			let force;
			let giveUp;
			const stopped = () => finish();
			const finish = (error) => {
				if (settled) return;
				settled = true;
				this.exited = true;
				clearTimeout(force);
				clearTimeout(giveUp);
				child.off?.("exit", stopped);
				child.off?.("error", stopped);
				if (error === undefined) resolve();
				else reject(error);
			};
			child.once("exit", stopped);
			child.once("error", stopped);
			try {
				child.kill("SIGTERM");
			} catch {
				finish();
				return;
			}
			force = setTimeout(() => {
				try {
					child.kill("SIGKILL");
				} catch {
					finish();
				}
			}, this.shutdownTimeoutMilliseconds);
			force.unref?.();
			giveUp = setTimeout(
				() => finish(new Error(`Timed out stopping Delve (${this.toolPath})`)),
				this.shutdownTimeoutMilliseconds * 2
			);
		});
	}
}

module.exports = { DlvDAPProcess, listenPattern };
