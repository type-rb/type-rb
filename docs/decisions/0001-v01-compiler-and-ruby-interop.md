# 0001: v0.1 Compiler Pipeline and Ruby Interoperability

Status: accepted for v0.1

## Context

TypeRB must produce Ruby usable by an ordinary Rails application while also
having statically checked, portable Go and TypeScript targets. Rails DSL is open
ended: gems add macros and blocks which cannot be enumerated by a closed TypeRB
grammar.

At the same time, a text-rewrite transpiler would lose the semantic boundary
needed for type checking and reliable multi-target code generation.

## Decision

The compiler uses these explicit phases:

1. A lossless lexer preserving comments, literal spelling, newlines, and spans.
2. A handwritten parser producing a syntax AST.
3. Name resolution and type checking with local `:=` inference.
4. Lowering to a separate typed IR.
5. Backends that consume IR only.

Portable syntax has dedicated AST and IR nodes. Ruby syntax which is valid in a
Rails application but outside the portable grammar is represented by dedicated
`NativeStatement`, `NativeBlock`, and `NativeExpression` nodes. These nodes
retain source spans/text, pass through the Ruby backend, and are compile errors
for Go and TypeScript.

The formatter parses the program, then prints from the lossless token stream.
This preserves comments and heredocs independently from backend lowering.

## Consequences

- Rails libraries can continue defining new DSL macros without a TypeRB release.
- Type annotations and local inference remain available inside Rails methods.
- Go and TypeScript compilation cannot silently accept Ruby-only behavior.
- Native Ruby nodes are intentionally less type-checkable than portable nodes.
- More Ruby constructs can migrate from native nodes to typed nodes over time
  without changing the AST/IR phase boundaries.

## Alternatives considered

- Full Ruby grammar and semantics in v0.1: too large and still cannot know every
  gem-defined DSL semantic.
- Regex/text rewriting: rejected because it does not provide a reliable AST,
  checker, typed IR, or target isolation.
- Restricting Rails to a small built-in DSL list: rejected because real Rails
  applications and gems routinely extend the DSL.
