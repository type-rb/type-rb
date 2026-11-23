# Source-module global bindings in MIR

Status: accepted implementation direction; basic-language completion remains
owned by [issue #454](https://github.com/type-rb/type-rb-native/issues/454).

## Contract

Follow the accepted reference's top-level lowercase bindings. A declaration has
source-module identity and an immutable or mutable binding capability. Functions,
closures and default expressions use that identity even when another scope later
declares the same spelling. Lowercase storage is not a named or bare import export.
An imported function still reaches its own module's storage. Ordinary readonly
alias restrictions apply to globals as they do to local bindings.

The reference update in [TypeRB #782](https://github.com/type-rb/type-rb/pull/782)
fixes cross-target identity and invalidation of nullable proofs after calls that
may replace mutable globals. Native follows those corrected semantics. Fresh
guards and immutable globals retain valid proofs; local shadowing is independent.

## Ownership

The parsed and checked global catalog replaces the constants-only catalog while
retaining uppercase constant behavior. MIR global rows carry exact semantic type,
mutability, initializer identity, source origin and persistent-root requirements.
Instruction 51 reads storage and instruction 59 writes an available value of the
declared type. Independent verification rejects missing or immutable targets,
invalid operands and operation shapes, and direct uninitialized accesses.

Nullable flow uses versioned declaration identities for globals as well as locals.
Assignments, possibly mutating calls and branch/loop joins invalidate old facts.
QBE consumes verified rows and operations mechanically; it does not rediscover
binding, type, initialization or nullable semantics. Global root slots always
trace the current managed value, and ordinary local roots keep replaced values
alive while they remain reachable.

Retained REPL programs key project globals by source and declaration identity,
so catalog changes do not move storage. Compaction preserves aliases and closures;
explicit replay starts fresh and reexecutes the submitted effects as before.

## Verification and remaining boundaries

Paired file check/build/run and REPL cases cover scalar/managed values, defaults,
closure reads and writes, lexical shadowing, independent modules, mutability,
nullable replacement, stale proofs and rejected declarations/imports. Independent
MIR controls erase the frontend and reorder catalogs before execution. Forced-GC
tests retain replaced Unicode/NUL values, release mutable roots and check exact
reclamation; project sessions cover retained function values and replay.

Interactive variables visible to later named functions are extended by
[decision 0081](0081-repl-global-identity.md), and lowercase namespace-body bindings
by [decision 0082](0082-namespace-binding-mir.md). Forward initialization and untyped
empty collection inference remain explicit pending contracts. Compiler state
remains per invocation; compiler-source use of new module-storage syntax depends
on an accepted immutable seed handoff. These boundaries remain in the generated
inventory, and this change makes no full-language or performance claim.
