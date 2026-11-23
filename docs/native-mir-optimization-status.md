# Native MIR ownership status

The active [consolidation milestone](mir-consolidation.md) prioritizes complete
verified MIR ownership, removal of the ordinary direct path and cohesive source
organization. Correctness and reproducibility remain required. Detailed
performance qualification follows the architecture milestone; daily and weekly
measurements provide intermediate feedback.

## Implemented ownership

| Area | Current boundary |
| --- | --- |
| Typed scalar values | Integer, Boolean and Float literals, unary/binary operations and numeric conversion have explicit operands, result types, origins and failure edges. Verified numeric plans select bounded operation/call expansion from CFG/SSA facts; straight-line leaves retain verified Integer range guards. |
| Scalar control | Mutable scalar locals and parameters, nested `if`/`elsif`/`else`, `while`, `break`/`next`, early returns and continuing joins publish typed blocks and live-value arguments. Loop transfers carry only the enclosing environment; branch/body locals remain lexical. QBE consumes admitted bodies without rereading their source. |
| Short-circuit expressions | Scalar `&&` and `||` publish conditional RHS blocks and Boolean result joins, including nested call arguments and loop predicates. Dominance preserves earlier expression temporaries; skipped RHS calls and traps stay unexecuted. |
| Calls and declarations | Declaration identities, parameter types/mutability and return types are captured before body checking. Ordinary calls with supported scalar and managed arguments/results, including nominal records and Hash/Range carriers, retain explicit arguments. Module-local direct-call summaries propagate local effects over reverse call edges to a fixed point, including recursion. Calls proved to contain only reads, scalar work and control flow need no GC safe point, and stable Array loop proofs can cross them. The standard `Math.sqrt` adapter is pure. Allocating, mutating, I/O, indirect and unknown calls retain conservative effects throughout their callers. The verifier independently rebuilds summaries from current bodies without trusting stored call effects. |
| Function values and lexical capture | Authored `fn` bodies, supported named function values, structural callback signatures, selected captured environments and indirect calls use verified MIR. Module-owned singleton code identities propagate through SSA block arguments and ordinary direct-call parameters, including forwarding cycles. The verifier independently rebuilds call plans before the backend can invoke the ordinary body with the unchanged captured environment. Mixed identities and unsupported producers remain indirect; allocation, effects and roots are unchanged. Mutable captures share cells; the REPL retains checked code, environments and nominal type identities across submissions. |
| Namespaces and constants | Source-owned lexical declarations resolve before lowering. Exact global types, initializer functions, order, guarded reads and persistent managed roots live in MIR; the adapter consumes the verified catalog. Nullable global facts retain declaration identity across lexical scopes. See [decision 0067](decisions/0067-namespaces-and-constant-mir.md). |
| Managed Strings and roots | String literals, concatenation, equality, size, indexing, Integer/String/Float conversions and String/Boolean output use typed operations. Integer output explicitly converts first. Managed parameters, rebinding, returns and control joins use the same value/block path. MIR derives live-before roots at allocating operations and ordinary calls; verification recomputes the complete plan. |
| Literal String splits | Verified per-function literal identities propagate through SSA block arguments, including seeded loops. Bounded constant splitting retains the canonical allocating operation and fresh mutable Array; only immutable fragment bytes are shared. Dynamic, conflicting, empty-delimiter and over-budget cases keep the runtime path. |
| Dynamic String splitting | The runtime reuses fragment byte lengths as point counts when the source header proves every decoding unit has width one, including malformed bytes. Other sources use bounded decoding. The full-result path can derive the larger fragment’s point count when there is exactly one ASCII delimiter. If MIR proves that a fresh Array is observed only through its size and constant nonnegative reads, the runtime copies just the inclusive range between the smallest and largest index read. It still scans all fields and preserves the full Array length, bounds checks and ownership of visible strings. |
| String concatenation chains | Same-block, single-use concatenations separated only by literal materialization have verified ordered fragment and intermediate-elision plans. Root liveness retains every original fragment through the final allocation. The adapter copies each byte once without changing observable String values or crossing calls, allocation, mutation, I/O, failure or CFG edges. |
| Managed Arrays | Supported scalar and managed element Arrays, including nominal records and their nesting, retain semantic element identity. Literals, live size, checked indexing, assignment, compound assignment and push use typed operations with allocation/mutation/failure effects. Admitted parameters, returns, rebinding and loop/branch values share managed liveness. Assignment captures its checked logical position before RHS evaluation, then reloads storage for the final store. A verified within-block plan reuses that position for the same Array until an effect can change collection state. |
| Single-use String Array joins | Independently verified same-block plans elide a bounded intermediate container, preserve canonical operations and directly root its String fragments. The adapter reuses one prologue pointer table and the shared byte-joining runtime. Unsupported uses and effects retain ordinary materialization. |
| Nominal records | Declaration identity, authored names/origins and ordered field types live in MIR. Construction consumes an ordered operand span with allocating/failing effects; projections verify nominal owner and result type. Recursive records through containers retain their shells and field references. GC descriptors consume this verified table. |
| Numeric Array induction | Selected zero-based traversal and Integer/Float reductions retain verified induction, address and accumulator relationships. The adapter consumes the established plan. |
| Array effects and headers | CFG dominance, induction identities and conservative operation effects select stable headers. The retired token regions and direct-adapter header caches are removed; checks remain unless the typed plan proves them redundant. Checked accesses use one comparison for the common nonnegative index, normalize negative indexes against the current length, and take a terminal failure path on invalid indexes. |
| Hash | Typed operations cover construction, lookup, update, deletion, copies and collection projections, including empty-Hash inference and managed carriers. Source-bound plans remain for checking and the REPL; QBE consumes only value/block MIR. |
| Range and iteration | Construction, captured bounds/receiver, live Array length, lexical transfers and Array/Range carriers use the general value/block route. Source-bound shapes are shared by checking and the REPL. |
| Assignment and Boolean operations | Typed operations own captured assignment targets, evaluation order and binary/logical result semantics. The retained `checked_operator_plans` projection serves the REPL, not QBE. |

