# Repository ownership and decomposition

Current source names describe responsibilities. The supported recovery paths stay
separate from ordinary compilation and share implementation where their semantics
agree. Completed naming and extraction history is available in the
[historical documentation](history.md); it is not the current decomposition plan.

The directory responsibility map and enforced dependency rules are in
[Compiler source responsibilities](compiler-source-layout.md).

## Current ownership map

| Current area | Responsibility |
| --- | --- |
| `compiler/src/` | Ordinary compiler: `CompilerState`, `CheckedLocals`, `CheckedValue`, `QbeEmitContext`, and `QbeValue` use role names. Lexing and source slicing live in `compiler/src/frontend/syntax/lexer.trb`; MIR records, construction, analysis, rewrites, verification and QBE adaptation have separate modules; see the [architecture map](architecture.md). |
| `compiler/src/state/state.trb`, `compiler/src/state/source_state.trb`, `compiler/src/state/catalog_state.trb`, `compiler/src/state/local_state.trb`, `compiler/src/frontend/checking/program/generic_fork.trb` | `CompilerState` composes source, catalog, nominal and checked-local owners. Its checking mode is immutable construction configuration; ordinary states and template forks are non-interactive, while the REPL selects interactive mode before loading source. `CompilerSourceTokens` owns the shared token table and source-origin annotations; `CompilerParsedProgram` groups parsed control regions and default initializer plans. `CompilerTypeResolution` groups alias/generic resolution; `CompilerDeclarationCatalog` groups record, field, function and parameter storage plus the function name index. `CheckedLocals` owns lexical binding facts and function-local MIR construction storage. Template-checking forks share source tokens and parsed syntax while copying mutable catalogs, default plans, lookups and progress cells. |
| `compiler/src/state/resolution_locals.trb` | Body name resolution owns independent names, declared receiver types and logical lookup order. Checked binding identities, flow facts and MIR storage remain in `CheckedLocals`; anonymous-body resolution takes an ordered snapshot of its active names. |
| `compiler/src/support/storage.trb` | Integer and String storage share their direct chunk arrays across record aliases. Explicit storage copies clone every populated chunk and own their limits and high-water cells; sparse holes stay unallocated. Scalar cells retain alias-visible updates under readonly record fields and the recovery subset. |
| `compiler/src/project/package_json.trb`, `compiler/src/project/package_checksum.trb`, `compiler/src/project/package_manifest.trb`, `compiler/src/project/packages.trb` | Strict package JSON, portable SHA-256 and manifest defaults, dependency-scoped package identities and deterministic lock serialization. The compiler source loader owns filesystem reads; the CLI publishes locks atomically. Package metadata is separate from checked language declarations. |
| `compiler/src/project/config_json.trb`, `compiler/src/project/config.trb` | JSONC cursor, comments, values and object boundaries are separate from configuration records, field validation and defaults. The JSONC reader preserves the first diagnostic and its source line; strict package JSON retains its own acceptance contract. |
| `compiler/src/backend/qbe/output.trb` | QBE output appends to zero-origin String storage. Its used-line boundary is the single shared length for emission, concatenation and chunking; output aliases see every append. Appending one output to another preserves independent subsequent appends. |
| `compiler/src/mir/model/effects.trb`, `compiler/src/mir/analysis/call_effects.trb` | Named allocation, mutation, I/O and failure bits and their supported combinations have one encoding owner. Instruction classification and the distinct none/failure/conservative call-summary lattice consume those names. Numeric maximum is a join only for those three summary classes, not for arbitrary effect masks. |
| `compiler/src/state/nominal_state.trb` | `CompilerNominalTypes` owns authored class, newtype and enum declarations alongside concrete class instances and receiver lookup. Template checks share authored declarations while copying concrete object and enum catalogs and mutable receiver bindings. |
| `compiler/src/state/global_state.trb`, `compiler/src/state/callable_state.trb` | Global declarations, inferred bindings, lambda syntax and checked callable bodies have separate state owners. Template forks share authored syntax and ordering, while each fork owns inferred global types, function bindings, callable progress and checked bodies. |
| `compiler/src/frontend/syntax/parser.trb`, `compiler/src/frontend/syntax/body_syntax.trb`, `compiler/src/frontend/syntax/declaration_syntax.trb`, `compiler/src/frontend/syntax/syntax_forms.trb` | Module/declaration routing, recursive expression/statement parsing, callable/record/default declarations, and shared token forms have distinct owners. Checking and REPL adapters import shared forms directly; there are no forwarding parser aliases. |
| `compiler/src/frontend/types/object_model.trb`, `compiler/src/frontend/syntax/object_syntax.trb`, `compiler/src/frontend/types/object_types.trb`, `compiler/src/frontend/resolution/object_resolution.trb`, `compiler/src/frontend/resolution/object_methods.trb`, `compiler/src/frontend/checking/nominal/object_fields.trb`, `compiler/src/mir/verify/object_initialization.trb`, `compiler/src/frontend/resolution/object_dispatch.trb`, `compiler/src/mir/model/object_dispatch.trb`, `compiler/src/mir/lowering/object_dispatch.trb`, `compiler/src/mir/model/objects.trb`, `compiler/src/mir/lowering/objects.trb`, `compiler/src/backend/qbe/emit/objects.trb`, `compiler/src/backend/qbe/emit/object_dispatch.trb` | Authored declarations, nominal types, method specialization, initialization proofs and verified field/interface operations have separate owners. Recursive expansion shares `compiler/src/frontend/resolution/type_resolution.trb`; QBE adapts verified layouts and witness tables. Initialized superclasses and inherited overrides retain explicit guards. |
| `compiler/src/state/checked_body.trb` | Concrete function-owned checked projections; shared parsed syntax stays in the program, and backend emission needs only verified MIR. |
| `compiler/src/frontend/syntax/lambda_syntax.trb`, `compiler/src/frontend/resolution/lambda_resolution.trb` | Anonymous parameter grammar and per-body resolved signatures; recursive body parsing lives in `compiler/src/frontend/syntax/body_syntax.trb`, while `compiler/src/frontend/checking/body/lambdas.trb` and `compiler/src/mir/lowering/lambdas.trb` analyze and lower lexical captures. |
| `compiler/src/frontend/syntax/iteration_syntax.trb`, `compiler/src/mir/model/iteration.trb` | Parsed source regions are immutable syntax; concrete checked traversal plans bind those regions to receiver/element types and lexical loop owners. Executable traversal is ordinary MIR control flow. |
| `compiler/src/frontend/resolution/import_resolution.trb`, `compiler/src/frontend/checking/program/resolution.trb`, `compiler/src/frontend/resolution/enum_resolution.trb`, `compiler/src/frontend/resolution/entry_resolution.trb`, `compiler/src/frontend/resolution/body_resolution.trb`, `compiler/src/frontend/checking/collections/inference.trb`, `compiler/src/frontend/checking/body/expressions.trb` | Import path and binding checks, declaration resolution, enum payload validation, compiler entry/intrinsic identity, and function-body name binding have separate owners. The collection inference owner refines empty Array and Hash bindings and records scoped numeric candidates; recursive expression, call, member, statement, control and iteration checking has explicit mutually recursive owners; see the directory responsibility map. |
| `compiler/src/mir/build/value_control.trb` | Typed branch exits, common result blocks, selection of evaluated values and numeric join conversions; recursive source checking and REPL evaluation consume shared frontend regions. |
| `compiler/src/frontend/checking/builtins/string_methods.trb`, `compiler/src/backend/qbe/runtime/string_queries.trb`, `compiler/src/backend/qbe/runtime/string_sequences.trb`, `compiler/src/backend/qbe/runtime/string_slices.trb`, `compiler/src/backend/qbe/runtime/string_trimming.trb` | Receiver method construction and bounded query/sequence/slice runtimes are separate; verified String operation contracts stay in `compiler/src/mir/lowering/strings.trb`. |
| `compiler/src/frontend/syntax/string_escapes.trb`, `compiler/src/backend/qbe/runtime/string_literals.trb` | Escape syntax and validation are separate from the lexer scanner and the private byte-construction runtime. |
| `compiler/src/frontend/checking/builtins/array_methods.trb`, `compiler/src/frontend/checking/builtins/array_queries.trb`, `compiler/src/frontend/checking/builtins/array_sorting.trb`, `compiler/src/backend/qbe/runtime/array_copies.trb`, `compiler/src/backend/qbe/runtime/array_mutations.trb` | Array method construction, query/sort loops, copy allocation and in-place insertion/removal have separate owners. Types, effects, loop safety and roots remain MIR responsibilities. |
| `compiler/src/frontend/resolution/default_arguments.trb` | Private initializer declaration identities and preceding typed slots; ordinary checked functions and MIR own their bodies and calls. |
| `newtype_*` modules in `frontend/syntax`, `frontend/types`, `frontend/resolution` and `mir` | Declaration syntax, nominal identities, representation/mutability resolution, method lookup, MIR construction and independent storage verification. Backend adapters use MIR storage facts while checking retains nominal identities. |
| `compiler/src/backend/qbe/runtime/hash_key_runtime.trb`, `compiler/src/backend/qbe/runtime/hash_runtime.trb` | Scalar/union key hashing and probing are separate from table allocation, growth, deletion and snapshots. `compiler/src/mir/lowering/hashes.trb` owns checked key layout selection. |
| `compiler/src/frontend/checking/program/web_endpoints.trb`, `web_openapi.trb`, `web_openapi_schema.trb` | Owned endpoint declaration checking, checked-catalog validation and OpenAPI schema semantics. The CLI owns file publication; `project/web_openapi_json.trb` owns deterministic JSON serialization. |
| `compiler/src/frontend/checking/program/web_client.trb`, `web_client_types.trb`, `web_client_request.trb` | Checked browser-client source generation, shared-type import identity, request encoding and deterministic source formatting. These consumers do not execute application code or emit target-machine operations. |
| `compiler/cli/repl/project.trb` | REPL project discovery, generated-import filtering and visible nominal type names. Session checking/evaluation remains in the REPL adapters. |
| `compiler/cli/repl/operators.trb`, `compiler/cli/repl/eval.trb` | Checked scalar value operations and portable arithmetic failures are separate from token evaluation, calls and control flow. Operator dispatch unwraps values and promotes numeric operands before selecting Integer or Float evaluation; it does not repeat compiler type checking. |
| `compiler/src/frontend/checking/program/submission.trb`, `compiler/cli/repl/check.trb` | Ordinary checked entry/exit facts and result types, plus REPL source loading and boundary mapping. Session completion and replay own fact persistence; QBE consumes only verified MIR. |
| `recovery/src/snapshot/validation.trb` and shared snapshot/diagnostic/MIR modules | Snapshot boundary validation and shared support. |
| `recovery/src/scalar/`, `recovery/src/aggregate/`, `recovery/src/managed/` | Retained scalar, aggregate and managed snapshot recovery, including layout, QBE, runtime and differential tests. These paths cover distinct supported capabilities. |
| `recovery/src/driver/main.trb`, `recovery/src/driver/generation.trb`, `recovery/src/driver/matched_go.trb`, `recovery/src/compiler/source.trb`, `recovery/src/compiler/layout.trb` | Recovery orchestration, comparison and strict derivation from the canonical compiler modules. Ordinary builds keep their file-root closure. |
| `compiler/conformance/`, `corpus/`, `fixtures/` | Active correctness cases, grouped by feature or recovery capability. Names and callers move together. |
| `tools/recovery-bootstrap.sh`, `normalize-compiler.sh`, `linux-amd64-targets.sh`, `external-qbe-build.sh`, `measure-command.py` | Recovery, deterministic linking, target validation and external measurement. |
| `.github/workflows/native-validation.yml`, `linux-amd64-targets.yml` | Current correctness/recovery and target authorities. |
| `results/`, dated plans/decisions, immutable seeds | Historical evidence with original names, commands, hashes and revisions. |

