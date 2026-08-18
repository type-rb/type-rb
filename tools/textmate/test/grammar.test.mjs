import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import oniguruma from "vscode-oniguruma";
import textmate from "vscode-textmate";

const require = createRequire(import.meta.url);
const testDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(testDirectory, "../../..");
const syntaxDirectory = resolve(repositoryRoot, "syntaxes");
const manifestPath = resolve(syntaxDirectory, "manifest.json");
const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
const grammarPath = resolve(syntaxDirectory, manifest.grammar);

const wasmPath = require.resolve("vscode-oniguruma/release/onig.wasm");
const wasm = await readFile(wasmPath);
await oniguruma.loadWASM(wasm.buffer.slice(wasm.byteOffset, wasm.byteOffset + wasm.byteLength));

const registry = new textmate.Registry({
  onigLib: Promise.resolve({
    createOnigScanner(sources) {
      return new oniguruma.OnigScanner(sources);
    },
    createOnigString(source) {
      return new oniguruma.OnigString(source);
    }
  }),
  async loadGrammar(scopeName) {
    if (scopeName !== manifest.scopeName) {
      return null;
    }
    return textmate.parseRawGrammar(await readFile(grammarPath, "utf8"), grammarPath);
  }
});

const grammar = await registry.loadGrammar(manifest.scopeName);

function tokenize(source) {
  let ruleStack = textmate.INITIAL;
  const lines = source.split(/\r?\n/).map((line, lineIndex) => {
    const result = grammar.tokenizeLine(line, ruleStack);
    ruleStack = result.ruleStack;
    return result.tokens.map((token) => ({
      line: lineIndex + 1,
      text: line.slice(token.startIndex, token.endIndex),
      scopes: token.scopes
    }));
  });
  return { tokens: lines.flat(), ruleStack };
}

function assertScope(tokens, text, scope, message = "") {
  const token = tokens.find((candidate) => candidate.text === text && candidate.scopes.includes(scope));
  assert.ok(token, message || `expected ${JSON.stringify(text)} to include ${scope}`);
}

function assertNotInScope(tokens, text, scopePrefix) {
  const token = tokens.find((candidate) => candidate.text.trim() === text);
  assert.ok(token, `expected token ${JSON.stringify(text)}`);
  assert.equal(
    token.scopes.some((scope) => scope.startsWith(scopePrefix)),
    false,
    `${text} unexpectedly remained in ${scopePrefix}`
  );
}

async function fixture(name) {
  return readFile(resolve(testDirectory, "fixtures", name), "utf8");
}

test("publishes reusable TypeRB metadata and loads in vscode-textmate", async () => {
  assert.deepEqual(manifest, {
    name: "TypeRB",
    languageId: "trb",
    scopeName: "source.trb",
    extensions: [".trb"],
    grammar: "./typerb.tmLanguage.json",
    license: "MIT"
  });
  assert.ok(grammar, "vscode-textmate did not load source.trb");
});

test("scopes representative portable TypeRB syntax", async () => {
  const { tokens, ruleStack } = tokenize(await fixture("representative.trb"));

  assertScope(tokens, "import", "keyword.control.import.trb");
  assertScope(tokens, "trb/std/result", "string.unquoted.import-path.trb");
  assertScope(tokens, "module", "storage.type.trb");
  assertScope(tokens, "Demo", "entity.name.type.trb");
  assertScope(tokens, "API_VERSION", "constant.other.trb");
  assertScope(tokens, "record", "storage.type.trb");
  assertScope(tokens, "User", "entity.name.type.trb");
  assertScope(tokens, "T", "entity.name.type.parameter.trb");
  assertScope(tokens, "@name", "variable.other.instance.trb");
  assertScope(tokens, "render", "entity.name.function.trb");
  assertScope(tokens, "fn", "storage.type.trb");
  assertScope(tokens, "String", "support.type.builtin.trb");
  assertScope(tokens, "Outcome", "entity.name.type.trb");
  assertScope(tokens, "::", "punctuation.accessor.namespace.trb");
  assertScope(tokens, "Ok", "constant.other.enum-member.trb");
  assertScope(tokens, "if", "keyword.control.trb");
  assertScope(tokens, "#{", "punctuation.section.interpolation.begin.trb");
  assertScope(tokens, "puts", "support.function.builtin.trb");
  assertScope(tokens, ">", "keyword.operator.trb");
  assertScope(tokens, "(", "punctuation.section.parens.trb");
  assert.equal(ruleStack.depth, 1, "representative fixture left an open TextMate rule");
});

test("keeps ambiguous and multiline constructs bounded", async () => {
  const { tokens, ruleStack } = tokenize(await fixture("ambiguous.trb"));

  assertScope(tokens, "ready?", "entity.name.function.trb");
  assertScope(tokens, "not # a comment ", "string.quoted.double.trb");
  assertScope(tokens, "still not # a comment", "string.quoted.single.trb");
  assertScope(tokens, "Hash", "support.type.builtin.trb");
  assertScope(tokens, "...", "keyword.operator.trb");
  assertScope(tokens, "<<~", "punctuation.definition.string.begin.trb");
  assertScope(tokens, " highlighting resumes here", "comment.line.number-sign.trb");
  assertScope(tokens, "42", "constant.numeric.trb");
  assertNotInScope(tokens, "after", "string.");
  assert.equal(ruleStack.depth, 1, "ambiguous fixture left an open TextMate rule");
});

test("scopes Result propagation and recovery keywords", () => {
  const { tokens, ruleStack } = tokenize(`value := try load()
fallback := load() catch |error|
\treturn error.message
end`);

  assertScope(tokens, "try", "keyword.control.trb");
  assertScope(tokens, "catch", "keyword.control.trb");
  assert.equal(ruleStack.depth, 1, "Result control-flow fixture left an open TextMate rule");
});

test("does not highlight callable keyword prefixes as control flow", () => {
  const { tokens, ruleStack } = tokenize(`def catch?(): Boolean
\treturn true
end

def try!()
\treturn
end`);

  assertNotInScope(tokens, "catch?", "keyword.control");
  assertNotInScope(tokens, "try!", "keyword.control");
  assert.equal(ruleStack.depth, 1, "callable suffix fixture left an open TextMate rule");
});
