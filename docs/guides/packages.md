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
  "declarationAdapters": {
    "typescript": "declarations/typescript.json"
  }
}
```

`formatVersion`, `name`, and a semantic `version` are required. `sourceDir`
defaults to `src`, and omitted `modes` support all three backends. Dependency
aliases are local to the declaring package. Native dependencies are selected
only for the application's active mode and merge into its generated target
manifest.

An optional declaration adapter supplies semantic TypeRB declarations for a
target-native dependency. The semantic catalog format is mode-independent; a
mode key selects the native-ecosystem adapter that consumes it and validates
any adapter-specific bridge kinds. TypeScript is the first
implemented adapter. It overlays declarations inferred from installed `.d.ts`
files while application source continues to import the npm package directly.

An adapter file is declarative data and cannot execute compiler code. The
common host strictly decodes the catalog, validates its protocol shape, and
verifies its checksum. The selected ecosystem adapter checks that every
declared module belongs to the package's native dependencies for that mode,
rejects name conflicts across exports and supporting records, and validates
adapter-specific bridge kinds. Ruby and Go declaration importers are not
implemented yet; configuring either mode produces an explicit
unsupported-adapter error.

The adapter file uses Declaration/Adapter Protocol version 2:

```json
{
  "protocolVersion": 2,
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

This protocol is an alpha Tier 1 extension for package authors. It supports
`function`, `component`, `class`, `interface`, `record`, and transparent
`type_alias` declarations. `typeParameters` names explicit generic parameters
on functions, classes, interfaces, records, and aliases; `aliasTarget`
describes an alias's semantic target. Semantic types may refer to those
parameters and may use literal and union types for discriminated result
contracts. Nested semantic type arguments use the `arguments` field. Every
semantic type includes its canonical TypeRB `name`; collection and union kinds
use their required argument counts, while a function's final argument is its
return type. TypeRB calls still provide explicit type arguments. Adapter
declarations cannot use `Any`; unrepresentable boundaries remain explicit
diagnostics.

Call parameters are valid on functions, components, and classes. Fields are
valid on records, classes, and interfaces. Interface fields are readonly and
describe properties of a native object without making it constructible. A
function or component uses `members` for compound function or component
exports such as `Table.Row`. A class instead
uses `instanceMembers` and `classMembers`, so a method such as
`router.navigate()` cannot accidentally become a class call such as
`Router.navigate()`. An interface uses fields and `instanceMembers` and cannot
be constructed, which is appropriate for opaque native objects returned by a
function. Members cannot themselves contain nested members. A component
accepts at most one non-variadic props parameter and does not declare explicit
type parameters. Class, interface, record, and alias declarations use their
exported name as their named self type.

For example, a native class method stays distinct from an instance method:

```json
{
  "kind": "class",
  "type": { "kind": "named", "name": "Router" },
  "instanceMembers": {
    "navigate": {
      "kind": "function",
      "type": { "kind": "void", "name": "Void" },
      "parameters": [
        { "kind": "string", "name": "String" }
      ],
      "required": 1
    }
  }
}
```

An adapter may expose a native Promise callback as a checked `Result`-returning
TypeRB function. `resultBridge` declares that an `Err` becomes a rejected
Promise while an `Ok` becomes its resolved value:

```json
{
  "name": "queryFn",
  "type": {
    "kind": "function",
    "name": "Function",
    "arguments": [{ "kind": "named", "name": "TData" }],
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

The opposite direction is a call-level bridge on a native function or
instance member. The native Promise success type must match the `Result`
success type. The initial bridge intentionally accepts only `String` errors:
an `Error` rejection contributes its message, while any other rejection uses
`String(...)`. If that conversion itself throws, the stable fallback is
`"Unknown native rejection"`.

```json
{
  "kind": "function",
  "type": {
    "kind": "named",
    "name": "Result",
    "arguments": [
      { "kind": "string", "name": "String" },
      { "kind": "string", "name": "String" }
    ]
  },
  "resultBridge": {
    "kind": "promise_rejection_to_result",
    "error": { "kind": "string", "name": "String" }
  }
}
```

The call is a backend suspension root even though TypeRB adds no `async` or
`await` syntax. A resolved native `Promise<T>` becomes `Result<T, String>::Ok`;
`Promise<void>` is represented by `Result<Unit, String>::Ok(Unit.new())`.
Generated TypeScript first checks the native expression against that exact
Promise type, so conformance testing rejects a bridge attached to a synchronous
return. Synchronous throws from the call use the same checked error path. Rich
record or enum errors require a future package-owned rejection mapper and are
rejected by this first bridge rather than filled with partial values. The
bridge does not invent cancellation: a package must declare an explicit native
cancellation parameter when its API provides one.

Declaration/Adapter Protocol version 2 distinguishes class instance members
from class members. To migrate a version 1 catalog, set `protocolVersion` to
`2` and replace each class's `members` with `instanceMembers` or
`classMembers` according to how callers access it.

For the older TypeScript-only `nativeTypeProviders` format version 2, rename
the manifest field to `declarationAdapters`, replace the file's
`formatVersion` with `protocolVersion: 2`, rename nested semantic type `args`
fields to `arguments`, and split class members by access kind. Existing
`resultBridge` contracts remain valid. Then run `trb install` to regenerate
`.trb/native-types.json`; its cache format is independent from the
package-owned protocol.

Validate an adapter package from its repository root before publishing it:

```sh
trb adapter check
trb adapter check --format json
```

The command validates the package manifest and every configured adapter through
the same selected ecosystem consumer used by `trb install`. It therefore
checks the common catalog shape as well as native-dependency ownership, name
conflicts within the catalog, and ecosystem-specific bridge kinds. An explicit
package root may be passed as the only positional argument.

`adapter check` does not install the native package or prove that a projected
contract remains assignable to that package's declarations. Keep the native
dependency at an explicit version and run a native type-checking conformance
project when changing it. The repository's
`examples/adapters/tanstack-query` fixture demonstrates this split with a
checked catalog plus a strict TypeScript project pinned to TanStack Query.
The `examples/adapters/tanstack-router` fixture also verifies a
non-constructible interface and its instance member against the native package.
It intentionally omits route-tree-derived generics, exact parameters, and
destinations: those values require a future project provider rather than a
fixed declaration catalog.
The `examples/adapters/auth0-react` fixture verifies readonly properties on a
non-constructible interface, a React provider projection, and native Promise
calls exposed as checked Results. Its token, login, and logout methods use the
`promise_rejection_to_result` call bridge described above.

An adapter package may connect one conformance project to each adapter mode:

```json
{
  "adapterTests": {
    "typescript": {
      "config": "conformance/trbconfig.jsonc",
      "command": ["bun", "run", "check"]
    }
  }
}
```

The config and any relative command executable stay below the package root.
TypeRB passes the command as an argument vector instead of interpreting a shell
string; the invoked tool may retain its own script behavior. Its project must
install the package under test from the current package root so the checked
catalog and source cannot silently come from another copy.

Install the conformance project's dependencies explicitly, then test it from
the adapter root:

```sh
trb install --frozen --config conformance/trbconfig.jsonc
trb adapter test
trb adapter test --format json
```

`adapter test` runs three ordered phases: `adapter_check`, `build`, and
`native_check`. It does not install dependencies or contact the network on
TypeRB's behalf. The native command runs only when the author invokes
`adapter test`; ordinary package resolution, installation, build, and import
never execute it. Failed native output is written to standard error so JSON
reports remain parseable.

The JSON report uses the adapter-tooling protocol version and records stable
phase states without embedding native-tool logs:

```json
{
  "protocolVersion": 1,
  "compilerVersion": "0.3.16",
  "package": {
    "name": "github.com/acme/ui-types",
    "version": "0.1.0",
    "manifestPath": "/workspace/ui-types/trbpackage.json"
  },
  "tests": [
    {
      "mode": "typescript",
      "configPath": "/workspace/ui-types/conformance/trbconfig.jsonc",
      "command": ["bun", "run", "check"],
      "passed": true,
      "phases": [
        {"name": "adapter_check", "status": "passed"},
        {"name": "build", "status": "passed"},
        {"name": "native_check", "status": "passed"}
      ]
    }
  ],
  "diagnostics": [],
  "summary": {
    "tests": 1,
    "passedTests": 1,
    "failedTests": 0,
    "errors": 0,
    "warnings": 0
  }
}
```

`adapter check --format json` output is deterministic and versioned
independently from the declaration catalog. It contains package identity, one
result per adapter in mode order, semantic declaration counts, diagnostics,
and a summary. Invalid input still
produces the report on standard output and returns a nonzero status, allowing
CI and AI agents to use one contract for success and failure:

```json
{
  "protocolVersion": 1,
  "compilerVersion": "0.3.16",
  "package": {
    "name": "github.com/acme/ui-types",
    "version": "0.1.0",
    "manifestPath": "/workspace/ui-types/trbpackage.json"
  },
  "adapters": [
    {
      "mode": "typescript",
      "path": "/workspace/ui-types/declarations/typescript.json",
      "valid": true,
      "declarationProtocolVersion": 2,
      "modules": 1,
      "exports": 1,
      "supportingRecords": 1
    }
  ],
  "diagnostics": [],
  "summary": {
    "adapters": 1,
    "validAdapters": 1,
    "modules": 1,
    "exports": 1,
    "supportingRecords": 1,
    "errors": 0,
    "warnings": 0
  }
}
```

Adapter-only supporting records may describe props or parameter objects without becoming
application-importable names. When a selected contract refers to real native
package types, generated TypeScript adds the required type-only imports while
keeping those transitive names invisible to TypeRB source and completion.

Compiler-triggered executable extensions are intentionally unavailable.
Package imports that need to inspect the TypeRB project during compilation or
change syntax, checking, or code generation must wait for a versioned and
sandboxed extension protocol rather than importing compiler internals. The
declaration adapter above is the safe non-executable subset.

An explicitly invoked external generator is a different boundary. A tool may
read an OpenAPI document or another external schema and emit ordinary `.trb`
source or a declarative adapter catalog. TypeRB then parses, checks, formats,
maps, and compiles those artifacts through the normal pipeline. Such a tool is
not executed by importing a package, receives no compiler privileges, and
remains under the application's dependency and command policy. Prefer this
model when generation does not require TypeRB project declarations; reserve a
future compiler provider for cases that genuinely need project-aware discovery.

### Experimental bundled declaration providers

TypeRB 0.x has experimental data boundaries for declaration discovery and
output from bundled, compiler-integrated packages. `trb/orm` and `trb/jobs` are
the first consumers. The ORM provider discovers project models and schema
metadata, while the Jobs provider derives typed enqueue methods from Job
classes. Their resulting catalogs cross a versioned, JSON-serializable
Declaration Protocol before resolution and checking. The current Declaration
Protocol is version 2; parameters may explicitly identify a representation
boundary for nominal source newtypes. The compiler host
validates and copies that data into its private semantic representation.

The Jobs and ORM declaration providers also receive versioned, validated
Project Declaration Input snapshots. Version 3 contains canonical module and
import identity, transparent type aliases, nominal newtypes with concrete
representations, enum and class declarations, method signatures, authored,
resolved, and boundary representation types, declarative class-body call
values, structural block summaries, and source spans. It does not contain
parser nodes, method or block bodies, resolver or checker state, filesystem
handles, or backend objects. These two consumers characterize a small reusable read-only
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
- parameters that explicitly accept a nominal source type through its concrete
  serialization or persistence representation;
- structured block contracts, including portable control and `Result`
  boundaries; and
- located project-declaration references and runtime value types needed by the
  generated declarations.

A declaration may name an opaque `runtimeOperation`, but only bundled compiler
backends can implement those operation names today. ORM runtime manifests and
Job worker lifecycle use separate bundled runtime paths rather than becoming
declaration fields. Jobs now exposes a small package-level `JobAdapter`
interface in ordinary TypeRB source, but its SQL implementation still reaches
bundled native primitives. These protocols therefore narrow the declaration
boundary; they do not yet make ORM or Jobs an ordinary external package or
define a generic external native-runtime ABI.

The declaration type format deliberately rejects call-site-only record
inspection and source-definition metadata. Those facts remain scoped to call
specialization until namespace-stable type identities are designed. The
read-only project input may carry source-definition identity because providers
need to distinguish an imported canonical type from a same-shaped local type.
Applying the same capability to Jobs and ORM established the reusable
declaration facts without introducing an arbitrary provider-data bag. The
remaining ORM integration and the normalized native operations below the Jobs
adapter can now guide a separate, minimal runtime-operation descriptor.
Capability negotiation and sandboxing remain deferred until an external
provider boundary is justified.

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

Jobs is the first consumer. The Jobs provider generates one portable worker
dispatcher plus one stable fragment for each Job's typed enqueue wrappers. The
enqueue fragments encode scalar payloads, normalize relative scheduling, and
call the ordinary TypeRB `JobAdapter` contract. The dispatcher validates
payload versions, decodes arguments, invokes the typed Job, and returns
`JobResult`. Go, Ruby, and Bun workers call that compiled function from thin
target-specific panic or exception wrappers. SQL persistence, claims,
heartbeats, retry transitions, signals, and worker process lifecycle remain
adapter/backend responsibilities. This split exercises reusable generated
source and per-fragment authored origins without turning Jobs metadata into
generic protocol fields or prematurely defining a generic native-runtime ABI.

The compiler currently keeps a small internal runtime-operation descriptor for
the two call-graph effects that proved common across Jobs and ORM: an operation
may suspend, and it may propagate the hidden execution scope used for
cancellation. Typed parameters and return values remain in package
declarations, while target lowering and native-error-to-`Result` conversion
remain package-specific. The descriptor is compiler-owned metadata, not an
external runtime ABI or permission for a package to add native operations.

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
selected provider, call-site identity, resolved type arguments, nominal
newtype representation metadata, and the record shape needed by that provider.
The current request protocol is version 2. It returns ordinary TypeRB helper
source, named imports required by that source, and a narrow replacement of the
original call.
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
supported modes, native dependencies, or declaration adapter. Adapter file
changes also require `trb install`; a build diagnoses a stale cached adapter.
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