Function adapter kinds distinguish scalar leaves and general scalar/managed
control; independent verifier fixtures retain their separate model kind. The
ordinary checker requires a MIR body for every accepted function. Verification checks their operation and shape
contracts as well as table bounds, value definitions, types, argument availability,
control targets, declaration/call/return types, conservative call effects and source origins. Block arguments are parallel assignments;
QBE adaptation captures edge values before overwriting destinations. Instruction
results retain stable MIR operand names even when block storage is reordered.

`compiler/src/mir/analysis/flow.trb` derives reachability, reverse postorder, immediate dominators and
value definition sites and function-local value/type and block indexes after structural/identity validation.
Its predecessor and incoming-argument indexes retain both arms when a branch
targets the same block. Array-loop alias propagation and natural-loop traversal
reuse these derived edges instead of rescanning every block for every parameter.
The verifier rebuilds this information from current MIR; it is not a persistent
cache or an input optimization fact. Type indexes are rebuilt from current rows,
so later raw-MIR edits cannot reuse stale types. Function parameters
are available everywhere; reachable blocks may use dominating definitions, while
unreachable blocks may only use function parameters and their own earlier
values. Same-block instruction uses must follow their definitions. Compact IDs use bounded arrays; sparse or malformed identities use Hash fallbacks.
Block/edge arrays avoid a quadratic dominance matrix. Instruction failure edges must end in
empty traps; they cannot carry a value into an ordinary continuation. This is
verifier analysis, not caller-supplied optimization metadata.

Independent verifier tests reject malformed values, effects, roots, control
edges and source-dependent plans. Differential fixtures exercise ordinary
execution and error behavior across generations. Erased frontend state,
reordered MIR storage and forced collection check that verified MIR retains the
required semantics and managed lifetimes. These controls establish correctness,
not a performance improvement.

## Responsibility boundaries

- `compiler/src/mir/model/module.trb`: typed module, function, block, instruction and value records, with
  shared scalar encodings and identity/range queries, plus portable declarations
  and conservative call records.
- `compiler/src/mir/analysis/ranges.trb`, `compiler/src/mir/analysis/numeric.trb`, `compiler/src/mir/analysis/flow.trb`, `compiler/src/mir/passes/optimize.trb`: portable numeric proofs, expansion selection, CFG/dominance
  and rewrites. `compiler/src/mir/verify/module.trb` checks structure/edges; `compiler/src/mir/verify/instructions.trb`
  checks typed instruction contracts.
- Constant Integer remainders retain their literal divisor, exact reciprocal and shift
  in the verified numeric selection. Existing expansion budgets select positive
  non-power divisors below `2^32`; QBE consumes those facts with an unsigned
  numerator guard. Numerators below twice the divisor use quotient-zero/one
  paths; other nonnegative 31-bit numerators use an exact wide product and shift
  without correction. Remaining numerators keep signed remainder. Zero, negative, dynamic,
  oversized and power-of-two divisors retain the ordinary selection. Verification
  rederives the complete selection rather than trusting stored reciprocal facts.