The compiler's private generated QBE symbols use `trbn` rather than an experiment
number. Current recovery driver commands and GC report prefixes describe their
roles. No stable public protocol or snapshot version is changed by this rename.

## Remaining consolidation

The [MIR milestone](mir-consolidation.md) owns the remaining structural work.
Accepted ordinary functions now require verified typed values, operations and
control flow. The direct emitter and its token Array/header/assignment analyses
are removed. Numeric expansion, Array-header validity and root plans are selected
and verified above QBE; function ABI emission lives in `compiler/src/backend/qbe/emit/functions.trb`.
Split the large checker, MIR and emitter modules by those responsibilities, with
explicit dependencies rather than copied helpers or forwarding aliases.
The value-join builder and nullable type, flow-fact, MIR, QBE and REPL helpers
have been extracted. Expression and body checking use separate mutually recursive
owners. Ordinary imports permit cycles within a compilation unit, with checked
initialization dependencies. The accepted cycle-capable seed and historical
consumer bridge support this compiler source shape; ordinary and recovery
validation exercise the canonical nested module closure.

The checked-body ownership change moves type applications, nullable, enum, Result,
Hash, Range and control projections together. Its exit criteria are independent
facts at identical source origins, caller restoration in the REPL, unchanged
emission after projection erasure, and complete ordinary/recovery checks.
See [decision 0047](decisions/0047-checked-body-ownership.md).

