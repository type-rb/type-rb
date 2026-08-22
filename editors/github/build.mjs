import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

const require = createRequire(import.meta.url);
const packageDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(packageDirectory, "../..");
const outputPath = resolve(packageDirectory, "typerb-github.user.js");
const checkOnly = process.argv.includes("--check");

const grammarPath = resolve(repositoryRoot, "syntaxes/typerb.tmLanguage.json");
const grammarJson = await readFile(grammarPath, "utf8");
const wasmPath = require.resolve("vscode-oniguruma/release/onig.wasm");
const wasm = await readFile(wasmPath);

const metadata = `// ==UserScript==
// @name         TypeRB syntax highlighting for GitHub
// @namespace    https://type-rb.github.io/
// @version      0.1.0
// @description  Highlight TypeRB files, pull request diffs, and Markdown code blocks on GitHub.
// @author       TypeRB contributors
// @license      MIT
// @homepageURL  https://github.com/type-rb/type-rb/tree/main/editors/github
// @supportURL   https://github.com/type-rb/type-rb/issues
// @updateURL    https://raw.githubusercontent.com/type-rb/type-rb/main/editors/github/typerb-github.user.js
// @downloadURL  https://raw.githubusercontent.com/type-rb/type-rb/main/editors/github/typerb-github.user.js
// @match        https://github.com/*
// @run-at       document-idle
// @sandbox      DOM
// @grant        GM_addStyle
// @noframes
// ==/UserScript==`;

const notices = await Promise.all([
  ["TypeRB", resolve(repositoryRoot, "LICENSE")],
  ["vscode-textmate", require.resolve("vscode-textmate/LICENSE.md")],
  ["vscode-oniguruma", require.resolve("vscode-oniguruma/LICENSE.txt")],
  ["vscode-oniguruma third-party notices", require.resolve("vscode-oniguruma/NOTICES.txt")]
].map(async ([name, path]) => `\n${name}\n${"=".repeat(name.length)}\n${await readFile(path, "utf8")}`));

const licenseBanner = `/*
Bundled license notices
${notices.join("\n").replaceAll("*/", "* /")}
*/`;

const result = await build({
  entryPoints: [resolve(packageDirectory, "src/main.js")],
  bundle: true,
  charset: "utf8",
  format: "iife",
  legalComments: "none",
  minify: true,
  platform: "browser",
  target: ["chrome120", "firefox128"],
  write: false,
  banner: { js: `${metadata}\n${licenseBanner}` },
  plugins: [{
    name: "typerb-assets",
    setup(builder) {
      builder.onResolve({ filter: /^typerb:assets$/ }, () => ({
        path: "assets",
        namespace: "typerb"
      }));
      builder.onLoad({ filter: /^assets$/, namespace: "typerb" }, () => ({
        contents: `export const grammarJson = ${JSON.stringify(grammarJson)};\nexport const onigWasmBase64 = ${JSON.stringify(wasm.toString("base64"))};`,
        loader: "js"
      }));
    }
  }]
});

assert.equal(result.outputFiles.length, 1, "expected one bundled userscript");
const bundled = `${result.outputFiles[0].text.trimEnd()}\n`;

if (checkOnly) {
  const committed = await readFile(outputPath, "utf8").catch(() => "");
  assert.equal(
    committed,
    bundled,
    "typerb-github.user.js is stale; run npm run build --prefix editors/github"
  );
} else {
  await writeFile(outputPath, bundled);
}
