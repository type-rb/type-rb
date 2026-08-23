# TanStack Router declaration adapter example

This adapter-only package is a dogfood fixture, not a published compatibility
package. It projects the stable TanStack Router surface needed by the included
TypeRB example: the current router, instance navigation, and `Outlet`.

The native dependency is pinned to `@tanstack/react-router` 1.170.29. Revalidate
the adapter and its generated TypeScript before changing that version.
The conformance project enables `skipLibCheck` because that release contains
an internal declaration-file error under TypeScript 6.0.3. Strict checking of
the generated application remains enabled; only errors wholly inside dependency
`.d.ts` files are skipped.

From a TypeRB source checkout, validate the package-owned catalog with:

```sh
./trb adapter check --format json examples/adapters/tanstack-router
```

The example deliberately characterizes the boundary between a reusable native
declaration adapter and project-specific route generation:

- the adapter can describe stable native functions, records, interfaces, and
  React components without compiler integration;
- an adapter interface exposes `instanceMembers` without becoming
  constructible, so `router.navigate(...)` remains an instance call through
  TypeRB checking and generated TypeScript; and
- the route graph, route-construction generics, exact parameter fields, and
  valid navigation destinations are deliberately absent from the fixed
  catalog.

TanStack Router derives those omitted types from the complete route tree. Its
public `Route`, `RouteOptions`, and `Router` contracts cannot be projected as
one fixed set of non-generic declarations without losing native assignability.
A reusable router integration therefore needs a project provider that reads a
narrow route model and returns checked TypeRB source or declarations for route
construction, route-specific parameters, search values, and destinations. It
does not need raw compiler AST, typed IR, or target-language source.

The catalog is a checked projection, not a copy of the complete TanStack Router
API. Unsupported APIs remain absent rather than becoming `Any`.

Install the conformance project's locked dependencies once from the TypeRB
checkout:

```sh
./trb install --frozen --config examples/adapters/tanstack-router/conformance/trbconfig.jsonc
```

Then run all declared conformance phases from the adapter package root:

```sh
./trb adapter test --format json examples/adapters/tanstack-router
```

`adapter test` validates the catalog, builds the TypeRB project, and invokes
the declared `bun run check` argv directly. TypeRB does not install dependencies
implicitly; Bun retains its ordinary script and process behavior.
