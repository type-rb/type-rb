# htmx attribute conformance

This integration fixture uses `htmx.org` **4.0.0**, React **19.3.0**, and
TypeScript **6.0.3**. It is a bounded example, not a published compatibility
package or a promise to support every htmx API. The upstream
[release announcement](https://four.htmx.org/announcements/2026-08-28-htmx-4.0.0-is-released)
and [installation guide](https://four.htmx.org/docs) describe this version.

## Verified boundary

TypeRB JSX produces `hx-get`, `hx-target`, and `hx-swap`, as well as their
`data-hx-*` equivalents. The test renders the generated component to static
HTML, loads the real htmx browser runtime, clicks each button, and checks the
request header and resulting DOM replacement. React is only the markup
renderer in this fixture; there is no hydrated React tree sharing DOM ownership
with htmx.

The application-owned `src/native/bootstrap.ts` imports htmx and initializes
it. That authored file is copied and checked against the original `.d.ts`;
generated TypeScript is never edited. No declaration adapter, runtime adapter,
or compiler extension is necessary for this attribute workflow. Consequently,
this fixture uses ordinary build/native conformance instead of declaring an
empty catalog merely to run `trb adapter test`.

The native check uses TypeScript's `Bundler` module resolution, matching the
browser bundle and htmx's ESM `module` entry. The package's CommonJS-style
`main` entry is not the entry used by this browser fixture.

## Run

From a source checkout with Go, Bun, and Node.js available:

```sh
./trb install --frozen --config examples/adapters/htmx/trbconfig.jsonc
./trb check --diagnostic-format json --config examples/adapters/htmx/trbconfig.jsonc
./trb build --config examples/adapters/htmx/trbconfig.jsonc
cd examples/adapters/htmx
bunx playwright install chromium
bun run check
```

Linux CI can install browser system dependencies with
`bunx playwright install --with-deps chromium`. The browser test serves only
local assets on an ephemeral loopback port and performs no CDN requests.

`bun run check` runs strict TypeScript checking, the expected TypeRB diagnostic
below, and real browser requests/DOM swaps. Re-run all steps after changing the
compiler, native package, or toolchain versions. Native versions and integrity
values are fixed in `bun.lock`. This fixture has no external TypeRB package
dependencies, so it does not need a `trb.lock`.
The React provider currently requires the literal `latest` dependency request;
the committed Bun lock fixes React and its type packages to 19.3.0. Use frozen
installation to reproduce that selection.

## Direct API boundary and remaining work

The published htmx declaration exposes its API as a **default-exported object**.
It does not supply named runtime functions such as `parseInterval`. The
`unsupported/default-import.trb` probe therefore remains intentionally rejected:

```trb
import { default as htmx } from "htmx.org"
```

The conformance runner checks that the JSON diagnostic identifies the
unsupported default-export shape, rather than falsely claiming the package
has no such export. It writes a temporary alternate config to check the probe
separately from the positive application, then removes that config.

| Observation | Classification | Next boundary |
| --- | --- | --- |
| Ordinary `hx-*` and `data-hx-*` markup works | Documentation | Use the existing JSX path and native application bootstrap |
| A default export was previously omitted without an explanation | Tooling | Preserve the unsupported export in the native index and report it on import |
| Calling methods on the default object from TypeRB is unavailable | Generic capability | Design default-value imports and object-member projection together if a consumer needs them |
| DOM handles, event overloads, and Promise-returning methods have richer types | Library contract and generic capabilities | Select a small callable workflow before projecting signatures or failures |

The fixture does not claim type validation of htmx attribute values: selectors,
swap names, and URLs are strings interpreted by htmx. It also does not cover
colon-modified attributes, extensions, live React reconciliation, or direct
TypeRB calls to htmx methods. Do not invent named exports, copy the entire htmx
signature surface, or replace unsupported types with `Any`.

For the reusable workflow, see the
[native adapter authoring guide](../../../docs/guides/native-package-adapters.md).
