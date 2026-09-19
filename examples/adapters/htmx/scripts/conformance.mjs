import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFile, writeFile, unlink } from "node:fs/promises";
import { resolve } from "node:path";
import { chromium } from "playwright";
import { renderToStaticMarkup } from "react-dom/server";
import { FragmentButton, DataFragmentButton } from "../build/app.tsx";

const root = resolve(import.meta.dir, "..");
const trb = resolve(root, "../../../trb");
const config = JSON.parse(await readFile(resolve(root, "trbconfig.jsonc"), "utf8"));
const negativeConfig = resolve(root, `.conformance-${process.pid}.json`);
try {
  await writeFile(negativeConfig, JSON.stringify({ ...config, sourceDir: "unsupported" }), { flag: "wx" });
  const checked = spawnSync(trb, ["check", "--diagnostic-format", "json", "--config", negativeConfig], {
    cwd: root, encoding: "utf8",
  });
  assert.equal(checked.status, 1, checked.stderr);
  const report = JSON.parse(checked.stdout);
  assert.equal(report.summary.errors, 1);
  assert.equal(report.diagnostics[0].code, "TRB2000");
  assert.match(report.diagnostics[0].message, /default export; automatic indexing supports named exports only/);
  assert.doesNotMatch(report.diagnostics[0].message, /does not export/);
  assert.equal(report.diagnostics[0].location.span.start.line, 1);
} finally {
  await unlink(negativeConfig);
}

const bundle = await Bun.build({ entrypoints: [resolve(root, "build/src/native/bootstrap.ts")], target: "browser" });
assert.equal(bundle.success, true, JSON.stringify(bundle.logs));
const bootstrap = await bundle.outputs[0].text();
const markup = renderToStaticMarkup(FragmentButton()) + renderToStaticMarkup(DataFragmentButton());
let requests = 0;
const server = Bun.serve({
  hostname: "127.0.0.1", port: 0,
  fetch(request) {
    const { pathname } = new URL(request.url);
    if (pathname === "/bootstrap.js") return new Response(bootstrap, { headers: { "Content-Type": "text/javascript" } });
    if (pathname === "/fragment") {
      assert.equal(request.headers.get("HX-Request"), "true");
      requests++;
      return new Response(`<p id="loaded">Fragment ${requests}</p>`, { headers: { "Content-Type": "text/html" } });
    }
    if (pathname !== "/") return new Response("Not found", { status: 404 });
    return new Response(`<!doctype html><html><head><meta charset="utf-8"><title>Attribute conformance</title></head><body>${markup}<section id="result"></section><script type="module" src="/bootstrap.js"></script></body></html>`, {
      headers: { "Content-Type": "text/html" },
    });
  },
});
let browser;
try {
  browser = await chromium.launch();
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(server.url.href);
  for (const [id, prefix] of [["load", "hx-"], ["load-data", "data-hx-"]]) {
    const button = page.locator(`#${id}`);
    assert.equal(await button.getAttribute(`${prefix}get`), "/fragment");
    assert.equal(await button.getAttribute(`${prefix}target`), "#result");
    assert.equal(await button.getAttribute(`${prefix}swap`), "innerHTML");
    const next = requests + 1;
    await button.click();
    await page.waitForFunction(expected => document.querySelector("#loaded")?.textContent === expected, `Fragment ${next}`);
    assert.equal(requests, next);
  }
  assert.deepEqual(errors, []);
  console.log("PASS: default-export diagnostic, strict native types, hx-/data-hx- attributes, requests and DOM swaps");
} finally {
  await browser?.close();
  await server.stop(true);
}
