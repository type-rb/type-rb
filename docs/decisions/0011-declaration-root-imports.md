# 0011: Declaration-root imports

Status: accepted and implemented in TypeRB 0.4.0; operation ownership is
clarified by [ADR 0013](0013-canonical-operation-owners.md)

## Context

TypeRB needs an import model that keeps ordinary calls concise without giving
one operation several interchangeable spellings. A declaration-root form is
clear and completion-friendly:

```trb
import trb/std/math

Math.sqrt(9)
```

A named form is more compact when one module exposes several independent
declarations:

```trb
import { Date, Duration } from trb/std/time

def schedule(started_at: Date, timeout: Duration)
	return
end
```

Treating these as two ways to import every member would make both
`Math.sqrt(9)` and `sqrt(9)` normal spellings. Choosing between them by
package-specific metadata, symbol count, or a lint heuristic would make the
same source operation change style as an API or call site evolves.

Some existing imports have a different purpose. A Ruby-native or framework
integration can enable syntax or declarations without creating a useful
source binding. Treating that capability as a synthetic `Native` or `Rails`
declaration would expose a marker in completion, aliasing, collision, and
unused-binding rules. Treating a zero-match bare import as activation would
instead make the meaning of `import path` depend on hidden package metadata.

Ruby demonstrates the readability of uppercase classes and modules as call
owners. Go normally keeps functions qualified by a declared package name and
uses a distinct blank import for initialization-only dependencies. TypeScript
and JavaScript make peer declarations convenient through named imports and
distinguish bindingless module loading. TypeRB combines the declaration
properties without copying Ruby file loading, Go's lowercase package
identifiers, or general JavaScript side-effect imports.

The import path cannot mechanically determine capitalization. In particular,
`json` may intentionally expose `JSON`, not `Json`, and `request_id` may expose
`RequestID`, not `RequestId`. The rule must handle snake-case paths and
acronyms without an inflector, acronym registry, or package-specific override.

## Decision

An importable module exposes public top-level declarations. The language's
separate visibility decision determines which declarations are public; this
decision defines how those declarations become bindings in an importing
module.

Declaration imports and capability activation are distinct operations:

- `import` selects one or more real declarations and creates source bindings.
- `activate` enables a declared compiler or provider capability without
  creating a source binding.

### Named top-level declarations

A named import selects declarations by their exact authored names:

```trb
import { Hex, Base64 } from trb/std/encoding
import { describe, expect, test } from trb/std/test
```

The initial named-import surface contains every public top-level declaration:
modules, classes, records, enums, interfaces, type aliases, newtypes,
constants, and functions. Lowercase top-level functions are therefore
available from the initial implementation.

A named import never searches inside another declaration. If `sqrt` is a
member of `Math`, the following import is invalid because the module has no
top-level declaration named `sqrt`:

```trb
import { sqrt } from trb/std/math
```

The diagnostic should identify `Math.sqrt` as the available qualified member
when there is one unambiguous match.

Every named declaration may use `as` to resolve a local binding conflict. The
declaration is still selected by its exact authored name:

```trb
import { Response as WebResponse } from trb/web
import { Response as BrowserResponse } from trb/platform/typescript/browser
```

The alias changes the source and generated local binding. It does not change
the canonical exported or native declaration identity, owner, runtime export
name, or provider selection.

Within one authored source scope, a module binds one imported declaration
identity under at most one local name. An identical repeated binding is
redundant. Importing the same declaration again under a different alias is an
error rather than a way to create synonymous local spellings.

### Bare root shorthand

A bare import binds one root-eligible public top-level declaration whose name
matches the logical final segment of the resolved module path:

```trb
import trb/std/math

Math.sqrt(9)
```

Root-eligible declarations are modules, classes, records, enums, interfaces,
type aliases, newtypes, and constants. Top-level functions are
named-importable but are never bare-root candidates. For example, the
top-level `test` function does not make `import trb/std/test` a function
import.

The shorthand is not a lowercase package namespace, a wildcard import, or an
import of every public declaration. Adding an unrelated export therefore does
not add a source binding or change existing name resolution.

