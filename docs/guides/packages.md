# TypeRB packages

TypeRB uses distributed Git repositories rather than a required central
registry. A package contains TypeRB source and a strict `trbpackage.json`
manifest. Application and package source share the same grammar, type checker,
typed IR, and three backends.

## Package categories

TypeRB uses three package boundaries and one distribution term:

| Term | Purpose | Versioning |
| --- | --- | --- |
| Standard library | Foundational portable APIs below `trb/std/*` | Released with the compiler |
| Compiler-integrated package | An official package that currently requires compiler providers, typed-IR operations, or backend integration, such as `trb/web`, `trb/orm`, or `trb/jobs` | Released with the compiler while integration is required |
| TypeRB package | An ordinary package resolved from Git or a local path | Independently versioned and locked |
| Bundled package | An ordinary TypeRB package included in the TypeRB distribution for availability or offline use | Independently versioned and locked |

Compiler integration is a capability boundary, not a mark of importance. An
official package does not receive compiler privileges merely because TypeRB
maintains or bundles it. Compiler-integrated packages may move to ordinary
packages as the versioned extension protocol gains the capabilities they need.

A bundled package still has to be declared as a project dependency. Bundling
does not make its APIs implicit, couple its version to the compiler, or exempt
it from `trb.lock`.

The following labels describe independent properties rather than additional
package boundaries:

- **official** or **community** identifies who maintains the package;
- **portable** or **platform** identifies whether it supports shared TypeRB
  semantics or an explicit target ecosystem; and
- **bundled** or **downloaded** describes distribution of an ordinary TypeRB
  package, not its privileges.

For example, a future `trb/csv` could be an official, portable, bundled TypeRB
package without becoming part of the standard library or gaining compiler
privileges. A React router integration could instead be a downloaded platform
package maintained by TypeRB or the community.

## Install a package

The short name `acme/contracts` defaults to
`github.com/acme/contracts`:

```sh
trb add acme/contracts v1.2.3
trb install
```

Source keeps the short explicit import:

```trb
import { Contract } from acme/contracts
```

Use a full repository name when no abbreviation is wanted, or select another
Git host explicitly:

```sh
trb add github.com/acme/contracts v1.2.3
trb add --source gitlab.com/company/auth company/auth v2.0.0
```

Git credentials and private-repository access remain the responsibility of the
installed Git client. Source URLs containing embedded passwords, query strings,
or fragments are rejected so credentials cannot leak into project config or
the lock. The initial resolver accepts exact semantic-version tags
such as `v1.2.3`, `latest`, and Git revisions. `latest` resolves only during
install or update and is then pinned.

## Package manifest

A repository publishes `trbpackage.json` at its root:

```json
{
  "formatVersion": 1,
  "name": "github.com/acme/contracts",
  "version": "1.2.3",
  "sourceDir": "src",
  "modes": ["go", "ruby", "typescript"],
  "packages": {
    "acme/validation": "v1.0.0"
  },
  "nativeDependencies": {
    "go": {
      "github.com/google/uuid": "v1.6.0"
    },
    "ruby": {
      "uuid7": "0.1.0"
    },
    "typescript": {
      "@acme/ui": "1.0.0",
      "uuid": "11.1.0"
    }
  },
  "nativeTypeProviders": {
    "typescript": "native-types/typescript.json"
  }
}
```

`formatVersion`, `name`, and a semantic `version` are required. `sourceDir`
defaults to `src`, and omitted `modes` support all three backends. Dependency
aliases are local to the declaring package. Native dependencies are selected
only for the application's active mode and merge into its generated target
manifest.

An optional TypeScript native type provider corrects declarations inferred from
installed `.d.ts` files while application source continues to import the npm
package directly. The provider is declarative data and cannot execute compiler
code. Each declared module must belong to the package's TypeScript
`nativeDependencies`; two providers cannot replace the same export or record.

The provider file uses a versioned semantic type format:

```json
{
  "formatVersion": 2,
  "modules": {
    "@acme/ui": {
      "exports": {
        "Button": {
          "kind": "component",
          "type": { "kind": "named", "name": "ReactNode" },
          "parameters": [
            { "kind": "named", "name": "ButtonProps" }
          ],
          "required": 1
        }
      },
      "records": {
        "ButtonProps": {
          "kind": "record",
          "type": { "kind": "named", "name": "ButtonProps" },
          "fields": [
            {
              "name": "label",
              "type": { "kind": "string", "name": "String" }
            }
          ]
        }
      }
    }
  }
}
```

