# String queries and verified search operations

Status: implemented subset under the basic-language and MIR consolidation milestones.

Ordinary programs and the REPL support String `empty?`, `include?`, `start_with?`,
`end_with?`, `index` and `rindex`. Arguments execute once after the receiver;
safe navigation skips them for an absent receiver. Search returns `Integer?`,
and optional receiver search flattens to that same optional result. Indexes count
Unicode code points, including combining marks and the components of a grapheme
cluster. Matching is literal, without normalization or locale rules.

`string_methods.trb` (originally `string_queries.trb`) owns method classification
and typed construction. The
primitive argument checker is shared with Integer and Float receivers. Empty
testing uses existing String size and comparison. Internal instruction 44 takes
two available Strings and returns an Integer search outcome with no failure edge.
Its verifier checks both operand types and availability, the Integer result and
the exact mode: first/last decoded search, prefix/suffix or contains predicate.
First/last results are code-point offsets or -1. Predicate modes return zero for
a match and -1 otherwise. MIR turns these into Booleans or explicit none/present
values, so optional boxing and its allocation effects remain visible to root
planning. A search itself neither allocates nor mutates either String.

The QBE adapter invokes the selected operation mechanically. The separate
`qbe_string_queries.trb` runtime owner compares bounded spans without allocating
temporary substrings. Valid UTF-8 indexed search visits code-point boundaries;
last-match search retains overlapping occurrences. Prefix and suffix check one
bounded span. Empty substrings match at zero for first search and at String size
for last search. Oversized patterns fail without reading outside the input.

External byte input needs a distinction present in the exact reference:
`index`/`rindex` compare decoded code points, including replacement characters for
invalid UTF-8, while the three literal predicates compare exact bytes. Indexed
search validates the input before selecting the byte fast path; malformed input
uses a bounded scalar-decoding path. Predicate search can therefore find a raw
byte fragment without inventing a code-point index for it. Embedded NUL uses
stored lengths and is not a terminator for these operations.

Shared ordinary check/build/run/REPL probes cover Unicode offsets, overlaps,
empty and longer patterns, optional results, captured receivers, argument order,
aliases, lexical loop transfers and rejected signatures. Source-erased,
reordered and forced-GC MIR controls use ASCII literal counterparts because
Go-hosted canonical recovery emission retains its documented String-byte
boundary. Ordinary Native UTF-8 controls retain the original Unicode fixtures,
force collection while evaluating allocating arguments and cover NUL and raw
external bytes. Malformed instruction controls reject forged search operands,
results, modes and failure edges.

The extracted runtime observer also counts allocations and puts receiver and
pattern spans immediately before inaccessible pages. These controls check empty,
oversized, Unicode and malformed inputs without relying on a trailing terminator.

The ordinary core closure contains 104 modules. Recovery import boundaries and
module mutations include both new owners. The implementation uses the existing
immutable seed syntax; compiler self-use of newly accepted receiver methods
still requires an accepted supporting seed. Remaining String operations,
diagnostic parity and performance qualification stay open.
