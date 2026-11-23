# Array queries and copies as ordinary MIR loops

Status: implemented subset of the basic-language completion milestone.

`include?`, `count` and `index` compare elements using portable equality.
`include?` stops at the first match, `count` visits all elements and `index`
returns the first zero-based position as `Integer?`. Absence returns false,
zero or nil respectively. Equality is admitted only for supported primitive
values and payloadless enums; nullable elements, records, nested collections,
Functions and payload enums remain explicit rejections under the reference
contract. String comparison preserves all bytes, including NUL and UTF-8.

`uniq` keeps the first occurrence in source order. NaN does not equal itself,
and positive/negative zero compare equal while retaining the first value's raw
representation. `concat` appends both inputs in order to fresh outer storage.
Both copies are shallow: managed elements retain their existing identity.
Neither method mutates the input nor requires a mutable receiver. Mutating a
returned Array still requires a mutable binding.

Capture the receiver before argument evaluation. Evaluate the argument exactly
once, then traverse the retained Array's current contents. Argument-side growth,
removal and rebinding follow the same rule as the existing Array methods.
Optional absence skips the argument and traversal; lexical transfer does not
manufacture a result or execute the operation.

The bounded `array_queries` owner constructs existing typed MIR instructions:
Array length/read/allocation/push, scalar/String/enum comparisons, nullable
construction and ordinary control-flow edges. Private loop bindings carry all
live values through verified block arguments. No new runtime operation or
backend source-pattern recognition is introduced. Existing MIR effect and root
analysis therefore owns copy allocation and input/output lifetimes.

The initial searches are linear and uniqueness uses repeated equality scans,
matching the reference's portable semantics. Future acceleration must preserve
NaN, signed zero, first-occurrence order and String bytes; it is not a condition
for integrating language support under the current MIR milestone.

Source-erased/reordered fixtures, forged root controls and forced collection
exercise ordinary MIR ownership. Recovery-unit literals retain the existing
seed subset; ordinary CLI tests additionally exercise Unicode/NUL and retained
REPL enum identities. Shared cases cover all four ordinary paths and regenerate
Capabilities. Joining, sorting and safe collection APIs remain open; this is
not complete Array coverage or final performance qualification.
