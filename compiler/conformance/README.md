# Native Compiler Conformance Corpus

This corpus fixes the behavioral boundary of the self-hosted compiler. Every
source is passed to the compiler at runtime. The harness runs each applicable
valid source through `check` and two independent `emit-qbe` invocations at
every required generation, requires byte-identical QBE, links that QBE, and
compares the program output with the checked-in `.out` file. The original
cases remain shared by recovery B0/B1 and every later generation. Feature
fixtures added after the retained snapshot begin at their first updated Native
B2 and do not retroactively widen recovery.

The valid cases cover:

- ordinary Array `each` and `each.with_index` using retained live receivers,
  growth/reallocation, future-element replacement, source rebinding, block
  shadowing, scalar/managed elements and nested header placements in
  `valid/array-iteration-live.trb`; nested transfers and returns in
  `valid/array-iteration-control.trb`; and roots of replaced managed elements
  across automatic collection in `valid/array-iteration-managed.trb`. These
  begin with the updated ordinary B2 and are also exercised through the REPL;

- stable Array assignment positions across RHS growth and index-side effects
  in `valid/array-assignment-targets.trb`, retained nested owners and compound
  String values across collection in `valid/array-assignment-managed.trb`,
  and snapshot v4 parity in `valid/array-assignment-recovery.trb`. The paired
  runtime-invalid fixture checks that an initially invalid target skips RHS
  effects. These cases begin with the updated ordinary B2;

- statement `elsif` chains, ordered conditions, skipped traps, optional else,
  branch-local bindings and nested returns in `valid/elsif-control.trb`;
  managed-root retention and Array mutation across conditional edges in
  `valid/elsif-managed.trb`, beginning with the updated ordinary B2. Paired
  invalid and runtime-invalid cases preserve diagnostics and required failures;

- paired short-circuit Boolean operators, precedence, unary operands,
  nested exactly-once effects and skipped traps in `valid/logical-short-circuit.trb`,
  with managed RHS allocation in `valid/logical-managed-rhs.trb` and required
  RHS traps and invalid non-Boolean operands in the paired failure fixtures;
- typed direct calls, mutable locals, conditionals, loops, and checked Integer
  arithmetic in `valid/control-flow.trb`;
- contextual `next` identifiers in mutable locals, parameters and record fields,
  plus nearby ordinary names, in `valid/next-identifiers.trb`. The paired
  `invalid/next-binding.source` and `invalid/next-typed-binding.source` reject
  statement-leading inferred and typed declarations before code generation;
- canonical Boolean literals, bindings, record fields, arguments and returns,
  direct and selected scalar-MIR negation, Integer/Float/String comparisons
  including NaN and signed zero, and exactly-once condition effects in
  `valid/boolean-conditions.trb`;
- named record Array snapshot recovery of scalar-only and managed records,
  three nested depths, function returns, retained RHS-growth positions and
  allocation loops in `valid/record-array-recovery.trb`;
- Boolean Array snapshot recovery of three nested depths, record/function
  carriers, scalar values and RHS reallocation in
  `valid/boolean-array-recovery.trb`;
- nominal record construction and projection plus homogeneous Integer Array
  construction, indexing, growth, and mutation in `valid/records-arrays.trb`;
- managed String concatenation and content inequality in
  `valid/strings-calls.trb`;
- compile-time literal-only String concatenation, dynamic controls, decoded
  escapes, chains, and postfix binding in `valid/literal-string-concat.trb`;
- deterministic dependency-free compression, initialization, indexing,
  escapes, terminating zero, and equality
  for multiple long static Strings in `valid/static-strings.trb`;
- finite binary64 literals, signed zero, subnormal and underflow behavior,
  infinity, NaN, mixed Integer widening, arithmetic, comparisons, calls,
  mutable locals, and record fields in `valid/float-scalars.trb`;
- contextual and inferred Float Arrays, Integer element widening, parameters,
  results, records, aliases, growth, positive and negative indexing, mutation,
  compound arithmetic, and nested Float Arrays in
  `valid/float-arrays.trb`, starting at its updated Native B2;
- shared outer Integer Array identity across aliases and mutable parameters,
  including growth, element mutation, and parameter-local rebinding in
  `valid/array-aliases.trb`;
