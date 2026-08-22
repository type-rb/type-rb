import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const testDirectory = dirname(fileURLToPath(import.meta.url));
const userscript = await readFile(resolve(testDirectory, "../typerb-github.user.js"), "utf8");
const chromeContentScript = await readFile(
  resolve(testDirectory, "../dist/chrome-extension/content.js"),
  "utf8"
).catch(() => null);
const chromeContentStyles = await readFile(
  resolve(testDirectory, "../dist/chrome-extension/content.css"),
  "utf8"
).catch(() => null);
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

const storePreviewSource = `import { Router } from trb/http

record Greeting
\tmessage: String
\tstatus: 200
end

class GreetingController
\tdef show(name: String): Greeting
\t\tmessage := "Hello, #{name}!"
\t\treturn Greeting.new(message: message, status: 200)
\tend
end

def main()
\trouter := Router.new()
\trouter.get("/hello/:name", GreetingController.new().show)
\trouter.listen(3000)
end`;

function escapeHtml(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function page(body, distribution) {
  const assets = distribution === "chrome"
    ? '<link rel="stylesheet" href="/content.css"><script src="/content.js"></script>'
    : '<script src="/typerb-github.user.js"></script>';
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
    ${assets}
  </body>
</html>`;
}

function blobPage(distribution) {
  return page(`
    <textarea data-testid="read-only-cursor-text-area">def greet(name: String): String

  return "Hello, #{name}"
end</textarea>
    <div class="react-code-lines">
      <div class="react-file-line" id="LC1">def greet(name: String): String</div>
      <div class="react-file-line" id="LC2"><br></div>
      <div class="react-file-line" id="LC3">  return "Hello, #{name}"</div>
      <div class="react-file-line" id="LC4">end</div>
    </div>
    <pre lang="trb"><code># Markdown comment
puts("Markdown string")</code></pre>
  `, distribution);
}

function pullRequestPage(includePayload = true, distribution) {
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
  `, distribution);
}

