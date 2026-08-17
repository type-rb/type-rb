"use strict";

const { randomUUID } = require("node:crypto");
const { mkdir, mkdtemp, rm } = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");

class DebugArtifactStore {
	constructor(options = {}) {
		this.temporaryRoot = options.temporaryRoot ?? os.tmpdir();
		this.createDirectory = options.createDirectory ?? mkdtemp;
		this.removeDirectory = options.removeDirectory ?? ((directory) => rm(directory, {
			recursive: true,
			force: true,
			maxRetries: 5,
			retryDelay: 100
		}));
		this.directories = new Set();
		this.releases = new Map();
	}

	async create(filename) {
		const directory = await this.createDirectory(path.join(this.temporaryRoot, "typerb-vscode-debug-"));
		this.directories.add(directory);
		return { directory, program: path.join(directory, filename) };
	}

	async prepare(filename, prepare) {
		const artifact = await this.create(filename);
		try {
			await prepare(artifact.program);
			return artifact;
		} catch (error) {
			try {
				await this.release(artifact.directory);
			} catch {
				// Preserve the build or cancellation error that caused cleanup.
			}
			throw error;
		}
	}

	async prepareReserved(artifact, prepare) {
		const directory = path.resolve(artifact.directory);
		const program = path.resolve(artifact.program);
		if (
			path.dirname(directory) !== path.resolve(this.temporaryRoot) ||
			!path.basename(directory).startsWith("typerb-vscode-debug-") ||
			path.dirname(program) !== directory
		) {
			throw new Error("Invalid reserved TypeRB debug artifact path");
		}
		await mkdir(directory, { mode: 0o700 });
		this.directories.add(directory);
		try {
			await prepare(program);
			return { directory, program };
		} catch (error) {
			try {
				await this.release(directory);
			} catch {
				// Preserve the build or cancellation error that caused cleanup.
			}
			throw error;
		}
	}

	async release(value) {
		const directory = typeof value === "string" ? value : value?.directory;
		if (directory === undefined || !this.directories.has(directory)) {
			return;
		}
		const pending = this.releases.get(directory);
		if (pending !== undefined) {
			await pending;
			return;
		}
		const release = (async () => {
			await this.removeDirectory(directory);
			this.directories.delete(directory);
		})().finally(() => this.releases.delete(directory));
		this.releases.set(directory, release);
		await release;
	}

	async dispose() {
		await Promise.all([...this.directories].map((directory) => this.release(directory)));
	}
}

class DebugArtifactSessions {
	constructor(store = new DebugArtifactStore()) {
		this.store = store;
		this.sessions = new Map();
	}

	async prepare(sessionID, filename, prepare, reservation) {
		if (this.sessions.has(sessionID)) {
			throw new Error(`Debug artifacts already exist for session ${sessionID}`);
		}
		const state = { abort: new AbortController(), preparation: undefined };
		this.sessions.set(sessionID, state);
		const prepareProgram = (program) => prepare(program, state.abort.signal);
		state.preparation = reservation === undefined
			? this.store.prepare(filename, prepareProgram)
			: this.store.prepareReserved(reservation, prepareProgram);
		try {
			const artifact = await state.preparation;
			if (state.abort.signal.aborted) {
				await this.store.release(artifact);
				throw debugArtifactAbortError();
			}
			state.artifact = artifact;
			return artifact;
		} catch (error) {
			if (this.sessions.get(sessionID) === state) {
				this.sessions.delete(sessionID);
			}
			throw error;
		}
	}

	async release(sessionID) {
		const state = this.sessions.get(sessionID);
		if (state === undefined) {
			return;
		}
		state.abort.abort();
		try {
			const artifact = state.artifact ?? await state.preparation.catch(() => undefined);
			await this.store.release(artifact);
		} finally {
			if (this.sessions.get(sessionID) === state) {
				this.sessions.delete(sessionID);
			}
		}
	}

	async dispose() {
		await Promise.all([...this.sessions.keys()].map((sessionID) => this.release(sessionID)));
	}
}

function debugArtifactAbortError() {
	const error = new Error("TypeRB debug artifact preparation was cancelled");
	error.name = "AbortError";
	return error;
}

function reserveDebugArtifact(filename, options = {}) {
	const temporaryRoot = options.temporaryRoot ?? os.tmpdir();
	const identifier = options.randomID?.() ?? randomUUID();
	const directory = path.join(temporaryRoot, `typerb-vscode-debug-${identifier}`);
	return { directory, program: path.join(directory, filename) };
}

module.exports = { DebugArtifactSessions, DebugArtifactStore, reserveDebugArtifact };
