# Authoring native package adapters

Start with one application workflow and a fixed native package version. The
goal is to expose the smallest useful, checked contract while keeping the
native library responsible for its implementation. Sometimes direct imports
or application startup code already provide everything needed.

This guide describes the authoring workflow. The
[package reference](packages.md) owns manifests, protocol schemas, supported
types, bridge kinds, and validation rules. The
[CLI reference](../cli.md) owns command and JSON-report details.

## 1. Fix the scope and native input

Write down two or three calls or interactions, their expected outputs, and a
failure or boundary case. Pin the native dependency and native toolchain, then
commit the native lockfile and, when using external TypeRB packages, `trb.lock`.
Distinguish tested workflows from untested APIs; successfully compiling one
call does not establish whole-library compatibility.

Use upstream declarations and documentation for the selected version. Check
the actual runtime exports as well as the `.d.ts`, RBS, or other metadata. A
declaration correction must describe a real call: it cannot create a runtime
export that the native package does not have.

## 2. Try the existing path

For a TypeScript package, configure the native dependency and the supported
TypeScript 6.x toolchain, then use `trb install`. Installation indexes supported
named exports from the package's `.d.ts` files. Write the ordinary native import
in `.trb` source and obtain structured diagnostics:

```sh
trb fmt
trb check --diagnostic-format json
trb build
```

Read the diagnostic code, source span, and explanation before writing a
catalog. Automatic indexing does not support all TypeScript types or default
exports. A package that exports a default object must not be treated as though
its methods were named module exports. Regenerate the index with `trb install`
after upgrading the compiler; `.trb/native-types.json` is an internal cache,
not an author-edited signature file or a stable inspection protocol.

Markup-driven libraries may need no TypeRB-callable API. The
[htmx fixture](../../examples/adapters/htmx/README.md) emits attributes from
TypeRB JSX and keeps native startup in a small authored TypeScript file. It
checks the generated output against native types and executes browser requests
and DOM swaps. It also retains an expected diagnostic for the unsupported
default-object import. This is a useful result without an adapter catalog.

Ruby can use a package-owned fixed declaration provider for gem classes and
mixins, with runtime loading owned by its ordinary TypeRB root source. Go and
Ruby do not currently have the automatic native declaration importer available
for TypeScript. Consult the package reference rather than assuming the same
indexing path works in every ecosystem.

## 3. Choose the narrowest sufficient boundary

| Need | Boundary | Author responsibility |
| --- | --- | --- |
| The indexed named export already checks and runs | Direct native import | Pin dependencies and verify generated output |
| HTML attributes and browser startup suffice | Ordinary application source plus native bootstrap | Keep DOM ownership explicit and execute the interaction |
| Existing native exports need a smaller type projection | Declaration adapter | Describe only real exports and preserve generics, nullability, and failure semantics |
| A Ruby gem contributes classes or mixins | Fixed Ruby declaration provider | Own the catalog, root activation, and native `require` |
| A native operation needs conversion or error normalization | Package-owned runtime shim with the supported runtime adapter contract | Own conversion, native errors, and conformance in every declared mode |
| An external schema determines declarations | Explicit generator | Produce ordinary source or declaration data, then run normal checks |
| Application declarations determine new APIs | Project declaration/generation capability | Verify that existing declared inputs can express the requirement before requesting a new provider capability |

A declaration adapter is data; it does not implement runtime conversion.
Native Runtime Adapter v1 has a narrow, non-generic `String -> String` leaf
ABI. Do not pass arbitrary objects, handles, or lifecycle protocols through it
by pretending they are already supported. Fixed Ruby providers and
project-aware bundled providers also have different capabilities.

For a new requirement, keep the competing choices concrete. For example, a
markup interaction can keep the native library in application bootstrap;
a representable named function can use a direct import; a real complex export
may need a declaration projection. Choose using the workflow's ownership,
types, and failures, rather than the library's popularity.

## 4. Validate a projection in layers

An adapter package owns its `trbpackage.json`, declaration data, native version
requirements, and conformance project. Use existing small examples instead of
copying a complete upstream type surface:

- [TanStack Query](../../examples/adapters/tanstack-query/README.md): explicit
  generic specialization, discriminated results, and Result-returning callbacks.
- [TanStack Router](../../examples/adapters/tanstack-router/README.md): typed
  members and interfaces.
- [Auth0 React](../../examples/adapters/auth0-react/README.md): readonly fields
  and Promise rejection mapping.
- [Amplify Auth](../../examples/adapters/amplify-auth/README.md): direct
  asynchronous calls with a checked Result boundary.

From a TypeRB source checkout, this pinned example demonstrates the complete
catalog workflow:

```sh
./trb adapter check --format json examples/adapters/tanstack-query
./trb install --frozen --config examples/adapters/tanstack-query/conformance/trbconfig.jsonc
./trb adapter test --format json examples/adapters/tanstack-query
```

`adapter check` validates the manifest and catalog using the selected native
consumer. It does not prove compatibility with installed native signatures.
`adapter test` runs adapter validation, a TypeRB build, and the package's
declared native conformance command. Dependencies must already be installed;
the command never installs them implicitly. The configured native command is
an explicitly executed program, not sandboxed compiler data.

Run generated code when runtime behavior matters. A native type check alone
cannot establish Promise rejection mapping, DOM effects, resource cleanup, or
native loading. Include failure and boundary inputs in those runtime checks,
and preserve diagnostics for intentionally unsupported operations. Run an
existing, structurally different fixture when changing shared adapter code.

## 5. Classify gaps and leave a reproducible result

Record whether a gap is documentation, author tooling, a reusable compiler or
protocol capability, or a library-specific integration requirement. Reduce a
compiler problem to an independent example with expected behavior. Do not add
library-name branches to the compiler, weaken the projection to `Any`, or edit
generated target output to get a green test.

An adapter README should state:

- the native and toolchain versions, installation command, and lockfiles;
- supported calls or interactions and their ownership/failure boundaries;
- the exact validation commands and what each command proves;
- intentionally unsupported shapes and their diagnostics; and
- when to revalidate, including compiler, protocol, or upstream changes.

An upstream library author or community maintainer can own the resulting
package and conformance tests independently of the compiler. Keep the workflow
human-readable; AI assistance should follow this same guide and consume the
same CLI diagnostics.
