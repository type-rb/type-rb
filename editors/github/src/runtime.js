import { grammarJson } from "typerb:assets";

import { highlightGitHubDocument } from "./github.js";
import { createTextMateHighlighter } from "./textmate-highlighter.js";

export async function startTypeRBHighlighting({ addStyles } = {}) {
  addStyles?.();

  const highlighter = await createTextMateHighlighter({ grammarJson });
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

export function runTypeRBHighlighting(options) {
  startTypeRBHighlighting(options).catch((error) => {
    console.warn("[TypeRB GitHub] Initialization failed", error);
  });
}
