# TanStack Query declaration adapter example

This adapter-only package is a dogfood fixture, not a published compatibility
package. It projects the small TanStack Query surface needed by the included
TypeRB example: `QueryClient`, `QueryClientProvider`, `UseQueryOptions`,
`queryOptions`, `useQuery`, and the discriminated `UseQueryResult` states.

The native dependency is pinned to `@tanstack/react-query` 5.101.4 because the
library may improve TypeScript declarations in patch releases. Revalidate the
adapter and its generated TypeScript before changing that version.

From a TypeRB source checkout, validate the package-owned catalog with:

```sh
./trb adapter check --format json examples/adapters/tanstack-query
```

The example deliberately demonstrates three adapter capabilities together:

- explicit generic specialization for query options and results;
- status-based narrowing of the native discriminated union; and
- conversion of a suspending `Result<Todo, RequestError>` query callback into
  native Promise resolution and rejection at the declared boundary.

The catalog is a checked projection, not a copy of every TanStack Query option
or result member. Unsupported APIs remain absent rather than becoming `Any`.

The `conformance` project installs the adapter as a local TypeRB package and
compiles `src/app.trb` against the pinned native package. From that directory,
run:

```sh
../../../../trb install --config trbconfig.jsonc
../../../../trb build --config trbconfig.jsonc
bun run check
```