function storePreviewPage() {
  const lines = storePreviewSource.split("\n").map((line, index) => `
    <div class="line-row">
      <span class="line-number">${index + 1}</span>
      <span class="react-file-line" id="LC${index + 1}">${escapeHtml(line)}</span>
    </div>`).join("");
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="/content.css">
    <style>
      :root {
        --color-prettylights-syntax-comment: #8c959f;
        --color-prettylights-syntax-constant: #79c0ff;
        --color-prettylights-syntax-entity: #d2a8ff;
        --color-prettylights-syntax-keyword: #ff7b72;
        --color-prettylights-syntax-storage-modifier-import: #ff7b72;
        --color-prettylights-syntax-string: #a5d6ff;
        --color-prettylights-syntax-variable: #ffa657;
      }
      * { box-sizing: border-box; }
      body {
        margin: 0;
        color: #f0f6fc;
        background: #0d1117;
        font: 14px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }
      header.site {
        height: 72px;
        display: flex;
        align-items: center;
        gap: 16px;
        padding: 0 48px;
        background: #010409;
        border-bottom: 1px solid #30363d;
      }
      .mark { width: 38px; height: 38px; }
      .title { font-size: 19px; font-weight: 650; }
      .subtitle { margin-left: auto; color: #8c959f; }
      main { width: 1120px; margin: 38px auto; }
      .path { margin-bottom: 18px; color: #8c959f; font-size: 16px; }
      .path strong { color: #58a6ff; }
      .badge {
        float: right;
        padding: 4px 10px;
        color: #3fb950;
        background: #0f2d1b;
        border: 1px solid #238636;
        border-radius: 999px;
        font-size: 12px;
        font-weight: 600;
      }
      .file { overflow: hidden; border: 1px solid #30363d; border-radius: 8px; }
      .file-header {
        display: flex;
        align-items: center;
        height: 54px;
        padding: 0 20px;
        color: #8c959f;
        background: #161b22;
        border-bottom: 1px solid #30363d;
      }
      .language { margin-left: auto; color: #f0f6fc; }
      .code { padding: 16px 0 20px; font: 15px/1.55 ui-monospace, SFMono-Regular, Menlo, monospace; }
      .line-row { display: flex; min-height: 23px; }
      .line-row:hover { background: #161b22; }
      .line-number { width: 64px; padding-right: 18px; color: #484f58; text-align: right; user-select: none; }
      .react-file-line { flex: 1; white-space: pre; }
      textarea { display: none; }
    </style>
  </head>
  <body>
    <header class="site">
      <svg class="mark" viewBox="0 0 128 128" aria-hidden="true">
        <rect x="10" y="10" width="108" height="108" rx="24" fill="#161b22"/>
        <rect x="10.5" y="10.5" width="107" height="107" rx="23.5" fill="none" stroke="#484f58"/>
        <circle cx="27" cy="27" r="3" fill="#ff7b72"/>
        <circle cx="38" cy="27" r="3" fill="#ffa657"/>
        <circle cx="49" cy="27" r="3" fill="#7ee787"/>
        <path d="M11 40h106" stroke="#30363d" stroke-width="2"/>
        <g stroke-linecap="round" stroke-width="8">
          <path d="M25 57h17" stroke="#d2a8ff"/>
          <path d="M51 57h38" stroke="#79c0ff"/>
          <path d="M98 57h7" stroke="#8c959f"/>
          <path d="M34 77h25" stroke="#ffa657"/>
          <path d="M68 77h37" stroke="#7ee787"/>
          <path d="M25 97h29" stroke="#79c0ff"/>
          <path d="M63 97h12" stroke="#ff7b72"/>
          <path d="M84 97h21" stroke="#d2a8ff"/>
        </g>
      </svg>
      <div class="title">TypeRB Syntax Highlighting for GitHub</div>
      <div class="subtitle">Canonical TextMate grammar · Manifest V3</div>
    </header>
    <main>
      <div class="path"><strong>type-rb</strong> / examples / web / greeting.trb <span class="badge">Highlighted</span></div>
      <section class="file">
        <div class="file-header">greeting.trb <span class="language">TypeRB</span></div>
        <textarea data-testid="read-only-cursor-text-area">${escapeHtml(storePreviewSource)}</textarea>
        <div class="code react-code-lines">${lines}</div>
      </section>
    </main>
    <script src="/content.js"></script>
  </body>
</html>`;
}

const server = createServer((request, response) => {
  const url = new URL(request.url, `http://${request.headers.host}`);
  if (url.searchParams.has("store-preview")) {
    response.writeHead(200, {
      "content-security-policy": "script-src 'self'; style-src 'self' 'unsafe-inline'",
      "content-type": "text/html; charset=utf-8"
    });
    response.end(storePreviewPage());
    return;
  }
  if (url.pathname === "/typerb-github.user.js") {
    response.writeHead(200, { "content-type": "text/javascript; charset=utf-8" });
    response.end(userscript);
    return;
  }
  if (url.pathname === "/content.js" && chromeContentScript) {
    response.writeHead(200, { "content-type": "text/javascript; charset=utf-8" });
    response.end(chromeContentScript);
    return;
  }
  if (url.pathname === "/content.css" && chromeContentStyles) {
    response.writeHead(200, { "content-type": "text/css; charset=utf-8" });
    response.end(chromeContentStyles);
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
    response.end(pullRequestPage(
      !url.pathname.includes("/pull/2/"),
      url.searchParams.get("distribution")
    ));
    return;
  }
  response.writeHead(200, {
    "content-security-policy": "script-src 'self'",
    "content-type": "text/html; charset=utf-8"
  });
  response.end(blobPage(url.searchParams.get("distribution")));
});

server.listen(port, "127.0.0.1", () => {
  console.log(`TypeRB GitHub fixture listening on http://127.0.0.1:${port}`);
});