The selected root's declaration kind remains significant. A bare import may
bind an actual class or value type; it does not convert that declaration into a
namespace. The separate canonical-operation-owner rules decide whether a
standard concept is a type or a module. Declaration-root imports do not require
every package to expose a module-shaped root.

The resolver determines the root as follows:

1. Resolve the import to its canonical logical module. When the resolved
   module is a directory `index`, use the parent directory name rather than
   `index` as its final segment.
2. Consider only root-eligible public top-level declarations.
3. Form the path root key by removing ASCII `_` from the final path segment
   and folding ASCII `A` through `Z` to lowercase.
4. Form each declaration root key by folding ASCII `A` through `Z` to
   lowercase without removing characters from the declaration name.
5. Select the declaration whose complete key equals the path root key.
6. Bind the selected declaration under its exact authored name.

Only ASCII letter case and underscores in the path segment receive special
treatment. Declaration underscores, path hyphens, digits, and all other
characters remain significant. The asymmetric rule maps a snake-case path to
an existing identifier without making declaration underscores invisible. It
compares existing declarations and never synthesizes a PascalCase or acronym
spelling from the path.

Examples include:

| Final path segment | Declaration | Match |
| --- | --- | --- |
| `math` | `Math` | yes |
| `json` | `JSON` | yes |
| `url` | `URL` | yes |
| `hmac` | `HMAC` | yes |
| `base64` | `Base64` | yes |
| `filesystem` | `FileSystem` | yes |
| `file_system` | `FileSystem` | yes |
| `secure_random` | `SecureRandom` | yes |
| `string_builder` | `StringBuilder` | yes |
| `request_id` | `RequestID` | yes |
| `secure_random` | `SECURE_RANDOM` | no |

If there is no match, the bare declaration import is an error and the
diagnostic lists available top-level declarations that can be imported by
name. A module is not required to provide a matching root. For example, the
`time` module intentionally exposes peer declarations such as `Date` and
`Duration` without adding a `Time` root.

A root must also be a valid ordinary binding in the importing scope. Standard
and official package authors must not choose a root that collides with an
unrelated prelude declaration. A compiler-owned standard package may expose a
built-in type as its root only when the imported declaration is that same
canonical type, rather than a utility namespace with a second meaning. This
narrow rule allows the actual `StringBuilder` type to own its factories through
`trb/std/string_builder`; built-in receiver operations need no utility root or
import. The rule is not available to official or third-party packages. In
particular, the digest API uses `import trb/std/digest` and
`Digest.sha256(...)`; it does not reuse `Hash`, which is already the built-in
`Hash<K, V>` collection type.

If more than one declaration has the same root key as the path, the bare
import is ambiguous. Exact named imports remain available when the declaration
source is allowed to contain the collision:

```trb
import { JSON, Json as LegacyJson } from some/json
```

A bare import may use `as` to resolve a local binding conflict. The resolver
first finds the same unique root and then binds it under the explicit alias:

```trb
import trb/std/json as WireJSON

WireJSON.encode(value)
```

### Canonical directory entry paths

A direct TypeRB module `name.trb` and a directory entry module
`name/index.trb` define the same authored import root and cannot coexist in one
resolved TypeRB source graph. The conflict is a project or package error rather
than a precedence rule. Other files below the `name` directory remain valid;
only its `index.trb` conflicts with the peer `name.trb`.

When only `name/index.trb` exists, both `name` and an explicit `name/index`
resolve to that module identity. The shorter `name` form is canonical, and
project-aware formatting removes the terminal `/index`. When only `name.trb`
exists, `name` resolves directly to it. An unresolved import or source-only
formatting without a project snapshot preserves its authored path rather than
guessing.

This invariant prevents a formatter-shortened import from silently changing
identity when a direct peer file is added later: the new file creates a module
graph conflict until the source layout and imports are changed explicitly.

### Root stability and declaration provenance

Root stability does not change bare-import resolution. Both root-stable and
non-root-stable targets use the same current zero-, one-, or multiple-match
rule. It determines only whether tooling may automatically convert between a
bare root and its exact named form while canonicalizing imports.

