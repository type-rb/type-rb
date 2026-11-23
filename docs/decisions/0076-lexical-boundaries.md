# Callable suffixes and lexical boundaries

Follow the pinned reference's identifier and operator token boundaries. An ASCII
name whose first character is not uppercase can end in one `?` or `!`. The suffix
is part of declaration and lookup identity, including imports, enum methods,
first-class function values and retained REPL declarations. It does not imply a
return type or receiver mutation. Nullable type punctuation remains separate from
an uppercase type name.

The lexer retains maximal operator spellings, including `>>`, `<=>`, `**` and
compound assignment names. The expression parser alone selects Symbol context
and turns its name into a String literal. Recognizing the token does not add the
corresponding executable operation; unsupported operators still fail checking.
Quoted Symbol and Hash/named-argument context selection is unchanged.

A type cursor can split `>>` into two generic closing angles. It copies only the
unparsed token suffix and preserves previous origins, like the existing deferred
String expansion. Generic-application lookahead counts both closing angles
without interpreting comparison expressions as type arguments. Subsequent
checking and MIR receive normalized type syntax; nested annotations and explicit
applications do not need a separate emitter path.

Declarations, parameters, members, type names and named arguments reject the
reference's reserved words and compiler-owned `__trb` prefix. Symbols and Hash
labels remain literal Strings even when their text is reserved as an identifier.
The existing resolver rejects undeclared value references.

Shared cases cover ordinary execution and REPL behavior, suffix identity and
imports, callbacks and captures, enum methods, nested generic closing angles,
default/interpolation origins, literal operator names and malformed/rejected
identifiers. Source-erased/reordered MIR executes under forced collection. A
retained CLI session checks Unicode/NUL returns, failed declaration isolation,
function identity and replay. No new runtime representation or MIR instruction is
needed; compiler self-use retains the immutable seed's accepted spelling.

Unicode identifiers and single-quoted Strings and Symbols now have shared
ordinary and REPL cases. The reference still retains operator/control Symbol
framing boundaries under [TypeRB #749](https://github.com/type-rb/type-rb/issues/749).
Malformed-source origins and wider quoted/interpolated combinations remain open;
this does not establish complete lexical or basic-language coverage.
