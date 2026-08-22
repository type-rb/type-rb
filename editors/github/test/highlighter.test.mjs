import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import { createTextMateHighlighter } from "../src/textmate-highlighter.js";

const testDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(testDirectory, "../../..");
const grammarJson = await readFile(resolve(repositoryRoot, "syntaxes/typerb.tmLanguage.json"), "utf8");
const highlighter = await createTextMateHighlighter({ grammarJson });

function segment(lines, text) {
  return lines.flat().find((candidate) => candidate.text === text);
}

test("maps canonical TypeRB scopes to GitHub-aware token classes", () => {
  const lines = highlighter.tokenizeSource(`def greet(name: String): String
\t# greeting
\treturn "Hello, #{name}"
end`);

  assert.equal(segment(lines, "def").className, "typerb-token-storage");
  assert.equal(segment(lines, "greet").className, "typerb-token-function");
  assert.equal(segment(lines, "String").className, "typerb-token-type");
  assert.equal(segment(lines, "# greeting").className, "typerb-token-comment");
  assert.equal(segment(lines, "return").className, "typerb-token-keyword");
  assert.equal(
    segment(highlighter.tokenizeSource("@name = name"), "@name").className,
    "typerb-token-variable"
  );
});

test("retains TextMate state across multiline constructs", () => {
  const lines = highlighter.tokenizeSource(`message := <<~TEXT
  hello # still a string
TEXT
# comment`);

  assert.equal(segment(lines, "  hello # still a string").className, "typerb-token-string");
  assert.equal(segment(lines, "# comment").className, "typerb-token-comment");
});

test("tokenizes the canonical grammar fixtures without losing source text", async () => {
  for (const fixture of ["representative.trb", "ambiguous.trb"]) {
    const source = await readFile(
      resolve(repositoryRoot, "tools/textmate/test/fixtures", fixture),
      "utf8"
    );
    const lines = highlighter.tokenizeSource(source);
    assert.equal(
      lines.map((line) => line.map((part) => part.text).join("")).join("\n"),
      source.replace(/\r\n|\r/g, "\n")
    );
  }
});
