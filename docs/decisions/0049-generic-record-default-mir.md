# Generic record defaults through ordinary initializer MIR

Status: implemented; integration acceptance requires the ordinary and recovery
checks below. This extends [generic nominal types](0045-generic-nominal-mir.md),
[generic functions](0048-generic-function-mir.md) and
[default initializer MIR](0040-default-initializer-mir.md).

## Contract

Explicit generic records accept field defaults with the same declaration scope,
required-before-default ordering and evaluation order as ordinary records.
Earlier fields retain their concrete types; current/later fields and caller
locals are unavailable. Explicit expressions run in source order before omitted
defaults run in field order. Omitted managed defaults allocate fresh values.

Defaults support nested nominal/container types, nullable and Result values,
generic function calls and ordinary value-producing controls. Every default is
validated even if its record is unused or every call explicitly supplies it.
Concrete Integer calls cannot legalize arithmetic on an unconstrained `T`.

## Ownership

The ordinary record parser now handles both generic and non-generic declarations;
it uses the existing expression parser to retain each authored default span.
The former second generic record parser is removed. Generic enum parsing remains
in `generic_syntax.trb`, without an unused record branch.

Concrete record instantiation registers the same `DefaultInitializer` entries as
ordinary records. `default_arguments.trb` declares their private typed functions;
resolution, checking, MIR calls, verification, root planning and REPL evaluation
reuse the established path. There is no new runtime or QBE specialization path.

`GenericBinding` and `generic_bindings.trb` own declaration source, parameters,
arguments and expansion depth for both function instances and record defaults.
A checked body selects a binding independently of generic function instance IDs.
Function and nominal abstract parameter identities include distinct declaration
owners. Nominal field type resolution clears the caller's binding environment.

The isolated semantic program fork checks all generic record defaults with
abstract parameters, alongside existing generic function templates. Each
canonical declaration is checked once without recursively specializing abstract
bodies. Concrete instantiation still produces and verifies every required MIR
body. The existing expansion limit also bounds growing cycles through defaults
and generic function calls; finite recursion reuses canonical instances.

## Verification and remaining work

Compiler tests cover default order, fresh managed values, distinct type instances,
nested records, containers, Result, nullable values, full controls, generic calls
and invalid unused defaults. They erase source, templates, bindings and checked
projections after checking, verify unchanged QBE, and force collection around
calls and allocations. A growing-instantiation control requires a bounded
compiler diagnostic. The shared 230-case inventory records ordinary check,
build, execution and REPL expectations, with fixed-reference failures explicit.

Acceptance also requires immutable-seed core/CLI fixed points, the exact
87-module recovery closure, snapshot/target/lifetime suites and cleanup checks.
Compiler self-use of generic syntax awaits an accepted seed refresh; this change
does not change the seed, reference pin or snapshot contract. Generic methods,
aliases, classes/interfaces, constraints and remaining display parity are still
separate gaps. Correctness acceptance does not qualify final Pure Go performance.
