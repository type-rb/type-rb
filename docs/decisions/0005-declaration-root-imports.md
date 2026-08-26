# 0005: Declaration-root imports

Status: accepted design; implementation deferred

## Context

TypeRB needs an import model that keeps ordinary calls concise without giving
one operation several interchangeable spellings. A package-qualified form is
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

Treating these as two ways to import every function would make both
`Math.sqrt(9)` and `sqrt(9)` normal spellings. Choosing between them by
package-specific metadata, symbol count, or a lint heuristic would make the
same source operation change style as an API or call site evolves.

Ruby demonstrates the readability of uppercase namespaces and owned nested
declarations. Go normally keeps functions qualified by a declared package
name. TypeScript and JavaScript make peer declarations convenient through
named imports. TypeRB combines those properties without copying Ruby file
loading, Go's lowercase package identifiers, or the full set of JavaScript
import forms.

The import path cannot mechanically determine capitalization. In particular,
`json` may intentionally expose `JSON`, not `Json`. The rule must handle
acronyms without an inflector, acronym registry, or package-specific override.

## Decision

An importable module exposes public top-level declarations. The language's
separate visibility decision determines which declarations are public; this
decision defines how those declarations become bindings in an importing
module.

### Named top-level declarations

A named import selects declarations by their exact authored names:

```trb
import { Hex, Base64 } from trb/std/encoding
```

The initial named-import surface contains uppercase top-level declarations:
modules, classes, records, enums, interfaces, aliases, newtypes, and constants.
A named import never searches inside one of those declarations. If `sqrt` is a
member of `Math`, the following import is invalid because the module has no
top-level declaration named `sqrt`:

```trb
import { sqrt } from trb/std/math
```

The diagnostic should identify `Math.sqrt` as the available qualified member
when there is one unambiguous match.

### Bare root shorthand

A bare import binds one public top-level declaration whose name matches the
logical final segment of the resolved module path:

```trb
import trb/std/math

Math.sqrt(9)
```

This is a shorthand for importing the matching declaration. It is not a
lowercase package namespace, a wildcard import, or an import of every public
declaration. Adding an unrelated export therefore does not add a source
binding or change existing name resolution.

The resolver determines the root as follows:

1. Resolve the import to its canonical logical module. When the resolved
   module is a directory `index`, use the parent directory name rather than
   `index` as its final segment.
2. Fold ASCII `A` through `Z` to lowercase in both the final path segment and
   every public top-level declaration name.
3. Select the declaration whose complete folded name equals the folded path
   segment.
4. Bind the selected declaration under its exact authored name.

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

If there is no match, the bare import is an error and the diagnostic lists
available top-level declarations that can be imported by name. A module is not
required to provide a matching root. For example, an `encoding` module may
intentionally expose only the peer declarations `Hex` and `Base64`.

If more than one declaration matches after ASCII case folding, the bare import
is ambiguous. For example, a module exporting both `JSON` and `Json` cannot be
imported with a `json` root shorthand. An exact named import remains available:

```trb
import { JSON } from some/json
```

A bare import may use `as` to resolve a local binding conflict. The resolver
first finds the same unique root and then binds it under the explicit alias:

```trb
import trb/std/json as WireJSON

WireJSON.encode(value)
```

Named-import aliases are not introduced by this decision.

### Canonical formatting

When resolution proves that a singleton named import selects the unique
matching root, project-aware `trb fmt` uses the bare shorthand:

```trb
import { JSON } from trb/std/json
```

becomes:

```trb
import trb/std/json
```

This rewrite changes neither the imported binding nor any use site. A named
import containing multiple declarations, a declaration that does not match
the path root, an unresolved import, or source-only formatting without a
project snapshot is preserved.

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

## Consequences

- Common module operations have one qualified spelling, such as
  `Math.sqrt(9)`, and member completion starts from an explicit owner.
- Multiple peer declarations from one module still fit on one import line.
- `JSON`, `URL`, and other acronyms retain their declared capitalization
  without compiler vocabulary or inflection configuration.
- A bare import exposes exactly one binding. Source scope does not silently
  grow when a dependency adds unrelated exports.
- Packages whose path contains a separator that is absent from the root name
  use an explicit named import in the initial model.
- Supporting types can arrive with their owner through one root import, while
  frequently used independent types remain concise in annotations.
- Existing package imports that currently create lowercase namespaces or
  import all project exports require migration when this decision is
  implemented.
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
- Selecting import style by package metadata makes syntax knowledge depend on
  an invisible per-package policy.
- Selecting import style by the number or kind of used symbols makes small
  source edits trigger unrelated import and use-site rewrites.
- Converting snake case to PascalCase and maintaining acronym exceptions adds
  an inflection system where comparison with actual exports is sufficient.
- Nesting every declaration produces repetitive names such as
  `Time::Duration`; flattening every declaration loses useful ownership and
  encourages prefixed names such as `JsonError`.

## Deferred work

- Named imports of lowercase top-level functions. This can be added by
  extending the allowed top-level declaration kinds without changing the
  syntax or the root rule.
- Direct imports of members such as `Math.sqrt`. This is distinct from
  importing a top-level function and would require separate syntax and
  justification.
- Separator-insensitive root matching, such as matching `secure_random` to
  `SecureRandom`. The initial rule ignores ASCII letter case only.
- Named-import aliases, wildcard imports, and general public re-export syntax.
- The standard-library and project-module migration, diagnostics, formatter,
  completion, compiler, and backend changes required to implement this
  decision.
