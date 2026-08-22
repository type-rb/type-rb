import oniguruma from "vscode-oniguruma";
import textmate from "vscode-textmate";

let onigurumaReady;

function decodeBase64(value) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function wasmBuffer(value) {
  const bytes = typeof value === "string" ? decodeBase64(value) : value;
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}

function tokenClass(scopes) {
  for (let index = scopes.length - 1; index >= 0; index -= 1) {
    const scope = scopes[index];
    if (scope.startsWith("invalid.") || scope.startsWith("invalid")) {
      return "typerb-token-invalid";
    }
    if (scope.startsWith("comment.")) {
      return "typerb-token-comment";
    }
    if (scope.startsWith("constant.character.escape")) {
      return "typerb-token-escape";
    }
    if (scope.startsWith("constant.")) {
      return "typerb-token-constant";
    }
    if (scope.startsWith("keyword.")) {
      return "typerb-token-keyword";
    }
    if (scope.startsWith("storage.")) {
      return "typerb-token-storage";
    }
    if (scope.startsWith("entity.name.function") || scope.startsWith("support.function")) {
      return "typerb-token-function";
    }
    if (scope.startsWith("entity.name.type") || scope.startsWith("support.type")) {
      return "typerb-token-type";
    }
    if (scope.startsWith("variable.")) {
      return "typerb-token-variable";
    }
    if (scope.startsWith("string.")) {
      return "typerb-token-string";
    }
  }
  return null;
}

function segmentsForTokens(line, tokens) {
  const segments = [];
  for (const token of tokens) {
    const text = line.slice(token.startIndex, token.endIndex);
    if (text.length === 0) {
      continue;
    }
    const className = tokenClass(token.scopes);
    const previous = segments.at(-1);
    if (previous?.className === className) {
      previous.text += text;
    } else {
      segments.push({ text, className });
    }
  }
  return segments;
}

export async function createTextMateHighlighter({ grammarJson, onigWasm }) {
  onigurumaReady ??= oniguruma.loadWASM(wasmBuffer(onigWasm));
  await onigurumaReady;

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
      if (scopeName !== "source.trb") {
        return null;
      }
      return textmate.parseRawGrammar(grammarJson, "typerb.tmLanguage.json");
    }
  });
  const grammar = await registry.loadGrammar("source.trb");
  if (!grammar) {
    throw new Error("TypeRB TextMate grammar could not be loaded");
  }

  return {
    initialState() {
      return textmate.INITIAL;
    },
    tokenizeLine(line, ruleStack = textmate.INITIAL) {
      const result = grammar.tokenizeLine(line, ruleStack);
      return {
        segments: segmentsForTokens(line, result.tokens),
        ruleStack: result.ruleStack
      };
    },
    tokenizeSource(source) {
      let ruleStack = textmate.INITIAL;
      return source.split(/\r\n|\n|\r/).map((line) => {
        const result = grammar.tokenizeLine(line, ruleStack);
        ruleStack = result.ruleStack;
        return segmentsForTokens(line, result.tokens);
      });
    }
  };
}

export { tokenClass };
