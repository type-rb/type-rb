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

The `acme` names and versions below are illustrative placeholders rather than
packages expected to resolve as written.

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

### Fixed declaration providers

A Ruby platform package may attach fixed type declarations for classes and
modules supplied by one of its native gems. This is useful for APIs such as a
gem-owned mixin: application code imports the TypeRB package, the declaration
catalog makes the mixin methods type-check, and the package root loads the gem
at runtime.

The package manifest selects one Declaration Protocol catalog for Ruby mode:

```json
{
  "formatVersion": 1,
  "name": "github.com/acme/pagy",
  "version": "0.1.0",
  "sourceDir": "src",
  "modes": ["ruby"],
  "nativeDependencies": {
    "ruby": { "pagy": "43.6.1" }
  },
  "declarationProviders": {
    "ruby": "declarations/ruby.json"
  }
}
```

`src/index.trb` is required and owns ordinary runtime loading:

```trb
activate trb/platform/ruby/native

require "pagy"
```

The application explicitly imports that package root. The import is a semantic
use even when it does not import a TypeRB symbol, because it activates the
fixed declarations and keeps the package root in generated Ruby:

```trb
import github.com/acme/pagy
activate trb/platform/ruby/native

class ProductsController
	include Pagy::Method
end
```

The JSON file uses Declaration Protocol version 3, including its generic,
literal-dependent, instance-member, and class-member type shapes. Its
`provider` must exactly equal the canonical package name. For example, a
catalog may declare `Pagy::Offset#page` and the `Pagy::Method#pagy` mixin
method:

```json
{
  "protocolVersion": 3,
  "provider": "github.com/acme/pagy",
  "types": [
    {
      "name": "Pagy::Offset",
      "instanceMembers": [
        {
          "name": "page",
          "kind": "property",
          "return": { "kind": "int", "name": "Integer" }
        }
      ]
    }
  ],
  "modules": [
    {
      "name": "Pagy::Method",
      "instanceMembers": [
        {
          "name": "pagy",
          "kind": "method",
          "typeParameters": ["T"],
          "parameters": [
            {
              "name": "paginator",
              "type": { "kind": "string", "name": "String" },
              "literalValues": ["offset"]
            },
            {
              "name": "collection",
              "type": {
                "kind": "array",
                "name": "Array",
                "arguments": [{ "kind": "named", "name": "T" }]
              }
            }
          ],
          "return": {
            "kind": "named",
            "name": "Tuple",
            "arguments": [
              { "kind": "named", "name": "Pagy::Offset" },
              {
                "kind": "array",
                "name": "Array",
                "arguments": [{ "kind": "named", "name": "T" }]
              }
            ]
          }
        }
      ]
    }
  ]
}
```

This capability reads fixed data; importing a package never executes a
provider program. The external subset cannot select compiler runtime
operations or call specializers, claim project source modules, inject project
rules or runtime types, declare compiler-controlled block contracts, or weaken
nominal representation boundaries. The catalog must be a regular package file,
not a symlink, and contain at least one declaration. Unknown JSON fields,
trailing data, unsafe `Any` or invalid signature types, compiler-derived
representation metadata, and conflicts with active built-in or project
declarations are errors. These restrictions keep ordinary signatures useful
without turning a dependency into compiler code.

Fixed declaration providers initially support Ruby only. They describe
gem-owned global classes, modules, and mixins whose runtime loading remains in
ordinary TypeRB package source. A declaration adapter instead projects
target-native module exports and may require the separate runtime mapping
below. Project-aware discovery remains limited to bundled compiler-integrated
providers.

Fixed declaration catalogs use Declaration Protocol version 3. Its
project-aware class-body declaration rules are reserved for bundled providers
and are rejected in fixed catalogs.

### Declaration adapters

An optional declaration adapter supplies semantic TypeRB declarations for a
target-native dependency. The semantic catalog format is mode-independent; a
mode key selects the native-ecosystem adapter that consumes it and validates
any adapter-specific bridge kinds. TypeScript can overlay declarations inferred
from installed `.d.ts` files while application source continues to import the
npm package directly. Within the declaration-adapter path, Go and Ruby require
the separate native runtime mapping described below. Ruby fixed declarations
for gem-owned global types and mixins use the narrower capability above.

An adapter file is declarative data and cannot execute compiler code. The
common host strictly decodes the catalog, validates its protocol shape, and
verifies its checksum. The selected ecosystem adapter checks that every
declared module belongs to the package's native dependencies for that mode,
rejects name conflicts across exports and supporting records, and validates
adapter-specific bridge kinds. A declaration catalog never grants permission to
call native code by itself.

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