- mutable record/Array binding aliases, fresh and returned values, scalar and
  String copies, and shallow nested field/Array reads in
  `valid/mutable-reference-bindings.trb`. The four `immutable-*-local` and
  `immutable-*-parameter` invalid cases reject direct readonly capability
  upgrades, including grouped and explicitly typed initializers. Mutable
  record bindings are reassigned to fresh records, not used to write record
  fields. Record-field assignment is a separate known reference discrepancy
  tracked in [TypeRB #647](https://github.com/type-rb/type-rb/issues/647);
- managed record-field and Array-element aliases retained through owner
  reassignment, lexical-block exit, loop-root compaction, and forced
  collection in `valid/managed-alias-roots.trb`;
- safe-point-free zero-iteration, nested, and early-return loops followed by
  collection pressure in `valid/safe-point-free-loop-roots.trb`;
- managed Array element order, positive and negative indexing, mutation,
  nesting, and forced collection across geometric growth boundaries in
  `valid/managed-array-growth.trb`, with scalar Array growth as a control;
- Integer Array order, aliases, nested retention, mutation, append, negative
  indexing, and forced collection across scalar geometric growth
  boundaries in `valid/scalar-array-growth.trb`;
- repeated Array-header reads in an owned-managed loop, with exact invalidation
  after an opaque mutating call and local Array rebinding in
  `valid/array-header-cache-invalidation.trb`;
- verified stable Array-header reuse in nested loops, including empty input,
  ordinary positive and negative indexes, growth, parameter rebinding,
  allocation, ordinary calls, element mutation through a possible alias, and
  outer-loop cache invalidation before inner-loop growth in
  `valid/stable-array-header-mir.trb`;
- zero-based and bounded-derived unit-step induction loops whose nested Array
  reads retain the unsigned upper-bounds check, followed by a reset and
  ordinary negative indexing in `valid/nonnegative-loop-index.trb`;
- verified two-phi `Array<Integer>` reduction, including the empty identity,
  in `valid/integer-array-reduction.trb`;
- left-to-right, exactly-once argument evaluation through a bounded scalar
  leaf call in `valid/scalar-leaf-inline.trb`;
- checked Integer multiplication at the bounded nonnegative fast-entry
  threshold, its safe slow path, negative operands, zero, and the portable
  extrema in `valid/integer-multiply-fast-path.trb`;
- one-sided checked Integer addition for bounded nonnegative literals on
  either side, including the classifier boundary, its dynamic fallback, zero,
  and the portable extrema in `valid/integer-literal-add-one-sided.trb`; and
- the compiler source closure itself, exercised by the bootstrap test outside
  this directory.

Each invalid `.source` file has an exact `.diag` result. The non-`.trb`
extension keeps intentionally malformed programs out of repository-wide source
formatting. Together they require the
lexer, parser, resolver, and checker to remain active and cover unsupported
characters, unfinished syntax, duplicate declarations, unresolved calls,
arity errors, type mismatches, unsupported types, malformed and overflowing
Float literals, narrowing, remainder, and unavailable methods. `emit-qbe` must
return the same diagnostic without emitting an executable function, proving
that the failure occurs before code generation. The Integer and Float boundary
cases keep the self-hosted compiler aligned with TypeRB's portable ranges
rather than target or QBE fallback behavior.

The candidate-only Float Array diagnostics additionally preserve incompatible
element rejection, mutable Array invariance, immutable mutation rejection, and
unsupported-method rejection. Each source in `runtime-invalid` has an exact
`.stderr` sidecar. The directory covers positive and too-negative Array bounds
failures, checked Integer overflow through an inlined scalar leaf, and the
bounded nonnegative multiplication entry at its exact power-of-two threshold.
Negative, zero, maximum, safe slow-path, and overflow behavior are fixed by
`valid/integer-multiply-fast-path.trb` and
`runtime-invalid/integer-multiply-fast-path-overflow.trb`; status, empty
stdout, and panic text must agree across the updated B2/B3/B4 compilers.
The bounded literal-addition entry is likewise paired with
`runtime-invalid/integer-literal-add-one-sided-overflow.trb` so the removed
lower check cannot weaken the retained upper-bound failure.
The verified reduction accumulator is paired with
`runtime-invalid/integer-array-reduction-overflow.trb` so its phi lowering
cannot weaken checked Integer addition.
The negative addition and subtraction overflow cases preserve the lower end
of the portable range when the two bound checks share one emitted comparison.
The stable-header regressions retain the bounds panic when an index changes
before its access, the loop bound extends past the Array length, or a later
decrement invalidates the next iteration's nonnegative-index premise. These
fixtures deliberately use helpers outside the complete reduction-MIR subset.

The `mutations` directory contains a base program and two independently changed
sources. All three must produce distinct QBE and distinct runtime output. This
guards against an embedded artifact, a source-insensitive generator, or a
compiler path that bypasses its runtime input.

The `file-root` directory is the representative file-root import closure used by
the permanent bootstrap differential tests. `file-root-flattened.trb` preserves
the same workload and observable output in one source file so the benchmark can
isolate file-root module overhead from ordinary Native code generation.

This is an experimental subset corpus, not the TypeRB language conformance
suite. Full language and package compatibility remains outside the implemented Native subset.

`elsif-recovery` restricts its values to the v4 snapshot subset and verifies
ordered conditions and managed String joins through snapshot recovery and
ordinary current compiler generations.

`boolean-array-values`, `boolean-array-effects` and `boolean-array-managed`
exercise Boolean element typing, required mutability, negative indexes, shared
and nested aliases, typed empty arrays, left-to-right effects, growth and nested
arrays across automatic collection. Their invalid and runtime-invalid controls
retain element/index type rejection, readonly capabilities, the existing
recursive element-type validation and required out-of-bounds failures. This ordinary
coverage does not establish snapshot support or numeric-reduction MIR support.

`valid/local-array-header-mir.trb` covers immutable local and record-field
bindings, nested loops, empty Arrays, element aliasing, opaque growth and lexical
slot reuse. `runtime-invalid/local-array-header-bounds.trb` retains negative-index
failure after a local header is selected. Both also run through ordinary CLI
compilation and the independent REPL evaluator.

`valid/stable-array-bindings.trb` covers two mutable parameter/local bindings,
element writes through identical aliases, parameter/local rebinding, growth and
empty inputs. `runtime-invalid/stable-array-bindings-bounds.trb` uses unequal
lengths to require the indexed Array's own check. Both also run through ordinary
CLI compilation and the independent REPL evaluator. MIR tests reject missing,
duplicate, reordered and foreign entries in the complete stable-header list.

`valid/string-index-lifetime.trb` retains indexed String values in Arrays and
Hash keys while heap-allocated receivers are replaced and automatic collection
occurs. It checks both index signs and byte contents. The separate runtime ABI
observer exercises all 256 byte payloads, NUL termination and required bounds
failures; that internal byte coverage does not claim ordinary Unicode support.

`valid/default-arguments-mir.trb` covers declaration-order defaults after explicit
arguments, preceding parameter/field bindings, fresh managed defaults, contextual
Float conversion and skipped short-circuit traps. The ordinary CLI and REPL run
the same fixture. MIR tests erase source/default binding metadata, reorder blocks
and force collection before allocating operations and user/initializer calls.
