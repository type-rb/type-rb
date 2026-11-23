# Verified callable signatures and indirect calls

Status: authored `fn` checking, building and execution use verified MIR; the REPL
retains checked function values across submissions. Named declarations used as
values and remaining signature/capability boundaries remain gaps. This is not
complete function-value coverage or a public callable ABI.

## Parsed anonymous bodies

The frontend now recognizes `fn` as an expression with its own typed positional
parameters, optional result annotation and body terminator. Nested expressions
and declaration defaults retain immutable `LambdaSyntax` regions; their parameter
rows never enter the surrounding named function's parameter/default catalog.
Reparsing unreachable syntax reuses the original region instead of replacing it.
The restricted header grammar rejects defaults, named-only/rest parameters and
generic lambda parameters, following the reference contract.

Resolved signatures live in the concrete `CheckedBody`, so generic instances
sharing the same source region do not share substituted types. Abstract template
validation visits nested header annotations too. The containing name resolver
skips anonymous bodies and excludes their iteration, catch and pattern bindings;
it cannot accidentally expose nested names outside their lexical boundary.
Body checking and capture selection now use the independent frames below.

The REPL collects a complete anonymous body, including nested `fn` and compact
statement separators, and retains the analyzed body with its selected environment.
Ordinary files materialize these bodies through the analysis and lowering phases
below. The shared inventory records the file and interactive paths separately.

## Anonymous lexical checking

Each anonymous body has an independent checked projection, inherited generic
binding and return/transfer scope. Named and anonymous bodies share name
resolution and expression/statement checking. Resolution's permissive name
inventory is separate from the typed environment advanced in source order, so
later declarations cannot become captures or make forward references valid.

The anonymous frame contains a visible inherited prefix followed by its own
parameters and declarations. The checker records actual uses of inherited
bindings, retaining only their stable parent identities, declared types and
mutable capabilities. Nested uses propagate through intermediate closures.
Parameters and declarations can shadow inherited names; ended iteration slots
cannot donate identities, and hidden receiver/cursor bindings are excluded.
Unreferenced inherited values do not become retained captures.

The frame inherits no SSA values, mutable/field nullable proofs, pending Result
obligations, loop targets or interactive submission state. Immutable binding
proofs are copied with the new lexical identity; captured storage keeps its
declared type. Return, try/catch and must-use
checks belong to that body, including unused nested bodies and every completing
return path. Generic instances sharing source tokens retain distinct resolved
projections. Parsed anonymous parameters stay outside the named declaration
catalog. Analysis materializes separate concrete signatures after selecting captures; nested
expression facts stay in their own body tables. Checking an anonymous body does
not execute it or change the parent's path facts.

## Analysis and materialization

Programs containing `fn` first check their concrete named/default bodies without
emitting instructions. Anonymous analysis discovers nested captures, generic calls
and default initializers through a worklist. Each closure receives a private
function identity with captured parameters first and authored parameters after
that prefix. All declarations are complete before MIR types/signatures or runtime
adapters are emitted. File programs without `fn` keep their existing single pass.
Interactive checking
also analyzes retained shared bindings before lowering, even when no new `fn`
appears. Semantic widening retains its Float type without requiring MIR emission.

`lambda_lowering.trb` owns catalog materialization, inherited frame binding and
environment construction. Each concrete body retains resolved type applications,
anonymous identities and selected shared-binding plans. Lowering rechecks into
fresh operation tables, avoiding duplicate expression facts from analysis. Empty
Hash literals retain their analyzed concrete initializer types before being placed
in shared cells; later inference cannot change the cell's storage type. Diverging
Hash literals have no allocation type and are rechecked without requiring an
initialized type-table entry.

Only actual captures occupy runtime parameters. Sparse child declaration IDs
preserve the analyzed lexical identities even when unreferenced inherited names
have no active slot. Mutable captures adopt the parent's cell; they are never
boxed again. Authored mutable parameters reserve their incoming SSA IDs before
allocating their own cells. Immutable nullable binding proofs remain available in
the child; mutable and field proofs are not inherited.

