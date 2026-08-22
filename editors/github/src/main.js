import { grammarJson, onigWasmBase64 } from "typerb:assets";

import {
  HIGHLIGHT_STYLES,
  highlightGitHubDocument
} from "./github.js";
import { createTextMateHighlighter } from "./textmate-highlighter.js";

function addStyles() {
  if (typeof GM_addStyle === "function") {
    GM_addStyle(HIGHLIGHT_STYLES);
    return;
  }
  const style = document.createElement("style");
  style.textContent = HIGHLIGHT_STYLES;
  document.head.append(style);
}

async function start() {
  addStyles();
  const highlighter = await createTextMateHighlighter({
    grammarJson,
    onigWasm: onigWasmBase64
  });
  const cache = new Map();
  let timer;
  let scanning = false;
  let queued = false;

  async function scan() {
    if (scanning) {
      queued = true;
      return;
    }
    scanning = true;
    try {
      await highlightGitHubDocument({ document, location, highlighter, cache });
    } catch (error) {
      console.warn("[TypeRB GitHub] Highlighting failed", error);
    } finally {
      scanning = false;
      if (queued) {
        queued = false;
        schedule();
      }
    }
  }

  function schedule() {
    clearTimeout(timer);
    timer = setTimeout(scan, 80);
  }

  new MutationObserver(schedule).observe(document.documentElement, {
    childList: true,
    subtree: true
  });
  document.addEventListener("turbo:load", schedule);
  document.addEventListener("pjax:end", schedule);
  window.addEventListener("popstate", schedule);
  window.addEventListener("urlchange", schedule);
  await scan();
}

start().catch((error) => {
  console.warn("[TypeRB GitHub] Initialization failed", error);
});
