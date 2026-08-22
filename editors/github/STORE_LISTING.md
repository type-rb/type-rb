# Chrome Web Store listing

## Product details

- Name: `TypeRB Syntax Highlighting for GitHub`
- Summary: `Highlight TypeRB files, pull request diffs, and Markdown code blocks on GitHub.`
- Category: `Developer Tools`
- Primary language: `English`
- Chrome Web Store: `https://chromewebstore.google.com/detail/typerb-syntax-highlightin/icogpeecnhfgfdbdjihfhmngpengkcni`
- Homepage: `https://github.com/type-rb/type-rb/tree/main/editors/github`
- Support: `https://github.com/type-rb/type-rb/issues`
- Privacy policy: `https://github.com/type-rb/type-rb/blob/main/editors/github/PRIVACY.md`

## Detailed description

TypeRB Syntax Highlighting for GitHub makes TypeRB code easier to read while
browsing repositories and reviewing changes on github.com.

The extension highlights `.trb` file views, unified and split pull request
diffs, rendered `trb` and `typerb` Markdown code blocks, and suggestion blocks
attached to TypeRB diffs. It uses TypeRB's canonical TextMate grammar, including
multiline lexical state, so the result matches other TypeRB editor tooling.

The extension is self-contained, has no settings or telemetry, and does not
download executable code. It requests no optional Chrome API permissions.

## Privacy declarations

- Single purpose: syntax-highlight TypeRB source displayed on github.com.
- Site access justification: the content script must read and style GitHub code
  views; for accurate pull request highlighting it may fetch the exact `.trb`
  revisions referenced by the current GitHub page.
- Data collection: none.
- Remote code: none. The grammar and all executable dependencies are bundled.
- Authentication information: not collected. Same-origin source requests use
  the GitHub session already managed by the browser.

## Graphic assets

- Store icon: [`assets/icon-128.png`](assets/icon-128.png)
- Screenshot: `store-assets/screenshot-1280x800.png`
- Small promo tile: `store-assets/promo-small-440x280.png`