Unknown direct and indirect calls invalidate nullable facts for mutable captured
bindings. Version changes also invalidate readonly-field proofs and propagate
through conditional joins. Repeated regions discard these proofs before their
first condition/body, so a later iteration cannot reuse a pre-call proof. This
is conservative call-effect handling; unsafe reference acceptance is not copied.

## Ownership and representation

Function annotations use the reference spelling `(A, B) -> R`, including `Void`
results, nested signatures and generic aliases. The frontend canonicalizes them
to a private structural spelling whose leading `$` cannot be an authored name.
Parameters must be value types; only a result slot can be `Void`. Unused callback
bodies receive the same arity, argument and return checks as ordinary functions.

`callable_types.trb` owns this spelling. Each callable MIR type has an independently
verified signature row containing parameter type IDs and its result type ID.
`mir_callables.trb` owns closure construction, indirect calls and validation;
`qbe_callables.trb` adapts only those verified facts. Recursive expression checking
remains in `checked_program.trb` to avoid a checker import cycle.

Instruction 41 binds a captured prefix of an ordinary checked MIR declaration's
parameters. Its argument span contains the captured SSA values; its result type
contains the remaining parameters and the declaration's result. Instruction 42
invokes an available callable SSA value with an exact argument span and result
shape. The verifier rejects invalid catalog ownership, missing bodies, unavailable
captures, wrong capture/argument types or counts, malformed spans and invalid
Void results. Host/runtime adapters cannot be captured through this operation.

The internal value is now a managed closure object, replacing the initial bare
code pointer. After the GC header it holds an adapter address and one word for
each captured value. The verified parameter prefix determines each field's type
and traced status. One adapter per declaration/prefix length loads the captured
arguments and calls the ordinary function with its existing ABI. The remaining
arguments arrive through the callable ABI, preceded by the environment pointer.
Adapters allocate nothing before entering the checked body; the caller roots the
environment and arguments, and the checked body owns its subsequent roots.

Closure construction has allocation/failure effects and roots its captured
managed values. Indirect calls conservatively have allocation, mutation, I/O and
failure effects and root the environment itself. Callable fields/elements are
traced in Array, Hash and record storage, including nested closures and cycles.
Float, Boolean and Integer captures occupy untraced slots. No capture layout or
code association depends on retained frontend declarations or source tokens.

Lexical registration now assigns identities separately from reusable active
slots, through one binding helper for declarations, parameters and synthetic
iteration/catch bindings. Nullable facts carry that identity, so ended or
shadowed bindings cannot donate flow facts to a later occupant of the same slot.
Each checked body owns these identities. Capture analysis records the unique
mutable declaration identities requiring shared storage in each concrete body.

This is not a public callable ABI or completed function-value coverage. Ordinary
files materialize selected captures and checked bodies while preserving readonly
and mutable capabilities. Named declarations used as values remain a separate
implementation boundary.

The REPL pool now separates reusable name-lookup slots from capturable binding
identity. Ordinary bindings retain direct value IDs. On first capture, a binding
moves into one internal pool cell; later captures reuse it, and assignment updates
its child value. Reusing a scope slot creates a fresh binding without changing an
escaped cell. Pool compaction follows these cells through the existing identity
map, preserving sharing, cycles and managed rebinding while dropping unreachable
temporaries. An ordinary Native-built internal fixture covers those properties
and fresh iteration cells across repeated compactions.

### Retained interactive code and types

`repl_callables.trb` constructs environments from the checked capture rows. Mutable
captures adopt pool cells; immutable captures retain values. Invocation binds the
semantic captures separately from authored parameters and evaluates the concrete
checked body. Code is never reconstructed from a bare function ID or by replaying
its original initializer.

