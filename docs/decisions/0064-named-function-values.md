# Named function values through existing callable MIR

## Reference contract

A local or explicitly imported TypeRB function with a nongeneric, required
positional signature is a value of that function type. Import aliases preserve
the resolved declaration and nominal parameter/result identities. Lexical
bindings take precedence. Declarations with defaults, named-only parameters or
type parameters require an explicit typed `fn` wrapper. Direct declaration calls
retain their existing argument rules; a declaration returning a Function is
separate from invoking the Function it returns.

Function values may be passed, returned, captured and stored in Arrays, Hashes
and records. Their calls preserve argument order, managed results, mutable
parameter bindings and Result obligations. A nullable Function is a container;
only a proven non-null payload is callable. Function types can be written
as concrete generic arguments without an intermediate alias.

## Ownership

Name resolution retains the declaration ID in the checked body at the authored
origin. Lowering constructs the existing MIR 41 callable with an empty captured
parameter prefix. MIR 42 remains the indirect call, with its existing signature,
effects, ABI and root validation. No new runtime operation, backend source
inspection or second callable representation is introduced.

The generic application lookahead recognizes function parameter groups only
with their arrow. The ordinary type parser validates the complete argument;
parenthesized comparison operands remain ordinary expressions. Optional type
wrappers are distinguished from the `$Callable` payload before MIR classification.

The REPL stores the resolved function in its owning checked program, just as it
retains anonymous code. Named functions have no lexical capture prefix. Field
reads resolve a callback before evaluating its arguments, and share ordinary
indirect-call evaluation, failure restoration and optional receiver handling.
Named declarations display as `#<callable>` and anonymous values as `#<fn>`,
following the reference REPL.

The authored MIR fixture now uses named function values instead of manually
constructing its entry function. Ordinary core and CLI fixed points continue to
check the complete compiler source. Core self-use of this new spelling remains
blocked by the preceding immutable seed's parser; this change does not refresh
that seed or add a compatibility path.

## Validation and remaining work

Shared file check/build/run/REPL cases cover local/imported names, return values,
scalar/managed signatures, storage, nullable narrowing, mutation, capture,
recursion, generic applications, explicit adapters and Result handling. Negative
cases reject unsupported declaration signatures, wrong types/arity, named
function-value arguments, unproven nullable calls and ignored Result values.

Independent MIR controls execute source-erased and reordered functions under
forced collection and reject malformed captures, signatures, operands and roots.
Retained REPL controls cross new declarations and reloads, call between old and
new program contexts, and restore the session after failed calls. The shared
contract preserves remaining rejection and diagnostic differences explicitly.
Callable equality, method references and broader declaration/generic families
remain open in the basic-language inventory.
