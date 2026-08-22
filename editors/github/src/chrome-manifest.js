export function chromeManifest(version) {
  return {
    manifest_version: 3,
    name: "TypeRB Syntax Highlighting for GitHub",
    version,
    description: "Highlight TypeRB files, pull request diffs, and Markdown code blocks on GitHub.",
    homepage_url: "https://github.com/type-rb/type-rb/tree/main/editors/github",
    icons: {
      16: "images/icon-16.png",
      32: "images/icon-32.png",
      48: "images/icon-48.png",
      128: "images/icon-128.png"
    },
    content_scripts: [{
      matches: ["https://github.com/*"],
      css: ["content.css"],
      js: ["content.js"],
      run_at: "document_idle"
    }]
  };
}