Root stability is a compiler-derived proof for a resolved target, selected
mode, and locked version. It is not a package-authored boolean or formatting
preference. The proof covers the target's effective export surface, not merely
its source file. A target is root-stable only when every contribution that can
add a root-eligible public declaration is subject to the TypeRB root-collision
rule. Contributions include authored TypeRB source, compiler-generated
declarations, attached provider catalogs, and native declaration data.

For a root-stable target, at most one root-eligible declaration may match the
final path segment under the root-key rule. The compiler validates the rule
after assembling all effective declaration contributions. Publishing a second
matching root is an authoring error. This restriction applies only to
declarations competing for that target root; it is not a global ban on all
case-fold-equivalent names.

The unique root candidate set is part of a root-stable target's public API.
Removing or renaming its unique root is a breaking change. Adding a second
matching root is invalid rather than a compatible extension. Changing a
root-stable target to non-root-stable is also a breaking change even when its
current root remains unique, because it withdraws the compatibility guarantee
used by formatter canonicalization.

A target is non-root-stable when any contribution preserves names from an
ecosystem that TypeRB cannot constrain. Examples include TypeScript
declarations, future RBS or Go export data, and a TypeRB-authored package root
augmented by an unconstrained native provider catalog. These declarations
retain their exact names. If the effective target exposes both `JSON` and
`Json`, it remains usable through exact named imports, but its bare root form
is ambiguous. TypeRB does not reject or silently rename such declarations
merely to satisfy the shorthand.

### Capability activation

Capability-only use has a separate bindingless form:

```trb
activate trb/platform/ruby/native
```

`activate` is a contextual source-module top-level statement introducer, not a
globally reserved identifier. The unparenthesized command-shaped form
`activate PATH` enters activation grammar before target resolution and then
requires exactly one syntactically valid import path. An unknown target,
unsupported mode, missing capability, or malformed activation produces an
activation diagnostic; the parser never falls back to a Ruby-native call based
on resolver metadata.

Identifier uses outside that statement form remain valid, including
`activate(user)`, `receiver.activate(user)`, and declarations named `activate`.
In Ruby mode, an unparenthesized top-level native call such as `activate user`
or `activate "plugin"` conflicts with the activation form and must use
parentheses. Calls in class or method bodies are not source-module activation
statements.

`activate` uses the same canonical path and mode resolution as an import, but
does not perform declaration-root matching and does not accept a named list or
`as` alias. It is valid only when the resolved target explicitly declares one
or more of the following:

- a compiler-owned syntax, type, JSX, or framework capability; or
- a manifest-authorized and validated declaration-provider activation.

The target declares a mode-specific set of applicable capabilities. Both an
authored `activate` and any authored declaration import from that target enable
the full applicable set. Capability-specific selectors are deferred. Adding or
removing an applicable capability can change source or runtime behavior and is
therefore a breaking change to the target's public contract.

Activating an ordinary module with no declared capability is an error. An
arbitrary module cannot use this form merely to request runtime side effects.
The compiler never executes package-supplied source code as part of capability
activation or provider evaluation.

An authored declaration import also enables the declared applicable
capabilities attached to its resolved target. A module that imports and uses
`ReactNode`, for example, does not need a second activation line to select the
React JSX and type support:

```trb
import { ReactNode } from trb/platform/typescript/react
```

If a source module needs only the capability and no declaration binding, it
uses `activate`. A Ruby-native or Rails integration is the initial common
case:

```trb
activate trb/platform/ruby/rails

class ProductsController < ActionController::Base
end
```

Each source module has its own active capability set, derived from that
module's authored imports and authored `activate` forms. Module-local syntax
gates, JSX rules, unbound provider declarations, and framework DSL or
name-resolution rules are available only when their capability is active in
that source module. Activation in one file does not grant those facilities to
another file and is not re-exported transitively.

A compiler-generated required import is a binding dependency scoped to the
generated fragment or fragments for which the host accepted it. It may bind
declarations needed by generated source, but it does not enable syntax, JSX,
type, project, or framework providers, expose unbound declarations to authored
source, include a capability runtime root, satisfy an authored activation
requirement, or activate another provider. A generated fragment may use the
capabilities already activated by authored source in its owning module, but the
initial model has no generated-source operation that enables an additional
capability. Compiler-generated source cannot contain `activate`; a generated
import does not contribute to the authored source module's active capability
set or affect an unrelated generated fragment.