- `compiler/src/frontend/checking/body/values.trb`, `compiler/src/mir/build/blocks.trb`, `compiler/src/mir/build/functions.trb`, `compiler/src/mir/build/control.trb`:
  checked value projection, block construction and function publication.
- `compiler/src/mir/analysis/identities.trb`: fresh sparse function/value identities and definition counts, avoiding whole-module pairwise comparisons.
- `compiler/src/mir/model/types.trb`: canonical composite type identities and shared managed/element
  classification, with unique container identities and predeclared nominal shells for recursive record fields.
- `compiler/src/mir/lowering/arrays.trb`: typed Array construction, selection, load/store and push
  contracts; no backend address is retained across a right-hand side.
- `compiler/src/mir/analysis/loop_calls.trb`: independently verified lazy scalar
  call reuse within a natural loop. Every argument keeps the same SSA identity
  on all backedges; the whole loop and direct callee must exclude allocation,
  mutation, I/O and unknown effects. Cache initialization belongs to the single
  external preheader. The first call stays at its original position, preserving
  skipped paths, zero-trip behavior and failure order. Managed return values and
  indirect or external calls remain excluded. The cache stores only a successful
  Integer, Boolean or Float result and is reset on loop reentry.
- `compiler/src/mir/analysis/array_loops.trb`: verified natural-loop plans, composed
  from separate header, bounds, append and value analyses. Stable storage alone
  permits header reuse; it does not prove an index safe. A dominating successful
  length comparison can establish bounds for another Array. Its true edge must
  be exclusive, and the bound and owner must remain unchanged on all backedges.
  Checked endpoint arithmetic also supports reflected indexes.
- `compiler/src/mir/passes/array_loop_versions.trb`: bounded fast/fallback loop
  duplication with explicit length and, where useful, storage-identity guards.
  The original checked path handles short or aliased Arrays. Cloned values,
  calls, origins and edge arguments remain canonical MIR. Expansion is limited
  per function and disabled for large modules to bound compiler cost.
- `compiler/src/mir/analysis/array_values.trb`: primitive load reuse and store
  forwarding within one block. SSA identity alone never proves disjoint storage;
  intervening writes need an independently verified identity guard. Normalized
  assignment indexes retain the original length-stability proof.
- `compiler/src/mir/analysis/array_promotions.trb`: lazy primitive-element
  promotion across a two-block loop with an exclusive exit. A moving index starts
  at checked `i + 1` and cannot alias the unchanged nonnegative index `i`;
  foreign owners require disjointness guards. The region excludes observers,
  allocation, resize and possible failures. Values load only after first body
  entry and flush before exit work, preserving zero iterations and Float order.
- `compiler/src/mir/analysis/array_peeling.trb`: a small reflected-index loop may
  duplicate its first checked header/body visit. The verified schedule retains
  every condition and failure edge; the QBE adapter only routes labels and
  ordinary parallel edge copies. The original canonical instructions remain
  available to independently reconstruct bounds, numeric and value plans.
- `compiler/src/mir/lowering/records.trb`, `compiler/src/backend/qbe/emit/records.trb`: nominal construction/projection contracts and their ABI adaptation. Readonly field bindings remain distinct from SSA IDs and mutable projected values.
- `compiler/src/backend/qbe/emit/arrays.trb`: ABI adaptation of verified Array operations.
- `compiler/src/mir/lowering/calls.trb`: declaration capture, checked calls and their verification.
- `compiler/src/mir/analysis/call_effects.trb`: ephemeral direct-call graph and monotone effect summaries, rebuilt independently for verification. Each function is scanned once and each summary increases at most twice.
- `compiler/src/mir/analysis/string_splits.trb`: bounded String literal-flow and split plans, independently rederived before static fragment adaptation.
- `compiler/src/mir/analysis/split_fields.trb` follows Array aliases and rejects field selection when a dynamic read, write, escape or ambiguous merge can expose another element.
- `compiler/src/mir/analysis/array_joins.trb`: fresh single-use join fragments and
  adjacent size consumers can avoid intermediate Arrays or Strings. Literal
  separator boundaries must prove that UTF-8 code-point counts compose;
  otherwise the ordinary runtime path checks the joined bytes. Observable
  intermediate values, managed roots and evaluation order remain unchanged.
