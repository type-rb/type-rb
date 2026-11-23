# Builtin Result values, conversions and Unicode identifiers

Status: implemented direction under basic-language completion. This combines the
frontend, MIR, runtime and retained-session paths for one builtin value family.

`Array.try_fetch`, `String.try_fetch`, `Array.try_slice`, `String.try_slice` and
`Hash.try_fetch` return the canonical standard `Result` and structured error
declarations. The receiver and arguments are evaluated once, in authored order.
The retained receiver's size or key table is read after argument effects.
Negative indexes normalize against that live size; errors preserve the original
index. Slices require ordered nonnegative bounds, allowing an empty exclusive
range at the end. Hash errors preserve the requested Integer or String key.
Aliases do not replace canonical declaration identity.

These operations lower to existing typed MIR comparisons, branches, collection
reads and enum/record construction. No backend method-name analysis or alternate
Result representation is introduced. Implicit standard declarations are loaded
before type resolution, and a preliminary body pass completes inferred Result
instances before MIR catalogs freeze. The final catch expression receives the
known success type, including a directly returned empty Array; unrelated earlier
statements and explicit returns retain their own typing boundaries.

`String.try_to_i`, `String.try_to_f` and strict `String.to_f` use portable decimal
syntax and range rules. MIR verifies the scalar conversion and nonthrowing parse
status independently; existing typed branches construct success or the precise
`NumberParseErrorKind`. The shared runtime validates the entire byte sequence
before reporting range errors, rejects whitespace/NUL/hex/NaN/infinity spellings,
and preserves Float signed zero and subnormal values. Bounded short decimals
use an exact significand and power of ten followed by one rounded operation;
all other validated inputs retain system `strtod`. Neither conversion path
decides authored syntax or the Result failure model. The REPL uses the same
core parsing operations, removing its duplicate Integer parser.

After validating the complete grammar, the short-decimal path scans only a
bounded coefficient. Conversion is eligible only when the complete mantissa
has at most 16 digits, its magnitude is below 2^52, and the authored exponent
has at most two digits.
After subtracting the fraction's digit count, a decimal scale from -22 through
22 selects an exactly representable power of ten. The integer conversion is
exact; a single binary64 multiplication or division therefore rounds the exact
decimal value once. The sign bit is set before that operation to preserve
negative zero. Nonzero results in this range are normal and finite.

The original validation loops do not accumulate long significands or exponents.
Only at most 16 digits and one decimal point enter the coefficient loop; its
integer accumulator cannot overflow. One or two exponent digits are decoded
only after validation. Syntax validation still examines every byte before
reporting range, and failure leaves the output slot untouched.
Long significands, large exponents, subnormals and overflow retain the existing
fallback. `tools/native-float-parsing-test.py` checks emitted arithmetic against
an independent rounding oracle, counts fallback calls, and exercises guard-page
boundaries and failure sentinels. Ordinary CLI/reference and retained-session
checks qualify the public paths separately. The four artifact targets run the
small emitted-arithmetic test; host CLI tests also run the ordinary checks.

Unicode identifier classification uses generated tables for the pinned Go
toolchain's Letter, Decimal_Number and Uppercase_Letter categories. A strict UTF-8
decoder and byte-preserving names retain distinct spellings without normalization.
ASCII constant/reserved-name rules remain unchanged. The immutable seed compiles
ASCII implementation source; the ordinary Native compiler's existing byte adapter
handles Unicode input. Recovered Native generations exercise these names directly.
B0 and the matched Go frontend execute the canonical ASCII byte-access fallback;
their explicit rejection is checked separately. B1 and every later recovered
Native generation must check and emit the Unicode fixture through both literal
source and ordinary file inputs. B0 does not establish Unicode frontend coverage.

Paired ordinary check/build/execution/REPL cases retain rejected inputs and
presentation gaps. Independent MIR verification rejects forged conversion types,
payloads and instruction shapes; source-body erasure, reordered blocks and forced
collections exercise managed success/error payloads. Retained sessions cover
declaration growth, imported aliases, explicit replay and failed submissions.
Capabilities are generated from those reviewed outcomes. Untyped discarded empty
collections, nested contextual inference, remaining String APIs, malformed-source
origins and broader display parity remain explicit contracts. No full-language or
performance qualification follows from this family alone.
