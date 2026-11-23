# Private failure recovery for Web dispatch

Status: implementation decision for [task #1197](https://github.com/type-rb/type-rb-native/issues/1197).
Integration evidence belongs to its implementation PR.

## Contract

The pinned reference dispatcher recovers ordinary handler and middleware panics
into an internal-server-error response. Its core boundary returns that response
through the root middleware's Next call, allowing the root to continue. A root
middleware failure abandons its remaining statements through the outer boundary.
HEAD responses still suppress their body. Observable effects before a failure
remain; failed statements do not resume and handlers are not retried.

This implements that existing contract without public exception syntax. The
synchronous testing dispatcher has no cancellation or deadline input. Network
execution scopes remain a separate requirement of Decision 0086; their typed
cancellation failures must propagate rather than entering this ordinary recovery
class.

## Ownership and execution

The internal loader supplies `Runtime.recover<T>(action: () -> T, fallback: T): T`.
The frontend admits only the complete owned declaration. MIR standard adapter 17
retains the concrete callback, fallback and result identities and conservative
allocation/mutation/I/O/failure effects. The independent verifier checks this
contract before source-erased emission. Ordinary TypeRB dispatch code selects
where to invoke recovery; the backend has no route or middleware policy.

A small QBE runtime trampoline owns libc `setjmp` and `longjmp`. Only that
trampoline has a returns-twice call. Before it, an aligned stack frame records the
parent scope, root count, closure and fallback. Those fields are not modified by
the callback. After either return, the trampoline restores the previous scope and
temporary-root count; it never preserves a pointer to the movable root buffer.
Separate Integer/pointer and Float adapters preserve the callback result ABI.
Caller live-before roots retain the closure, its captures and the fallback.
No allocating operation occurs between dropping abandoned roots and returning.

The system jump environment occupies a bounded 1024-byte region aligned to 16
bytes inside that frame. Target validation compiles size and alignment assertions
against its libc headers and executes nested and repeated recoveries. The scope
is process-local under the existing single-mutator model. This is not a foreign
callback, multithreading or cooperative scheduling ABI.

Language failures enter the current scope when one exists. Without a scope they
retain their existing diagnostics and exit status. Allocation failures and
impossible internal state remain terminal. Failure class comes from the owned
operation and entry point, never diagnostic text. Future typed cancellation and
resource cleanup must extend the owned contract before networking or foreign
handles rely on it.

## Alternatives and cost

Explicit failure-result propagation through every operation would avoid a
nonlocal system boundary, but would change every ordinary call and built-in
runtime helper and add successful-path checks. The isolated trampoline confines
that cost to explicit recovery calls and keeps existing compiled functions and
failure edges unchanged. Placing setjmp in arbitrary ordinary functions would
expose optimizer and local-variable returns-twice hazards and is not used.

The dependency is the target's system libc. No repository-owned C runtime,
extra executable, interpreter or process rollback is introduced. Each active
scope adds a bounded stack frame; code-size and toolchain results remain part of
ordinary integration evidence. This decision does not change the public ABI or
make the internal runtime interface stable.

## Required evidence

Keep all reference/Native check, build, execute and REPL outcomes. Exercise
handler, nested and root failures, before/after/double Next, HEAD suppression,
prior effects and subsequent successful requests. Verify captured managed values,
root-buffer movement and repeated failure under forced GC; reject forged
signatures/effects and compare source-erased output. Target ABI tests cover
normal/nested/Float returns and terminal failure, alongside ordinary fixed points,
recovery and existing adjacent cases.
