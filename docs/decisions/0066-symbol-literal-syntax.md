# Symbol spelling with String semantics

Portable Symbol expressions such as `:ready` and `:"a b"` have the reference
String type. Equality, receiver methods, nullable values, generic arguments,
defaults, captures and collection storage use the existing String contracts.
They introduce no intern table, new runtime representation or MIR operation.

A quoted Symbol decodes String escapes without evaluating interpolation-looking
text. The lexer therefore preserves a quote containing live `#{` as raw token
kind 7. The expression parser selects ordinary String expansion or literal
Symbol decoding. Hash entry and named-argument colons retain their own grammar;
the lexer does not infer meaning from a preceding colon.

Expansion occurs at the forward syntax cursor before semantic checking. It
replaces that raw token and moves only the unparsed suffix. Previously published
declarations, defaults and controls keep their origins; later declarations are
cataloged after expansion. Nested expressions follow the same path. Ordinary
quotes without interpolation continue through the existing lexer. A Symbol name
becomes a checked String literal, consumed identically by MIR and the REPL.

This retains the existing token-based parser architecture. Expanding many quoted
expressions still copies future tokens; a linear token-buffer representation is
future compiler organization/performance work. No speedup or complete lexical
coverage is claimed here.

Shared cases cover quoted/identifier/operator names, Unicode and escaped bytes,
literal interpolation spelling, actual String interpolation, Hash labels, named
arguments, defaults, controls, generic aliases, optional results and retained
values. Source-erased and reordered MIR executes under forced GC. Separate CLI
checks retain Unicode/NUL through callbacks and Hashes, preserve later diagnostic
lines and recover a retained session after an invalid escape. Compiler core
self-use of Symbol spelling remains blocked by the preceding immutable seed;
ordinary core and CLI fixed points still verify the complete implementation.

The reference quoted-Symbol validation and Ruby quoting correction is tracked in
[TypeRB PR #747](https://github.com/type-rb/type-rb/pull/747). The later
[TypeRB PR #821](https://github.com/type-rb/type-rb/pull/821) defines portable
single-quoted Strings and Symbols. Native decodes them through the same checked
String/MIR path; shared ordinary and REPL cases include valid escapes, Unicode,
literal interpolation spelling and rejection. Remaining operator names under
[TypeRB #749](https://github.com/type-rb/type-rb/issues/749) and multiline
interpolation retain explicit boundaries. These cases do not establish complete
Symbol or basic-language coverage. Invalid interpolated escapes reject the whole
REPL declaration without a cascade of unrelated body errors.

The current reference fixes control-keyword Symbol framing. Native submission
framing now uses the ordinary parser to distinguish these literal names from
actual blocks, including a `fn` following a named-argument colon. See
[namespace and constant integration](0067-namespaces-and-constant-mir.md).
