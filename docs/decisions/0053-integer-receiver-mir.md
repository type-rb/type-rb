# Integer receiver operations in MIR

Status: implemented subset under the basic-language and MIR consolidation milestones.

Ordinary Integer receivers support `abs`, `zero?`, `positive?`, `negative?`,
`even?`, `odd?`, `min`, `max`, `clamp` and `to_f`, following the exact reference.
These methods require parentheses and their declared positional arity. Nullable
receivers use the existing safe-navigation boundary; Integer aliases retain the
same canonical operations. Float receiver APIs and the wider numbers package
remain separate coverage work.

`integer_methods.trb` owns receiver classification, arity and MIR construction.
The checker evaluates the receiver once, then every argument from left to right.
A lexical transfer or earlier failure prevents subsequent evaluation. Optional
absence skips the arguments through the ordinary safe-navigation CFG. The REPL
uses the shared classification and a separate small Integer evaluator.

Predicates, absolute value, bounds and widening lower to existing scalar
operations and typed CFG joins. The portable Integer interval is symmetric, so
negating a valid Integer for absolute value cannot introduce an additional
failure. Remainder comparisons handle negative odd values without assuming a
positive remainder. Converting Integer to Float is exact across that interval.

Clamping first evaluates both limits, then checks their ordering. A reversed
interval fails even when the receiver would otherwise lie outside it. MIR
instruction 43 is a checked Boolean guard: it has no result or second operand,
an explicit trap target, and reason 1 for an invalid clamp interval. Independent
verification rejects unavailable/non-Boolean conditions, unsupported reasons and
malformed result/failure shapes. Its failure effect is retained by existing
operation-effect analysis. The QBE adapter consumes that verified condition and
reason; it does not reconstruct the numeric relation from source or operands.

Shared ordinary check/build/run/REPL probes cover extrema, negative parity,
chained receivers, source-order effects, absence, closures, invalid argument
counts/types and reversed-limit failure. Source-erased, reordered MIR and forced
collection controls cover both the primitive operations and surrounding managed
values. Raw MIR mutation controls independently check the guard contract.

The ordinary core closure now contains 101 modules. Recovery import prefixes,
module mutations and the frontend closure inventory include the new owner.
Implementation syntax remains compatible with the existing immutable seed;
compiler self-use of these receiver APIs awaits an accepted supporting seed.
No reference pin, bootstrap seed, release, timeout or performance qualification
changes are implied by this integration.
