# Concrete generic nominal types before MIR

## Boundary

Required-field generic records and enums use explicit type arguments, following
the public TypeRB generic declaration/application contract. A case pattern names
the enum template and inherits its concrete arguments from the selector. Type
arguments are invariant, including when individual numeric constructor arguments
can widen to a field's Float type. Generic enum variants require payloads.

Authored templates live separately from concrete record/enum catalogs. Each
instance is indexed by declaration identity and canonical semantic arguments.
Instantiation reserves the complete field/variant spans and publishes a nominal
shell before resolving substituted fields. Recursive references reuse that
shell; nested applications cannot interleave fields belonging to different
instances. Imported argument types are resolved at the call site, while authored
field names are resolved in the template's declaration module.

Template validation rejects duplicate declarations, parameters, members and
unknown field types even when no instance is constructed. Application resolution
checks arity and field types. A 128-level active expansion guard diagnoses types
whose changing arguments keep expanding instead of exhausting the host stack.
Cached recursive identities do not consume another expansion level.

Declaration signatures, constructor applications and local annotations are
resolved before the compiler declares MIR types and signatures. The checker
publishes normal concrete record/enum operations. Verified MIR catalogs and
root plans own all executable type/layout facts; QBE does not inspect templates,
type-argument syntax or constructor tokens. This introduces no new runtime
boxing layer or target-specific generic path.

## REPL and source organization

Constructor evaluation consumes the checked concrete application projection.
Retained witnesses include explicit arguments and visible import aliases. When
new declarations reorder nominal indexes, remapping follows the template's
module/declaration identity and recursively remaps its argument identities.
Runtime value rendering uses the declaration name, while type rendering retains
the explicit arguments, as in the reference implementation.

`declaration_lookup.trb` owns visible name lookup; `type_resolution.trb` owns
semantic type identity and concrete instance creation. The generic model,
argument substitution, parser and template validation have separate modules.
Canonical recovery imports and per-module mutation controls include all six new
modules. The ordinary core closure has 79 modules. The implementation itself
continues to use syntax accepted by the unchanged immutable seed.

## Validation and remaining work

Compiler tests execute nested, recursive, named-argument and managed-container
examples through QBE, with collection forced before allocation and ordinary
calls. Erasing checked source, template data and application projections leaves
MIR verification and emitted output unchanged. Negative controls cover arity,
invariance, malformed templates, missing explicit arguments, nonexhaustive
patterns and nonconverging expansion. CLI controls retain values across new
declarations, imported nominal arguments, failed submissions and replay.
The shared ordinary-language inventory adds these paths and preserves measured
reference differences rather than rewriting the exact reference pin.

Generic record defaults, generic functions/methods, generic aliases,
classes/interfaces and standard Result/try/catch/must-use are not implemented by
this change. Function instantiation must provide instance-specific checked facts
before sharing a generic body; token-indexed projections alone are insufficient.
New syntax must not be adopted in compiler source until the seed handoff policy
permits it. Integration correctness is separate from milestone performance
qualification; no Pure Go comparison is claimed here.

The compiler recovery snapshot exceeded its former 48 MiB byte envelope during
this integration (approximately 48.35 MiB with the canonical source inventory).
The compiler-only envelope is now 64 MiB, with exact-limit and one-byte-over
controls. Ordinary managed recovery retains 4 MiB; function, type, block and
other schema limits remain unchanged. This metadata capacity is separate from
ordinary Native output size and does not alter runtime input handling.
