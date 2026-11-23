# Compiler source responsibilities

The ordinary compiler entry is `compiler/src/compiler.trb`. Other production
modules live under the following responsibility owners. Redundant filename
prefixes are removed when the directory supplies the same context; meaningful names and
function names are retained. `compiler/cli` composes the core with CLI and
REPL adapters. `recovery/src` contains the independent recovery implementation.

| Owner under `compiler/src` | Responsibility | Permitted other owners |
| --- | --- | --- |
| `frontend/syntax` | Scanning, escape decoding, parsed declarations and source regions | types, resolution, state, support |
| `frontend/types` | Language type models, nominal identities and substitutions | resolution, state, support; the two projection exceptions below |
| `frontend/resolution` | Import and declaration lookup, specialization and argument binding | types, syntax, providers, state, support, MIR, project |
| `frontend/checking` | Expression, statement and body checking; checked operation construction | types, resolution, syntax, providers, state, support, MIR, project |
| `frontend/providers` | Loader-owned package declarations and checked-schema specialization into ordinary TypeRB bodies | types, resolution, syntax, state, support, project |
| `state` | Compilation state and function-owned checked projections | types, support, MIR; the parsed-iteration exception below |
| `mir` | Target-independent IR, construction, proofs, passes and verification | checking, types, syntax, resolution, state, support |
| `backend/qbe` | QBE serialization and emitted runtime and data routines | MIR, state, support, types; the intrinsic-identity exception below |
| `project` | Configuration, packages, standard-library source catalog and host files | support, syntax, providers, state |
| `support` | Storage, path and literal helpers, Unicode identifier data | none |

Owners may import themselves. The entry composes core owners. Reciprocal
frontend/MIR imports are permitted; a directory cycle is not an error. These
rules constrain architecture independently of the language's compilation-unit
and initialization rules.

Four declaration-specific edges supplement the table:

- `backend/qbe/emit/constants` imports `frontend/resolution/entry_resolution`
  for declaration-bound intrinsic identity.
- `frontend/types/transform_model` imports `frontend/syntax/iteration_syntax`
  and `mir/model/iteration` for its shared parsed and checked projections.
- `state/source_state` imports `frontend/syntax/iteration_syntax` for the parsed
  iteration regions retained by `CompilerParsedProgram`.

The exceptions do not grant other modules access to those whole layers.
Core modules cannot import CLI adapters. Production modules cannot import test
modules, `testing` helpers or `trb/std/test`. Unit tests live beside their owner
as `<filename>_test.trb` for `<filename>.trb`, even when preparing an input uses
other components. Tests may exercise the complete core pipeline and share
explicit helpers under `testing`, but cannot import another test file.
Compiler system suites live in `compiler/src/tests/compiler/`, split into source
loading, scale and execution contracts. CLI tests can exercise the composed CLI
and core. All discovered `.trb` files, including
modules outside the entry's reachable closure, participate in the ownership
check; unknown directories, unresolved imports and composed path collisions fail.

Test placement follows the contract being verified, not the number of imports
or whether setup runs QBE. Feature contracts spanning checking, MIR and execution
belong in `tests/<family>` using the shared language registry's family ids.
System contracts belong with their subsystem. Local invariants remain colocated;
move unit tests with their production owner when splitting a directory. A large
local suite may have concern-specific `<filename>_<concern>_test.trb` companions.
Do not create tests merely to match every implementation file.

Split a mixed suite by its individual guarantees. Malformed escape rejection
belongs beside the decoder; literal execution across the pipeline belongs in
`tests/lexical`. Argument evaluation with source erasure and forced collection
belongs in `tests/arguments`, while binding identities remain beside the
resolver. Intrinsic declaration identity remains a resolver unit even when
execution checks that ordinary sibling declarations are not intercepted.
Malformed MIR rejection belongs beside the verifier, independently of where
the test obtains or constructs its MIR fixture.

