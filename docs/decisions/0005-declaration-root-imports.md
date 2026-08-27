# 0005: Declaration-root imports

Status: accepted design; implementation deferred

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
import { Hex, Base64 } from trb/std/encoding

hex_text := Hex.encode(payload)
base64_text := Base64.encode(payload)
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

Ruby demonstrates the readability of uppercase namespaces and owned nested
declarations. Go normally keeps functions qualified by a declared package
name and uses a distinct blank import for initialization-only dependencies.
TypeScript and JavaScript make peer declarations convenient through named
imports and distinguish bindingless module loading. TypeRB combines the
declaration properties without copying Ruby file loading, Go's lowercase
package identifiers, or general JavaScript side-effect imports.

The import path cannot mechanically determine capitalization. In particular,
`json` may intentionally expose `JSON`, not `Json`. The rule must handle
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

A source module binds one imported declaration identity under at most one local
name. An identical repeated binding is redundant. Importing the same
declaration again under a different alias is an error rather than a way to
create synonymous local spellings.

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

The resolver determines the root as follows:

1. Resolve the import to its canonical logical module. When the resolved
   module is a directory `index`, use the parent directory name rather than
   `index` as its final segment.
2. Consider only root-eligible public top-level declarations.
3. Fold ASCII `A` through `Z` to lowercase in both the final path segment and
   every candidate declaration name.
4. Select the declaration whose complete folded name equals the folded path
   segment.
5. Bind the selected declaration under its exact authored name.

Only ASCII letter case is ignored. Underscores, hyphens, digits, and all other
characters remain significant. The rule compares existing declarations; it
does not synthesize a PascalCase name from the path.

Examples include:

| Final path segment | Declaration | Match |
| --- | --- | --- |
| `math` | `Math` | yes |
| `json` | `JSON` | yes |
| `url` | `URL` | yes |
| `hmac` | `HMAC` | yes |
| `base64` | `Base64` | yes |
| `filesystem` | `FileSystem` | yes |
| `secure_random` | `SecureRandom` | no |
| `string_builder` | `StringBuilder` | no |

If there is no match, the bare declaration import is an error and the
diagnostic lists available top-level declarations that can be imported by
name. A module is not required to provide a matching root. For example, an
`encoding` module may intentionally expose only the peer declarations `Hex`
and `Base64`.

If more than one declaration matches after ASCII case folding, the bare import
is ambiguous. Exact named imports remain available when the declaration source
is allowed to contain the collision:

```trb
import { JSON, Json as LegacyJson } from some/json
```

A bare import may use `as` to resolve a local binding conflict. The resolver
first finds the same unique root and then binds it under the explicit alias:

```trb
import trb/std/json as WireJSON

WireJSON.encode(value)
```

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
final path segment after ASCII case folding. The compiler validates the rule
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
package version may add a case-fold-equivalent declaration that TypeRB cannot
forbid, while the exact named import remains valid. This includes mixed targets
whose TypeRB source is augmented by an unconstrained native provider. Tooling
may offer an explicit code action to adopt the shorter bare form.

An already-authored bare import is preserved for a non-root-stable target when
the current effective catalog has one matching root. The formatter does not
expand it back to an exact named import; a dependency migration may offer that
change as an explicit code action. Automatic conversion between exact named
and bare forms in either direction is outside the formatter's scope when
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

Likewise, `Hex` and `Base64` are peer capability modules selected from the
`encoding` group, not implementation details nested under an `Encoding` root.
This is an API ownership rule, not an exception table maintained by the
compiler.

### Implementation order and migration

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

Existing activation-only imports retain their current behavior until that
migration. The dedicated `activate` form must be available before any existing
activation-only import is rejected. During a published compatibility window,
the compiler may continue to accept the legacy spelling for already-recognized
capability targets with a non-fatal deprecation diagnostic and a migration code
action. The exception remains narrow and does not make new arbitrary targets
activatable through `import`.

Workspace-owned source can be migrated directly. Locked dependency source must
continue to compile during that compatibility window because the application
author cannot edit it in place. Hard zero-match rejection for the legacy form
requires a declared breaking compatibility boundary after bundled and official
packages have migrated; users of an affected external package then need an
updated package release. The legacy spelling does not remain as a permanent
alias.

Migration must also account for the current project-wide provider catalog. If
a source module uses module-local syntax, an unbound provider declaration, or
a framework rule without its own declaration import or activation edge, the
compiler diagnoses that module and offers a code action to add the appropriate
normal import or `activate` form. This does not apply to the effective public
members of a nominal declaration reached through an already-valid declaration
or contract edge. Replacing only the legacy import that originally enabled the
provider is not sufficient.

Declaration-bearing root imports may be implemented incrementally, but any
temporary zero-binding exception remains limited to existing registered
compiler integrations and already-supported fixed-provider roots. It is not a
public extension mechanism for arbitrary package activation.

## Consequences

- Common module operations have one qualified spelling, such as
  `Math.sqrt(9)`, and member completion starts from an explicit owner.
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
- Packages whose path contains a separator that is absent from the root name
  use an explicit named import in the initial model.
- Capability-only source remains rare and explicit without synthetic marker
  declarations or a metadata-dependent meaning for ordinary imports.
- Supporting types can arrive with their owner through one root import, while
  frequently used independent types remain concise in annotations.
- Existing package imports that currently create lowercase namespaces, import
  all project exports, or activate an integration require migration when this
  decision is implemented.
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
- Converting snake case to PascalCase and maintaining acronym exceptions adds
  an inflection system where comparison with actual exports is sufficient.
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
- Separator-insensitive root matching, such as matching `secure_random` to
  `SecureRandom`. The initial rule ignores ASCII letter case only.
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
- The standard-library and project-module migration, diagnostics, formatter,
  completion, compiler, and backend changes required to implement this
  decision.