### Native runtime adapters

A package may pair a declaration adapter with a Native Runtime Adapter Protocol
file for the same mode. The declaration file remains the portable semantic
contract; the runtime file maps its canonical export identities to
package-owned target shims. This is the initial supported path for calling
native Go and Ruby dependencies from an independent TypeRB package, and it is
also available in TypeScript mode when a fixed `.d.ts` projection is not an
appropriate boundary.

```json
{
  "nativeDependencies": {
    "go": { "github.com/acme/aws-s3-wire": "v0.1.0" },
    "ruby": { "acme-aws-s3-wire": "0.1.0" },
    "typescript": { "@acme/aws-s3-wire": "0.1.0" }
  },
  "declarationAdapters": {
    "go": "declarations/go.json",
    "ruby": "declarations/ruby.json",
    "typescript": "declarations/typescript.json"
  },
  "runtimeAdapters": {
    "go": "runtime/go.json",
    "ruby": "runtime/ruby.json",
    "typescript": "runtime/typescript.json"
  }
}
```

Each `runtimeAdapters.<mode>` entry requires
`declarationAdapters.<mode>`. Runtime Protocol version 1 maps a stable
`canonical-module#export` identity to one top-level target function:

```json
{
  "protocolVersion": 1,
  "bindings": {
    "github.com/acme/aws-s3/native#head_object": {
      "dependency": "github.com/acme/aws-s3-wire",
      "module": "github.com/acme/aws-s3-wire/s3",
      "symbol": "HeadObject",
      "callConvention": "function",
      "maySuspend": true,
      "propagatesExecutionScope": true
    }
  }
}
```

The initial wire contract is deliberately narrow. Every export in a
runtime-backed declaration module must be a non-generic top-level function with
exactly one required `String` parameter and a `String` return. A module cannot
mix direct native declarations and runtime-backed exports, and a runtime export
cannot also declare a `resultBridge`. Package TypeRB source should expose the
domain API: it serializes a request to JSON, calls the private wire function,
validates and decodes the response envelope, and converts its success or error
variant to the package's ordinary records, enums, and `Result` values. The
compiler treats the wire payload as an opaque string and does not invent an SDK
error mapping.

The target shim owns SDK construction, target exceptions or errors, and the
stable JSON envelope. `dependency` must name a native dependency declared for
the selected mode. A Go or TypeScript `module` must be that dependency or one
of its submodules; a Ruby `module` is a safe relative `require` path. Go symbols
are exported function identifiers, Ruby symbols are constant-qualified module
methods, and TypeScript symbols are named function exports. Package import,
resolution, adapter checking, and compilation validate this data but never
execute the shim.

`maySuspend` makes the mapped TypeScript call and all portable callers that
reach it suspend; TypeRB source still adds no `async` or `await` syntax.
`propagatesExecutionScope` passes one compiler-owned argument before the
declared string parameter: `context.Context` in Go, the TypeRB execution-scope
object in Ruby, and `AbortSignal | undefined` in TypeScript. Set either flag
only when the shim implements that contract. Direct FFI, native object handles,
structural wire types, lifecycle hooks, and arbitrary error-mapper callbacks
remain outside Protocol version 1.

Validate an adapter package from its repository root before publishing it:

```sh
trb adapter check
trb adapter check --format json
```