Physical deduplication of imports or runtime dependencies does not merge their
source-consumption edges. A generated reference consumes the generated
required-import edge for its fragment and never marks an authored import or
specifier as used. An authored alias and a generated exact binding remain in
separate scopes even when they resolve to the same declaration identity and the
backend emits one canonical dependency.

A provider-generated public member belongs to its owning nominal declaration
identity and remains visible wherever that declaration or a value of its type
is reached through a valid declaration or contract edge. Such an edge may be a
direct import, alias, inheritance relationship, field or generic contract, or
an imported function's return type. Direct import of the owning type name is
not a member-visibility gate. For example, a model returned by an imported
function retains its public generated query methods even when the source did
not import the model type by name. This does not make the unqualified type name
available for source annotations or activate the provider capability in the
consuming module. Native syntax, JSX, unbound provider declarations, unrelated
framework rules, and members added to an ambient declaration with no reached
owner remain capability-gated.

A provider may compute and cache project-wide data once and may inspect the
project inputs allowed by its documented contract. Declaration computation is
project-wide work; capability gates and unbound name visibility remain
module-local.

A declared capability is either compile-only or may declare a canonical
runtime root. When present, the runtime root is included exactly once in the
generated program, regardless of how many source modules activate or import
the target. It follows the same deterministic dependency-graph initialization
order as an ordinary package module, and ordinary imports and activations
deduplicate the same canonical runtime root.

An explicit `activate` authorizes inclusion of such a declared runtime root;
the compiler does not claim to prove that the root has no runtime side
effects. The package manifest and lock validate its identity, dependency,
provider data, and declared association with the capability. They do not make
package source executable inside the compiler. Compile-only capabilities add
no runtime root.

An imported declaration must still be referenced even when the same import
enables a capability. If only the capability is needed, `activate` is the
canonical form. Activating and normally importing the same canonical target
in one source module is redundant and produces a diagnostic. Repeated work is
deduplicated by stable capability, provider, and runtime-root identity.
Different targets are not considered redundant merely because they declare
overlapping capabilities; incompatible overlaps are diagnosed separately.

### Canonical formatting

After ordinary import-path canonicalization, `trb fmt` combines compatible
named imports of the same canonical target and import group into one named
list. This operation preserves every exact imported name and local alias, so it
does not depend on root stability:

```trb
import { JSON } from vendor/json
import { Json as LegacyJson } from vendor/json
```

becomes:

```trb
import { JSON, Json as LegacyJson } from vendor/json
```

Specifier ordering follows one deterministic formatter rule. Identical
specifier bindings may be deduplicated, but binding one declaration identity
under different aliases or reusing one local name for different declarations
remains a diagnostic rather than being renamed by the formatter. Imports
remain separate across a blank-line group boundary or when merging would lose
an attached comment or directive. An `activate` form is never merged with a
declaration import because it expresses different source semantics.

For a root-stable target, the formatter may also expand a bare root to its
proven exact name when that allows compatible imports of the same target to use
one named list:

```trb
import trb/std/json
import { Parser } from trb/std/json
```

becomes:

```trb
import { JSON, Parser } from trb/std/json
```

The same rule preserves a bare root alias as an exact named-specifier alias.
For a non-root-stable target, a bare import and named import remain separate;
the formatter does not invent an exact name from a root whose continued
uniqueness is not guaranteed.

When project-aware resolution proves that a singleton named import selects the
unique matching root of a root-stable target, `trb fmt` uses the bare
shorthand:

```trb
import { JSON } from trb/std/json
import { JSON as WireJSON } from acme/json
```

becomes:

```trb
import trb/std/json
import acme/json as WireJSON
```

The target-wide root-stability rule makes this rewrite stable against a future
second matching root. After compatible named imports are merged, a named list
containing multiple declarations remains named. A declaration that does not
match the path root, an unresolved import, or source-only formatting without a
project snapshot is also preserved rather than rewritten between named and
bare forms.