Generic function orchestration is split into instance declaration, an isolated
semantic program fork, and template checking. Shared syntax stays immutable;
concrete body facts and MIR remain function-owned. These modules extend the same
source-erasure and ordinary/recovery checks without a second backend path; see
[decision 0048](decisions/0048-generic-function-mir.md). Generic record defaults
reuse the ordinary record parser and private initializer path. The shared
`compiler/src/frontend/types/generic_bindings.trb` owns declaration-scoped substitutions for functions and
record defaults; separate function/nominal abstract identities prevent accidental
capture. See [decision 0049](decisions/0049-generic-record-default-mir.md).

Iteration ownership separates parsed `IterationSyntax` from the concrete
`MirIteration` projections. Parsing no longer constructs placeholder receiver
types or loop owners. Checking, transfer validation, and REPL execution share
the same immutable regions; independently verified MIR remains the only input
to backend execution. This preparation for value-producing traversal preserves
existing Array/Range `each` semantics. Its exit criteria are unchanged ordinary
and REPL behavior, malformed-plan rejection, emission after source erasure and
typed traversal plan invalidation, and ordinary/recovery fixed points.

Keep source moves and their recovery derivation, imports, tests and operational
consumers together. Useful shared code remains one implementation. Complete
cohesive ownership changes without requiring another performance qualification
for each helper extraction. Required correctness and reproducibility remain blocking.