This format is an alpha Tier 1 extension for package authors. It supports
`function`, `component`, `class`, `record`, and transparent `type_alias`
declarations. `typeParameters` names explicit generic parameters on functions,
classes, records, and aliases; `aliasTarget` describes an alias's semantic
target. Semantic types may refer to those parameters and may use literal and
union types for discriminated result contracts. TypeRB calls still provide
explicit type arguments. Provider declarations cannot use `Any`;
unrepresentable boundaries remain explicit diagnostics.

A provider may expose a native Promise callback as a checked `Result`-returning
TypeRB function. `resultBridge` declares that an `Err` becomes a rejected
Promise while an `Ok` becomes its resolved value:

```json
{
  "name": "queryFn",
  "type": {
    "kind": "function",
    "name": "Function",
    "args": [{ "kind": "named", "name": "TData" }],
    "resultBridge": {
      "kind": "result_to_promise_rejection",
      "error": { "kind": "named", "name": "TError" }
    }
  }
}
```

The TypeRB callback must return the compiler-owned `Result<TData, TError>` or
a transparent alias. The generated native callback accepts either that Result
or a Promise of it, resolves `Ok(value)`, and rejects with the exact
`Err(error)` payload. A native `Void` success uses `Result<Unit, E>` and emits
`Promise<void>`; a generic success instantiated as `Unit` remains
`Promise<Unit>`. The bridge is allowed only at the declared native boundary;
it does not make Promise rejection part of portable TypeRB.

Provider format version 2 is Result-only. Version 1 providers used the removed
`fails` and `effectBridge` fields and are not accepted by TypeRB 0.3. Rewrite
those callback contracts with `resultBridge`, update the provider's
`formatVersion` to `2`, and run `trb install` to regenerate
`.trb/native-types.json`. A version 1 generated cache can be regenerated with
`trb install` after every referenced provider has been updated.

Provider-only records may describe props or parameter objects without becoming
application-importable names. When a selected contract refers to real native
package types, generated TypeScript adds the required type-only imports while
keeping those transitive names invisible to TypeRB source and completion.

External executable compiler extensions are intentionally unavailable.
Packages that need syntax, code generation, or dynamic type discovery must
wait for a versioned and sandboxed extension protocol rather than importing
compiler internals. The declarative native type provider above is the safe
non-executable subset.

### Experimental bundled declaration providers

TypeRB 0.x has experimental data boundaries for declaration discovery and
output from bundled, compiler-integrated packages. `trb/orm` and `trb/jobs` are
the first consumers. The ORM provider discovers project models and schema
metadata, while the Jobs provider derives typed enqueue methods from Job
classes. Their resulting catalogs cross a versioned, JSON-serializable
Declaration Protocol before resolution and checking. The compiler host
validates and copies that data into its private semantic representation.

The Jobs and ORM declaration providers also receive versioned, validated
Project Declaration Input snapshots. Version 2 contains canonical module and
import identity, transparent type aliases, enum and class declarations, method
signatures, authored and resolved types, declarative class-body call values,
structural block summaries, and source spans. It does not contain parser nodes,
method or block bodies, resolver or checker state, filesystem handles, or
backend objects. These two consumers characterize a small reusable read-only
project view; it is not yet a general external provider API.

ORM declaration discovery composes that generic project snapshot with a
separate versioned ORM Declaration Input. Its schema section contains only the
adapter plus table, column, foreign-key, and unique-constraint facts required
to derive the catalog. The compiler host still owns schema-lock loading or live
introspection, and database locations and environment names are excluded from
the provider input. Provider-specific data therefore remains typed and local
to ORM instead of becoming arbitrary fields in the generic project protocol.

The initial declaration catalog can describe:

- package-owned types, modules, methods, and properties;
- generic and literal-dependent call signatures such as typed ORM projections;
- structured block contracts, including portable control and `Result`
  boundaries; and
- located project-declaration references and runtime value types needed by the
  generated declarations.

A declaration may name an opaque `runtimeOperation`, but only bundled compiler
backends can implement those operation names today. ORM runtime manifests and
Job persistence, retry policy, and worker lifecycle use separate bundled
runtime paths rather than becoming declaration fields. These protocols
therefore narrow the declaration boundary; they do not yet make ORM or Jobs an
ordinary external package or define a public runtime adapter ABI.

The declaration type format deliberately rejects call-site-only record
inspection and source-definition metadata. Those facts remain scoped to call
specialization until namespace-stable type identities are designed. The
read-only project input may carry source-definition identity because providers
need to distinguish an imported canonical type from a same-shaped local type.
Applying the same capability to Jobs and ORM established the reusable
declaration facts without introducing an arbitrary provider-data bag. The
remaining ORM and Jobs runtime integrations can now guide a separate, minimal
runtime adapter ABI. Capability negotiation and sandboxing remain deferred
until an external provider boundary is justified.