For a non-root-stable target, `trb fmt` preserves an authored exact named
import even when the current effective catalog has one matching root. A future
package version may add another declaration with the same root key that TypeRB
cannot forbid, while the exact named import remains valid. This includes mixed
targets whose TypeRB source is augmented by an unconstrained native provider.

An already-authored bare import is preserved for a non-root-stable target when
the current effective catalog has one matching root. The formatter does not
expand it back to an exact named import. Automatic conversion between exact
named and bare forms in either direction is outside the formatter's scope when
continued root uniqueness cannot be proven.

Formatting never converts between `import` and `activate`; they express
different source semantics.

### Owned nested declarations

Declaration ownership, rather than physical package grouping, determines
whether a type is nested. Operations use `.` and nested declarations use
`::`:

```trb
import trb/std/json
import trb/std/result

def decode_user(text: String): Result<User, JSON::Error>
	return JSON.decode<User>(text)
end
```

A supporting declaration is nested when its unqualified name is generic or it
has meaning only as part of one capability. Examples are `JSON::Error`,
`FileSystem::Error`, `Process::Output`, and `Hex::DecodeError`. Nesting keeps
ownership visible and avoids peer names such as `JsonError`, `FileError`, and
`ProcessResult`.

A declaration remains a peer top-level export when it is an independently
useful part of the program's vocabulary. Types such as `Instant`, `Duration`,
`Date`, and `TimeZone` are imported by name rather than being forced under a
`Time` namespace:

```trb
import { Instant, Duration, TimeZone } from trb/std/time
```

Likewise, `Hex` and `Base64` are peer capability modules in the `encoding`
group, not implementation details nested under an `Encoding` root. Each owns
its operations and decode error family, so the standard library keeps them as
separate canonical leaf modules:

```trb
import trb/std/encoding/hex
import trb/std/encoding/base64
```

The standard library does not add an aggregator or re-export solely to reduce
the number of import lines, because that would create synonymous import paths.
A shared package is appropriate when it is itself the canonical owner of peer
declarations, as with `trb/std/time`. This is an API ownership rule, not an
exception table maintained by the compiler.

### Implementation order

The `activate` source contract is accepted here, but its implementation is
deferred with the rest of the declaration-root import work. It need not ship
as a standalone language feature.

Implementation resolves and validates the canonical target, selected mode,
and applicable capability set before loading attached validated provider data.
That data may then participate in declaration selection. A final import that
fails declaration resolution does not activate the target. Provider selection
must derive from validated dependency edges rather than unresolved source
strings.

The compiler must preserve authored and generated provenance while deriving
capability sets. Existing generated-import markers must be applied consistently
to native syntax, JSX, provider discovery, unbound declarations, and runtime
roots rather than only to declaration-binding visibility. Required imports may
be collected or emitted together as an implementation detail, but their
semantic scope remains limited to the generated fragments that requested them.

Root-stability classification uses the effective target after provider data is
loaded and is derived by the compiler rather than asserted by package
metadata. If any declaration contributor is not subject to the root-collision
rule, the entire target is non-root-stable for formatter canonicalization even
when the selected declaration itself came from TypeRB source.

The implementation introduces the new import and activation semantics as one
source-model change. It does not retain lowercase package namespaces, legacy
activation-only imports, wildcard project imports, deprecation aliases, a
compatibility mode, or migration tooling. Compiler-owned and official source
is updated in the same change. Other source using an old form must be rewritten
to the declaration binding or `activate` form required by the new model.

Declaration-bearing root imports may be implemented in reviewable stacked
changes, but no intermediate compatibility behavior becomes part of the
public language contract. Each implementation stage must test the final
semantics it owns.

## Consequences

- Common domain operations have one qualified spelling, such as
  `Math.sqrt(9.0)`, and member completion starts from an explicit owner.
  Operations on an existing value use their receiver instead of a duplicate
  utility root.
- Public top-level functions such as `describe`, `expect`, and `test` remain
  directly importable without making module members directly importable.
- Multiple peer declarations and collision-resolving aliases still fit on one
  import line, and compatible repeated named imports are formatted into that
  canonical list without discarding exact identities.