Semantic names must preserve their meaning across producers, validators and
consumers. Effect names are parameterless Integer-literal-return functions so
ordinary compilation and the recovery subset share one representation.
`mir/passes/literal_calls.trb` replaces eligible verified direct calls with the
literal while preserving the caller's value identity and origin; it does not
recognize names or expand general helper bodies. Representation changes still
need generated-code and compiler-cost checks. Ordinary numeric data, layout
encodings and algorithm constants need no cosmetic wrapper.

## Current source and historical consumers

The [compiler-name seed handoff](bootstrap-seed-updates.md) removed the predecessor
intrinsic declarations, wrappers and dual recognition. MIR admission, intrinsic
calls and ordinary body omission share declaration-bound identity. Extracted helpers and imported aliases resolve to the same owner; unrelated
same-named functions retain ordinary calls. CLI adapters follow the imported core
compiler for that ownership. The source-name
check covers current paths, implementation names, comments and visible Markdown; immutable history links and result records remain reproduction evidence.

Cross-revision measurement tools retain a historical project-layout resolver and
explicit deletion/rename tests. Frozen standalone comparisons and the historical
portable-entry workflow use their exact old source paths. Initial-root metadata,
immutable seeds and recorded results retain their authenticated identities.
These are reproduction inputs, not alternate current implementation owners.

See [compiler layout](compiler-project-layout.md), [retired controllers](retired-experiment-tools.md)
and [validation](ci-validation.md) for those boundaries.

`compiler/src/backend/qbe/runtime/array_join.trb` owns the Array-to-String assembly runtime;
typed validation and effects remain in the shared String MIR and root planners.
