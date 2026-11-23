# 0067: Declaration namespaces and constant MIR

Nested and reopened `module` declarations are lexical namespaces within a source
module. Source-module identity remains the import and loading boundary. A
declaration is selected by that source identity and its qualified owner, so
same-spelled methods, constants and nominal types in different owners stay
distinct. Lexical lookup searches the current owner, its parents, then the root
and imports. Private methods retain their canonical declaring owner even through
an imported alias.

The parser records namespace ownership and expression boundaries. Checking
records the selected declaration and consumed token span for the REPL. Qualified
nominal applications, aliases, defaults and enum patterns retain the same owner
through instantiation. QBE receives resolved functions, types and values; it does
not interpret namespace spellings.

## Runtime-initialized constants

Top-level and module constants have ordinary expression initializers. Each
initializer becomes a private zero-argument checked function; the expression-body
mechanism is shared with declaration-owned defaults. Type inference checks those
expressions without executing them before the MIR signature catalog is frozen.
Imported dependencies precede their consumers, and constants within a source
retain declaration order. A configured project checks all its loaded production
sources, but runtime initialization follows only the entry's import closure.
Unreferenced sources still receive static diagnostics without executing their
initializers. A file-root build uses the closure selected by its loader. REPL
imports contribute their ordinary dependency closures. Native test-root loading
remains part of the test-runner coverage work.

`MirGlobal` stores a declaration identity, source origin, exact type, initializer
function identity, runtime membership and managed-root classification. An explicit
initialization list names identities, independently of catalog row order. Instruction 51 reads
a global without source operands. Its contract includes failure if initialization
has not completed. Verification checks catalog uniqueness, the full order,
initializer signatures and bodies, result types, origins and persistent-root
classification. It also rejects direct and transitively known ordinary-call
reads of a later global or a runtime initializer's dependency on an inactive
global. Verified indirect calls retain their usual effects;
runtime checks protect global reads reached dynamically.

The adapter creates typed storage and initialized flags from verified MIR, calls
each runtime initializer once before authored entry code, and publishes its result
before resetting initializer temporaries. Managed globals have a generated root-address
vector which the collector scans. It has no fixed slot limit and does not reuse
the external-root array. Earlier initializer results remain rooted while later
initializers allocate. Nullable constant facts use declaration identities rather
than lexical binding slots, preserving guards across qualified spellings, scopes
and calls without pretending a constant is a mutable local.

Constant bindings cannot be reassigned, used as destructive receivers, or promoted
to mutable reference aliases. Scalar copies can be mutable. A source function's
`mut` parameter remains an implementation binding, following the reference
parameter contract; it is not a caller-side ownership mode. This removes the
earlier Hash-only call restriction without changing direct receiver checks.

## Interactive state and verification

The REPL keys persistent values by source and declaration identity, rather than
current catalog indexes. It evaluates new initializers once, preserves reachable
values and their checked programs during compaction, and reconstructs the state
on explicit replay. A failed initializer does not commit its new bindings;
already-performed effects remain observable. Type queries do not initialize
values. Submission framing uses the ordinary parser's literal decisions, so a
keyword Symbol is a value while a named callback argument can open a real body.

Independent tests erase initializer source and frontend constant catalogs, reorder
MIR globals, forge metadata/read shapes and initialization order, and force GC
between initializers and before entry. The ordinary CLI test retains twenty
managed globals, closures and imported values across collection, failures and
declaration-only replay. The immutable bootstrap seed still constrains syntax
used by the compiler implementation itself; it is not replaced by this change.

Forward initializer semantics remain tracked in [TypeRB #755](https://github.com/type-rb/type-rb/issues/755).
Bare module method declarations, imported inferred constant types and qualified
generic record alias constructors retain their reference boundaries in
[TypeRB #756](https://github.com/type-rb/type-rb/issues/756),
[TypeRB #758](https://github.com/type-rb/type-rb/issues/758) and
[TypeRB #759](https://github.com/type-rb/type-rb/issues/759).
Module generic methods and direct module-method function values remain rejected
where the reference requires a supported declaration or explicit `fn` wrapper.
Untyped empty Array/Hash constants still require annotations in Native; their
reference inference remains visible in the shared cases. Top-level lowercase
bindings and classes with their constants remain in the completion inventory.

The complete compiler snapshot is 69,813,214 bytes, so compiler recovery uses an
80 MiB input bound instead of 64 MiB. Exact-limit and one-byte-over controls cover
both this entry and the unchanged ordinary 4 MiB entry. This is a bounded budget
for verbose recovery metadata, not a binary-size or application-memory limit;
the normal file-read default and other snapshot structural limits do not change.
