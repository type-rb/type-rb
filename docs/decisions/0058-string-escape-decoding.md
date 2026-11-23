# Byte-preserving String escape decoding

Status: implemented under the basic-language and MIR consolidation milestones.

Ordinary Native files and the REPL now accept the reference String escape
family: control escapes, exactly three octal digits, exactly two hexadecimal
digits, four-digit `u` and eight-digit `U` Unicode escapes. Byte escapes preserve
values through 255, including NUL and invalid UTF-8 bytes. Unicode escapes must
name scalar values through `10FFFF`, excluding surrogates. Short, malformed and
out-of-range forms reject; the shorthand `\0` and brace Unicode syntax remain
invalid, consistently with the pinned reference.

`string_escapes.trb` owns cursor advancement, digit widths, byte/scalar validation
and UTF-8 encoding. The lexer adds the decoded bytes to the current literal
without rescanning them. A decoded hash cannot start interpolation, and a decoded
quote cannot terminate the source literal. Normal source interpolation remains
ordered and shares this decoder in files and the REPL. Existing String
concatenation recounts code points when byte fragments complete a UTF-8 sequence
across their boundary; literal construction retains that behavior.

The internal `compiler_literal_byte(Integer): String` constructor has a narrowly
resolved declaration identity. The compiler's source-slice owner resolves its
visible `decode_escape` declaration, then that decoder's visible constructor.
An unrelated ordinary or sibling declaration with the same name is not adapted.
MIR runtime adapter 12 independently checks the exact signature, immutable
parameter and result, and retains conservative call effects and root planning.
The QBE adapter consumes that verified identity; it does not inspect user names
or reimplement escape parsing. `qbe_string_literals.trb` owns the bounded byte
constructor, rejecting values outside 0..255 before allocation.

The source implementation retains the preceding seed's existing ASCII subset.
Replacement Native compilers use the full byte constructor. An unsupported byte
in the narrower Go-hosted recovery body raises an explicit bootstrap diagnostic,
rather than silently substituting an empty value. As with existing compiler byte
access, this recovery boundary is distinct from ordinary Native language support.
Compiler implementation source uses only syntax supported by the unchanged
immutable seed; its setup transitions and core/CLI fixed points remain required.
No reference pin or seed publication is part of this change.

MIR stores the decoded String value, including raw bytes, and the existing
literal emitter obtains its exact byte sequence and code-point length. The
shared contract contains 397 cases: 35 new escape probes plus updated older
Unicode/NUL outcomes and retained invalid-syntax diagnostics. Cases include all
256 byte values, scalar boundaries, malformed digits, interpolation, and byte
joins across literals, calls and runtime concatenation. Independent tests erase
frontend source, validate private adapter identities, reject forged signatures
and force collection between runtime calls. UTF-8 controls stress long escaped
literals and retained results. The canonical recovery closure has 108 modules.

Unicode identifiers, remaining String receiver APIs, full diagnostic presentation
parity and final performance qualification remain open.
