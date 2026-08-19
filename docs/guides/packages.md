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

The initial provider file uses a versioned semantic type format:

```json
{
  "formatVersion": 1,
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

A provider may expose a native Promise callback as a checked fallible TypeRB
function. `fails` names the callback's error type and
`effectBridge: "promise_rejection"` declares that an `Err` becomes a rejected
Promise while an `Ok` becomes its resolved value:

```json
{
  "name": "queryFn",
  "type": {
    "kind": "function",
    "name": "Function",
    "args": [{ "kind": "named", "name": "TData" }],
    "fails": { "kind": "named", "name": "TError" },
    "effectBridge": "promise_rejection"
  }
}
```

Application code still uses ordinary `fn ... fails ...`, `attempt`, and
`Result` semantics. The bridge is allowed only at the declared native
boundary; it does not make Promise rejection part of portable TypeRB.

The additive Result-only bridge spells the same native contract without a
fallible function effect:

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
`Promise<Unit>`. `resultBridge` is mutually exclusive with `fails` and
`effectBridge`.

Provider-only records may describe props or parameter objects without becoming
application-importable names. When a selected contract refers to real native
package types, generated TypeScript adds the required type-only imports while
keeping those transitive names invisible to TypeRB source and completion.

External executable compiler extensions are intentionally unavailable.
Packages that need syntax, code generation, or dynamic type discovery must
wait for a versioned and sandboxed extension protocol rather than importing
compiler internals. The declarative native type provider above is the safe
non-executable subset.

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
