# String transforms through verified MIR

Status: accepted implementation boundary within the basic-language milestone.

Ordinary `String.split`, `replace_all`, `upcase` and `downcase` share typed MIR
operations with the regenerated CLI. Receiver and argument expressions execute
once, left to right, before runtime validation. Optional receivers retain the
existing short-circuit contract, including skipped argument effects.

MIR kinds 60, 61 and 62 represent literal splitting, literal replacement and
simple case conversion. The replacement instruction stores its third SSA operand
in `payload`, following the existing Array/Hash store convention. The independent
verifier checks every operand's availability and type, result type, conversion
mode and failure shape. Liveness includes all three replacement operands; the
verified allocation root plan survives source-body erasure and block reordering.
The QBE adapter selects runtime calls without rediscovering source semantics.

Splitting and replacement search literal bytes, including embedded NUL and
malformed UTF-8. Empty separators/patterns fail after argument evaluation.
Splitting retains leading, adjacent and trailing empty fields and returns one
empty field for an empty input. Replacement consumes non-overlapping matches;
replacement text has no pattern/backreference syntax and is not searched again.
The runtime measures replacement output before allocating one String, checks
growth before multiplication, copies the spans and recounts code points across
new byte boundaries. Split results retain their Array root while releasing
temporary fragment roots after each append.

Case conversion follows the pinned reference Go implementation's Unicode 17.0.0
simple scalar mappings. It has no locale, multi-scalar expansion or contextual
final-sigma rules. Generated ordered mapping tables are checked against every Go
Unicode code point, and a compiled runtime test independently exercises lookup
at all 1,114,112 code points. UTF-8 decoding replaces malformed bytes with U+FFFD;
two passes measure and write the mapped output without per-character allocation.
Output-mode differences remain an explicit reference defect in
[TypeRB #791](https://github.com/type-rb/type-rb/issues/791), not a new portable
semantics decision or a claim of full three-language parity.

The ordinary CLI is compiled after core regeneration, so its retained evaluator
can use these new operations directly. It checks empty patterns before calling
the terminal-failure runtime, preserving the session after an invalid submission.
The canonical core source remains compatible with the immutable bootstrap seed;
new runtime modules are registered in recovery closure and mutation controls.

Shared ordinary-path cases cover boundaries, effects, optional calls, generic
containers, function values and rejection. Additional controls cover byte-width
changes, NUL, malformed bytes, forced collections, exact reclamation, bounded
allocation and retained sessions. The Go-hosted recovery frontend's ASCII-only
literal construction remains separate from ordinary Unicode runtime evidence.
These operations do not complete contextual inference, general object support,
diagnostic presentation or final performance qualification.