The command validates the package manifest and every configured adapter through
the same selected ecosystem consumer used by `trb install` or compilation. It
therefore checks the common catalog shape, native-dependency ownership, name
conflicts within the catalog, ecosystem-specific bridge kinds, runtime export
coverage, and the narrow runtime signature and target rules. An explicit
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
The `examples/adapters/amplify-auth` fixture applies the same String bridge to
Amplify Auth 6.20.0 and separately observes the native error name, message, and
recovery suggestion that a future package-owned rich rejection mapper would
need to preserve. Its application-owned TypeScript bootstrap imports
`amplify_outputs.json` and calls `Amplify.configure`; project-specific generated
configuration does not require adapter code or a compiler provider.

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
  "protocolVersion": 2,
  "compilerVersion": "0.3.23",
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
  "protocolVersion": 2,
  "compilerVersion": "0.3.23",
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
      "supportingRecords": 1,
      "runtimePath": "/workspace/ui-types/runtime/typescript.json",
      "runtimeProtocolVersion": 1,
      "runtimeBindings": 1
    }
  ],
  "diagnostics": [],
  "summary": {
    "adapters": 1,
    "validAdapters": 1,
    "modules": 1,
    "exports": 1,
    "supportingRecords": 1,
    "runtimeBindings": 1,
    "errors": 0,
    "warnings": 0
  }
}
```

Adapter-only supporting records may describe props or parameter objects without becoming
application-importable names. When a selected contract refers to real native
package types, generated TypeScript adds the required type-only imports while
keeping those transitive names invisible to TypeRB source and completion.

Arbitrary compiler-triggered executable extensions are intentionally
unavailable. Package imports that need to inspect the TypeRB project during
compilation or change syntax, checking, or code generation must wait for a
versioned and sandboxed extension protocol rather than importing compiler
internals. Declaration adapters and the fixed native runtime mapping above are
strictly decoded, non-executable subsets; neither permits package-supplied
compiler logic.

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
output from bundled, compiler-integrated packages. `trb/orm`, `trb/jobs`,
`trb/web`, and `trb/cli` are the first consumers. The ORM provider
discovers project models and schema metadata, the Jobs provider derives typed
enqueue methods from Job classes, the Web provider identifies exact endpoint
contract classes, and the CLI provider derives a closed argument schema.
Their resulting catalogs cross a versioned, JSON-serializable
Declaration Protocol before resolution and checking. The current Declaration
Protocol is version 3. It can identify a direct package-function call in one
exact project class as declarative metadata: the checker still validates the
ordinary call signature, while every backend omits the call from runtime
output. Parameters may also explicitly identify a representation boundary for
nominal source newtypes. The compiler host validates and copies that data into
its private semantic representation.

These bundled providers also receive versioned, validated Project Declaration
Input snapshots. Version 7 separates each declaration's semantic `identity`
from its source/display `name`. An identity contains the canonical module path
and source-qualified declaration name. Nested records and enums additionally
carry a structured `owner` identity, so `CLI::Options` can remain distinct
while its display name stays `Options`. Generated Go, Ruby, and TypeScript
identifiers are not part of this boundary. The exposed declaration categories
are unchanged: nested records and enums are visible, while other declaration
categories and imports remain top-level-only. Version 6 added record-default
presence and enum-member attributes. Version 5 added record declarations and
their declarative field attributes, top-level function signatures, and
resolved type arguments on direct class-body calls. Version 4 added named-only
parameter identity. The snapshot also contains canonical module and import
identity, transparent type aliases, nominal newtypes with concrete
representations, enum and class declarations, method signatures, authored,
resolved, and boundary representation types, declarative class-body call
values, structural block summaries, and source spans. A record declaration in
this snapshot is an authored project declaration; it does not expose the
call-site-only record-inspection metadata used by call specialization. The
snapshot does not contain parser nodes, function, method, or block bodies,
default expressions, resolver or checker state, filesystem handles, or backend
objects. These consumers characterize a small reusable read-only project
view; it is not yet a general external provider API.

The Web provider uses the generic declaration catalog only to mark `handles`,
`input`, and `response` calls in exact `Endpoint` classes as declaration-only.
Its project integration then binds those declarations to file routes and owns
a separate endpoint catalog protocol. HTTP methods, paths, status codes, and
OpenAPI document fields do not enter the generic Project Declaration Input,
Declaration Protocol, or compiler IR schema.

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
- exact direct class-body calls that are checked as ordinary package calls but
  retained only as declaration metadata;
- structured block contracts, including portable control and `Result`
  boundaries; and
- located project-declaration references and runtime value types needed by the
  generated declarations.

A declaration may name an opaque `runtimeOperation`, but only bundled compiler
backends can implement those operation names today. ORM runtime manifests and
Job worker lifecycle use separate bundled runtime paths rather than becoming
declaration fields. Jobs exposes a small package-level `JobAdapter`
interface in ordinary TypeRB source, but its SQL implementation still reaches
bundled native primitives. These protocols therefore narrow the declaration
boundary; they do not yet make ORM or Jobs an ordinary external package or
define a rich external native-runtime ABI. Independent packages can use the
fixed string-wire function ABI described earlier, but it does not expose these
bundled operation names or lifecycle paths.

The declaration type format deliberately rejects call-site-only record
inspection and source-definition metadata. Those facts remain scoped to call
specialization until namespace-stable type identities are designed. The
read-only project input may carry source-definition identity because providers
need to distinguish an imported canonical type from a same-shaped local type.
Applying the same capability to Jobs and ORM established the reusable
declaration facts without introducing an arbitrary provider-data bag. The
remaining ORM integration and the normalized native operations below the Jobs
adapter can continue to test whether the initial string-wire runtime protocol
needs another general capability. Capability negotiation and sandboxing remain
deferred until an external project-aware provider boundary is justified.

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
remain package-specific. Those findings also produced the independent Native
Runtime Adapter Protocol's two explicit effect flags. The external protocol
remains limited to the fixed `String -> String` function wire and does not
expose compiler-owned operation names or package-specific ORM and Jobs
lowering.

The project-source protocol in this section is still bundled and experimental,
not a package-manifest capability or external plugin API. Like call
specialization below, its required imports use ordinary named imports.
Namespace-stable public type identities, capability negotiation, isolation, and
compatibility policy remain necessary before independent packages can generate
project-aware source safely. Independent packages may use only the declaration
and fixed runtime adapter files described earlier.

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

# Re-resolve one direct alias and the dependencies selected by its new manifest.
trb update acme/contracts
```

