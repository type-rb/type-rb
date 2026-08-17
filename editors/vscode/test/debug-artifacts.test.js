"use strict";

const assert = require("node:assert/strict");
const { access, mkdtemp, readdir, rm, stat } = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { DebugArtifactSessions, DebugArtifactStore, reserveDebugArtifact } = require("../debug-artifacts");

test("allocates unique artifacts for concurrent debug sessions", async () => {
	const temporaryRoot = await mkdtemp(path.join(os.tmpdir(), "typerb-vscode-artifacts-test-"));
	const store = new DebugArtifactStore({ temporaryRoot });
	const sessions = new DebugArtifactSessions(store);
	try {
		const [first, second] = await Promise.all([
			sessions.prepare("first", "app", async () => {}),
			sessions.prepare("second", "app", async () => {})
		]);
		assert.notEqual(first.directory, second.directory);
		assert.notEqual(first.program, second.program);
		await Promise.all([access(first.directory), access(second.directory)]);
		if (process.platform !== "win32") {
			assert.equal((await stat(first.directory)).mode & 0o777, 0o700);
			assert.equal((await stat(second.directory)).mode & 0o777, 0o700);
		}

		await Promise.all([sessions.release("first"), sessions.release("first")]);
		await assert.rejects(access(first.directory), { code: "ENOENT" });
		await access(second.directory);

		await sessions.dispose();
		await assert.rejects(access(second.directory), { code: "ENOENT" });
	} finally {
		await sessions.dispose();
		await store.dispose();
		await rm(temporaryRoot, { recursive: true, force: true });
	}
});

test("removes a private artifact when preparation fails or is cancelled", async () => {
	const temporaryRoot = await mkdtemp(path.join(os.tmpdir(), "typerb-vscode-artifacts-test-"));
	const store = new DebugArtifactStore({ temporaryRoot });
	const sessions = new DebugArtifactSessions(store);
	try {
		await assert.rejects(
			sessions.prepare("failure", "app", async () => {
				throw new Error("build failed");
			}),
			/build failed/
		);
		let preparationStarted;
		const started = new Promise((resolve) => {
			preparationStarted = resolve;
		});
		const preparation = sessions.prepare("cancelled", "app", async (_program, signal) => {
			preparationStarted();
			await new Promise((resolve, reject) => {
				signal.addEventListener("abort", () => {
					const error = new Error("cancelled");
					error.name = "AbortError";
					reject(error);
				}, { once: true });
			});
		});
		await started;
		const release = sessions.release("cancelled");
		await assert.rejects(
			preparation,
			{ name: "AbortError" }
		);
		await release;
		assert.deepEqual(await readdir(temporaryRoot), []);
	} finally {
		await sessions.dispose();
		await store.dispose();
		await rm(temporaryRoot, { recursive: true, force: true });
	}
});

test("does not materialize a reserved debug path until a concrete session prepares it", async () => {
	const temporaryRoot = await mkdtemp(path.join(os.tmpdir(), "typerb-vscode-artifacts-test-"));
	const store = new DebugArtifactStore({ temporaryRoot });
	const sessions = new DebugArtifactSessions(store);
	try {
		const reservation = reserveDebugArtifact("app", {
			temporaryRoot,
			randomID: () => "reserved-session"
		});
		await assert.rejects(access(reservation.directory), { code: "ENOENT" });
		const artifact = await sessions.prepare(
			"reserved",
			"app",
			async () => {},
			reservation
		);
		assert.deepEqual(artifact, reservation);
		await access(artifact.directory);
		if (process.platform !== "win32") {
			assert.equal((await stat(artifact.directory)).mode & 0o777, 0o700);
		}
		await sessions.release("reserved");
		await assert.rejects(access(artifact.directory), { code: "ENOENT" });
	} finally {
		await sessions.dispose();
		await store.dispose();
		await rm(temporaryRoot, { recursive: true, force: true });
	}
});