- `JSON`, `URL`, and other acronyms retain their declared capitalization
  without compiler vocabulary or inflection configuration.
- A bare declaration import exposes exactly one binding. Source scope does not
  silently grow when a dependency adds unrelated exports.
- Root-stable targets guarantee a unique root across their effective export
  surface, while targets with unconstrained native or provider contributions
  retain exact names and exact-import escape hatches.
- Generated required imports cannot widen authored syntax, provider, framework,
  or runtime capabilities. Generated fragments initially use only capabilities
  already activated by authored source in their owning module.
- Provider-generated public members follow their nominal declaration identity
  through resolved contracts without implicitly exposing the declaration name
  or activating its provider.
- Snake-case module paths bind matching declarations such as `SecureRandom`,
  `StringBuilder`, and `RequestID` without synthesizing their capitalization.
- A direct module and directory `index` cannot compete for the same canonical
  authored import path, so formatter shortening cannot silently rebind later.
- Capability-only source remains rare and explicit without synthetic marker
  declarations or a metadata-dependent meaning for ordinary imports.
- Supporting types can arrive with their owner through one root import, while
  frequently used independent types remain concise in annotations.
- Existing package imports that create lowercase namespaces, import all
  project exports, or activate an integration are invalid under the new model
  and must be rewritten with the source change that adopts it.
- The compiler must support imported modules with public methods and nested
  declarations, including namespace-stable type identity across modules and
  backends. Current implementation limitations do not alter the source model.

## Rejected alternatives

- Allowing both qualified module members and direct imports of those members
  by default creates synonymous spellings for the same operation.
- Treating every bare import as a lowercase namespace preserves a Go-like
  surface but conflicts with TypeRB's uppercase module model and makes
  `Math`, `JSON`, and application roots behave differently from imported
  classes and modules.
- Importing every top-level export with a bare project import makes source
  scope change when a dependency adds an export and differs from standard and
  native package behavior.
- Treating zero-match bare imports as activation keeps one keyword but makes
  the meaning of `import path` depend on capability metadata and weakens the
  zero-match rule.
- Adding synthetic `Native`, `Rails`, or similar marker declarations exposes
  names that applications do not actually use and entangles activation with
  binding usage, aliasing, completion, and collision rules.
- A Go-like blank alias such as `import path as _` looks like a discarded root
  even when the target has no root and invites general side-effect imports.
- Selecting declaration import style by package metadata makes syntax
  knowledge depend on an invisible per-package policy.
- Selecting declaration import style by the number or kind of used symbols
  makes small source edits trigger unrelated import and use-site rewrites.
- Synthesizing PascalCase from snake case and maintaining acronym exceptions
  adds an inflection system where normalized comparison with actual exports is
  sufficient.
- Allowing both `name.trb` and `name/index.trb` and selecting one by precedence
  lets a later file addition silently change a formatter-shortened import's
  module identity.
- Banning every case-fold-equivalent declaration globally would reject or
  force adapters to rename valid native APIs unrelated to a module root.
- Preserving every matching singleton named import avoids formatter-induced
  compatibility risk but gives root-stable targets two canonical root
  spellings even though their effective export rule guarantees root
  uniqueness.
- Nesting every declaration produces repetitive names such as
  `Time::Duration`; flattening every declaration loses useful ownership and
  encourages prefixed names such as `JsonError`.

## Deferred work

- Direct imports of members such as `Math.sqrt`. This is distinct from
  importing a top-level function and would require separate syntax and
  justification.
- Additional path-separator normalization, such as matching `react-query` to
  `ReactQuery`. The initial rule removes ASCII `_` only.
- Wildcard imports and general public re-export syntax.
- Capability-specific selectors for a target that declares more than one
  capability.
- Structured, host-validated, fragment-scoped capability edges for generated
  source. Any future protocol must authorize provider identity, fragment
  identity, target capability, provider-discovery behavior, and runtime-root
  inclusion separately rather than treating generated source text as authority.
- Arbitrary runtime side-effect imports, conditional activation, and
  package-supplied executable compiler extensions.
- A general third-party executable activation protocol. Existing validated
  data-only provider manifests remain the maximum external activation scope.