- `compiler/src/mir/lowering/strings.trb`: String, conversion and output construction/contracts.
- `compiler/src/mir/analysis/literal_data.trb`: portable bytes and code-point counts
  for ordinary String literals and String singletons, indexed by MIR payload.
  Input MIR cannot supply these plans; the optimized verifier independently
  reconstructs and compares them after instruction verification. Non-String pool
  entries retain no materialized String data. Static packing/compression consumes
  the verified facts; remaining declaration metadata uses the same portable
  constructor. Reference-host byte limitations retain the existing emit-phase
  diagnostic. Runtime declaration identity follows the bound portable-data
  constructor to its byte accessor, leaving sibling declarations ordinary. This
  ownership change does not claim runtime acceleration.
- `compiler/src/mir/analysis/string_concats.trb`: function-local operand occurrence
  counts include call/constructor/capture arguments, mutation operands and both
  CFG edges and returns. Exact single-use concat results may join a same-block
  chain only across literal instructions. Derived plans retain ordered original
  fragments and elided intermediate indexes; input MIR cannot supply them, and
  optimized verification re-derives them from current instructions before roots
  or adaptation. Original SSA, type and origin records remain intact. The
  runtime handles empty fragments and UTF-8 sequences spanning joins without
  mutating an observable String. Three-fragment chains pass arguments directly to a static constructor
  whose emitter is shared with binary concatenation. Longer chains use a
  function-entry fragment table sized to the largest verified chain and reused
  across loop iterations; recursive calls own separate frames. The optional runtime helper is emitted only when required.
  This ownership does not establish a measured
  performance qualification.
- `compiler/src/mir/analysis/roots.trb`: operation effects, backward managed-value liveness and exact
  safe-point root plans. Block parameters transfer only demanded values. Instruction
  uses and definitions are summarized once per block; a bounded predecessor worklist solves live-in sets
  before the exact safe-point walk. These summaries are rebuilt from verified MIR
  and are never input optimization facts.
  Verified String chains use their original fragments at the surviving safe
  point and omit the elided intermediate safe points.
- `compiler/src/backend/qbe/emit/strings.trb`, `compiler/src/backend/qbe/emit/roots.trb`: runtime adaptation of those operations and
  publication of the verified root lists. The adapter reuses unchanged leading
  slots within one MIR block and resets the exact root count at every safe point;
  it does not perform a separate backend lifetime analysis.
- `compiler/src/mir/build/logical.trb`: conditional RHS and expression-result join construction.
- `compiler/src/frontend/checking/body/types.trb`: assignability, operator result types and diagnostics.
- `compiler/src/frontend/resolution/argument_binding.trb`: shared required positional/named slots and record labels; authored evaluation precedes operand reordering.
- `frontend/checking/body/expressions.trb` and its call, collection, member,
  statement, control and iteration owners: mutually recursive source checking
  that invokes the shared MIR construction owners.
- `compiler/src/backend/qbe/context.trb`, `compiler/src/backend/qbe/emit/functions.trb`, `compiler/src/backend/qbe/emit/numeric.trb`, `compiler/src/backend/qbe/emit/constants.trb`:
  target operands/labels, function ABI and MIR-selected root-frame prologues,
  selected numeric spelling and static data. No lexical local/header/root analysis.
- `compiler/src/backend/qbe/emit/mir.trb`, `compiler/src/backend/qbe/emit/control.trb`: existing optimized MIR, explicit calls and scalar-block
  adaptation, sharing one scalar operation adapter.

The [architecture map](architecture.md) records the complete ordinary closure.
Recovery uses an exact module/import inventory and the same source bodies; no
extracted implementation is copied or kept behind an old-name wrapper.

## Nominal record validation

`compiler/src/mir/lowering/records_test.trb` rejects forged nominal identities, field owners/types and
origins, wrong constructor arguments and missing initializer roots. Same-shaped
records remain distinct. Grouping must retain readonly field bindings while
projected mutable Array values remain usable. The `managed-record-mir` fixture
covers recursive and cyclic record Arrays, ordered initializer calls, Float
conversion, branches, retained aliases and allocation pressure. Its test erases
admitted source bodies and the old checked field metadata, requires identical
QBE, reorders blocks, then runs with collection before allocating operations.

Constructors evaluate authored fields once in source order, store their operands
by declaration slot and publish a
fully initialized object from explicit MIR operands. Managed initializers remain
live across later initializers and the allocation itself; no partially initialized
record escapes the operation. The backend's function-local operand types and the
verifier's Hash index avoid repeating whole-module type searches as admission
grows. These are derived lookup structures, not unverified optimization facts.

## Sole MIR emission and numeric selection

