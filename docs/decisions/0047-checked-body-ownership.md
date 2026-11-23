# Function-owned checked body projections

## Context

The ordinary compiler stored several checked expression facts by source-token
origin in shared program tables. That is sufficient while every authored body
has exactly one type environment. Generic function instances and generic field
defaults need independent checking environments even when they retain the same
authored source origins. The retained REPL also needs the callee's projections
while evaluating a call, followed by the caller's projections on return.

## Decision

`CheckedBody` owns concrete type applications, nullable reads, safe access,
control result types, enum patterns, Hash and Range operations, typed iteration,
Result operations, operator plans and lexical transfer targets. The program
registers these bodies by semantic function identity. Resolution and checking
select the same body; REPL calls select and restore it explicitly, including
failure and early-return paths. Private default initializer functions use the
same mechanism.

Body storage translates authored coordinates relative to its source span. Empty
prefixes from earlier declarations therefore do not accumulate once per function.
Read/write bounds and diagnostics retain absolute source coordinates, while
unwritten chunks remain unallocated.

Parsed controls and iteration regions remain shared immutable source structure.
Checking publishes typed iteration plans separately and stores non-completing
Hash expression facts in the checked body rather than modifying parsed control
origins. Source coordinates remain authored coordinates; bodies do not obtain
identity by copying or rewriting source tokens.

Whole-program Hash validation visits every body's own receiver projections.
Native emission consumes verified MIR and does not require any checked body.
Range descriptor demand now comes from MIR Range construction instructions,
removing a remaining dependency on a frontend construction cache.

## Validation and remaining work

Controls check two semantic functions with different parameter types against
one authored source region, including nullable and Hash projections. Separate
controls discard every body projection and source expression before identical
emission and execution, including Range allocation. The CLI suite covers nested
calls, defaults, nullable/enum/Result values and replay through ordinary Native
fixed points. Recovery keeps the canonical closure and per-module mutation
controls synchronized.

This establishes checked-body ownership. Generic function declaration parsing,
bound type parameters, instance scheduling and generic record initializer
specialization remain language work. It does not claim new syntax coverage,
a seed refresh or final performance qualification.
