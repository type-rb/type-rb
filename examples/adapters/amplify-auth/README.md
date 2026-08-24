# Amplify Auth declaration adapter example

This adapter-only package is a dogfood fixture, not a published compatibility
package. It projects the small `aws-amplify/auth` surface needed to submit a
username and password and observe whether sign-in completed.

The native dependency is pinned to `aws-amplify` 6.20.0. Revalidate the
adapter, generated TypeScript, and runtime observations before changing that
version.

The fixture deliberately keeps two boundaries separate:

- TypeRB calls `signIn` through the existing
  `promise_rejection_to_result` bridge. A validation failure becomes
  `Err<String>` and retains the native error message.
- An application-owned TypeScript bootstrap imports the generated
  `amplify_outputs.json`, calls `Amplify.configure`, and then calls the
  generated TypeRB function. Configuration generation remains an explicit
  Amplify toolchain step; the compiler and package adapter do not inspect a
  project-specific file.

Amplify failures can carry more information than a message. The runtime
observation verifies an unconfigured `signIn` rejection with the native name
`AuthUserPoolException`, a message, and a recovery suggestion. The current
bridge intentionally exposes only the message because Protocol v2 has no
package-owned, total rejection mapper for record or enum errors. Adding a
generic mapper from this one direct-call example would be premature: packages
that need a richer SDK boundary can still normalize an opaque value in a
package-owned shim, while a reusable direct Promise mapper needs another
concrete shape and explicit rules for unknown rejection values.

The complete multi-step `nextStep` result is also omitted rather than becoming
`Any`. It is a large discriminated native union and should be projected only
when an application exercises that flow.

Install the conformance project's locked dependencies once from the TypeRB
checkout:

```sh
./trb install --frozen --config examples/adapters/amplify-auth/conformance/trbconfig.jsonc
```

Then run all declared conformance phases from the adapter package root:

```sh
./trb adapter test --format json examples/adapters/amplify-auth
```

`adapter test` validates the catalog, builds the TypeRB project, checks the
generated TypeScript against Amplify 6.20.0, observes the rich native error in
one Bun process, and verifies the String bridge plus generated configuration
bootstrap in another. TypeRB does not install dependencies implicitly.
