# Float receiver operations through existing MIR

Status: implemented subset under the basic-language and MIR consolidation milestones.

Ordinary Float receivers support `abs`, `floor`, `ceil`, `round`, `finite?`,
`infinite?` and `nan?`, in addition to the existing `to_i` conversion. Their
zero-argument signatures follow the exact reference. Safe navigation preserves
absence, receivers execute once, and aliases retain their canonical Float type.
Float `to_s` is covered by [decision 0068](0068-scalar-string-conversion.md);
the wider numeric standard library remains a separate gap.

`float_methods.trb` owns classification and typed MIR construction. The ordinary
numeric receiver checker shares argument validation with Integer methods.
`mir_value_control.trb` now owns selection of already-evaluated typed values;
Integer bounds and absolute value use that same helper. Work that may fail stays
inside its selected branch. The REPL has a separate small Float evaluator and
one checked conversion helper, including the existing `to_i` path.

No new MIR instruction, QBE dispatch or runtime hook is introduced. Classification
uses IEEE comparisons and subtraction: finite values have a non-NaN self
difference; infinity is non-NaN with a NaN self difference. Absolute value negates
negative inputs and adds positive zero on the other edge, normalizing negative
zero while preserving NaN. These are ordinary verified Float operations and
typed control-flow joins, not backend recognition of a source pattern.

Rounding first uses the existing checked Float-to-Integer conversion. The portable
Integer interval is symmetric, and binary64 has no fractional representable value
between either endpoint and its adjacent out-of-range Integer. Checking the input
range before rounding therefore preserves which conversions are valid. Floor and
ceiling adjust the truncated Integer only on the required edge. Round compares
the fractional remainder with positive and negative one-half, resolving ties away
from zero. Adding one-half to the original Float would incorrectly change some
large integral values; that shortcut is not used. Every Integer adjustment keeps
its normal checked arithmetic and failure contract.

Non-finite conversion fails with `Float cannot be converted to Integer`; finite
out-of-range conversion fails with `Integer is outside the portable range`.
The REPL now preserves that distinction for `to_i` as well as rounding. Broader
diagnostic column, code and presentation parity remains tracked separately.

Shared ordinary check/build/run/REPL cases cover rounding ties and adjacent
values, both Integer limits, subnormal and extreme finite inputs, signed zero,
NaN and both infinities, aliases, optional calls, captured receivers, managed
values and invalid argument/result types. Independent fixtures erase source and
reorder MIR before execution with forced collection. Existing malformed-operation
and control-flow checks remain the admission boundary.

Those reorder controls exposed a scalar-leaf storage-order assumption. Scalar
adapter verification, Integer guard analysis and numeric call expansion now find
the entry by block identity. Reordering leaf and control blocks preserves the
independently verified guard and numeric plans; it does not choose an optimization
budget or silently remove a proof. Source-erased Float fixtures exercise reordered
leaf execution, and Integer controls retain their guards and emitted code.

The ordinary core closure now contains 102 modules. Recovery import boundaries,
module mutations and the closure inventory include the new owner. Compiler
implementation syntax remains compatible with the existing immutable seed;
self-use of the newly added receiver syntax awaits an accepted supporting seed.
No reference pin, seed, release, timeout or performance qualification changes
are part of this integration.
