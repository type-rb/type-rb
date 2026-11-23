# Transparent aliases resolve before Native MIR

Status: implemented; integration acceptance requires the checks below. This
extends generic nominal types, generic functions and checked REPL submissions.

## Contract and ownership

Top-level `alias Name = Target` and explicit generic applications are transparent
and declaration scoped. Aliases cover existing scalar, Array/Hash/Range, nullable,
record, enum and Result types. They retain canonical nominal identity through
imports, constructors, defaults and recursive record fields. Alias cycles are
rejected; recursion crossing a nominal declaration reuses its published shell.

`alias_syntax.trb` and `alias_model.trb` own declarations. Type resolution expands
concrete aliases with declaration-owned parameters and a bounded active expansion
path. Nominal expansion saves and restores the entire alias path, so separate
generic arguments cannot overwrite their enclosing cycle check. Successful instances are cached with their original arguments.
The isolated
semantic fork validates every alias with distinct abstract parameter identities,
even when unused. Failures publish diagnostics without reading absent patterns.

`alias_patterns.trb` checks enum pattern owners using canonical structural type
patterns. Parameters may be reordered, nested or repeated; repeated parameters
must match the same actual type, and concrete arguments must match exactly.
Aliases do not create another nominal identity or runtime layout.

MIR receives only existing canonical checked types and operations. Constructor,
field, enum, Result and GC semantics stay with their established owners. Backend
emission never looks up alias declarations or expansion patterns. Tests erase the
source, aliases, instances, templates, bindings and checked projections, then
require identical verified QBE and forced-collection execution.

REPL local declarations consume the final checked type rather than reparsing
an annotation into runtime semantics. Replay and retained binding witnesses use
a visible checked alias when an underlying nominal name is hidden. Cached
arguments preserve even parameters absent from the target. Display spelling is
separate and currently may show the underlying type.

## Verification and remaining work

The 258-case ordinary inventory records check/build/execution/REPL independently,
including import aliases, declaration scope, defaults, nullable/container/Result
values, reordered/nested patterns, recursive nominal targets and invalid unused
aliases. The CLI additionally verifies retained hidden nominal values and replay.

Integration requires immutable-seed core/CLI fixed points, the exact 90-module
recovery closure, snapshot recovery, target and lifetime suites. No seed, reference
pin, runtime layout or snapshot format changes. Compiler source does not yet
adopt alias syntax because the immutable seed does not accept it.

Literal/union and class/interface alias targets, nested module aliases, complete
authored display spelling and final Pure Go performance remain open. Callable
alias signatures now have a [MIR foundation](0051-callable-mir-foundation.md);
authored function values and lexical captures still require their own lowering.
The fixed reference's alias record construction and generic identity-alias Go
output failures remain explicit differences. Its ordinary Go package output also
collides when different files in the same package define the same nominal name.
Separate REPL declarations cannot introduce mutually forward-referencing types;
recursive aliases use a project/import boundary for interactive execution.

Reference follow-ups: [identity alias Go output](https://github.com/type-rb/type-rb/issues/707)
and [same-package nominal name collisions](https://github.com/type-rb/type-rb/issues/708).
