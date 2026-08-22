import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const packageDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const userscript = await readFile(resolve(packageDirectory, "typerb-github.user.js"), "utf8");

test("ships a self-contained Tampermonkey userscript", () => {
  assert.ok(userscript.startsWith("// ==UserScript=="));
  assert.match(userscript, /\/\/ @sandbox\s+DOM/);
  assert.match(userscript, /\/\/ @match\s+https:\/\/github\.com\/\*/);
  assert.match(userscript, /vscode-textmate/);
  assert.match(userscript, /Oniguruma LICENSE/);
  assert.doesNotMatch(userscript, /cdn\.jsdelivr\.net|unpkg\.com/);
});
