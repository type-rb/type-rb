"use strict";

const { mkdirSync } = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const extensionRoot = path.resolve(__dirname, "../..");
const repositoryRoot = path.resolve(extensionRoot, "../..");
const binaryDirectory = path.join(extensionRoot, ".vscode-test", "bin");
const binaryPath = path.join(binaryDirectory, process.platform === "win32" ? "trb.exe" : "trb");

mkdirSync(binaryDirectory, { recursive: true });
const result = spawnSync("go", ["build", "-o", binaryPath, "./cmd/trb"], {
	cwd: repositoryRoot,
	env: {
		...process.env,
		GOCACHE: process.env.GOCACHE || path.join(os.tmpdir(), "type-rb-go-cache"),
	},
	stdio: "inherit",
});
if (result.error !== undefined) {
	throw result.error;
}
if (result.status !== 0) {
	throw new Error(`go build exited with status ${result.status}`);
}
