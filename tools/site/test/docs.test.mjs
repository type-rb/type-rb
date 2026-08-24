import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import vm from "node:vm";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const scriptPath = resolve(repositoryRoot, "internal/site/assets/docs.js");

test("adds an accessible copy button to documentation code blocks", async () => {
  const source = await readFile(scriptPath, "utf8");
  const listeners = new Map();
  const copied = [];
  const code = { tagName: "CODE", textContent: "puts(\"Hello\")\n" };
  const pre = {
    firstElementChild: code,
    before(element) {
      this.wrapper = element;
    }
  };
  const wrapper = {
    children: [],
    append(element) {
      this.children.push(element);
    }
  };
  const button = {
    dataset: {},
    attributes: new Map(),
    setAttribute(name, value) {
      this.attributes.set(name, value);
    },
    addEventListener(name, listener) {
      listeners.set(name, listener);
    }
  };
  const document = {
    querySelectorAll(selector) {
      assert.equal(selector, ".markdown-body pre");
      return [pre];
    },
    createElement(tagName) {
      if (tagName === "div") return wrapper;
      if (tagName === "button") return button;
      throw new Error(`unexpected element: ${tagName}`);
    }
  };
  const window = {
    isSecureContext: true,
    clearTimeout() {},
    setTimeout() {
      return 1;
    }
  };
  const navigator = {
    clipboard: {
      async writeText(value) {
        copied.push(value);
      }
    }
  };

  vm.runInNewContext(source, { document, navigator, window });

  assert.equal(pre.wrapper, wrapper);
  assert.deepEqual(wrapper.children, [pre, button]);
  assert.equal(button.textContent, "Copy");
  assert.equal(button.attributes.get("aria-label"), "Copy code");
  assert.equal(button.attributes.get("aria-live"), "polite");

  await listeners.get("click")();

  assert.deepEqual(copied, [code.textContent]);
  assert.equal(button.textContent, "Copied");
  assert.equal(button.dataset.state, "copied");
  assert.equal(button.disabled, false);
});
