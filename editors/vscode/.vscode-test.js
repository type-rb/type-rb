"use strict";

const { defineConfig } = require("@vscode/test-cli");

const common = {
	files: "test/integration/**/*.test.js",
	workspaceFolder: "test/fixtures/standalone",
	extensionDevelopmentPath: ".",
	mocha: {
		ui: "tdd",
		timeout: 30000,
	},
	launchArgs: [
		"--disable-extensions",
		"--disable-telemetry",
		"--skip-release-notes",
		"--skip-welcome",
	],
};

module.exports = defineConfig([
	{ ...common, label: "stable", version: "stable" },
	{ ...common, label: "insiders", version: "insiders" },
]);
