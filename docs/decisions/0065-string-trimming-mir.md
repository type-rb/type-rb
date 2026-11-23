# Unicode String trimming through typed MIR

`String.strip`, `lstrip` and `rstrip` take no arguments and return an immutable
String with the selected edge whitespace removed. They use the pinned reference
contract's Unicode 17.0 `White_Space` property, including U+0085 and excluding
U+FEFF. Empty and all-whitespace inputs produce an empty String. Interior bytes,
embedded NUL and malformed UTF-8 remain intact. Receiver expressions execute
once; optional absence skips the operation.

The checker admits the ordinary String receiver and exact signature. MIR 50
owns the String input/result, direction (both, left or right), allocation/failure
effects and live input root. Its verifier rejects non-String operands/results,
unavailable values, extra operands, invalid directions and failure edges. The
QBE adapter only translates this verified operation.

The runtime scans bounded code points to find the retained byte span. It copies
that span once into one allocation and counts the resulting code points. A no-op
returns the original immutable value. No intermediate character Array, repeated
concatenation or byte-to-code-point reconstruction is needed. Work and allocation
are linear in the input size; this is not final performance qualification.

The REPL implementation calls these ordinary methods itself, using the same
Unicode behavior. The immutable seed still builds the core from its preceding
language subset, and the resulting core builds the CLI. The new runtime owner
is included in source-layout, recovery-mutation and fixed-point checks. No seed
or snapshot-format update is involved.

Shared ordinary check/build/run/REPL cases cover every whitespace member and
neighboring nonmembers, directional and empty results, Unicode counts, NUL,
invalid bytes, optional receivers, generics, closures, receiver effects and
signature/type rejection. Independent MIR tests erase source, reorder blocks,
force collection and reject forged operations or missing roots. CLI controls
retain all 256 byte values, malformed argv and stored session values, collect
immediately before result allocation and bound long-input allocation without a
wall-clock assertion. Other String receiver methods remain in the basic-language
completion inventory.