Each callable owns its originating checked program. Nominal values, including
nested structural types, also retain the catalog that gives their raw type and
variant IDs meaning. `repl_types.trb` translates visible identities into the active
program by declaration identity and concrete generic arguments, including nested
callable signatures. Payloads are not globally rewritten. Calls save and restore
the active program, body and module, so an older closure can invoke a newer callback
and resume correctly, including after runtime failures.

Pool compaction copies raw identities with their owning programs, preserving
sharing and cycles. It retains the latest display/witness context and only older
contexts reachable from live values. Primitive-only values do not retain code.
The retained unit is currently a complete checked program, which trades memory
for explicit code/type ownership; it is not a compact bytecode serialization.
An independent Native-built fixture repeatedly checks that unreachable programs
are reclaimed and surviving closures keep a distinct owner.

Retained source bindings use typed witnesses that are checked and never evaluated.
A callable witness returns a projection from an empty typed Array, so even a
recursive result requires no eager constructor and cannot replay user effects.
Void bodies omit the return annotation as required by TypeRB syntax. Actual calls
always use the stored authored body. Retained shared binding names feed the same
capture plan before lowering; unknown calls invalidate their nullable proofs across
submissions. Analysis does not duplicate exported submission facts. Explicit
`:reload`/`:load` replay remains the existing separate operation.

## Shared binding storage

`mir_bindings.trb` owns declaration initialization, semantic reads, assignments
and capture operands. A checked body records selected mutable declaration IDs;
its lowering frame loads that plan before binding parameters or entering the
body. Selected bindings use private single-element `Array<T>` cells and existing
verified allocation/load/store operations. Unselected bindings keep direct SSA
values. The semantic type remains `T`; block and loop arguments use the storage
type `Array<T>` and carry the same cell through every edge. Backend adapters and
GC use ordinary verified MIR types without reading the capture plan.

Parameter SSA IDs are reserved before any cell allocation. Declarations initialize
cells where the binding is introduced, including inside each iteration body.
Reused lexical slots clear their old storage type, while capture operands resolve
stable declaration identities. Mutable captures require an already initialized
cell; late capture-site boxing and stale identities are rejected. Multiple closure
environments retain the same cell, and separate factory calls allocate distinct
cells. Managed payload replacement uses existing Array tracing and root plans.

The ordinary pipeline now supplies these plans before lowering and constructs
managed closures from the analyzed bindings. Internal explicit-plan fixtures remain
independent controls for malformed storage and stale identity rejection.

## Validation and completion boundary

Internal MIR fixtures explicitly construct entry calls and closure factories,
while callback bodies and consumers use the ordinary checker. They exercise
escaped captures, independent factory calls, shared mutable Array storage, nested
closures, cycles through Array storage, higher-order parameters/returns, Hash and
record storage, mixed Float/Integer/Boolean captures and Void calls. Tests erase
source bodies and frontend function/parameter types, require identical QBE, and
execute with collection forced before calls and allocations. Malformed catalog
and instruction controls independently reject unverifiable MIR. Removing capture
or environment roots is rejected independently of the runtime execution tests.
Repeated scope-slot reuse also verifies binding identity and nullable facts.

The shared ordinary inventory distinguishes signature checking, file execution
and retained REPL support. It includes higher-order calls, nested shared cells,
independent factories, iteration cells, generic/default closures, nullable and
managed captures. Ordinary check/build/execute observations update Capabilities;
retained REPL observations additionally cover initializer-once behavior and cyclic
containers. CLI controls cover cross-program callbacks, nominal ID changes,
recursive result witnesses, failures and replay. Additional compiler tests erase
frontend facts and force collection, and reject stale proofs after calls and at
loop entry. Compiler implementation does not self-use the new function syntax.

The canonical compiler closure contains 98 modules. Integration requires
unchanged-seed ordinary core/CLI fixed points, exact recovery-source validation,
snapshot-v4 compatibility, complete hosted recovery, target and lifetime checks.
Compiler implementation does not use the new function syntax. No seed, reference
pin, runtime ABI promise, benchmark baseline or performance qualification changes.
