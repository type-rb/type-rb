# Ordinary enum values and exhaustive patterns in MIR

## Scope and identity

Ordinary payloadless and payload enums now follow the pinned TypeRB semantics.
Variants have a nominal declaration identity, even when names or payload layouts
coincide. Named and bare imports preserve that identity. Required payload fields
support positional and named-only regions; explicit arguments run in authored
order before being arranged in declaration order. Numeric widening and managed
payloads use the same checked conversions as ordinary calls.

Case statements and expressions evaluate their selector once. Each qualified
variant appears at most once; without an else, every variant must be covered.
Patterns bind all fields, respecting their positional/named regions. Bindings
are immutable and scoped to their branch, and can shadow an outer binding.
Payloadless enum equality compares variants within the same nominal enum;
enums with payload variants do not gain structural equality.

Raw values and conversion, generic enums/Result, enum methods, attributes and
nested module declarations remain separate gaps. This is a basic-language
integration, not a claim of full enum, pattern or language coverage. The ordinary
compiler implementation does not yet use the newly supported syntax itself:
that requires an accepted immutable seed handoff.

## Checked representation and ownership

`enum_syntax.trb` parses declarations, `enum_types.trb` binds nominal variants
and argument slots, and `enum_checked.trb` checks exhaustive patterns and lexical
bindings. Declaration resolution retains the normal module/import authority.
`syntax_tokens.trb` owns the token/type syntax helpers shared with the main
parser, keeping declaration parsing acyclic and avoiding copied helpers.

MIR predeclares all record and enum shells before resolving fields, so nominal
fields may close recursive graphs through records, enums, optional values,
Arrays and Hashes. Its copied enum/variant/field catalog is independently
verified for identities, origins, ownership, spans and exact semantic types.
Checking metadata and REPL pattern projections can be erased after lowering.

Four typed operations own construction, variant testing, checked field extraction
and payloadless equality. Construction operands are in canonical field order;
MIR verifies their availability, number and types. Pattern tests produce Boolean
values and use ordinary control edges and block arguments. A verified exhaustive
source case can use its final covered variant as the final edge. Extraction
retains a runtime tag check, so a well-typed MIR extraction on another variant
fails instead of reading an incompatible field. Any later removal of that check
requires a verified variant fact above the backend.

## Lifetime and backend layout

Initially each enum value, including a payloadless variant, is an immutable
managed box. Its private layout is the GC header, an Integer variant tag, and
canonical eight-byte payload slots. Per-variant descriptors trace only fields
whose verified semantic types are managed. Constructors are allocating/failing
operations; argument roots and retained values use shared MIR liveness planning.
Arrays, Hashes, optional boxes and record fields therefore retain enum values
through the existing managed-value machinery.

`qbe_enums.trb` mechanically emits the verified operations and descriptors. It
performs no source pattern matching, exhaustiveness reasoning or payload type
inference. This layout favors one correct lifetime model for the complete family;
compact payloadless representations and elimination of proven checks remain
possible MIR improvements. No speedup or final Pure Go qualification is claimed.

## REPL and evidence

The REPL consumes checked constructor slots and pattern bindings, retains nominal
values and renders payloads in declaration order. Stable declaration and variant
names remap retained identities after imports or new declarations. Source
witnesses used only for checking retained bindings search for a finite constructor
path; recursive variants can precede their terminating alternative. Optional
and empty container fields terminate witness construction without allocation.
Explicit reload keeps the existing accepted-input replay contract.

Tests cover ordinary construction, equality, lazy exhaustive controls and loop
transfers, labels and evaluation order, shadowing and immutability, recursive
record/enum graphs, optional values and managed container aliases. MIR tests erase
source/declaration projections, reorder blocks, force collection at allocation
and call boundaries, and reject forged operations or descriptor catalogs. Direct
CLI controls cover imported aliases, retained values and replay. The shared
reference/Native cases and generated Capabilities views record accepted paths
and remaining gaps separately.

The canonical compiler closure contains 73 modules, with exact recovery import
boundaries and observable mutation controls for all 72 dependencies. Ordinary
replacement generations and recovery-enabled suites remain separate required
authorities. The verbose compiler recovery snapshot contains 515 functions, exceeding its
former 512-function decoder bound. The compiler-only bound is now 1,024; the
ordinary managed-snapshot entry remains at 512. Boundary tests cover 512, 513,
1,024 and 1,025. The 48 MiB compiler byte limit and all remaining schema/type/
instruction checks stay enforced. The initial failed recovery is retained as
evidence. No reference pin, seed, snapshot schema or performance budget changes.