`testing` contains fixtures, pipeline adapters and execution helpers without
test-case registration. Dependencies run from tests to helpers to production;
production cannot import helpers, and helpers cannot import test cases. These
are repository roles, not new TypeRB directory semantics. Staging and content
keys exclude `testing`, `tests` and `_test.trb` files using the same selection.
The ordinary compiler import closure and recovery inventories contain no test
helpers. CLI and recovery-specific helpers belong with those subsystems.

## Responsibility subdirectories

| Area | Subdirectories |
| --- | --- |
| `frontend/checking` | `program` (declarations, generics and submissions), `body` (expressions, calls, members, statements and control), `builtins`, `nominal`, `collections` |
| `mir` | `model`, `build`, `lowering`, `analysis`, `passes`, `verify` |
| `backend/qbe` | `emit` for verified-MIR adaptation, `runtime` for emitted support routines and tables; shared context and output stay at the owner root |
| `compiler/cli` (outside `src`) | `repl` for session orchestration, checking, evaluation, values and terminal interaction; the CLI entry, host adapter and diagnostics stay at the CLI root |
| `recovery/src` | `snapshot`, `scalar`, `aggregate`, `managed`, `compiler`, `driver`, `support`, plus test-only `testing` and `tests` |

Around 25–30 production files in one directory or more than three directory
levels below `compiler/src` prompts a responsibility review; neither is a limit.
Split cohesive responsibilities, not arbitrary batches. Test counts alone do
not justify moving a local invariant away from its owner. For example,
`mir/lowering/arrays.trb` keeps `arrays_test.trb` beside it, while an Array
execution contract spanning the toolchain lives under `tests/arrays/`.
A shared runtime helper can support a colocated unit without changing its role.

`frontend/checking/body/expressions.trb` owns primary and precedence-based
expression checking. Calls, collections, members, statements, control and
iteration have separate body owners; `lambdas.trb` owns lambda body checking.
Their explicit imports may be mutually recursive; there are no forwarding
copies. Checking constructs independently verified MIR; QBE consumes those facts.

Authored QBE runtime functions use word predicates for conditional branches.
QBE `jnz` examines only the low 32 bits: compare a pointer or other long value
with `cnel %value, 0` before branching on whether the complete value is nonzero.
Use the same explicit spelling for long Boolean flags, so reviewing a condition
does not require reconstructing its possible range. The source preflight checks
complete authored functions; runtime boundary tests exercise valid pointers with
zero low words and exponents across the 32-bit boundary.

## Semantic names and numeric encodings

Use names at both the producer and consumer of a semantic tag. Equal numbers
do not imply equal meanings: source operators, specialized MIR operators,
receiver methods, checked plans and type identities have independent owners.
Keep the mapping in that owner, including ordered group boundaries and offsets.
For example, `mir_op_string_index()` identifies an instruction, while
`string_method_slice()` identifies a checked receiver method. Neither is a
replacement for an arbitrary `instruction.payload` with the same value.

The compiler currently expresses these names as ordinary nullary functions
returning a nonnegative Integer literal. Verified MIR folds eligible calls;
the language has no new constant or enum syntax for this convention. Check the
generated code and compiler cost when introducing a different representation.
In particular, a helper returning a negative expression does not automatically
receive the same fold.

| Semantic domain | Definition owner |
| --- | --- |
| Token, parsed control, default and checking phase tags | `state/*_kinds`, `state/checking_phases` |
| Shared checked-state slot indices and packed use obligations | `state/local_slots`, `state/binding_kinds` |
| Source operators, checked operator plans, declaration receivers and methods | `frontend/types/operators`, `operator_plans`, `value_categories`, `method_kinds` |
| Declaration, generic, attribute, iteration and transform kinds | Corresponding `frontend/types/*_kinds` modules |
| MIR instructions, types, control, calls, Hash actions and numeric proofs | Corresponding `mir/model` modules |
| Instruction-specific conversion, query, copy, guard and field modes | `mir/model/operation_modes` |
| Parsed JSON node categories | `project/json_kinds` |
| Loader-owned standard source identities | `state/standard_source_kinds` |
| Shared QBE runtime requirement slots | `backend/qbe/runtime_slots` |
| REPL display, control flow and input status | `repl/display_kinds`, `flow_kinds`, `input_status` in the CLI |

