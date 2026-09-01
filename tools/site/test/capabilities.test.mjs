import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import vm from "node:vm";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const scriptPath = resolve(repositoryRoot, "internal/site/assets/capabilities.js");

test("capability-map enhancement remains valid JavaScript", async () => {
  const source = await readFile(scriptPath, "utf8");
  const script = new vm.Script(source, { filename: scriptPath });

  assert.ok(script);
  assert.match(source, /data-capability-status/);
  assert.match(source, /data-capability-scope/);
  assert.match(source, /data-capability-search/);
});
