# TypeRB Project Plan

This document defines how TypeRB work should be split, tracked, and delivered.
The goal is to make a long-running compiler project manageable through small,
loosely coupled, test-first increments.

For current status and immediate next tasks, read [status.md](status.md).
Product-level capability milestones are tracked separately in
[roadmap.md](roadmap.md).

## 1. Work Management

Use GitHub Projects as a Kanban board.

Recommended columns:

1. Backlog
2. Ready
3. In Progress
4. In Review
5. Blocked
6. Done

Recommended custom fields:

- `Area`: lexer, parser, ast, resolver, typechecker, ir, codegen, formatter, cli, docs, infra
- `Target`: go, ruby, typescript, all, none
- `Phase`: mvp, alpha, beta, later
- `Risk`: low, medium, high
- `Size`: xs, s, m, l

Issues should be small enough to finish with a focused pull request. Large
language features should be represented as an epic issue with linked child
issues for each compiler phase.

## 2. Kanban Policy

`Ready` means:

- Scope is clear.
- Expected test coverage is listed.
- Input/output behavior is described.
- Dependencies are linked.
- Acceptance criteria are concrete.

`Done` means:

- Tests are added or intentionally skipped with a reason.
- Public phase outputs are stable and documented when relevant.
- Diagnostics are covered for invalid input where applicable.
- No unrelated refactors are included.

Limit active work in progress. Prefer finishing one compiler phase increment
before opening several partially connected changes.

## 3. Issue Shape

Use this structure for implementation issues:

```markdown
## Goal

## Scope

## Out of Scope

## Acceptance Criteria

## Tests

## Dependencies
```

For language features, split work by phase:

1. Tokenization
2. Parsing
3. AST representation
4. Name resolution
5. Type checking
6. Code generation
7. Formatting
8. Documentation

Not every feature needs every phase, but the issue should say which phases are
included and which are deferred.

## 4. Component Boundaries

Compiler components should communicate through explicit data structures instead
of reaching into each other's implementation details.

Recommended package boundaries:

- `token`: token kinds, source positions, source spans
- `lexer`: source text to token/comment stream
- `ast`: syntax AST node definitions
- `parser`: token stream to syntax AST and parse diagnostics
- `diagnostic`: shared diagnostic model
- `resolver`: scopes, symbols, imports, member lookup
- `types`: type representation and assignability rules
- `checker`: syntax AST to typed AST or typed IR
- `ir`: target-independent typed representation if needed
- `codegen/go`: Go output
- `codegen/ruby`: Ruby output
- `codegen/typescript`: TypeScript output
- `formatter`: syntax AST plus token/comment stream to formatted source
- `cli`: command-line entrypoints such as `trb fmt`

No phase after parsing should depend on parser internals. No code generator
should depend on parser-specific node construction details. Formatter may use
both AST spans and the original token/comment stream.

## 5. TDD Strategy

Development should be test-first at phase boundaries.

Core test types:

- Lexer token tests.
- Parser AST golden tests.
- Parser diagnostic tests.
- Resolver symbol tests.
- Type-checker success and failure tests.
- Codegen golden tests for each target.
- Formatter golden tests.

Tests should assert public outputs: token streams, AST snapshots, diagnostics,
typed results, generated code, and formatted source. Avoid tests that lock down
private parser control flow or internal helper functions.

## 6. Pull Request Policy

Pull requests should be narrow and phase-oriented.

Good PR examples:

- Add token kinds and lexer tests for class/method syntax.
- Parse class declarations into syntax AST.
- Add private member resolution diagnostics.
- Generate TypeScript for fields and methods.

Avoid PRs that combine unrelated layers, such as parser rewrites plus formatter
changes plus backend changes, unless the issue explicitly requires the combined
change.

## 7. Decision Records

For decisions that affect long-term architecture, add a short decision record in
`docs/decisions/`.

Decision records should include:

- Context
- Decision
- Consequences
- Alternatives considered

Examples:

- Parser implementation strategy.
- Comment preservation strategy.
- First backend target.
- Whether to introduce typed IR.
