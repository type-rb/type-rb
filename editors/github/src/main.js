import { runTypeRBHighlighting } from "./runtime.js";
import { HIGHLIGHT_STYLES } from "./styles.js";

function addStyles() {
  if (typeof GM_addStyle === "function") {
    GM_addStyle(HIGHLIGHT_STYLES);
    return;
  }
  const style = document.createElement("style");
  style.textContent = HIGHLIGHT_STYLES;
  document.head.append(style);
}

runTypeRBHighlighting({ addStyles });
