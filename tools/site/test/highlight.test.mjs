import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { mkdtemp } from "node:fs/promises";
import test from "node:test";
import { highlightSite } from "../highlight.mjs";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const grammarPath = resolve(repositoryRoot, "syntaxes/typerb.tmLanguage.json");

test("statically highlights TypeRB and standard documentation fences", async () => {
  const output = await mkdtemp(resolve(tmpdir(), "typerb-site-highlight-"));
  const page = resolve(output, "docs/index.html");
  await mkdir(dirname(page), { recursive: true });
  await writeFile(page, `<!doctype html>
<pre><code class="language-trb">def greet(name: String): String
\treturn &quot;Hello, &quot; + name
end
</code></pre>
<pre><code class="language-sh">trb check
</code></pre>
<pre><code class="language-html">&lt;div id=&quot;root&quot;&gt;&lt;/div&gt;
</code></pre>
<pre><code class="language-sql">SELECT id FROM reports;
</code></pre>
<pre><code>leave this block unchanged</code></pre>
`);

  await highlightSite(output, grammarPath);
  const highlighted = await readFile(page, "utf8");

  assert.match(highlighted, /class="shiki github-dark-default"/);
  assert.match(highlighted, /<span style="color:/);
  assert.match(highlighted, />def<\/span>/);
  assert.match(highlighted, />trb<\/span>/);
  assert.match(highlighted, /id/);
  assert.match(highlighted, />SELECT<\/span>/);
  assert.doesNotMatch(highlighted, /language-(?:html|sql|trb|sh)/);
  assert.match(highlighted, /<pre><code>leave this block unchanged<\/code><\/pre>/);
});

test("rejects an undocumented fenced language", async () => {
  const output = await mkdtemp(resolve(tmpdir(), "typerb-site-highlight-"));
  const page = resolve(output, "index.html");
  await writeFile(page, `<pre><code class="language-unknown">value</code></pre>`);

  await assert.rejects(
    highlightSite(output, grammarPath),
    /unsupported documentation code-block language: unknown/
  );
});
