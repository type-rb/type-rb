# Auth0 React declaration adapter example

This adapter-only package is a dogfood fixture, not a published compatibility
package. It projects the small `@auth0/auth0-react` surface needed to configure
`Auth0Provider`, read loading and authentication state from `useAuth0()`, and
invoke the no-argument token, login, and logout operations.

The native dependency is pinned to `@auth0/auth0-react` 2.21.0. Revalidate the
adapter and its generated TypeScript before changing that version.

This fixture verifies that a declaration-adapter interface can expose readonly
native properties without becoming constructible. The state projection uses
`isLoading` and `isAuthenticated`; the complete user and native error values
are deliberately absent rather than becoming `Any`.

Auth0's token, login, and logout methods return Promises that may reject. The
`promise_rejection_to_result` call bridge exposes their no-argument forms as
checked `Result` values and marks their callers as backend-suspending. A
resolved Promise becomes `Ok`; an `Error` rejection becomes its message and
any other rejection is converted with `String(...)` before becoming
`Err<String>`. A failed String conversion uses `"Unknown native rejection"`.
Rich native error projections remain outside this first bridge.

Install the conformance project's locked dependencies once from the TypeRB
checkout:

```sh
./trb install --frozen --config examples/adapters/auth0-react/conformance/trbconfig.jsonc
```

Then run all declared conformance phases from the adapter package root:

```sh
./trb adapter test --format json examples/adapters/auth0-react
```

`adapter test` validates the catalog, builds the TypeRB project, and invokes
the declared `bun run check` argv directly. TypeRB does not install dependencies
implicitly; Bun retains its ordinary script and process behavior.
