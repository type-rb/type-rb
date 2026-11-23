# Generic enum receiver methods

Instance methods declared in a generic payload enum inherit its type parameters.
For example, a method on `Box<T>` receives a concrete `Box<String>` and substitutes
`String` through its arguments, result, defaults, private calls and closures.
Transparent aliases and imports preserve the original enum declaration identity.

## Declaration checking and discovery

Keep the receiver type on the generic function template and represent `self` as
its first immutable parameter. Check unused templates with abstract arguments,
so an operation that only works for one eventual instantiation is still rejected.
Fork receiver metadata during abstract checking; it must not change the ordinary
program's concrete receiver table.

Resolve a method from the receiver's enum declaration and type arguments.
Instantiate it through the existing generic function machinery and register its
default argument functions during discovery. Resolve implicit calls inside the
same enum to that identity, including recursive and private calls. Privacy uses
the original declaration name rather than an instantiated function's internal
name. A lexical callable binding continues to shadow an implicit method.

Discover concrete methods before freezing MIR type and function catalogs. This
includes methods that return another generic nominal value, methods selected
from ordinary functions without closures, and types introduced by defaults.

## MIR, lifetime and retained execution

Calls use the existing function MIR with an explicit receiver operand. The
backend needs neither generic templates nor receiver lookup tables. Implicit
receiver capture uses the same managed closure environment as explicit `self`.
Nullable calls retain lazy argument evaluation; managed payloads remain rooted
through methods, returned closures and collections.

The REPL consumes the checked call projections and concrete nominal identities.
Imported aliases, new declarations, rejected calls and replay must preserve
retained receiver and closure behavior.

## Verification and boundaries

Shared cases cover multiple instantiations, defaults and named arguments,
recursive calls, returned enums/records, nullable receivers, managed payloads,
aliases, imports, lexical privacy and invalid templates or arguments. Independent
MIR checks erase source, templates and receiver tables before code generation,
compare the generated result and run with forced collection. Retained-session
checks add declarations and rejected calls before replaying captured methods.

Method-specific type parameters, generic classes/interfaces and constraints
remain separate work. Generic raw-value enums and payloadless variants of generic
enums remain rejected according to the pinned reference contract. This change
does not complete all enum or basic-language contracts or qualify performance.

The published bootstrap seed is unchanged. Compiler source keeps ordinary
records and helper functions until a seed supporting these enum declarations
is independently accepted.
