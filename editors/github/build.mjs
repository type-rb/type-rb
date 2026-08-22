import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { copyFile, mkdir, readFile, rm, unlink, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

import { chromeManifest } from "./src/chrome-manifest.js";
import { HIGHLIGHT_STYLES } from "./src/styles.js";

const execFileAsync = promisify(execFile);
const packageDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(packageDirectory, "../..");
const extensionOutputDirectory = resolve(packageDirectory, "dist/chrome-extension");
const extensionArchivePath = resolve(packageDirectory, "dist/typerb-github-chrome-extension.zip");
const checkOnly = process.argv.includes("--check");
const packageChrome = process.argv.includes("--package-chrome");

const packageJson = JSON.parse(await readFile(resolve(packageDirectory, "package.json"), "utf8"));
const grammarPath = resolve(repositoryRoot, "syntaxes/typerb.tmLanguage.json");
const grammarJson = await readFile(grammarPath, "utf8");

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

async function bundle(entryPoint) {
  return build({
    absWorkingDir: packageDirectory,
    entryPoints: [resolve(packageDirectory, entryPoint)],
    bundle: true,
    charset: "ascii",
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
}

const chromeResult = await bundle("src/chrome.js");
assert.equal(chromeResult.outputFiles.length, 1, "expected one bundled Chrome content script");

const packageRoots = [...new Set(
  Object.keys(chromeResult.metafile.inputs)
    .map(packageRootFromInput)
    .filter(Boolean)
)].sort();
const notices = [
  `\nTypeRB\n======\n${await readFile(resolve(repositoryRoot, "LICENSE"), "utf8")}`,
  ...await Promise.all(packageRoots.map(packageNotice))
];
const licenseBanner = `/*
Bundled license notices
${notices.join("\n").replaceAll("*/", "* /")}
*/`;
const chromeContentScript = `${licenseBanner}\n${chromeResult.outputFiles[0].text.trimEnd()}\n`;
const manifest = chromeManifest(packageJson.version);
const manifestJson = `${JSON.stringify(manifest, null, 2)}\n`;

assert.equal(manifest.permissions, undefined, "Chrome extension must not request optional APIs");
assert.equal(manifest.host_permissions, undefined, "GitHub access belongs only in content script matches");
assert.doesNotMatch(
  chromeContentScript,
  /[^\x00-\x7F]/,
  "Chrome content script must contain only ASCII-compatible UTF-8"
);
assert.doesNotMatch(chromeContentScript, /WebAssembly\.instantiate|onig\.wasm|vscode-oniguruma/);
assert.doesNotMatch(chromeContentScript, /cdn\.jsdelivr\.net|unpkg\.com/);

if (!checkOnly) {
  await rm(extensionOutputDirectory, { recursive: true, force: true });
  await mkdir(resolve(extensionOutputDirectory, "images"), { recursive: true });
  await Promise.all([
    writeFile(resolve(extensionOutputDirectory, "manifest.json"), manifestJson),
    writeFile(resolve(extensionOutputDirectory, "content.js"), chromeContentScript),
    writeFile(resolve(extensionOutputDirectory, "content.css"), HIGHLIGHT_STYLES.trimStart()),
    ...[16, 32, 48, 128].map((size) => copyFile(
      resolve(packageDirectory, `assets/icon-${size}.png`),
      resolve(extensionOutputDirectory, `images/icon-${size}.png`)
    ))
  ]);
}

if (packageChrome) {
  await mkdir(dirname(extensionArchivePath), { recursive: true });
  await unlink(extensionArchivePath).catch((error) => {
    if (error.code !== "ENOENT") {
      throw error;
    }
  });
  await execFileAsync("zip", ["-X", "-q", "-r", extensionArchivePath, "."], {
    cwd: extensionOutputDirectory
  });
  console.log(extensionArchivePath);
}
