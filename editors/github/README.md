# TypeRB syntax highlighting for GitHub

This package applies TypeRB's canonical TextMate grammar to GitHub pages before
TypeRB is available through GitHub Linguist. It is distributed as a Manifest V3
Chrome extension.

The extension bundles its runtime, canonical grammar,
`@shikijs/vscode-textmate`, and Shiki JavaScript regular expression engine. It
does not download executable code or a second grammar at runtime.

## Install

Install
[TypeRB Syntax Highlighting for GitHub](https://chromewebstore.google.com/detail/typerb-syntax-highlightin/icogpeecnhfgfdbdjihfhmngpengkcni)
from the Chrome Web Store. No configuration is required.

## Local build

Build the unpacked extension and its Chrome Web Store ZIP:

```sh
npm ci --prefix editors/github
npm run package:chrome --prefix editors/github
```

Load `editors/github/dist/chrome-extension` from `chrome://extensions` with
Developer mode enabled. The Web Store upload is
`editors/github/dist/typerb-github-chrome-extension.zip`; its `manifest.json`
is at the ZIP root.

The extension requests no optional Chrome API permissions. Its only site access
is the `https://github.com/*` content-script match. See the
[privacy policy](PRIVACY.md) for the exact data behavior.

## Supported GitHub surfaces

- `.trb` repository blob views;
- unified and split pull request diffs, including dynamically loaded lines;
- rendered `trb` and `typerb` fenced Markdown blocks in files, pull requests,
  issues, discussions, and comments; and
- `suggestion` blocks attached to a `.trb` pull request diff.

For a pull request diff, the highlighter reads the base and head commit IDs from
GitHub's page data and fetches the exact file revisions from the repository
currently being viewed. Deleted lines use the base revision and added lines use
the head revision. If a complete revision is unavailable, contiguous displayed
lines are highlighted with a fresh lexical state at each gap.

Code search, commit and compare diffs, blame, raw files, Gists, `github.dev`,
GitHub Enterprise Server, and the GitHub mobile application are not supported
by the initial release. A diff that GitHub does not render cannot be augmented.

## Chrome Web Store release

The `Chrome extension release` GitHub Actions workflow always creates a
downloadable ZIP. With the `chrome-web-store` environment configured, it can
also upload through Chrome Web Store API v2, submit for review as a staged
release, and publish the approved staged revision in a separate manual run.

Configure these environment variables:

- `GCP_WORKLOAD_IDENTITY_PROVIDER`: GitHub Actions Workload Identity provider;
- `CWS_SERVICE_ACCOUNT`: service account linked in the Chrome Web Store
  Developer Dashboard;
- `CWS_PUBLISHER_ID`: Chrome Web Store publisher ID; and
- `CWS_EXTENSION_ID`: Chrome Web Store item ID.

The workflow uses short-lived GitHub OIDC credentials. Do not add a service
account key to the repository. Before the first API submission, create the item
and complete its Store listing, Privacy, and distribution settings in the
Developer Dashboard. A visibility change made later in the Dashboard must be
published manually once before the API can publish with that visibility.

## Development

Install dependencies, rebuild the extension, and run the tests:

```sh
npm ci --prefix editors/github
npm run build --prefix editors/github
npm test --prefix editors/github
```

Run `npm run fixture --prefix editors/github` to serve local blob, Markdown,
and pull request fixtures for browser-level checks.

Chrome extension files and the upload ZIP are generated under the ignored
`editors/github/dist` directory. Bundled license notices are generated from the
dependencies included by esbuild.
