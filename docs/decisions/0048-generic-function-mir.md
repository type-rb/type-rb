# Generic functions with instance-owned MIR

Status: implemented; integration acceptance requires the ordinary and recovery
checks listed below. This extends decisions [0045](0045-generic-nominal-mir.md)
and [0047](0047-checked-body-ownership.md).

## Contract

Explicit top-level function applications such as `identity<String>("held")`
substitute parameter and return types, local type annotations, nested nominal
applications and calls in declaration scope. Recursive calls reuse the same
canonical declaration-and-arguments identity. Named arguments and parameter
defaults use the existing argument binding and private initializer functions;
explicit expressions run in source order before omitted defaults.

Every generic declaration is checked, including unused declarations and unused
defaults. An unconstrained `T` does not acquire arithmetic, mutable-reference
permission or a concrete return type merely because all current callers pass
Integer. Local values shadow generic function names. Imported aliases retain the
original declaration and type identities.

Generic record field defaults are implemented in [decision 0049](0049-generic-record-default-mir.md).
Methods, aliases, classes/interfaces, constraints and implicit type-argument
inference remain outside this slice. Compiler self-use
of generic syntax still requires a separately accepted bootstrap seed.

## Ownership

`generic_model.trb` retains immutable authored declarations and body spans.
`generic_functions.trb` declares concrete signatures, canonical instances and
default initializers. Resolution grows the ordinary function worklist; each
instance selects its own `CheckedBody`, including call and type applications.
Tokens and parsed control/iteration regions are shared without rewriting.

`generic_program.trb` forks mutable semantic catalogs for template validation.
`generic_check.trb` checks one canonical abstract instance of every declaration
using declaration-owned type-parameter identities. It reuses ordinary expression,
flow, argument and Result checking, with no abstract runtime layout or MIR body.
Calls in these templates check substituted signatures; their declarations receive
the same independent universal validation. This bounded pass does not recursively
specialize abstract bodies. Import-use facts and diagnostics are the only results
copied back. Authored syntax is immutable in both programs.

Concrete instances must still construct and verify complete ordinary MIR. Type
parameters cannot pass MIR type/layout verification. Backend calls, root planning
and QBE emission consume concrete signatures and operations only; there is no
backend generic specialization or unchecked fallback. REPL calls use the same
selected identities and normal checked function evaluator. Nominal declaration
fields resolve outside the caller's type-parameter binding environment.

The fork is temporary compiler work proportional to its semantic catalogs, not a
per-call runtime allocation. Removing that copying cost later must retain unused
template validation and catalog isolation. Concrete expansion retains an explicit
nesting diagnostic instead of unbounded instantiation.

## Verification

The shared reference/Native inventory exercises check, build, execution and REPL
paths for independent scalar/managed instances, recursion, nested nominal and
container types, nullable and Result values, imports, iteration, default ordering,
declaration scope and invalid templates/applications. The fixed reference pin's
local-shadowing defect is recorded separately from Native's correct rejection.

Compiler tests force collection across generic calls and managed allocations,
erase authored bodies/templates/checked projections after checking, and require
unchanged verified QBE and output. The ordinary immutable-seed core/CLI fixed
points and synchronized 86-module recovery derivation remain required. Snapshot,
target, lifetime and cleanup authorities are unchanged. These correctness checks
do not qualify final performance against Pure Go.
