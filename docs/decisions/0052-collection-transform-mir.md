# Value-producing collection blocks in MIR

Status: implemented; integration validation is recorded with the reference update.

## Contract

`Array<T>` and `Range<Integer>` support `map`, `select`, `reduce(initial)`,
`any?`, `all?`, `none?`, `find` and `find_index`
with brace or `do` blocks. `map` and `select` also support `with_index`.
Their block parameters are mutable lexical bindings. Each completing block
must supply a result expression; selection requires Boolean, and reduction
preserves the initial accumulator's type. A return, escaping loop transfer or
prefix `try` cannot cross a transformation block. Nested loops and functions
retain their own control boundaries.

The source expression runs once. Array traversal retains that object and reads
its current length and next element for each invocation. Appends and future-slot
replacements are observed; rebinding the source variable does not retarget the
traversal. Selection retains the visited value before evaluating the predicate,
even if the predicate changes its parameter or the source slot. A readonly
source binding does not freeze an object reachable through mutable aliases.
Reduction evaluates its source before its initializer, then traverses the
retained source after initializer effects. Empty reductions return the initial
value. Range traversal streams its bounds and never computes inclusive end + 1.

Predicates require a non-nullable Boolean block result. `any?` stops at the first
true result, `all?` at the first false result, and `none?` at the first true
result. Empty sources produce false, true and true respectively. No later
element or block effect runs after a decisive result. Indexed predicate blocks
remain unsupported, matching the reference language.

`find` returns the first visited value whose predicate is true; `find_index`
returns its nonnegative traversal position. Both return `nil` when no value
matches, and use the same Boolean predicate and short-circuit rules. Search
retains the visited value before the block, including when the block reassigns
its parameter or replaces the source slot. Nullable element types remain
nullable rather than gaining another optional layer. Zero and false are present
results, not absence.

These are the reference language's sequential traversal rules. The integration
uses TypeRB PR #729; it does not introduce another mutation policy. Operations
that shorten/reorder Arrays remain separately tracked receiver-API gaps.

## Ownership

`TransformSyntax` records parser-owned suffix and block boundaries. Suffix
origins distinguish chained transformations on already evaluated receivers.
The checker validates those boundaries and binds a `MirIteration` to the source
and element types in each concrete `CheckedBody`. Analysis resolves map result
types before lowering, sharing the existing closure analysis pass. Generic
bodies and anonymous functions retain their own checked projections.

The result Array, reducer accumulator or predicate Boolean is a loop-carried binding
before the receiver/cursor exit boundary. Existing iteration builders emit
ordinary typed blocks, live Array size/load operations and Range comparisons.
Map appends the yielded value; selection conditionally appends the retained
visited value; reduction replaces the accumulator. Predicates update their
Boolean binding and branch directly to the existing loop exit on a decisive
result. No new backend instruction,
source-specific QBE path or external helper implements transformation semantics.

Search initializes a typed absent result. Only the matching edge creates a
present result through the existing nullable MIR operation and takes the loop
exit. `find_index` reads the internal cursor independently of user bindings;
for a Range, checked subtraction of the retained start gives the ordinal.
The checked transform verifier independently validates each search result type.

REPL execution consumes the checked projection and retains values in its normal
store. Native execution depends only on verified MIR: erasing source tokens,
syntax origins, inferred types and checked transform projections leaves the
output unchanged. Reordered-block execution and forced collection cover managed
selection values, nested Arrays, accumulators and captured block parameters.

Compiler self-use of transformation syntax requires a future compatible
immutable seed. This change keeps compiler implementation syntax within the
current seed and preserves the independent recovery chain.

The Native compiler fixture additionally checks short-circuiting a Range ending
at the portable maximum after two visits. Shared reference comparisons use a
bounded Range: the pinned generated Go implementation eagerly expands Range
transform sources, tracked by [TypeRB #731](https://github.com/type-rb/type-rb/issues/731).
Its REPL and the Native file/REPL paths stream this probe. This is a known
reference code-generation gap, not a reason to expand a Native Range.