### Experimental bundled project source generation

The Project Generated Source Protocol is a second versioned, data-only
boundary for bundled project providers. A version 1 response contains the
provider identity and deterministic TypeRB fragments. Each fragment has a
stable ID, the path of an existing project module, ordinary TypeRB source,
required named imports, and an authored origin span. A response may also
contain located issues. It cannot create a standalone virtual module or return
backend source, parser nodes, typed IR, opaque resources, filesystem handles,
or runtime import mappings.

The compiler host validates provider ownership and target modules, removes
stale fragments by stable identity, and appends the current fragments to their
owning source units. They then pass through the ordinary parser, resolver,
checker, typed IR, effect analysis, source mapping, and all three backends.
Diagnostics and generated source-map locations are mapped to the authored
origin. Fragment source and identity participate in compiler cache identity;
project edits currently re-enter full project analysis conservatively when a
project-generated fragment is active.

Jobs worker dispatch is the first consumer. The Jobs provider generates one
portable dispatcher that validates payload versions, decodes scalar payload
arguments, invokes the typed Job, and returns `JobResult`. Go, Ruby, and Bun
workers call that compiled function from thin target-specific panic or
exception wrappers. SQL persistence, claims, heartbeats, retry transitions,
signals, and worker process lifecycle remain adapter/backend responsibilities.
This split exercises a reusable generated-source capability without turning
Jobs metadata into generic protocol fields or prematurely defining a public
runtime adapter ABI.

This protocol is still bundled and experimental, not a package-manifest
capability or external plugin API. Like call specialization below, its required
imports use ordinary named imports. Namespace-stable public type identities,
capability negotiation, isolation, and compatibility policy remain necessary
before independent packages can use it safely.

### Experimental bundled call specialization

TypeRB 0.x also has an experimental call-specialization boundary for bundled,
compiler-integrated packages. It is not a package-manifest capability or a
supported external plugin API yet. The first consumer is
`trb/web`'s `Context#bind<T>()`.

A call specializer receives a versioned, serializable request containing the
selected provider, call-site identity, resolved type arguments, and the record
shape needed by that provider. It returns ordinary TypeRB helper source, named
imports required by that source, and a narrow replacement of the original call.
The compiler appends the helper to the call's owning module and runs it through
the normal parser, resolver, checker, typed IR, and backend pipeline. Package
code does not receive AST, checker, typed-IR, or backend objects, and it does not
emit Go, Ruby, or TypeScript source.

Generated diagnostics and source mappings point back to the original call, and
generated names use the compiler-reserved `__trb` namespace. The current
request shape deliberately supports only the facts needed by `Context#bind`;
it is expected to change as additional package cases identify reusable
capabilities. In particular, generated source currently relies on ordinary
named type imports, so namespace-stable type imports must be designed before
this boundary is opened to independent packages.

## Lock and cache

Commit `trb.lock`. It records direct import mappings, the complete dependency
graph, exact Git commits, and SHA-256 checksums. Remote content is stored by
checksum below `.trb/packages`, which is ignored by Git. Builds, checks, runs,
and the REPL read the lock and verify cached content without network access.

```sh
# Reject a missing or stale lock in CI.
trb install --frozen

# Never contact Git; fail if locked remote content is absent.
trb install --offline

# Re-resolve entries such as latest.
trb update
```

## Local development

Use the same package manifest through an editable path:

```sh
trb add --path ../contracts local/contracts
```

Local paths are recorded relative to the project and source content is not
locked, so code edits are visible immediately. The normalized manifest is
checksummed; run `trb install` after changing its identity, dependencies,
supported modes, native dependencies, or native type provider. Provider file
changes also require `trb install`; a build diagnoses a stale cached provider.
A local package's manifest name
remains its canonical identity; `local/contracts` is only the importing
project's alias.

`localPackages` remains available for older source-only workspaces, but new
reusable packages should use `trbpackage.json` and a path requirement.

The initial compiler still requires public user-defined type names to be
unique across one complete project graph. Namespace-stable type identities are
planned before the package system is considered production-compatible.

## Native dependencies

`dependencies` and `devDependencies` in `trbconfig.jsonc` describe target
language packages rather than TypeRB source packages. Manage them explicitly:

```sh
trb add --native PACKAGE VERSION
trb add --native --dev PACKAGE VERSION
trb remove --native PACKAGE
trb sync
```

Projects with `"packageManagement": "external"` may still resolve TypeRB
packages, but the host project remains responsible for installing all merged
native dependencies. In TypeScript mode, `trb install` still indexes declared
native package types after the host package manager has installed them.
