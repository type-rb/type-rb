# Checked String slicing through MIR

Status: implemented subset under the basic-language and MIR consolidation milestones.

Ordinary programs and the REPL accept `String.slice(Range<Integer>)`, matching
the pinned reference. The receiver is captured before evaluating the Range;
its endpoints run once in source order. Rebinding the receiver variable does
not retarget the call. An absent optional receiver skips the Range expression,
and lexical transfers in an argument retain their surrounding scope.

Positions count Unicode code points. Limits must be nonnegative and ordered.
An exclusive end may equal the String size; an inclusive end must be smaller.
Thus `size...size` is a valid empty slice, including `0...0` on an empty String.
Invalid ranges fail with `String slice range is out of bounds`. Validate before
incrementing an inclusive end, including at the portable Integer maximum.
Invalid external bytes normalize to one replacement character per invalid byte,
and embedded NUL remains part of the result. Combining marks and components of
a grapheme cluster retain their separate code-point positions.

Internal MIR instruction 46 is a checked slicing operation: an available String
and `Range<Integer>` produce a String, with zero payload and no resumable failure
edge. The verifier rejects malformed operands, result types and extra modes.
Allocation and terminal failure effects are explicit. Shared liveness retains
both operands until the operation executes; forged root plans are rejected.
This operation does not assert prevalidated bounds or an unchecked access fact.
Any future check elimination needs a separately verified MIR proof.

The QBE adapter maps the verified operation directly to its bounded runtime in
`qbe_string_slices.trb`. That runtime implements the operation's range check
before looking up byte offsets. It measures normalized output bytes, allocates
one result String and copies the selected code points in a second pass. Work is
linear in source length, without constructing a temporary code-point Array.
The internal byte-preserving source loader slice remains separate because it
must preserve source bytes rather than normalize invalid external text.
The retained REPL evaluator uses existing indexed String primitives and is
not a claim about ordinary runtime traversal cost.

Shared file check/build/run/REPL cases cover boundaries, failure order, captured
receivers, aliases, optional calls, Unicode and rejected signatures. Dedicated
MIR tests erase frontend information, reorder block storage and force collection
around retained Strings, Ranges and results. Runtime UTF-8 controls also force
collection during result allocation. The canonical closure now has 106 modules;
its independent recovery inventory and mutations include the new runtime owner.

Compiler source remains compatible with the existing immutable seed. Public
`try_slice`, Array slicing, remaining String APIs, diagnostic parity and final
performance qualification remain separate work. The combined shared String
query/sequence/slice case now passes all ordinary execution paths.
