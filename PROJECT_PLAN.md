# TypeRB Project Plan

This document defines how TypeRB work should be split, tracked, and delivered.
The goal is to make a long-running compiler project manageable through small,
loosely coupled, test-first increments.

For current status and immediate next tasks, read `STATUS.md`. Product-level
capability milestones are tracked separately in `ROADMAP.md`.

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
- `codegen/ruby`: Ruby output
- `codegen/typescript`: TypeScript output
- `codegen/go`: Go output
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

## 6. Milestones

### Milestone 1: Compiler Skeleton

- Go module and CLI skeleton.
- Source position and diagnostic model.
- Lexer with comment token preservation.
- Minimal parser infrastructure.
- Test harness and golden test helpers.

### Milestone 2: MVP Syntax

- Project mode selection through `trbconfig.jsonc`.
- explicit project, standard-library, and platform imports, with a deliberately
  small portable prelude for fundamentals such as `puts`.
- class declarations.
- field declarations.
- method declarations.
- local `:=` declarations.
- assignments.
- returns.
- method and constructor calls.

### Milestone 3: MVP Semantics

- scopes and symbol tables.
- class/member resolution.
- explicit field initialization checks.
- private name access checks.
- simple local inference from `:=`.
- return type validation.

### Milestone 4: First Backend

- Choose one backend first, preferably TypeScript or Ruby.
- Add golden codegen tests.
- Keep backend-specific behavior isolated.

### Milestone 5: Formatter MVP

- Format comment-free source.
- Preserve source spans.
- Reject or conservatively handle unsupported comment placement.
- Add golden formatting tests.

### Milestone 6: Multi-target Expansion

- Add remaining backends incrementally.
- Add target-specific tests.
- Keep all backends isolated behind the shared typed IR.

## 7. Pull Request Policy

Pull requests should be narrow and phase-oriented.

Good PR examples:

- Add token kinds and lexer tests for class/method syntax.
- Parse class declarations into syntax AST.
- Add private member resolution diagnostics.
- Generate TypeScript for fields and methods.

Avoid PRs that combine unrelated layers, such as parser rewrites plus formatter
changes plus backend changes, unless the issue explicitly requires the combined
change.

## 8. Decision Records

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
