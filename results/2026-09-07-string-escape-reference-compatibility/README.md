# Current reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`0504b5a735328f0c665c70f3cad2876baf3839f6`. It incorporates
[PR #826](https://github.com/type-rb/type-rb/pull/826) for open scalar `case`
fallthrough across backends,
[PR #827](https://github.com/type-rb/type-rb/pull/827) for Go predeclared-name
collisions in lexical bindings,
[PR #828](https://github.com/type-rb/type-rb/pull/828) for REPL coverage of raw
enum Result retention,
[PR #829](https://github.com/type-rb/type-rb/pull/829), which rejects
unsupported Range element annotations during checking, and
[PR #830](https://github.com/type-rb/type-rb/pull/830), which accepts `mode: trb`
as a declared mode without a backend in the reference. The reference AST is
unchanged. Exact cross-target, recovery and shared-language CI remain the
integration authority. This development pin does not change a release or
immutable seed. Earlier reference identities and evidence follow.

# Previous string-escape reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`977da4f4edbdd068eda484207ae2d220c861691f`. It incorporates
[PR #821](https://github.com/type-rb/type-rb/pull/821) for portable
single-quoted String and Symbol execution,
[PR #822](https://github.com/type-rb/type-rb/pull/822) for checked union
indexing, and
[PR #824](https://github.com/type-rb/type-rb/pull/824) for REPL
short-circuit assignment, and
[PR #825](https://github.com/type-rb/type-rb/pull/825) for class-method
`self.new(...)` checking and Go output. Native pairs the quoted-literal and
class-self contracts with lexer, verified MIR, ordinary execution and retained
REPL cases. Exact
cross-target, recovery and shared-language CI remain the integration authority.
This development pin does not change a release or immutable seed. Earlier
reference identities and evidence follow.

# Previous object-reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`71519dac58abc79811a35ef88fe9f38ccc435185`, incorporating
[PR #820](https://github.com/type-rb/type-rb/pull/820).
Transparent class aliases resolve to their target class for construction and
class-method calls, including generic, qualified and imported targets. Native
uses this merged reference to verify its corresponding MIR lowering. Focused
Native object execution and self-build passed locally; exact AST, shared
observations, recovery and target validation remain required. This development
pin does not change a release, immutable seed or performance qualification.
Previous reference identities and evidence follow.

# Earlier object-reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`92e170123eae0dc697138f3a1091441fe95e1b89`, incorporating
[PR #803](https://github.com/type-rb/type-rb/pull/803),
[#806](https://github.com/type-rb/type-rb/pull/806),
[#807](https://github.com/type-rb/type-rb/pull/807),
[#808](https://github.com/type-rb/type-rb/pull/808),
[#809](https://github.com/type-rb/type-rb/pull/809),
[#811](https://github.com/type-rb/type-rb/pull/811) and
[#812](https://github.com/type-rb/type-rb/pull/812).
Qualified records and interfaces retain nominal identity through namespace and
import aliases. Class methods preserve their lexical owners, private implicit
receivers and callable results across generated targets and the REPL.
Native pairs those contracts with ordinary path, forced-collection and retained
session controls. Exact AST, reference observations, recovery, self-hosting and
target validation remain required. No release, immutable seed or performance
qualification changes. Previous reference identities and their evidence follow.

# Previous standard-error reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`04d5ecf23a546abad5bd72ac283756282edb365f`, incorporating
[PR #792](https://github.com/type-rb/type-rb/pull/792).
Standard Result/error declarations preserve canonical identity through same-named
local declarations, import aliases and nested raw-enum conversions. Generated Go,
Ruby and TypeScript programs and retained REPL sessions have regression coverage.
Native adopts the corrected ordinary raw-enum error observation alongside String
transforms; exact AST, ordinary paths, recovery and target checks remain required.
Unicode case mapping follows the pinned Go implementation; output-mode differences
remain tracked in [TypeRB #791](https://github.com/type-rb/type-rb/issues/791).
This update changes no release or immutable seed and claims neither complete
language parity nor final performance qualification. Earlier evidence follows.

# Previous builtin-value reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`49c6d422e944e37f50a9b3a30941dd6c4d3ae256`, incorporating
[PR #788](https://github.com/type-rb/type-rb/pull/788),
[PR #789](https://github.com/type-rb/type-rb/pull/789) and
[PR #790](https://github.com/type-rb/type-rb/pull/790).
Unicode identifier spellings remain distinct in Go output, unsupported Unicode
source characters retain their diagnostic text and span, and builtin Result
operations preserve imported error aliases across generated targets.

Native uses reviewed ordinary cases for Unicode names, safe collection
retrieval/slicing and numeric String conversions. MIR constructs standard Result
values and errors through verified operations, and retained sessions reuse core
parsing. The exact AST, ordinary paths, recovery, target and lifetime checks remain
required. The Go-hosted recovery frontend's ASCII fallback and existing display
and contextual-inference gaps stay explicit. No release, immutable seed or
complete language/performance qualification is implied. Earlier identities and
their separate evidence follow.

# Previous namespace-binding reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`424d38cecf163153f47324a86ed793746368f153`, incorporating
[PR #786](https://github.com/type-rb/type-rb/pull/786).
Namespace-body lowercase bindings retain shared declaration storage and lexical
shadowing across module methods, closures and retained sessions. Calls invalidate
stale nullable proofs for mutable namespace storage. The reference correction has
executable Go/Ruby/TypeScript coverage and complete compiler tests.

Native lowers these lexical bindings through existing verified global MIR,
initialization and roots. Qualified binding-member access remains an explicit
reference boundary in [TypeRB #787](https://github.com/type-rb/type-rb/issues/787).
Ruby's separate same-named namespace collision is tracked in
[TypeRB #785](https://github.com/type-rb/type-rb/issues/785).
Exact ordinary-path, recovery, self-hosting, target and lifetime checks remain
required; this update does not qualify complete language support or performance,
or change a release or immutable seed. Previous identities and evidence follow.

# Previous shared-session reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`5912014558c0839ae86dd83e3e2f33446b2190ba`, incorporating
[PR #784](https://github.com/type-rb/type-rb/pull/784).
Earlier named-function references retain their declaration in the REPL after a
same-named value is introduced. TypeScript calls correctly select the later
lexical value. Defaults, generic bodies, closures and replay retain this
identity distinction.

Native shares retained session variables with later named functions while
preserving authored declaration order and single initialization. Exact AST,
ordinary paths, recovery, self-hosting, target and lifetime checks remain
required for this revision. No release, seed or complete language/performance
qualification is claimed. Previous accepted identities and observations follow.

# Previous global-binding reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`418090424759e2c32f37ebe9d3fdcd78c8703c66`, incorporating
[PR #782](https://github.com/type-rb/type-rb/pull/782).
Top-level lowercase bindings retain module identity across generated targets and
the REPL. Nullable proofs cannot survive a call that may replace mutable global
storage. Native follows those contracts with independently verified MIR reads,
writes and persistent managed roots. Interactive named-function access to session
variables remains explicit pending work.

Exact AST, ordinary-path observations, recovery, self-hosting, target and lifetime
checks remain required. No release, seed or full-language/performance qualification
is claimed. Previous accepted identities and observations follow.

# Previous literal reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`59cd287ffcc6bc2d8dcf7fadd6a5bf2a360e566f`, incorporating
[PR #780](https://github.com/type-rb/type-rb/pull/780) and
[PR #781](https://github.com/type-rb/type-rb/pull/781).
Nullable literal unions retain nil checks and explicit Integer-to-Float
conversions. Common record fields of unions perform the same numeric widening
in the REPL as in compiled execution, including singleton Integer fields.

Native adopts these corrections with explicit literal MIR and reviewed shared
cases. Exact AST, ordinary check/build/execution/REPL, recovery, self-hosting,
target and lifetime checks remain required. No release or seed is published,
and no full-language or performance qualification is claimed. Previous accepted
identities and observations follow.

# Previous union-alias reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`24dd15db447066cae8b8eb73a40cc063a2842f59`, incorporating
[PR #779](https://github.com/type-rb/type-rb/pull/779).
Nested and generic union aliases normalize before common-member and discriminant
checking. Scalar subsumption retains matching Go storage. Literal Integer
indexes, literal String receivers and standard collection parameters preserve
ordinary character semantics and expression-aware constraints.

Native adopts these corrections alongside verified literal constraints and
common union fields. Shared check/build/execution/REPL observations, exact AST
coverage, ordinary self-hosting, recovery and lifetime checks remain required.
This update publishes neither a release nor a seed and makes no full-language
or performance claim. Previous accepted identities and observations follow.

# Previous literal-boundary reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`ece27df01351e1499d23bb07bf39d140f81cc8d8`, incorporating
[PR #778](https://github.com/type-rb/type-rb/pull/778).
Literal-union parameters retain portable type syntax. Reassignment to singleton
or literal-union bindings and collection entries uses the same expression-aware
rules as initial declarations; arbitrary scalars still cannot narrow implicitly.

Native retains literal constraints in verified MIR and widens them explicitly.
Shared check/build/execution/REPL observations, exact AST coverage, recovery,
ordinary self-hosting, target and lifetime checks remain required. This update
does not publish a release or seed, complete basic-language coverage or qualify
performance. Previous accepted identities and observations remain below.

# Previous nominal reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`3476aabdc385fd2b422364f69bbc1aa16e0bc231`, incorporating
[PR #777](https://github.com/type-rb/type-rb/pull/777) and
[PR #776](https://github.com/type-rb/type-rb/pull/776).
Nominal constructor aliases and checked representation conversions survive typed
IR lowering, inferred unions retain Go storage, and conflicting declarations
produce diagnostics. Successfully decoded String literals have normalized backend
spelling. Remaining single-quote semantics retain their own reference issue.

Native newtypes preserve distinct MIR identities while erasing runtime storage.
Shared observations, exact AST coverage, recovery, ordinary self-hosting, CLI,
target and memory validation remain required before acceptance. This update does
not complete basic-language coverage, publish a release or seed, or qualify
performance. Previous identities remain below.

# Previous safe-block reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`909987e370a8ed4d8db1060ed5f0c6f00fd56dfd`, incorporating
[PR #775](https://github.com/type-rb/type-rb/pull/775).
Safe collection blocks preserve receiver-once evaluation, skip absent receivers
and operation arguments, and expose nullable transform results across targets
and the REPL. Nullable collection literal contexts preserve their element types.

Native safe blocks lower through existing nullable and iteration MIR. Shared
observations, exact AST coverage, recovery, ordinary self-hosting, CLI, target and
memory validation remain required before acceptance. This update does not
complete basic-language coverage, publish a release or seed, or qualify
performance. Previous identities remain below.

# Previous Array ordering reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`df2a60e35b7f4140c98dcb29a9eaee4fee27252e`, incorporating
[PR #773](https://github.com/type-rb/type-rb/pull/773).
Descending Array sorts in the REPL keep NaNs last in natural and key-based
ordering, matching compiled execution. Equal keys and signed zeros stay stable.

The Native implementation adds key-based Array sorting through existing typed
MIR loops and paired value/key buffers. Shared observations, exact AST coverage,
recovery, ordinary self-hosting, CLI, target and memory validation remain required
before acceptance. Safe block navigation remains an explicit reference defect
in [TypeRB #774](https://github.com/type-rb/type-rb/issues/774). This update does
not complete basic-language coverage, publish a release or seed, or qualify
performance. Previous identities remain below.

# Previous generic enum alias reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`22c2e36340ef3a58fde52e1120cecb2ea8f36091`, incorporating
[PR #771](https://github.com/type-rb/type-rb/pull/771).
Concrete and generic enum aliases keep their specialized methods in Go output
and the REPL. Nested imported constructors retain declaration identity; nullable
calls, defaults and captured private methods preserve ordinary behavior.

The 858-case registry adds generic enum receiver methods and their rejection
boundaries. Exact shared observations, AST coverage, recovery, ordinary
self-hosting, CLI, target and memory validation remain required before acceptance.
Method-specific type parameters, attributes and the recorded reference defects
remain explicit. This update does not complete basic-language coverage, publish
a release or seed, or qualify performance. Previous identities remain below.

# Previous enum call reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`e86ba0fdb4dd6a6845c176752fd5a6c29af3494c`, incorporating
[PR #767](https://github.com/type-rb/type-rb/pull/767).
Nullable enum calls skip argument effects when absent and evaluate the receiver
first when present. Methods returning functions and raw enum aliases preserve
their checked identity across portable backends and the REPL.

The 829-case registry includes raw enum conversions and ordinary enum methods.
Full shared observations, AST coverage, recovery, ordinary self-hosting, CLI,
target and memory validation remain required before acceptance. Generic enum
methods remain Native implementation work;
reference REPL replay and error-record shadowing remain explicit defects.
This update does not claim complete basic-language support, release a seed or
qualify performance. Previous identities remain below.

# Previous union reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`569d49cf38b9ec59242f18219c356988ed5e82cd`, incorporating
[PR #761](https://github.com/type-rb/type-rb/pull/761),
[PR #762](https://github.com/type-rb/type-rb/pull/762) and
[PR #763](https://github.com/type-rb/type-rb/pull/763).
Qualified generic aliases and imported inferred constants retain their checked
identity. Union assignments, discarded patterns, nullable injection/widening,
generic substitution and inferred Nil collections execute across the portable
backends and REPL.

The 768-case registry adds general union composition and inferred import cases.
Full shared observations, AST coverage, recovery, ordinary self-hosting, CLI,
target and memory validation remain required before acceptance. Reference
boundaries and presentation differences remain explicit; this update does not
claim complete basic-language support, release a seed or qualify performance.
Previous reference and measurement identities remain below.

# Previous namespace reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`7f30c7ec18c4c8ccb9fe37a7ae35d535b64e91ad`, including
[PR #751](https://github.com/type-rb/type-rb/pull/751),
[PR #753](https://github.com/type-rb/type-rb/pull/753),
[PR #754](https://github.com/type-rb/type-rb/pull/754),
[PR #757](https://github.com/type-rb/type-rb/pull/757) and
[PR #760](https://github.com/type-rb/type-rb/pull/760).
These correct keyword Symbols, module/constant member identity, raw enum
ownership and Go type/function name collisions in nested and imported modules.
Constant fixes also preserve mutable scalar copies, reopened scopes, imported
aliases, backend names and dependency-ordered REPL initialization.

The shared registry now contains 698 cases, including 41 namespace/constant
probes. Existing reference observations change only for module constants and
keyword Symbols. Existing Native observations improve for top-level/module
constants, keyword framing and invalid imported-project rejection. Hash REPL
presentation, qualified generic alias construction in the reference, and
untyped empty collection inference remain explicit differences. Review the
complete shared run against this exact checkout and AST.
Full shared, recovery, ordinary CLI, target and memory validation remains
required before acceptance. No release, seed or performance qualification is
inferred. Previous summaries and measurement identities remain below.

# Previous Symbol reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`ecfbcc4e0e72aaf09bba4f94e380d06d1be883be`, including
[TypeRB PR #747](https://github.com/type-rb/type-rb/pull/747).
Double-quoted Symbol contents validate their escapes, and portable Ruby output
preserves interpolation-looking text literally. Existing named-function,
nullable-call, callee-order and Hash-literal corrections remain included.

The shared contract contains 657 cases, including new Symbol and quote-boundary
observations. The control-keyword and multiline reference gaps remain explicit;
this is not complete lexical or basic-language coverage. Full exact-reference
shared, recovery, CLI, target and memory validation remains required before
acceptance. No release, seed or performance qualification is inferred. Previous
summaries and retained measurement identities remain below.

# Previous named-function reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`d26f0e19a0fb52cdff7ef23ef2cca6d05e673ba3`, including
[TypeRB PR #743](https://github.com/type-rb/type-rb/pull/743),
[PR #744](https://github.com/type-rb/type-rb/pull/744),
[PR #745](https://github.com/type-rb/type-rb/pull/745) and
[PR #746](https://github.com/type-rb/type-rb/pull/746).
Named function values retain their checked positional signatures and declaration
identities. Nullable functions require narrowing, callee selection precedes
arguments, and Hash literals preserve authored entry order and duplicate-key
overwrites. The pin also includes Float negative-zero and Array edge-arity fixes.

The shared contract contains 609 cases, including 30 new named-function cases
and six Hash key-expression cases. Full exact-reference shared, recovery, CLI,
target and memory validation remains required before acceptance. This integration
does not qualify performance or publish a release or immutable bootstrap seed.
The prior summaries and retained measurement identities remain below.

# Previous Array mutation reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`63c73105965c7ea7404c19b538b06f7f47c35a2d`, including
[TypeRB PR #740](https://github.com/type-rb/type-rb/pull/740).
Array insertion, concatenation and joining retain their receiver before argument
evaluation and inspect its current storage afterward. The pin also includes
[PR #738](https://github.com/type-rb/type-rb/pull/738) for callable library results
and [PR #739](https://github.com/type-rb/type-rb/pull/739) for generic library
parameter binding. The two previously rejected reference cases now pass ordinary
execution and the REPL; all other 434 existing expectations are unchanged.

This integration adds checked Native insertion/removal, live mutation and managed
lifetime cases. Full shared, recovery, CLI, target and memory validation remains
required before acceptance. No release, seed or performance qualification is
inferred, and retained measurements keep their original identities.

# Previous Array lookup reference compatibility

The selected development reference is TypeRB `0.4.9-dev` at
`b819d7815d39d7bf283868585f8a2d45bcb6414c`, including
[TypeRB PR #737](https://github.com/type-rb/type-rb/pull/737).
Array slicing and safe lookup retain the receiver before argument evaluation,
while observing mutations to its current storage. The Native Array receiver
implementation uses this rule for new shallow copies and strict edge access.

All 397 previously registered shared cases retain their reviewed outcomes at
the new pin. New Array cases explicitly record the remaining generic and
Function-result reference checker gaps. Complete correctness, recovery, CLI,
target and memory authorities remain required before acceptance. This current integration does not publish a release or a seed,
qualify performance, or change the retained measurements below.

# Previous sequential Array reference compatibility

The selected development reference is TypeRB `0.4.8-dev` at
`245ebcba45905037ea8f30d647d0893796441c88`, including sequential live Array
traversal from [TypeRB PR #729](https://github.com/type-rb/type-rb/pull/729).
[Native PR #497](https://github.com/type-rb/type-rb-native/pull/497) integrates
Array/Range map, select and reduce through MIR and the REPL.

The shared registry contains 282 cases. Local checks compare the exact reference
and Native through checking, building, execution and the REPL; the PR records
final correctness, recovery, target and memory authorities. Previously merged
reference corrections for nullable assignments, recursive enums, generic
imports/defaults, aliases and brace controls replace stale rejection outcomes.
Remaining unsupported paths and display differences stay explicit.

No new reference release, seed or performance qualification is inferred. The
retained compatibility cohorts below keep their original source, workflow and
measurement identities. This current integration summary shares the existing
compatibility evidence slot instead of adding another dated result directory.

# Previous released reference compatibility

The current candidate selects the released TypeRB `0.4.7` source at
`dc01dc490b86128ec7c29ad806f0047afaedeb84` in
[PR #459](https://github.com/type-rb/type-rb-native/pull/459).
The exact reference build embeds the declared stable version, independent of
local tag metadata. The nested-generic parameter regression matches through
check, build, execution and REPL in both implementations. The reference REPL's
conditional-transfer expectation now matches its compiled output; Native's
remaining rejection is an explicit coverage gap.

The updated shared registry has 92 cases. Complete PR correctness, recovery,
target and memory acceptance at this new pin is pending. Current integration
uses the explicit MIR migration cost policy; it is not formal performance
qualification. The retained compatibility cohorts and immutable seed/target
evidence below keep their original source and workflow identities.

## Previous Range compatibility prerequisite

The Range implementation candidate selects TypeRB `0.4.7-dev` at
`eb1f705e00235e608ea9283ffa90ea55cf9965f2`, including the Range element-argument
assignability correction in [TypeRB PR #683](https://github.com/type-rb/type-rb/pull/683).
The source slice is registered in
[issue #410](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5644960663).
Its compiler implementation is `e12512e69813ee92b1db4d307379103c7977b2ee`, stacked
on the separate short-circuit correctness fix in
[PR #437](https://github.com/type-rb/type-rb-native/pull/437).

This pin accompanies direct ordinary `Range<Integer>` values and iteration;
it does not reinterpret previous compatibility results. Maintained cases cover
endpoint retention, supported carriers, portable extrema, source-plan rejection,
lexical transfers and managed lifetimes. Unsupported Range annotation lowering
remains [TypeRB #684](https://github.com/type-rb/type-rb/issues/684), and logical
value expressions in snapshot export remain
[TypeRB #685](https://github.com/type-rb/type-rb/issues/685). The implementation
uses the supported recovery subset; authored Range snapshot export and compiler
source adoption require later independent work.

Current full recovery/target/cost acceptance is pending. The existing size and
1.05 limits are unchanged, including the currently known Range compiler-size
miss. This compatibility target is not a feature or cost exception. Immutable
seeds, frozen controls and previous cohort identities remain unchanged below.

## Previous Range endpoint compatibility prerequisite

The development pin is TypeRB `0.4.7-dev` at
`27a6bdd882084d5660fc9090006eb7e2a44d706c`, including the Range endpoint
evaluation fixes in [TypeRB PR #680](https://github.com/type-rb/type-rb/pull/680)
and [PR #681](https://github.com/type-rb/type-rb/pull/681). The compatibility
prerequisite is registered in
[issue #410](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5643802805)
against accepted Native `675f1c944f8f6fd28445f6fe0f1035f457dc1cd9`.

The exact reference build reports `0.4.7-dev`. Formatting and the root, compiler
and CLI checks pass (54, 39 and 30 source files). All 19 maintained language
cases match the selected reference and reviewed Native expectations, including
their separate check/build/execution/REPL paths. The 16 manifest/checkout tests,
seven registry-controller tests and generated table checks pass.

The captured-endpoint regression produces `0, 1, 2, 9, 1, 9, 3` in both Go
execution and the reference REPL. Against the previous reference executable,
Go execution instead produces `9, 9, 3`, while its REPL already matches the
specified result. Both direct Range and captured-endpoint cases remain known
Native rejections; the latter first encounters unsupported function-literal
syntax. They do not claim ordinary Range support or a runtime improvement.

Full hosted recovery, fixed-point, target, memory and ordinary cost authorities
are required before accepting this pin. The compiler source, immutable seed
evidence and historical comparison bounds are unchanged. Previous completed
authorities retain their original reference pins and source identities below.

## Previous Array iteration snapshot reference

The development pin is TypeRB `0.4.7-dev` at
`570bf6f64c7ca2412d0b4ffe06cd186cd53bdf4c`, including the accepted direct Array
iteration snapshot extension from
[TypeRB PR #679](https://github.com/type-rb/type-rb/pull/679).
No snapshot opcode or format version changes. Maintained value and managed
recovery fixtures cover live traversal, retained receivers, lexical bindings,
loop transfers, and roots across forced/automatic collections. Hash recovery
uses the same fixture runner, preserving its success and missing-key outcomes.

The six focused Array/Hash recovery tests and all 109 root and 198 compiler
tests pass with the exact new reference checkout at Native
`e7728081187f53ba275e8770b9029a622e262e79`. Recovery and QBE execution are enabled,
including ordinary regeneration, generation controls and managed collection.
Hosted acceptance is still required for this pin. No ordinary compiler-source
adoption, immutable seed handoff, performance acceptance or Pages update is
inferred from the local results. Previous completed authorities remain below.

## Previous collection iteration reference

The development pin is TypeRB `0.4.7-dev` at
`52f3928a0b5194cfae9c7a75e7f56e1f243ff8d1`, including the accepted live Array
iteration and repeatable Range/Iterable boundary fixes. All 17 selected checks
pass for Native `563036f26f84090339ae8d516f4d019caca1e16c` in
[run 34556980977](https://github.com/type-rb/type-rb-native/actions/runs/34556980977).
Local full recovery-enabled validation passes 107 root and 193 compiler tests;
formatting, reference checking and compatibility validation also pass.

The initial [run 34556559394](https://github.com/type-rb/type-rb-native/actions/runs/34556559394)
fails the quick job's exact reference identity check because the workflow still
checks out the previous pin. Updating the remaining active quick, worker-memory,
formal-build and amd64 workflow pins fixes that configuration mismatch. Frozen
source-era experiment pins remain unchanged. The successful run above retains
all ordinary recovery, target, memory and compiler-cost authorities.

This establishes reference compatibility for the upcoming ordinary iteration
work in [issue #410](https://github.com/type-rb/type-rb-native/issues/410).
It does not add Native Array/Range block support, extend snapshot v4, or change
immutable seed assets. Previous completed cohorts retain their identities below.

## Previous Hash reference compatibility

The development pin is TypeRB `0.4.7-dev` at
`bae19032aa1bb7b263bc827d02606edc6e981c52`, the accepted
[Hash snapshot-v4 extension](https://github.com/type-rb/type-rb/pull/668).
It adds data-only String/Integer Hash construction, assignment, required lookup
and key presence. Native recovery integration and the verified Hash checkout
seed are accepted in
[PR #405](https://github.com/type-rb/type-rb-native/pull/405). All 17 selected
checks pass for `fcb9e610eb6478d545b47950b027f05ff919e2c2` in
[run 34497846946](https://github.com/type-rb/type-rb-native/actions/runs/34497846946).
The first final attempt retains a failed Darwin wall ratio despite identical
control/candidate compiler and QBE bytes; one fresh-runner retry passes the
unchanged policy. Growth, forced collection, aliases, source effects, missing
keys and malformed Hash metadata are covered. This was the previous completed
compatibility cohort; older observations below retain their pins.

## Previous reference pin and retained authorities

The previous development pin is TypeRB `0.4.6-dev` at
`6cbd4025545d44a1de211335f9197772077bb478`, the merged
[record Array snapshot extension](https://github.com/type-rb/type-rb/pull/663) and
[recursive record registration correction](https://github.com/type-rb/type-rb/pull/664).
It retains canonical record element identities through aliases, nesting,
function signatures and closure captures. Snapshot v4 remains data-only and
keeps the existing operations/schema. Boolean Array support from
[PR #660](https://github.com/type-rb/type-rb/pull/660) and stable assignment
positions from [PR #659](https://github.com/type-rb/type-rb/pull/659) remain.
The new pin also includes the independent scalar-field call diagnostic and
imported nominal-contract fixes in PRs #661 and #662. Earlier observations
below retain their original reference pin and outcomes.

The accepted Boolean Array ordinary implementation is Native
`57cb41ad6be91716e31fa555ed8ea8c8ce7a5f51` from
[PR #345](https://github.com/type-rb/type-rb-native/pull/345). All 17 selected
checks passed at `06aedd81c291d732fe74afc88a9e131d3608765a` in
[run 34213179434](https://github.com/type-rb/type-rb-native/actions/runs/34213179434).
Hosted worker sizes are 349,296 Darwin arm64 and 325,864 Linux arm64 bytes,
675,160 combined. Local full recovery-enabled suites passed root 97/97 and
compiler 130/130, alongside 348 deterministic check/QBE observations, both
14-case language registries and ordinary CLI/REPL/automatic-GC checks.

Boolean Array snapshot recovery and its seed preparation observers are accepted
at Native `e99df93e81c36c765ead99526fe58b2c2a978ced` from
[PR #346](https://github.com/type-rb/type-rb-native/pull/346). All 17 checks passed
at `d33ec5e55362ef1ac3c685de8c536bc3cf01f6ff` in
[run 34215915536](https://github.com/type-rb/type-rb-native/actions/runs/34215915536).
Corrected local full suites passed root 99/99 and compiler 130/130; all 351
check/QBE observations passed. A fixture initially mixed snapshot-supported
alias/closure syntax with ordinary coverage. The shared case now uses supported
named functions, while a separate snapshot case retains closure coverage.
The original failed observation remains separate from the corrected cohort.
The ordinary compiler source and local binary are unchanged from PR #345.

The [verified Boolean Array seed handoff](https://github.com/type-rb/type-rb-native/blob/db48f64f6c3f6ce3fb904b2d28c95fa46522ebfc/docs/bootstrap-seed-updates.md#current-verified-checkout-seed)
records immutable assets, exact source and successful fresh published-asset
verification. The checkout handoff is accepted at
`65ab856abe55b4816ec6cc8e7e4cc91ce98293bd` from
[PR #347](https://github.com/type-rb/type-rb-native/pull/347). All 17 checks passed
at `c6b98d25483d766c09e5695a8c315e210f1111de` in
[run 34219868383](https://github.com/type-rb/type-rb-native/actions/runs/34219868383),
including the separate Linux amd64 Boolean-capability setup bridge.

The bounded Boolean flag self-use is accepted at
`4a594bc6562262502b6f901cae63c5288a705f5b` from
[PR #348](https://github.com/type-rb/type-rb-native/pull/348). All 17 selected
checks passed at `62860e4d45691aff6d5002db9715d150042eabbb` in
[run 34222798465](https://github.com/type-rb/type-rb-native/actions/runs/34222798465).
Both failure-flag carriers use Boolean elements; local enabled suites passed
root 99/99 and compiler 130/130, with unchanged application QBE observations.

The [readonly-field repair](https://github.com/type-rb/type-rb-native/pull/351)
is accepted at `f1d65b96d659f68376c7ec937be98ef95e02a838`, with all 17 checks
passing at `2d917d2e8feed1e0054edfe8f9c9958cedcab0c7` in
[run 34232237613](https://github.com/type-rb/type-rb-native/actions/runs/34232237613).
Field-binding rejection belongs to the checked frontend, while Array contents
retain their mutation capability. Its source-era reference pin `202cea8`
missed `(box.value)()` during checking; the current pin includes that separate
checker repair. The original mismatch remains a source-era observation.

The initial local enabled cohort passed compiler 131/131 but failed one of
99 root tests: the old `float-scalars` valid fixture assigned to a readonly
record field. The ordinary CLI also retained that obsolete expectation. The
fixture now rebinds the complete record, and REPL coverage checks both rejected
field replacement and valid complete rebinding. Those original failures remain
separate from the corrected-source cohort. All 378 corrected deterministic
check/QBE observations and the complete ordinary CLI/REPL/terminal suite pass.
Corrected enabled suites passed root 99/99 and compiler 131/131; hosted
acceptance is recorded above.

The accepted ordinary named-record Array implementation in
[issue #349](https://github.com/type-rb/type-rb-native/issues/349) follows that
readonly correction. Three execution cases match the reference, covering
typed and inferred arrays, nested aliases, RHS growth and automatic collection.
Ten diagnostic fixtures retain nominal types and mutation capabilities; depth
four remains an explicit unsupported subset. Both runtime bounds cases retain
panic exit 2. Full ordinary CLI/REPL/terminal checks and 423 deterministic check/QBE
observations pass; existing cases match the readonly control. Imported record aliases
preserve identity; a same-named local record and an inferred imported record
remain distinct. The source-era `202cea8` reference incorrectly accepted that latter check
and then failed generated Go with a duplicate declaration. Preserve that
failed comparison; the current pin rejects the nominal contract mismatch and
does not claim broader same-name code-generation support. Different names with identical shapes are
rejected by both compilers.

The initial Darwin prototype is 365,768 bytes versus its 349,256-byte control.
Its text section grows from 259,136 to 260,804 bytes and crosses a 16 KiB
segment boundary. This exceeded the source-era 350,000-byte Darwin ceiling and remains
a failed cost observation. The source after the readonly prerequisite
also produces 365,768 bytes locally (text 260,964 bytes) and requires its
own complete target/cost cohort. The first hosted cohort failed Darwin worker
and managed-runtime size guards at 365,808 bytes; Linux worker size was 327,736.
The separate budget PR #353 is accepted at
`8dc08fbd8e18a4b161460ccf5c3a2c0aefd7f2ec`, with all 17 checks at
`3cb5e159beff9d802981885878f94545599ca604` in
[run 34238461767](https://github.com/type-rb/type-rb-native/actions/runs/34238461767).
Decision 0030 changes only Darwin/combined ceilings to 366,000/694,000 bytes;
relative and all other acceptance requirements remain unchanged. Ordinary
record Arrays are accepted at `566d00d67172460df0f57dff6d5fe03db3b67cdc` from
[PR #352](https://github.com/type-rb/type-rb-native/pull/352), after all 17
selected authorities passed at `9e8a6f6149323a87710b2be33f9b39e7b71077e7`.
The failed first cost attempt and bounded confirmation are retained below.
No compiler implementation uses record Arrays yet.

The accepted record Array recovery implementation pins the reference above.
Local recovery-enabled root 102/102 and compiler 133/133 tests pass, including
the ordinary recovery fixture and a separate escaping closure fixture.
The complete ordinary CLI/REPL/terminal suite and both 15-case language
registries pass. Focused MIR tests reject seven element/index/receiver errors;
the lifetime test retains exact record Integer values after two constructing
calls, nested aliases, mutation and explicit collection, and traces dynamic
String and Integer-Array fields owned by another returned record. Removing
scalar-record boxing fails the exact-value lifetime test. An earlier Boolean-only
oracle did not expose that invalid stack storage and was strengthened; both
observations are retained separately. The recovery changes no ordinary compiler
source; exact-head hosted acceptance and its retained first failure are recorded
below.

No runtime-performance or Pure Go claim, benchmark-value update or seed change
is made by this recovery update. The cumulative baseline remains
`ac633935a7f248470c59d22666da14c819a131fa`.

The accepted assignment prerequisite is Native
`1baf5ad2cb6cc2bd6158c20f837a3479bfa3360d` from
[PR #343](https://github.com/type-rb/type-rb-native/pull/343). All 17 selected
checks passed at `ea58fd0391d4a252d26acf3ee284ad908b8c68f5` in
[run 34209892000](https://github.com/type-rb/type-rb-native/actions/runs/34209892000)
after the separately accepted budget [PR #344](https://github.com/type-rb/type-rb-native/pull/344).
Its worker sizes are 349,296 Darwin arm64 and 325,600 Linux arm64 bytes,
674,896 combined. Its Linux compiler comparison reports 313,248 baseline and
325,608 candidate bytes (1.039458), build median ratio 1.010417 and RSS ratio
1.000413. All original failures below retain their source-era status. The
cumulative implementation baseline remains
`ac633935a7f248470c59d22666da14c819a131fa`; acceptance does not establish a
Pure Go speedup.

Earlier assignment-repair observations follow.
Local ordinary Integer, Float, nested-owner and managed String assignment
cases match the selected reference, including actual RHS growth and an
initial bounds failure that skips RHS effects. The managed case records five
automatic collections and zero live bytes after final collection. The formerly
crashing Integer growth probe succeeds under GuardMalloc. The pinned published
seed builds the current core and CLI fixed points; ordinary CLI and REPL tests
passed with those cases. Its complete acceptance is recorded above.

Compiler recovery authoring failures are retained separately: an unsynchronized
exact import prefix, a helper initially placed after the required empty driver,
and logical expressions outside snapshot v4's supported source subset. The
candidate uses the existing statement forms and preserves the exact recovery
boundaries. Earlier tests that counted only initial bounds checks now account
for the independent final store check; their induction and inline-budget
assertions remain in place. No cost limit or failed measurement was waived.

## First assignment candidate and bounded carrier cleanup

The first full cohort for Native
`90435186fc5c06b662c06cb80584575e28d9d064` is
[run 34199705462](https://github.com/type-rb/type-rb-native/actions/runs/34199705462).
Recovery, both ordinary CLI targets, Darwin worker memory and the Linux amd64
target passed. Linux arm64 failed its unchanged 317,000-byte compiler ceiling:
the target chain reports 326,312 bytes, and the worker's stripped compiler
reports 326,304 bytes. The combined and performance authorities therefore did
not run; this candidate was not accepted. Keep that cohort and its failures.

The first bounded follow-up removes emitter-only mutability transport, whose
checks already belong to the checked frontend, and removes the redundant
Array-position carrier field. A tagged slot represents a binding address or
a validated Array position beside its owner. All internal callers change
directly. Required bounds, owner/old-value roots and target/store verification
remain. This is one revised-source correctness and cost cohort, not an
unchanged-head retry or a larger acceptance budget. Reassess any remaining
cost miss before another size-only attempt.

## Compact candidate and accepted budget decision

The compact source `885414012e0aa891d1af99f30069e60da300a6cb` completed
[run 34202210192](https://github.com/type-rb/type-rb-native/actions/runs/34202210192).
Recovery, both CLI targets, Darwin worker memory and Linux amd64 passed.
Linux worker size was 325,600 bytes; the Linux target chain was 325,608 bytes.
Both failed the source-era 317,000-byte ceiling. Combined, target-neutral
comparison and performance authorities were skipped, and acceptance failed.
This cohort and the earlier failure remain failures.

The [complete local diagnostic observations](array-assignment-diagnostic.csv)
include both warmups and all retained rounds; their
[identities and method](array-assignment-diagnostic-identities.json) distinguish
cumulative baseline, same-feature control and compact candidate. All 216
observations retained expected outputs and reproducible artifacts. A sandbox
restriction prevented the first timer invocation from reading `kern.clockrate`
before any valid observation; that stopped infrastructure record is separate
from this complete cohort. No failed performance sample was replaced.

[Decision 0029](../../docs/decisions/0029-array-assignment-compiler-budget.md)
records the cost assessment and accepted Linux 328,000 / combined 678,000-byte
budgets. [PR #344](https://github.com/type-rb/type-rb-native/pull/344) implements
that policy separately. Its compiler source is unchanged from cumulative
baseline `ac633935a7f248470c59d22666da14c819a131fa`; this budget revision does
not reset the cumulative comparison. The repair includes the policy for fresh
full validation and cannot merge before that policy is accepted and every
selected authority passes at the repair's exact head. Pages measurements and
all other acceptance requirements remain unchanged.

# Previous loop-transfer reference checkpoint

The current development pin is TypeRB `0.4.6-dev` at
`4e1327c9af1b4caec9963756e6ffbc0e2ef56341`, the merged
[loop-transfer snapshot update](https://github.com/type-rb/type-rb/pull/657).
Snapshot v3/v4 encode nearest-while transfers and early method returns with
existing jumps and live block arguments. The format and v2 boundary remain
unchanged. Both `elsif-recovery` and `loop-transfer-recovery` run through
snapshot encoding, strict program decoding, recovery QBE emission and execution.

The Native implementation is accepted
`4e1d0b4aee97b9a5bd73a98f918b31d47985da25` from PR #337.
[Exact-head acceptance](https://github.com/type-rb/type-rb-native/actions/runs/34166076931)
passed all selected recovery, ordinary CLI, target, memory and comparative-cost
authorities at the preceding reference pin. That is the implementation baseline,
not acceptance of this reference update. Fresh enabled suites and every
applicable hosted authority are required before merge. Seed registration is
separate from publication, actual published-asset verification and checkout pins.

The preceding pin was `f6229c5657a5acb40194cde71785a63754d00355` from
[TypeRB PR #655](https://github.com/type-rb/type-rb/pull/655).
Its [source-era compatibility record](https://github.com/type-rb/type-rb-native/blob/4e1d0b4aee97b9a5bd73a98f918b31d47985da25/results/2026-09-07-string-escape-reference-compatibility/README.md)
retains the earlier conditional and logical-condition checkpoint identities.

## Previous String-escape checkpoint

The preceding experimental update selected TypeRB `0.4.6-dev` at
`47a160cae05ddc2035c7430735c4762d36bbc9c4`, the source revision of
[TypeRB PR #651](https://github.com/type-rb/type-rb/pull/651).
The dependent Native update requires that reference PR to merge first.

Native implementation `07fc9af5e4d8b099865297820068140222ade47e` passed
[Native CLI workflow 34081476080](https://github.com/type-rb/type-rb-native/actions/runs/34081476080)
on Darwin arm64 and Linux arm64. Both jobs build from the unchanged pinned
Native seed, verify core and CLI fixed points, run CLI and typed REPL cases,
exercise escape and interpolation errors, render terminal editing and history,
and verify unchanged checkout inputs reuse the executable.

Local Darwin arm64 checks with the exact selected reference also pass:

- reference checks for the root, compiler and isolated CLI source projects;
- String escape and interpolation file/REPL differential tests;
- shared reference/Native REPL screen and history tests.

The implementation accepts escaped hashes in source while keeping JSON history
validation separate. The new reference requires typed filesystem paths and
immutable record fields; adapters construct `Path` values and CLI state uses
mutable Array elements or replacement session records.

This is bounded source, String and CLI compatibility evidence. It is not a
version range, complete language conformance claim or performance measurement.
Full recovery, target, memory and performance checks on the final PR revision
remain the required acceptance authorities. Published bootstrap assets and
previous measurement contracts retain their original revisions.

The [previous compatibility record](https://github.com/type-rb/type-rb-native/blob/5cf61c740aa600c34ed94f1b130ea2ffefd9e783/results/2026-08-31-typerb-0-4-4-compatibility-darwin-linux-arm64/README.md)
remains available at its exact archived revision.

## Record Array diagnostic assessment

The [prospectively registered local cohort](https://github.com/type-rb/type-rb-native/pull/352#issuecomment-5586493243)
completed all [105 raw observations](record-array-cost-diagnostic.csv), including
two warmups and five retained rotating rounds. The [identities and all medians](record-array-cost-diagnostic.json)
retain source/compiler/application hashes, exact inputs, toolchain and hardware.
All outputs and repeated artifacts matched. The old cumulative compiler was
reclosed through equal same-basename generations after the newer seed's initial
output; the initial bootstrap binary was not treated as the frozen compiler.

Spectral-norm(5500) wall medians were 2.373460 / 2.385774 / 2.367373 seconds for
cumulative / accepted control / candidate, with 50,984-byte applications.
Compiler self-build medians were 1.596516 / 1.657213 / 1.660604 seconds; candidate
ratios are 1.040143 cumulative and 1.002047 incremental. Both selected applications
retain byte-identical control/candidate QBE and executables. This establishes
neither a runtime gain nor a new Go comparison.

The cohort also exposes a cumulative regression: n-body(1000000) runtime medians
are 0.273365 / 0.472791 / 0.476320 seconds, a candidate/cumulative ratio of
1.742429. The immediate control already has that difference. Its cumulative
build wall ratio 1.054824 also crosses the 1.05 alarm; coarse runtime CPU medians
are 0.10 / 0.30 / 0.30 seconds. These observations are retained as regression
alarms, not omitted or relabeled as passing results. Static QBE Array-address
call counts rise from 4 to 16 before this candidate; that is a follow-up lead,
not a proven attribution. [Issue #354](https://github.com/type-rb/type-rb-native/issues/354)
tracks that cumulative investigation. No diagnostic budget is renewed by this assessment.

Hosted cohort [34234062970](https://github.com/type-rb/type-rb-native/actions/runs/34234062970)
passed enabled recovery suites and CLI/target controls, then failed both Darwin
managed-runtime smoke and worker size guards at 365,808 bytes. Linux worker
size is 327,736 bytes; the 693,544-byte sum is arithmetic because combined and
comparative checks were skipped. The separate [budget decision](https://github.com/type-rb/type-rb-native/pull/353)
was subsequently accepted; this source-era failed cohort remains unchanged.

## Ordinary record Array hosted acceptance

[Run 34241831590](https://github.com/type-rb/type-rb-native/actions/runs/34241831590)
at `9e8a6f6149323a87710b2be33f9b39e7b71077e7` first failed Darwin self-build
wall ratio 1.053299 (1.970 / 2.075 seconds). Correctness, recovery, both CLI
and target controls, worker/combined size and memory, and Linux comparative
cost passed. The Darwin comparison exited before writing cross-target
identities; the downstream failure does not establish a code mismatch.

A [separately preregistered single confirmation](https://github.com/type-rb/type-rb-native/pull/352#issuecomment-5587645688)
kept the exact source, ordinary 1.05 limits, two warmups and seven retained
rounds. Attempt 2 passed all 17 authorities, including cross-target identity.
Darwin self-build medians were 2.560 / 2.420 seconds (0.945312). The additional
preregistered check pooled all 28 retained observations per role from both
attempts: medians 2.300 / 2.360 seconds, ratio 1.026087. Pooled peak RSS ratio
is 0.996429. The first failed attempt is preserved and the one-run confirmation
budget is expired.

The [complete two-attempt Darwin observations](record-array-hosted-confirmation.json)
include every warmup and retained measurement, source/compiler/toolchain
identities, measurement policy, environments and original comparisons. Linux
self-build ratio was 1.011364. Persistent-worker compiler sizes were 365,808
Darwin and 327,736 Linux, with the verified 693,544-byte combined total below
694,000. Comparative fixed-point filenames produce separately recorded sizes
of 365,768 and 327,744 bytes; do not substitute one observer's artifact identity
for another. This checkpoint supports ordinary incremental acceptance only.
The frozen cumulative cohort and n-body alarm above remain; no runtime speedup
or Pure Go comparison is inferred.

## Recursive record Array recovery correction

A self-referential `Node` with `children: Array<Node>` executes correctly in
both ordinary compilers, including a retained cycle and 20,000 discarded cycles,
but the first v4 producer rejected its Array element while `Node` was still
being registered. [TypeRB #664](https://github.com/type-rb/type-rb/pull/664)
registers the record kind before traversing fields and completes the same field
slice. Self-recursive and mutually recursive type graphs retain complete fields,
unique nominal definitions and deterministic snapshots. The exact accepted
reference is `6cbd4025545d44a1de211335f9197772077bb478`; full Go tests, source
formatting and [hosted validation](https://github.com/type-rb/type-rb/actions/runs/34249335045)
passed at `6f344860f35337126b20d2dad38f2754df67f0c7`.

The existing ordinary recovery fixture now retains a cycle across allocation
pressure and creates unreachable record/Array cycles. Ordinary execution and
a focused real v4 snapshot/decoder/MIR/QBE execution pass. The final enabled local suites passed 102 root and 133 compiler tests with
this stronger fixture and the exact accepted reference.
The earlier recovery candidate CI was cancelled for this correction after
CLI, target/QBE and worker/combined checks passed; its cancelled root suite
and unrun comparative checks establish no acceptance. That earlier candidate
did not publish a seed.


## Record Array recovery hosted acceptance

[PR #355](https://github.com/type-rb/type-rb-native/pull/355) was accepted at
`b8133a3df537d3d8af5531fbd235322d87519d3f` and merged as
`db48f64f6c3f6ce3fb904b2d28c95fa46522ebfc`. Its first
[hosted attempt](https://github.com/type-rb/type-rb-native/actions/runs/34250207907/attempts/1)
passed correctness, recovery, both CLI and target controls, worker/combined
size and memory, and Linux comparative cost (0.992424), but failed Darwin
self-build wall ratio 1.065539 (2.365 / 2.520 seconds). Baseline and candidate
Darwin compilers were byte-identical at 365,768 bytes, SHA-256
`a95b8758af7d45ccbd40100cf00e15f2783a381b66a253a21fe683c06968e9bd`.

A [separate single confirmation](https://github.com/type-rb/type-rb-native/pull/355#issuecomment-5588686720)
retained the exact candidate and baseline, commands, observations and ordinary
limits. Attempt 2 passed all 17 authorities; Darwin medians were 2.040 / 1.995
seconds (0.977941). The additional preregistered pool of all 28 retained
observations per role gave 2.185 / 2.215 seconds (1.013730), with RSS ratio
0.998611. The [complete two-attempt evidence](record-array-recovery-hosted-confirmation.json)
retains all 72 warmup/retained observations, identities, environments, policies
and original failures. QBE binaries were identical between roles within each
attempt; fresh builds on separate runners had different tool binary digests,
which remain recorded separately. The confirmation budget is exhausted and
expired. This accepts the recovery prerequisite without changing any ordinary
limit or renewing a runtime diagnostic.


## Record Array bootstrap handoff

The [current seed record](../../docs/bootstrap-seed-updates.md#current-verified-checkout-seed)
pins immutable `bootstrap-seed-2026-09-09-record-arrays` at accepted
`db48f64f6c3f6ce3fb904b2d28c95fa46522ebfc`. Preparation 34254120749 passed both
targets and all 28 retained observations; actual-published verification 34255257731
passed both targets, seed/B1/B2/B3/B4 equality, the corpus and all 42 retained
observations. All four published asset attestations bind to that accepted source
and hosted preparation workflow. Compiler sizes are 365,768 Darwin and 327,744
Linux bytes, 693,512 combined. Exact hashes and asset IDs remain in the linked
handoff record. Historical releases and their source-era bounds are unchanged.

Checkout core/CLI bootstrap and active CI consumers now select that exact seed.
The historical Linux amd64 setup also builds accepted record-capable 566d00d6
before candidate source; this setup-only bridge retains separate source,
compiler/QBE and process identities. Compiler implementation has not adopted
record Arrays yet. The subsequent five-field MIR value carrier migration followed this
accepted checkout handoff. Frozen benchmark baselines and the
cumulative n-body alarm are unchanged; no runtime/Pure Go gain is claimed.


## Named MIR value carrier self-use

The [registered slice](https://github.com/type-rb/type-rb-native/issues/349#issuecomment-5589325244)
follows accepted seed handoff `2f503f993ca93e5d50ad3151755fc739039d1bb5` (PR #356),
whose [exact-head validation](https://github.com/type-rb/type-rb-native/actions/runs/34256015371)
passed all 17 authorities. Only `Gate4MirModule.values` and local staging use
`Array<Gate4MirValue>`, with named function/value/type/source/line fields.
Construction, lookup and verification share that type; other MIR row families
and semantic pass ownership remain unchanged. Wrong tuple lengths become
unrepresentable, while identity, type range, origins, uniqueness and exact
value-definition checks remain. Two pre-migration characterization tests retain
those diagnostics, and existing Integer/Float malformed-MIR tests replace whole
readonly records.

The [local correctness record](mir-value-carrier-correctness.json) binds all six
changed source files to the same-feature control, compiler identities and 426
ordinary check/repeated-QBE observations. Every observation equals the control.
Spectral-norm (100) and n-body (1000) retain byte-identical QBE and executables
and the exact expected output. Fresh verified-seed core/CLI fixed points and
full CLI/REPL/terminal checks pass. Local compiler and CLI sizes remain 365,768
and 499,144 bytes respectively; their contents change as expected for the new
internal representation. Formatting and both source checks pass. Full enabled local recovery suites passed root 102/102 and compiler 135/135.
The owned recovery workspace was removed after terminal completion. Exact-head
hosted acceptance and the retained first failure follow.

No application runtime measurement or speedup is claimed. The cumulative
`ac633935a7f248470c59d22666da14c819a131fa` baseline, n-body alarm and expired
measurement budgets above remain unchanged.


## MIR value carrier acceptance

[PR #357](https://github.com/type-rb/type-rb-native/pull/357) is accepted at
`89a35c16de504ffb9c2ad926867ff277f6bf529d`, merged as
`3bde3e0a06d764789fc478e68255ffe47c209e33`, after all 17 authorities in
[run 34258862533](https://github.com/type-rb/type-rb-native/actions/runs/34258862533)
and the separately registered additional check passed.

The first amd64 attempt failed one retained adjacent-generation observation:
B2-to-B3 took 1.978962366 seconds, 2.05398157 times its strongest adjacent
median. The B2-to-B3/B3-to-B4 medians were 0.963476204/0.966003788 seconds;
all three compilers were byte-identical at 280,608 bytes, SHA-256
`fc325b7998a704c69f8b48da49239e99ad5ce7dae38c95043eba1f1670518426`.
The unchanged 2.0 guard correctly failed that cohort. Its 88 observations
(16 warmups and 72 retained measurements) are preserved, including the outlier.
Environmental variation motivated investigation but is not proven attribution.

A [single fixed-input confirmation](https://github.com/type-rb/type-rb-native/pull/357#issuecomment-5589567975)
kept the exact source, compiler identity, observer, workloads, repetitions,
ordering and all ordinary limits. The new amd64 cohort passed every guard;
its adjacent medians were 1.283581789/1.270438763 seconds (spread 1.01034527).
Both hosts report the same CPU model, but absolute times differ and both
host environments remain recorded. The additional pool of all 14 retained
observations per generation role gave medians 1.253844056/1.255019547 seconds,
spread 1.00093751; RSS spread is 1.00043349. The original catastrophic failure
remains a separate failed cohort, not a rewritten passing observation.
The one-run budget is exhausted and expired, with no third cohort authorized.

The [complete hosted acceptance record](mir-value-hosted-acceptance.json)
retains all 176 amd64 observations and both environments, source/compiler/tool
identities, original failure, registration and pooled calculation. It also
retains the subsequent ordinary arm64 compiler A/B observations and bounds.
Darwin self-build medians were 1.960/1.985 seconds (1.012755), with RSS ratio
1.002785. Linux medians were 1.350/1.370 seconds (1.014815), with RSS ratio
0.999557. All ordinary 1.05 limits passed. Darwin compiler size remains
365,768 bytes; Linux falls from 327,744 to 327,448 bytes, for 693,216 combined.
Compiler QBE falls from 1,151,452 to 1,150,705 bytes. These are compiler costs,
not application runtime improvements. The frozen cumulative baseline and prior
n-body alarm remain; no new cumulative timing or Pure Go comparison is claimed.

This closes the named record Array slice through ordinary support, matching
recovery, verified immutable seed handoff and actual compiler self-use. Other
MIR carriers and optimization facts remain independently bounded follow-ups.

## Named MIR function carrier and structural validation

[Issue #359](https://github.com/type-rb/type-rb-native/issues/359) registers the
next bounded self-use slice against `923f4bd31c70f1e83979034916bf4f781523dcc0`.
`Gate4MirFunction` replaces the eight-cell function row with named identity,
entry, source origin, and parameter/block ranges. Construction, verifier and
pass helpers, adapter consumers and strict recovery imports move together.
Other row families and optimization admission are unchanged.

Pre-migration characterization exposed a verifier defect: malformed ranges and
short rows could be indexed while counting value definitions before their
structural validation. A standalone malformed function range raised an Array
bounds error, and four of five expanded tests failed with bounds errors or the
wrong diagnostic. Structural checks now run before value and cross-block
consumers. The existing identity, origin, type, uniqueness, definition and
control-flow checks remain required; malformed function row length becomes
unrepresentable through the named carrier.

The [local correctness record](mir-function-carrier-correctness.json) binds the
candidate sources and includes all 426 ordinary check/repeated-QBE observations.
Fresh published-seed core/CLI fixed points, the language registry and full
CLI/REPL/terminal suite pass. Enabled recovery/QBE suites pass root 102/102 and
compiler 140/140; focused MIR tests pass 17/17. The new malformed-MIR tests also
pass in an ordinary Native-compiled harness. Its first preparation used an
unsupported core-command package import and was rejected; the corrected harness
uses the existing `puts` builtin without a compiler change. The owned recovery
workspace was removed only after both suites terminated.

Spectral-norm at input 100 and n-body at input 1000 retain byte-identical QBE,
executables and outputs against the accepted same-feature control. The local
Darwin compiler shrinks from 365,768 to 349,256 bytes; CLI size stays 499,144.
This does not establish application runtime improvement. Earlier cumulative
n-body alarms remain unresolved, and no new cumulative timing, Pure Go parity,
or benchmark/Pages-value update is claimed.

[PR #360](https://github.com/type-rb/type-rb-native/pull/360) is accepted at
`f4ef0986678ad0870dea48cdeb28decc6eed4e30` and merged as
`a714f9b20b1b2d9ddec29878a1982a18ed110a6e`.
All 17 authorities pass on the first attempt in
[run 34309605826](https://github.com/type-rb/type-rb-native/actions/runs/34309605826).
The [hosted record](mir-function-hosted-acceptance.json) retains all 88 amd64
observations (16 warmups and 72 retained), both arm64 bootstrap and interleaved
comparison cohorts, environment/toolchain/identity records, and cross-target
results. The PR merge-ref checkout has the same tree as the candidate head.

| Metric | Darwin arm64 | Linux arm64 |
| --- | ---: | ---: |
| Complete compiler bytes, before → after | 365,768 → 349,256 | 327,448 → 327,264 |
| Code text bytes, before → after | 260,700 → 260,512 | 263,280 → 263,120 |
| Self-build wall ratio | 0.988201 | 0.978417 |
| Self-build peak RSS ratio | 0.999405 | 0.999842 |

Combined compiler size is 676,520 bytes within the existing 694,000-byte bound.
Compiler QBE grows by 701 bytes to 1,151,406 while both code sections shrink;
its target-neutral SHA-256 is
`11f58bff93f3b9290e19a5c935735793cbd7958b5f17cff3d8b8965f3d30cbb0`.
The amd64 compiler is 279,976 bytes within its unchanged 310,000-byte bound.
No special confirmation, threshold revision or historical failure relabeling
was used. The generated applications remain unchanged as recorded above.

## Named MIR block carrier self-use

[Issue #362](https://github.com/type-rb/type-rb-native/issues/362) registers the
block-carrier slice against `2360a9462199a612e916f67d6e38df17ad2ee9bd`.
`Gate4MirBlock` names the sixteen fields for identity, origin, parameter and
instruction ranges, terminator, condition, both successor/argument ranges, and
return value. The shared constructor retains its calling boundary; verifier,
passes and adapter readers use named fields. Strict recovery imports move with
the constructor. Other row families and optimization admission are unchanged.

Malformed block lengths are unrepresentable through the record type. Range
checks still precede traversal, and the origin, identity, successor, argument,
jump/branch/trap/return and value-definition checks remain required. Negative
fixtures replace a whole readonly record. The two new characterization tests
pass before and after migration; all 22 focused MIR/construction tests pass.
The first characterization incorrectly expected a duplicate-ID error after
renaming a referenced block had removed its target. The baseline correctly
reported the missing target first; the corrected duplicate case adds a trap
block without duplicating value definitions or removing a target. This was a
test expectation correction, not a change to diagnostic precedence.

The [local correctness record](mir-block-carrier-correctness.json) binds the
source and compiler identities, all 426 ordinary check/repeated-QBE observations,
and the ordinary Native-compiled malformed-MIR harness. Fresh published-seed
core/CLI fixed points, the language registry and full CLI/REPL/terminal checks
pass. Spectral-norm at input 100 and n-body at input 1000 retain identical QBE,
executables and outputs against the accepted same-feature control.

The local Darwin compiler and CLI remain 349,256 and 499,144 bytes. Compiler
code text shrinks from 260,512 to 258,664 bytes. These are compiler results;
there is no application speedup or new cumulative timing claim. Earlier
cumulative n-body alarms, deferred PR #307 and historical diagnostic outcomes
remain unchanged. No benchmark/Pages values or compatibility/seed pins change.

Complete local recovery/QBE-enabled suites pass root 102/102 and compiler
141/141. The owned recovery workspace was removed through its receipt after
both suites terminated. These correctness-suite durations are retained as
execution records, not substituted for the hosted self-build cost comparison.

[PR #363](https://github.com/type-rb/type-rb-native/pull/363) accepts source
`06f536a27198ab2cd85a98bdf060376ff4451862` as merge
`b2946fd0224957b994f6b199e2a996851c5ae5ae`.
[CI run 34314865533](https://github.com/type-rb/type-rb-native/actions/runs/34314865533)
passes all 17 selected authorities on its first attempt. The
[hosted acceptance record](mir-block-hosted-acceptance.json) retains artifact
identities, raw measurements, environments, comparison policy and checks,
including all 88 amd64 warm/retained rows with successful observer and process
statuses.

| Ordinary compiler metric | Darwin arm64 | Linux arm64 |
| --- | ---: | ---: |
| Executable bytes | 349,256 | 325,352 |
| Code text bytes | 258,664 | 261,232 |
| Self-build elapsed ratio | 1.005917 | 1.000000 |
| Self-build peak RSS ratio | 1.000397 | 0.999207 |

Combined compiler size decreases from 676,520 to 674,608 bytes within the
unchanged 694,000-byte bound. Compiler QBE decreases from 1,151,406 to
1,148,167 bytes, with target-neutral SHA-256
`f85615a563f30ae8b1866c0b48ba5eefa827f10ce1c8662b00a5c64fc0771241`.
The amd64 compiler is 277,440 bytes within its unchanged 310,000-byte bound.
No special confirmation or threshold revision was used. The amd64 adjacent
self-hosting-generation comparison is not an incremental application timing
comparison against the previous carrier slice.

## Named MIR instruction carrier self-use

[Issue #365](https://github.com/type-rb/type-rb-native/issues/365) registers the
eight-field instruction-carrier slice against
`4c448befd5c85950ab0369aa502c39b67ffd0fbc`.
`Gate4MirInstruction` names kind, result, two operands, opcode-specific payload,
failure target and source origin. The module table and function-local buffer
share this carrier. Construction, literal relocation, verified optimizer
rewrites, verifier and adapter readers move together. One payload-copy helper
preserves the remaining fields, and retained instruction snapshots survive
repeated optimization. Array-region rows and optimization admission are unchanged.

The 17-case malformed-instruction characterization passes before and after
migration with identical diagnostics. All 34 focused MIR tests pass, including
snapshot preservation and repeated optimizer application. Instruction length is
established by the record type; structural ranges and opcode, type, operand
availability, payload, origin and failure-target checks remain required.

The [local correctness record](mir-instruction-carrier-correctness.json)
retains source/compiler identities, all 426 ordinary check/repeated-QBE
observations, and the Native-compiled malformed function/block/instruction
harness. Fresh published-seed core/CLI fixed points, full CLI/project/REPL/
terminal checks, the language inventory and both project checks pass.
The initial sandbox seed download failed at DNS before compilation. A language
inventory invocation without the fixed QBE setting was cancelled because the
launcher repeatedly attempted external-tool downloads; the built CLI with the
fixed QBE passed. These setup observations are distinct from compiler behavior.

Spectral-norm at input 100 and n-body at input 1000 retain identical QBE,
executables and output against the accepted same-feature control. The local
Darwin compiler is 349,272 bytes versus 349,256, with code text decreasing from
258,664 to 256,544 bytes. The CLI decreases from 499,144 to 482,632 bytes.
These are compiler results, not a new application timing measurement. Existing
cumulative n-body alarms and deferred PR #307 remain unresolved. No Pure Go
parity, runtime speedup, seed/compatibility change or benchmark/Pages value
update is inferred.

Complete local recovery/QBE-enabled suites pass root 102/102 and compiler
142/142. The owned recovery workspace was removed after both suites terminated.
Their durations remain correctness-execution records, separate from hosted
self-build cost comparisons.

[PR #366](https://github.com/type-rb/type-rb-native/pull/366) accepts source
`f3e8844d38e51575763560f76b907724d4e955f1` as merge
`8065494a2d1a03d55642d0891dc15a703f36e957`.
[CI run 34318902941](https://github.com/type-rb/type-rb-native/actions/runs/34318902941)
passes all 17 selected authorities on its first attempt. The
[hosted acceptance record](mir-instruction-hosted-acceptance.json) retains
artifact identities, raw measurements, environments, policy and checks,
including all 88 amd64 warm/retained rows with successful process and observer
statuses. Public CI merge-ref tree identity is verified against the candidate.

| Ordinary compiler metric | Darwin arm64 | Linux arm64 |
| --- | ---: | ---: |
| Executable bytes | 349,272 | 323,240 |
| Code text bytes | 256,544 | 259,152 |
| Self-build elapsed ratio | 0.984043 | 1.003663 |
| Self-build peak RSS ratio | 1.000993 | 1.000317 |

Combined compiler size decreases from 674,608 to 672,512 bytes within the
unchanged 694,000-byte bound. Compiler QBE decreases from 1,148,167 to
1,144,662 bytes, with target-neutral SHA-256
`5eab5995b8a87fca95e8785e1e1c2da78e4d1288d76ac2793183a8edbdaa444d`.
The amd64 compiler is 274,624 bytes within its unchanged 310,000-byte bound.
No special confirmation or threshold revision was used. The amd64 adjacent
self-hosting-generation comparison is not an incremental application timing
comparison against the previous carrier slice.

## Named MIR Array-operation carrier self-use

[Issue #368](https://github.com/type-rb/type-rb-native/issues/368) registers
this slice against `2d7719681bb63067323da64fee7663636c28b81c`.
`Gate4MirArrayOperation` names kind, operand and source origin. Function-local
buffers and module regions now contain these records, and parameter ordinals
refer to operation positions without decoding three-cell offsets. Construction
preserves sticky opaque effects and final single-effect publication. The MIR
verifier retains parameter-prefix ordering, operand/origin checks, scalar and
mutable-Array exclusions, opaque barriers and forged-fact rejection. Partial
triples become unrepresentable; no new proof or optimization admission is added.

All 12 pre-migration characterization/verifier tests and 36 focused MIR tests
after migration pass. The
[local correctness record](mir-array-operation-carrier-correctness.json)
retains source/compiler identities, all 426 ordinary check/repeated-QBE
observations, and an ordinary Native-compiled malformed-MIR harness derived
from the maintained rejection tests. The temporary harness conversion initially
missed a multiline assertion; the checker correctly rejected an unresolved test
helper. Correcting that observer conversion passed without changing compiler
source or expected diagnostics. The initial setup failure is retained separately.

Both project checks, formatting, fresh published-seed core/CLI fixed points,
full CLI/project/REPL/terminal checks and the language inventory pass.
Spectral-norm at input 100 and n-body at input 1000 retain identical QBE,
executables and outputs against the accepted same-feature control. The local
Darwin core/CLI sizes stay 349,272 / 482,632 bytes; compiler code text changes
from 256,544 to 256,560 bytes. These are compiler observations, not new
application timings. Earlier cumulative n-body alarms and deferred PR #307
remain unresolved. No Pure Go parity, runtime speedup, seed/compatibility
change or benchmark/Pages value update is inferred.

Complete local recovery/QBE-enabled suites pass root 102/102 and compiler
144/144. The owned recovery workspace was removed after both suites terminated.
Their durations are retained as correctness-execution records, separately from
the hosted self-build cost comparison.

[PR #369](https://github.com/type-rb/type-rb-native/pull/369) accepts source
`a3d78ebc446ba94235f63a7946d19c76382e9abd` as merge
`7485da47fa66fd9b767d02ee88315aa4c9543a73`.
[CI run 34322428715](https://github.com/type-rb/type-rb-native/actions/runs/34322428715)
passes all 17 selected authorities on its first attempt. The
[hosted acceptance record](mir-array-operation-hosted-acceptance.json) retains
artifact identities, raw measurements, environments, policy and checks,
including all 88 amd64 warm/retained rows with successful process and observer
statuses. The public CI merge-ref tree is verified against the candidate.

| Ordinary compiler metric | Darwin arm64 | Linux arm64 |
| --- | ---: | ---: |
| Executable bytes | 349,272 | 323,280 |
| Code text bytes | 256,560 | 259,168 |
| Self-build elapsed ratio | 0.995360 | 1.003745 |
| Self-build peak RSS ratio | 0.999008 | 0.999683 |

Combined compiler size changes from 672,512 to 672,552 bytes within the
unchanged 694,000-byte bound. Compiler QBE changes from 1,144,662 to
1,144,617 bytes, with target-neutral SHA-256
`51377f43ef8c326a808c8fe4e628c6f50387d6ef98d4bf7f71eef4a8195b5e17`.
The amd64 compiler is 274,488 bytes within its unchanged 310,000-byte bound.
No special confirmation or threshold revision was used. The amd64 adjacent
self-hosting-generation comparison is not an incremental application timing
comparison against the previous carrier slice.
