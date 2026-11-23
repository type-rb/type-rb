# Ordinary Hash implementation

Status: initial ordinary implementation accepted in [PR #401](https://github.com/type-rb/type-rb-native/pull/401).
The separate [Hash cost decision](decisions/0033-hash-compiler-budget.md)
records that acceptance budget. The [ordinary-language registry](../tools/native-language-cases.json)
tracks subsequent coverage and remaining gaps; this document is not a claim of
complete TypeRB library support.

## Language boundary

Use the pinned TypeRB revision as the semantic authority. This slice supports
homogeneous Integer or String keys and existing scalar, Array, named record,
and recursively typed Hash values. Literals support `key => value`, String
label keys, contextual `Hash<K, V>` annotations, and a fresh mutable empty
binding refined by its first indexed write. Values written within the same
`if` or `case` can join numeric, `Nil`, and union alternatives; later
separate assignments and explicit Hash annotations retain their declared type.
An inferred Hash whose first value is `[]` uses `Array<Any>` as its value
boundary. Later differently typed Array elements are boxed through verified
MIR, so each stored Array can grow independently. Explicit `Array<Any>` and
`Hash<K, Any>` storage use the same representation; dynamic operations on an
extracted `Any` are still tracked as separate language coverage.
Function parameters/results and
record fields carry concrete Hash types. Existing immutable rules remain in
force; copy is shallow and iteration order is unspecified.

Indexed reads and `fetch` require a present key. Indexed writes insert or
replace. The method subset is `size`, `empty?`, `key?`, `delete`, `dup`, `merge`,
`update`, `keys`, `values`, and `fetch`. Deletion also requires a present key.
The returned Arrays use the ordinary Array element implementation. `try_fetch`
uses a structured Result, and Hash iteration blocks use entry snapshots. The
registry records the precise receiver and value combinations still pending.

An unresolved empty Hash can survive a REPL submission and answer `size` and
`empty?`; subsequent indexed insertion establishes its key/value type. Ordinary
compiled code needs a concrete type by the end of checking. Aliases of an
untyped binding require an explicit annotation. Hash keys use the shared UTF-8
String representation; additional Unicode APIs remain separate coverage work.

## Representation and ownership

`compiler/src/frontend/types/hash_types.trb` owns canonical type decomposition and managed layout
classification. `compiler/src/mir/model/hashes.trb` owns the named `MirHashOperation` carrier,
mutation requirements and literal capacity planning. The checked frontend
publishes one plan per operation; `compiler/src/frontend/checking/collections/hashes.trb` verifies source boundaries,
operation kinds, key/value metadata and mutation permissions before MIR construction
or REPL evaluation. `compiler/src/mir/lowering/hashes.trb` publishes typed allocation, lookup, store and
method instructions. `compiler/src/mir/lowering/collection_inference.trb` finalizes checked first-store types
across CFG arguments and parameters before publication. It inserts the required
Float, nullable, or union representation at a joined store, without speculative
allocation for exact-type stores; raw MIR verification
never infers or repairs malformed types. Independent empty bindings retain
independent constraints.

Admitted functions use `compiler/src/backend/qbe/emit/hashes.trb` without reading source operation plans.
Stores retain the evaluated Hash and key across RHS effects. `merge` becomes an
explicit copy followed by update, so the copied table is an SSA value with a
verified live root at the allocating update. Shared MIR effects and liveness own
managed roots for lookup results, updates, deletion and returned Arrays. Direct
functions with remaining intrinsic/result boundaries still consume checked source
plans; those paths remain until their enclosing functions migrate.

The runtime uses open addressing with linear probing, a power-of-two capacity
and a maximum occupied load of three quarters. A compact Hash header references
two word Arrays: one for keys and one for values. Each slot costs two machine
words; the Hash itself adds no per-entry box, while `Any` and union values
carry their own tagged boxes. Integer keys encode the entire portable range;
String keys compare length and bytes. Integer mixing and String hashing are
independent of the frontend representation.

Existing precise GC Array descriptors trace only managed keys/values. The Hash
header is 48 payload bytes plus its descriptor word; the existing collector's
allocation bookkeeping is additional. A removed value cell is cleared at once.
String deletion replaces its key with a static tombstone. An empty Hash drops
both backing Arrays; sparse tables shrink geometrically and accumulated
tombstones are rebuilt during insertion. These operations preserve Hash
identity and shallow aliases. GC reclaims detached backing and unreachable
payloads on collection; clearing a reference does not promise an immediate
reduction in OS resident memory.

The adapter roots the receiver and computed key across the right-hand side.
It finds the destination only after that expression finishes, so an allocating
call that grows or changes the table cannot leave a stale entry address. Reads
and deletion preserve managed results across subsequent allocation. The runtime
uses the existing collector and its cycle handling, without a separate GC kind.

## Stateful REPL

`compiler/cli/repl/model.trb` separates value identity/storage from rendering and evaluation.
`compiler/cli/repl/hash.trb` implements hash-table buckets containing pool identities, with
content equality for String keys. Its bounded byte scan reuses the internal
CLI byte reader without allocating a String per character. Scalar pool entries
share a read-only empty child list; language aggregate lists remain independent.
Deletion clears both bucket references;
empty tables release their Arrays and sparse ones shrink. `compiler/cli/repl/values.trb`
traces live Hash keys/values during the existing between-submission pool
compaction, preserving aliases and cycles. Runtime state is not reconstructed
by replaying previous input.

The REPL's existing value pool retains evaluation temporaries until the current
submission completes. A long loop can therefore use substantially more memory
than its live Hash entries. Bucket reclamation and whole-session RSS are tested
and reported separately. This slice does not introduce collection of active
interpreter stack temporaries.

## Verification and seed boundary

`compiler/src/frontend/checking/collections/hashes_test.trb` covers typing, immutable rejection, operation
metadata and malformed plans. `tools/native-hash-test.py` runs ordinary and REPL
fixtures, automatic-GC stress, cycles, alias preservation, recoverable missing
keys, and a compiled bucket-storage probe. Pass `--reference /path/to/trb` to
also verify the fixtures against the pinned reference compiler. The ordinary
CLI suite includes Hash fixtures; the recovery closure includes all four new
core modules and verifies their missing/malformed/mutated source boundaries.

The compiler uses `Hash<String, Integer>` for module names, module-qualified
function declarations and project source ordering. Function declarations enter
the index when parsing succeeds. A missing name remains `-1`; replacing an entry
preserves the previous last-declaration lookup rule without changing duplicate
diagnostics. Declaration vectors retain their stable ordering and identities.

This removes the former custom symbol index and its bucket/chain/growth/rebuild helpers.
`find_name_index` and `function_index_key` own the small lookup boundary; the
ordinary Hash runtime owns storage and content equality. The key separates the
decimal module identity from the name with `:`. Import-cycle traversal now owns
an invocation-local color array, instead of overwriting function-index storage.

Compiler self-use starts from the verified Hash seed in PR #405. Recovery uses
only its accepted String/Integer Hash subset and existing integer formatting;
ordinary `Integer.to_s` is outside that snapshot subset. No new snapshot
operation or broader source fallback is needed for the indexes.

## Compiler self-use recovery boundary

[Issue #403](https://github.com/type-rb/type-rb-native/issues/403) establishes
the prerequisites for replacing compiler name indexes. The reference pin
`bae19032aa1bb7b263bc827d02606edc6e981c52` supplies internal snapshot-v4 data
for `Hash<String, Integer>` construction, indexed assignment, required lookup
and key presence. It retains source origins and receiver/key/RHS order, with
explicit rejection of other Hash shapes and operations in this recovery path.
Ordinary Hash coverage remains wider.

The recovery MIR checks canonical key layout, Hash receiver identity, operand
types, entry-list arity and source origins. `recovery/src/managed/hash_runtime.trb` owns
only this snapshot ABI's storage: content-hashed String buckets, Integer values,
geometric growth and the existing precise collector's root frames. It is emitted
only for snapshots containing Hash values; existing snapshots keep their output.
Tests exercise empty and duplicate keys, record/closure aliases, growth, forced
collection, missing-key failure and malformed metadata.

Linux amd64 keeps the immutable initial root and existing setup bridges, then
builds exact accepted Hash source `8a6d9ff73b14a97bca1b010ddaad6a38972b5373`
before reading candidate source. Its process traces, source/compiler identities
and capability probe are setup-only. Ordinary candidate B2/B3/B4 and all existing
bounds remain unchanged. This neither republishes an old seed nor substitutes
snapshot recovery for the ordinary chain. PR #405 pairs this recovery support
with the
[verified checkout seed handoff](bootstrap-seed-updates.md#current-verified-checkout-seed);
compiler index adoption uses these same operations.