The checker rejects an accepted function without a MIR body. QBE uses the verified
signature for parameter/result ABI and the MIR root plan for frame capacity.
The direct expression/body emitter, token Array regions and assignment-effect
analyses, backend numeric budgets, lexical local slots, header caches and textual
root-publication filtering are removed. `compiler/src/backend/qbe/emit/functions.trb` owns function emission;
`compiler/src/compiler.trb` retains orchestration and declaration-bound runtime hooks. Output
storage grows with emitted lines instead of imposing a source-token size guess.

`compiler/src/mir/analysis/numeric.trb` derives natural-loop membership through dominance and selects
expansion in function/block identity order. Modules with at most 32 bodies have
64 loop operations, six outside operations and two leaf-call expansions; modules
with at most 96 bodies have 32 loop and three outside operations. Larger modules
use shared numeric helpers. This retains a bounded policy while removing the
old backend's lexical-loop and deferred-function budget state. Every ordinary
function definition is emitted; call expansion no longer decides definition
omission implicitly. A future use-graph pass can select dead bodies explicitly.

Small-operand facts mean Integer values in `[0, 1024]`, not arbitrary nonnegative
values. This bound permits a checked multiply without overflowing the machine
intermediate. Selected Integer additions and subtractions emit the portable
range guard directly with a shared failure block; proven nonnegative additions
need only the upper check. Other operations retain shared helpers, explicit
zero checks or guarded fallback.
Inline leaf copies can use verified caller argument bounds. Their emitted literal
operands do not silently become new caller proofs. Optimized-module verification
recomputes the entire plan, including owners, sites, operations and modes, only
after validating MIR structure, types and operands. Input MIR cannot supply plans.

## Remaining work

The ordinary emission route is unified; this does not establish full TypeRB
language, standard-library or source-backend coverage. Those gaps remain in the
[language coverage contract](native-language-coverage.md) and Capabilities.
General constant/range propagation, more precise effect classes, broader inlining and
explicit dead-body elimination still need shared MIR passes. Indirect and opaque calls
remain conservative allocation/mutation barriers; managed liveness is
intraprocedural. Root publication rewrites changed slots from the verified
live-before plan, republishes at block entries, reloads root-buffer addresses
across calls and preserves the exact count and managed-return ABI.

The recursive checker and compiler driver still need responsibility-based
splitting. Preserve source origins, recovery and ordinary fixed points during
that work. Full milestone acceptance also requires measurement and improvement
against the fixed Native/Pure Go baselines; structural completion alone does not
establish runtime, build-time or binary-size parity.

## Measurements and history

The accepted [numeric regression recovery](https://github.com/type-rb/type-rb-native/blob/04c6ca7263c066fd13e83b3faa09c4e80d707c13/docs/native-numeric-regression-recovery.md)
and [formal runtime measurements](../results/2026-09-10-benchmarksgame-runtime-loop-local-headers-linux-arm64/README.md)
retain their exact source identities and observations. They do not qualify later
MIR migration candidates. No new Pure Go comparison or formal Pages performance
claim follows from this structural change.

Earlier extraction narratives, rejected variants and measurement links remain in
[the immutable prior status](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/native-mir-optimization-status.md).
Original evidence and the fixed migration/cumulative baselines are unchanged.

## Function values and lexical capture

Structural callback signatures, closure construction and indirect calls share
verified MIR type, effect and root ownership. Ordinary `fn` bodies analyze selected
captures and lower through the same checked body and MIR pipeline as named
functions. Mutable captures share cells, and unknown calls invalidate their
nullable proofs. The backend consumes verified capture layouts and exact
parameter/result types, including Float and Void. The REPL retains checked code,
environments and originating nominal types across submissions. See
[decision 0051](decisions/0051-callable-mir-foundation.md).

Singleton callable plans use a finite worklist over verified value identities,
block arguments and ordinary direct-call parameters. Closure and object adapter
entry parameters remain unknown. Container/global/return producers and mixed
identities retain indirect dispatch. Unseeded graph components cannot establish
proofs. Plans bind an instruction to a declaration and captured parameter prefix;
verification reconstructs them from the current module without consulting stored
plans. QBE loads that existing environment prefix and invokes the ordinary body,
without cloning bodies, changing allocation or narrowing call effects. Internal
body storage order and erased frontend catalogs do not determine identity.

Shared ordinary-file and REPL probes cover higher-order calls, nested closures,
managed captures and mutable cells. Internal fixtures add source erasure,
reordered MIR storage, forced collection and malformed type/operand controls.
Named declarations used as values and the remaining signature/capability
boundaries stay open in the [language inventory](native-language-feature-inventory.md).
This does not establish complete function-value coverage or a public callable ABI.
