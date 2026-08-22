import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

const packageDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(packageDirectory, "../..");
const outputPath = resolve(packageDirectory, "typerb-github.user.js");
const checkOnly = process.argv.includes("--check");

const packageJson = JSON.parse(await readFile(resolve(packageDirectory, "package.json"), "utf8"));
const grammarPath = resolve(repositoryRoot, "syntaxes/typerb.tmLanguage.json");
const grammarJson = await readFile(grammarPath, "utf8");

const metadata = `// ==UserScript==
// @name         TypeRB syntax highlighting for GitHub
// @namespace    https://type-rb.github.io/
// @version      ${packageJson.version}
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

function packageRootFromInput(input) {
  const normalized = input.replaceAll("\\", "/");
  const marker = "node_modules/";
  const markerIndex = normalized.lastIndexOf(marker);
  if (markerIndex < 0) {
    return null;
  }
  const parts = normalized.slice(markerIndex + marker.length).split("/");
  const name = parts[0].startsWith("@") ? parts.slice(0, 2).join("/") : parts[0];
  return resolve(packageDirectory, normalized.slice(0, markerIndex + marker.length) + name);
}

async function packageNotice(packageRoot) {
  const manifest = JSON.parse(await readFile(resolve(packageRoot, "package.json"), "utf8"));
  const candidates = ["LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "LICENCE.md"];
  for (const candidate of candidates) {
    const contents = await readFile(resolve(packageRoot, candidate), "utf8").catch(() => null);
    if (contents) {
      const heading = `${manifest.name} ${manifest.version}`;
      return `\n${heading}\n${"=".repeat(heading.length)}\n${contents}`;
    }
  }
  throw new Error(`No bundled license file found for ${manifest.name}`);
}

const result = await build({
  absWorkingDir: packageDirectory,
  entryPoints: [resolve(packageDirectory, "src/main.js")],
  bundle: true,
  charset: "utf8",
  format: "iife",
  legalComments: "none",
  metafile: true,
  minify: true,
  platform: "browser",
  target: ["chrome120", "firefox128"],
  write: false,
  plugins: [{
    name: "typerb-assets",
    setup(builder) {
      builder.onResolve({ filter: /^typerb:assets$/ }, () => ({
        path: "assets",
        namespace: "typerb"
      }));
      builder.onLoad({ filter: /^assets$/, namespace: "typerb" }, () => ({
        contents: `export const grammarJson = ${JSON.stringify(grammarJson)};`,
        loader: "js"
      }));
    }
  }]
});

assert.equal(result.outputFiles.length, 1, "expected one bundled userscript");
const packageRoots = [...new Set(
  Object.keys(result.metafile.inputs).map(packageRootFromInput).filter(Boolean)
)].sort();
const notices = [
  `\nTypeRB\n======\n${await readFile(resolve(repositoryRoot, "LICENSE"), "utf8")}`,
  ...await Promise.all(packageRoots.map(packageNotice))
];
const licenseBanner = `/*
Bundled license notices
${notices.join("\n").replaceAll("*/", "* /")}
*/`;
const bundled = `${metadata}\n${licenseBanner}\n${result.outputFiles[0].text.trimEnd()}\n`;

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
