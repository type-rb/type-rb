import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const testDirectory = dirname(fileURLToPath(import.meta.url));
const userscript = await readFile(resolve(testDirectory, "../typerb-github.user.js"), "utf8");
const port = Number.parseInt(process.env.TYPERB_FIXTURE_PORT ?? "41739", 10);
const baseOid = "a".repeat(40);
const headOid = "b".repeat(40);

const sources = new Map([
  [`/type-rb/type-rb/raw/${baseOid}/src/main.trb`, `message := <<~TEXT
  old # still a string
TEXT
# old comment
`],
  [`/type-rb/type-rb/raw/${headOid}/src/main.trb`, `message := <<~TEXT
  new # still a string
TEXT
# new comment
`]
]);

function page(body) {
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <style>
      :root {
        --color-prettylights-syntax-comment: #6a737d;
        --color-prettylights-syntax-constant: #005cc5;
        --color-prettylights-syntax-entity: #6f42c1;
        --color-prettylights-syntax-keyword: #d73a49;
        --color-prettylights-syntax-storage-modifier-import: #d73a49;
        --color-prettylights-syntax-string: #032f62;
        --color-prettylights-syntax-variable: #e36209;
      }
      body { font-family: ui-monospace, monospace; }
      pre, .react-code-lines, [role="region"] { border: 1px solid #d0d7de; padding: 1rem; }
      .diff-text-cell { white-space: pre; }
    </style>
  </head>
  <body>
    ${body}
    <script src="/typerb-github.user.js"></script>
  </body>
</html>`;
}

function blobPage() {
  return page(`
    <textarea data-testid="read-only-cursor-text-area">def greet(name: String): String
  return "Hello, #{name}"
end</textarea>
    <div class="react-code-lines">
      <div class="react-file-line" id="LC1">def greet(name: String): String</div>
      <div class="react-file-line" id="LC2">  return "Hello, #{name}"</div>
      <div class="react-file-line" id="LC3">end</div>
    </div>
    <pre lang="trb"><code># Markdown comment
puts("Markdown string")</code></pre>
  `);
}

function pullRequestPage(includePayload = true) {
  const payload = JSON.stringify({
    payload: {
      pullRequestsChangesRoute: {
        comparison: { fullDiff: { baseOid, headOid } },
        pullRequest: {
          headRepositoryOwnerLogin: "type-rb",
          headRepositoryName: "type-rb"
        },
        diffSummaries: [{ path: "src/main.trb", pathDigest: "fixture" }],
        diffContents: [{
          path: "src/main.trb",
          pathDigest: "fixture",
          oldCommitOid: baseOid,
          newCommitOid: headOid,
          oldTreeEntry: { path: "src/main.trb" },
          newTreeEntry: { path: "src/main.trb" }
        }]
      }
    }
  });
  const leftLine = includePayload ? "  old # still a string" : '  return "old fallback"';
  const rightLine = includePayload ? "  new # still a string" : '  return "new fallback"';
  return page(`
    ${includePayload ? `<script type="application/json">${payload}</script>` : ""}
    <div role="region" id="diff-fixture">
      <h3><a href="#diff-fixture"><code>src/main.trb</code></a></h3>
      <div class="diff-text-cell" data-line-number="2" data-diff-side="left"><span>-</span><span class="diff-text-inner">${leftLine}</span></div>
      <div class="diff-text-cell" data-line-number="2" data-diff-side="right"><span>+</span><span class="diff-text-inner">${rightLine}</span></div>
    </div>
  `);
}

const server = createServer((request, response) => {
  const url = new URL(request.url, `http://${request.headers.host}`);
  if (url.pathname === "/typerb-github.user.js") {
    response.writeHead(200, { "content-type": "text/javascript; charset=utf-8" });
    response.end(userscript);
    return;
  }
  if (sources.has(url.pathname)) {
    response.writeHead(200, { "content-type": "text/plain; charset=utf-8" });
    response.end(sources.get(url.pathname));
    return;
  }
  if (/\/pull\/\d+\/(?:files|changes)\/?$/.test(url.pathname)) {
    response.writeHead(200, {
      "content-security-policy": "script-src 'self'",
      "content-type": "text/html; charset=utf-8"
    });
    response.end(pullRequestPage(!url.pathname.includes("/pull/2/")));
    return;
  }
  response.writeHead(200, {
    "content-security-policy": "script-src 'self'",
    "content-type": "text/html; charset=utf-8"
  });
  response.end(blobPage());
});

server.listen(port, "127.0.0.1", () => {
  console.log(`TypeRB GitHub fixture listening on http://127.0.0.1:${port}`);
});
