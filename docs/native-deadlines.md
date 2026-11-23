# Native Web deadlines and structured mapping

`trb/web/middleware/timeout` uses a compiler-owned deadline scope in ordinary
Native applications. `Timeout.call` keeps the portable response contract:
completed work returns its response, expired work returns the timeout response,
and a non-positive duration does not enter the handler. Nested scopes observe
the earliest active deadline; an inner recovery boundary cannot consume a
parent's timeout and continue its handler.

## Execution and cleanup

The frontend grants the private deadline operation only to its loader-owned
adapter. MIR owns checkpoint effects and locations: managed basic-block entry
and continuations after managed or foreign calls. Verification independently
checks the boundaries. QBE consumes these facts after frontend source and
checked-body metadata have been removed.

A monotonic libc clock supplies elapsed time. A dedicated returns-twice
trampoline records the deadline owner, recovery boundary, root count and
resource-stack boundary. Expiration unwinds owned resource frames in reverse
order exactly once, restores the root count and leaves the owning scope.
Normal returns and ordinary language failures restore the same dynamic state.
No unwind crosses a live foreign callback or library frame.

OIDC discovery and JWKS retrieval use the smaller of the remaining scope budget
and the existing five-second libcurl limit. After libcurl returns, the adapter
releases its easy handle and response buffer before the managed checkpoint can
expire the scope. Proxy, redirect, TLS and trust-store policy remain those of
[the OIDC adapter](native-oidc.md). This is cooperative cancellation, not arbitrary
preemption of OS calls. Other foreign operations require their own bounded
operation and cleanup qualification before making the same guarantee.

Programs without a deadline operation retain the ordinary runtime without
additional deadline polling or clock imports. The implementation adds no
external library; libc remains the clock and recovery provider.

## Structured concurrent mapping

`Array.concurrent_map` accepts the portable named positive `limit` (default 8),
evaluates it once even for empty input, and preserves input order in its result.
The blocking, single-mutator implementation admits one element at a time. A
nested call stays within the same serial admission bound, and every admitted
child finishes or unwinds before the call exits. It promises no CPU parallelism
or overlapping I/O. Managed deadlines stop subsequent admission.

The checker preserves the portable safety rules even with serial execution:
outer bindings cannot be assigned, captures must be concurrency-safe, borrowed
reference elements and aliases cannot be mutated, and transitively called
same-module functions and constructor defaults are audited. Imported authored
implementations retain the pinned reference boundary; this audit does not prove
that arbitrary imported code is safe for parallel execution. Fresh constructor field
initialization and independently created task-owned storage remain usable.
Interface and function-value calls require a future explicit safety contract.
Scoped File borrows cannot reach `concurrent_map`, directly or through helpers,
parameter defaults or constructors. Serial admission preserves this language
suspension boundary, including empty input; ordinary synchronous `map` remains
available while a File is borrowed.
This implementation follows [the structured mapping decision](https://github.com/type-rb/type-rb/blob/c49b67ec0f9e5941c52b787885f11a33c19f795f/docs/decisions/0004-bounded-structured-concurrent-map.md)
and [the Native runtime decision](decisions/0086-production-web-application-path.md).

## Qualification boundary

The frozen [initial deadline manifest](../fixtures/projects/native-web-deadlines.json)
separates application behavior, live file/socket lifetime observations and actual
Native runtime unwind controls. `tools/native-web-deadline-test.py` records source,
compiler and controller identities, commands and unsuccessful observations.
Its `--scope applications` selects the application groups explicitly and records
the omitted runtime-unit authority; the complete compiler-unit CI lane runs that
authority with QBE. A selected subset is never evidence for the full manifest.

The ordinary CLI jobs exercise the application groups on Darwin and Linux
arm64. Header qualification separately checks the clock and jump-environment ABI
on four hosts. Mapping tests cover check, build, execution and retained REPL,
plus source-independent MIR emission under forced collection. These authorities
do not establish a complete four-host release, source backends, general foreign
cancellation, or Web deadline execution in the REPL. Those remain separate v0.5
obligations.
