# Internal bootstrap snapshots

This package exports a deliberately bounded, versioned data format from checked
TypeRB IR. It is not a general backend or a public compiler extension API.
Unsupported source constructs return explicit diagnostics.

Versions 3 and 4 lower `if` and `while` Boolean condition trees containing
short-circuit `&&`, `||`, and nested `!` through existing branches, jumps and
block parameters. The left operand runs once; the right runs only when needed.
Loop backedges return to the original condition entry, and lexical bindings
are carried to each continuation explicitly. No format opcode is added.

Logical value expressions outside these condition trees, including logical
subexpressions in call arguments, remain unsupported. Supporting them requires
carrying enclosing expression temporaries across blocks, not simply making
binary instruction emission eager. Version 2 is unchanged. These are snapshot
coverage limits, not restrictions on ordinary TypeRB language semantics.
