# Direct binary64 decimal conversion

The scalar runtime converts a finite, nonzero binary64 value into a decimal
coefficient and power using fast unrounded scaling. It then expands those digits
into the fixed notation required by [Decision 0068](decisions/0068-scalar-string-conversion.md).
Zero and nonfinite values keep their existing static descriptors. Finite nonzero
results allocate one ordinary String through the existing managed boundary.

The algorithm is adapted from Go 1.27.3 at
[`2aae1000ea1d4b28923b96db28acf4e3b99d6aff`](https://github.com/golang/go/blob/2aae1000ea1d4b28923b96db28acf4e3b99d6aff/src/internal/strconv/uscale.go),
described in [Fast Unrounded Scaling](https://research.swtch.com/fp).
[The source pin](../third_party/go-float/source.json) records the exact inputs;
the [BSD license](../third_party/go-float/LICENSE) accompanies adapted source and
must accompany distributed artifacts. No Go runtime, external conversion
library, new linker input or QBE extension is required.

Development compiler packages include `GO-FLOAT-LICENSE`, this guide as
`FLOAT-DECIMAL.md`, and `float-decimal-source.json`; Linux amd64 qualification
bundles retain the same
notice and pin. Applications that distribute this emitted conversion must also
carry the BSD notice. A future bootstrap seed refresh must include it; the
previous immutable seed is unchanged by this source adaptation.

## Arithmetic invariants

- The mantissa is normalized into an unsigned 64-bit word. Subnormal values
  retain their original binary spacing; normal values retain 53 significant bits.
- The two binary rounding endpoints are scaled together. Powers of two above
  the minimum normal binade have a lower half-interval. Odd significands exclude
  endpoint ties; even significands include them.
- An unrounded scaled value reserves two low bits for rounding and sticky
  information. The low reciprocal product is needed only at an exact upper-word
  boundary. Its subtraction uses unsigned borrow and wraparound intentionally.
- `trbn_decimal_multiply_high` uses four 32-by-32 products. Its cross-product
  sums and final high word fit in 64 bits. These are private raw QBE words;
  checked TypeRB Integer arithmetic and its portable range do not change.
- A generated reciprocal pair represents `high * 2^64 - low`. Exact integer
  generation covers decimal powers -292 through 324, the binary64 formatting
  range. Runtime checks guard table indices and shift counts before use.
- The shortest coefficient comes from the scaled interval, with nearest-even
  selection when more than one coefficient is possible. Trailing coefficient
  zeros are removed by increasing the decimal power. Fixed expansion needs at
  most 384 stack bytes, including a sign and the smallest subnormal's zeros.
- Integer and Float inline the same decimal digit writer. It divides an unsigned
  magnitude by 100 and stores two ASCII digits at a time using the existing
  200-byte Integer table. Remainders use the quotient already computed; the
  leading group alone may contain one digit. There is no added runtime call.
  Integer's small-value cache, Float's decimal rounding and managed allocation
  remain separate from this shared digit output.

The generated table occupies 9,872 bytes before object-file alignment and linker
effects. Actual application and compiler costs are measured separately. The
table and helpers are emitted with the scalar conversion runtime; source presence
alone does not establish their cost in every executable.

The [initial evaluation](https://github.com/type-rb/type-rb-native/issues/1305#issuecomment-6567672783)
records the measured benefits, binary and compiler costs, and remaining
noninferiority and RSS failures. Its local results are specific to Darwin arm64;
they do not qualify the other hosts or establish public Linux performance.

## Qualification and maintenance

`tools/float-decimal-powers.py --write` regenerates the table without a Go
toolchain. Its default mode verifies the committed output. The emitted-runtime
test compares seeded binary64 outputs against an independent shortest-decimal
oracle and unsigned high products against exact integer arithmetic. The four
release-target CI lanes run this small arithmetic test with the pinned QBE.
That test supplies only an allocation boundary; ordinary compiler, scalar MIR,
reference, forced-GC and REPL tests remain separate authorities.
Passing `--compiler` to the arithmetic test also checks every extracted runtime
definition against ordinary compiler emission. Scalar conversion tests cover all
two-digit pairs, power-of-ten boundaries, both signs and portable Integer extrema.

The Native runtime maintainer owns this adaptation and its source/license pin.
Review relevant upstream conversion fixes when updating it, regenerate and check
the table, and requalify all four target profiles. Changes to output or rounding
semantics require a language decision. Raw arithmetic success does not establish
ordinary compiler availability or release qualification on a target.