Name shared-array indices without changing alias ownership: writes must remain
visible to the other holders of the same state cell. A sole `[0]` cell, ordinary
counts, source positions and SSA identities remain numbers. Negative checked
categories encode `-value_id - 1`; nullable replacement barriers and unknown
analysis states need their own explanations, not a shared "invalid" constant.

Keep ABI layouts, generated catalogs, protocol versions, Unicode tables and
algorithm data at their existing boundaries. Tests that independently assert an
encoding or deliberately forge invalid MIR may retain literal values. Such
fixtures should not derive their expected value from the implementation being
tested. Regenerate recovery inventories with `tools/recovery_layout_sync.py`
after moving or adding an owner; do not maintain a second mapping in recovery.

The remaining numerical boundaries are intentional:

- The official source generator assigns catalog IDs from 100. Handwritten
  loader code names that boundary; the generated catalog and upstream source
  bytes remain generator-owned.
- Process and Math use negative virtual import targets. Their translation to
  checked receivers and runtime adapters is documented at the translation
  sites, rather than treating the targets as source module IDs.
- MIR payloads can hold SSA operands, literal indices, capacity limits or an
  unused zero. Use an operation mode only when that opcode defines one.
  Runtime assembly strings preserve the corresponding wire representation.
- Callable flow distinguishes unresolved and unknown negative states;
  nullable facts distinguish a binding from a replacement barrier.
  These meanings stay with their analysis owners.
- One-cell mutable flags, terminal bytes, OS exit codes, word offsets,
  allocation sizes, hash parameters and checksum constants are data at their
  existing boundaries. Naming semantic tags does not require renaming them.

Recovery has its own `recovery/trbconfig.jsonc`, project name
`type-rb-native-recovery`, and Go module
`github.com/type-rb/type-rb-native/recovery`. Compiler production code cannot
import recovery. Recovery imports stay within its own root and read ordinary
compiler sources as data. Similar MIR/QBE names do not imply that independently
validated recovery implementations should share ordinary compiler code.

## Checking and repeating the migration

`python3 tools/compiler_layout.py` checks the ownership rules in preflight and
quick CI. Its synthetic tests demonstrate that permitted cycles pass and that
forbidden ownership edges, test-to-test imports and misplaced unit filenames
still fail. Filename checks support the placement rule; reviewers decide the
contract a suite guarantees. The complete compiler unit runner discovers
nested tests dynamically.

When rebasing source changes, retain the accepted helper extraction and test
splits first. The migration tool relocates assigned modules and imports from
flat or previous nested identities; it does not reconstruct historical test
splits. Then run:

```sh
python3 tools/migrate_compiler_layout.py --write
python3 tools/migrate_compiler_layout.py
python3 tools/recovery_layout_sync.py --check
python3 tools/compiler_layout.py
```

The migration manifest records module assignments, checker function ownership
and explicit current tooling consumers. A newly added flat module or checker
function needs a reviewed assignment before migration. The tool validates all
assignments and destinations before writing, retains function bodies, rewrites
leading imports and updates the registered current consumers. Repeating it is
idempotent. It also regenerates the canonical recovery inventories with the
shared synchronization tool, preserving reviewed mutation needles when their
module paths change and the needles still match exactly once.

Embedded test programs, recovery imports, immutable evidence and historical
cross-revision consumers are outside general import rewriting. Current probes
that deliberately import the compiler and own-frontend identity assertions have
explicit replacements. Frozen source paths retain their original identities;
`tools/compiler-project.sh` continues to resolve historical project layouts.

Source migration requires the accepted cycle-capable checkout seed and its
historical consumer handoff. Validate the resulting source with the exact pinned
reference, ordinary core/CLI fixed points, shared language contracts, and complete
hosted recovery and target checks. The count of production modules at the root
is a layout metric; passing ownership and execution checks establish the result.
