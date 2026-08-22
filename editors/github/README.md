# TypeRB syntax highlighting for GitHub

This Tampermonkey userscript applies TypeRB's canonical TextMate grammar to
GitHub pages before TypeRB is available through GitHub Linguist.

## Install

1. Install [Tampermonkey](https://www.tampermonkey.net/) in a desktop browser.
2. Open the
   [TypeRB GitHub userscript](https://raw.githubusercontent.com/type-rb/type-rb/main/editors/github/typerb-github.user.js).
3. Review the script and confirm the Tampermonkey installation prompt.

Tampermonkey checks the same URL for version updates. The userscript is
self-contained: it does not load code or the grammar from a CDN. It uses a
JavaScript regular expression engine so GitHub's Content Security Policy does
not need to permit WebAssembly compilation.

## Supported GitHub surfaces

- `.trb` repository blob views;
- unified and split pull request diffs, including dynamically loaded lines;
- rendered `trb` and `typerb` fenced Markdown blocks in files, pull requests,
  issues, discussions, and comments; and
- `suggestion` blocks attached to a `.trb` pull request diff.

For a pull request diff, the userscript reads the base and head commit IDs from
GitHub's page data and fetches the exact file revisions from the repository
currently being viewed. Deleted lines use the base revision and added lines use
the head revision. If a complete revision is unavailable, contiguous displayed
lines are highlighted with a fresh lexical state at each gap.

Code search, commit and compare diffs, blame, raw files, Gists, `github.dev`,
GitHub Enterprise Server, and the GitHub mobile application are not supported
by the initial release. A diff that GitHub does not render cannot be augmented
by the userscript.

## Privacy and permissions

The userscript runs only on `https://github.com/*`. It does not collect
telemetry, store repository contents, or send data to TypeRB services. For
accurate pull request highlighting it may fetch `.trb` source from the same
GitHub repository and commit already visible to the signed-in user. Source
tokens are cached only in memory for the current page session.

## Development

Install dependencies, rebuild the distributable userscript, and run the tests:

```sh
npm ci --prefix editors/github
npm run build --prefix editors/github
npm test --prefix editors/github
```

Run `npm run fixture --prefix editors/github` to serve local blob, Markdown,
and pull request fixtures for browser-level checks.

The build embeds
[`../../syntaxes/typerb.tmLanguage.json`](../../syntaxes/typerb.tmLanguage.json),
`@shikijs/vscode-textmate`, and the Shiki JavaScript regular expression engine
into [`typerb-github.user.js`](typerb-github.user.js). Commit the rebuilt
userscript whenever its source, dependency versions, metadata version, or
canonical grammar changes. Bundled license notices are included in the
generated file.