Without package arguments, `trb update` re-resolves the complete graph. With
one or more direct application aliases, it keeps every unselected direct graph
pinned and re-resolves each selected package and its transitive dependencies.
Selective update requires a current lock. If a selected graph and an
unselected graph require incompatible revisions of the same canonical package,
the command reports a conflict; the initial resolver does not choose a winner.

## Local development

Use the same package manifest through an editable path:

```sh
trb add --path ../contracts local/contracts
```

Local paths are recorded relative to the project and source content is not
locked, so code edits are visible immediately. The normalized manifest is
checksummed; run `trb install` after changing its identity, dependencies,
supported modes, native dependencies, fixed declaration provider, declaration
adapter, or runtime adapter. Edits to a local fixed declaration catalog are
strictly reloaded by the next build and participate in incremental compiler
invalidation. Declaration adapter file changes require `trb install`; a build
diagnoses a stale cached declaration adapter. Runtime adapter data is strictly
reloaded and validated by each build. Run `trb adapter check` before publishing
either adapter file.
A local package's manifest name
remains its canonical identity; `local/contracts` is only the importing
project's alias.

`localPackages` maps source-only directories that do not contain a package
manifest. Reusable packages should use `trbpackage.json` and a path
requirement.

## Production-readiness boundaries

The distributed resolver is usable without a central registry, but the
following capabilities remain intentionally staged:

- Requirements accept exact semantic-version tags, `latest`, or Git revisions.
  Semantic-version ranges and a policy for selecting one version from multiple
  requirements are not defined. Different requirements for one canonical
  package therefore remain a conflict.
- Publishing currently means committing a root `trbpackage.json` and tagging a
  matching semantic version in Git. There is no `trb publish`, registry,
  compatibility checker, signing policy, provenance verification, or
  vulnerability service.
- Remote content is cached below each project's `.trb/packages`. A shared
  user-level cache, concurrent-install policy, garbage collection, and cache
  permission model are not defined.
- Multiple local path packages may live in one repository, but TypeRB does not
  provide first-class workspace orchestration or select a package from a
  subdirectory of one remote Git source.
- Public user-defined type names must still be unique across the complete
  project graph. Namespace-stable type identity is required before independent
  packages may safely export the same authored type name.

Remote package checksum traversal rejects symbolic links, and every cache read
revalidates its checksum. Local path packages are editable developer inputs and
are not content locked as untrusted artifacts. Git fetches use shallow history,
but explicit transfer-size, file-count, and execution-time limits are not yet
part of the resolver. Checksums establish content integrity and reproducibility;
they do not establish author trust or absence of vulnerabilities.

`trb install` delegates native dependency installation to Go modules, Bundler,
or npm/Bun. Native ecosystem code, build steps, and install scripts remain
outside the TypeRB compiler-extension boundary. Package import itself does not
execute package-supplied compiler code; external project-aware executable
providers remain unavailable until a sandbox and resource limits are defined.

## Native dependencies

`dependencies` and `devDependencies` in `trbconfig.jsonc` describe target
language packages rather than TypeRB source packages. Manage them explicitly:

```sh
trb add --native PACKAGE VERSION
trb add --native --dev PACKAGE VERSION
trb remove --native PACKAGE
trb sync
```

Package-owned TypeScript dependencies below `@types/` are generated as
development dependencies unless the project already classifies them
explicitly. Other package-owned native dependencies are generated as runtime
dependencies.

Projects with `"packageManagement": "external"` may still resolve TypeRB
packages, but the host project remains responsible for installing all merged
native dependencies. In TypeScript mode, `trb install` still indexes declared
native package types after the host package manager has installed them.
