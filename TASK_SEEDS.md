# TypeRB Task Seeds

These are initial GitHub Issue candidates. After the GitHub Project is created,
convert these into issues and place them in `Backlog` or `Ready`.

## 1. Create GitHub Project Board

## Goal

Create the Kanban board described in `PROJECT_PLAN.md`.

## Scope

- Create project columns.
- Add custom fields.
- Add initial labels.
- Add milestones for compiler skeleton, MVP syntax, MVP semantics, first backend,
  formatter MVP, and multi-target expansion.

## Out of Scope

- Compiler implementation.

## Acceptance Criteria

- Board has the agreed columns.
- Custom fields exist.
- Milestones exist.
- Initial issues can be added and filtered by area, target, phase, risk, and size.

## Tests

- Not applicable.

## Dependencies

- None.

## 2. Initialize Go Module And CLI Skeleton

## Goal

Create the minimal Go module and `trb` CLI entrypoint.

## Scope

- Add `go.mod`.
- Add CLI package.
- Add placeholder `trb version` or equivalent smoke command.
- Add basic test command wiring.

## Out of Scope

- Lexer, parser, formatter, and codegen behavior.

## Acceptance Criteria

- `go test ./...` passes.
- CLI package builds.
- Project layout follows `PROJECT_PLAN.md`.

## Tests

- CLI smoke test.

## Dependencies

- GitHub Project board is helpful but not required.

## 3. Add Source Position And Diagnostic Model

## Goal

Define shared source position, source span, and diagnostic data structures.

## Scope

- Add `token` or source package for positions/spans.
- Add `diagnostic` package.
- Include severity, message, and source span.
- Add tests for span construction and diagnostic formatting.

## Out of Scope

- Lexer and parser implementation.

## Acceptance Criteria

- Source spans can represent start/end byte offsets and line/column positions.
- Diagnostics can point at a source span.
- Tests cover basic construction and formatting.

## Tests

- Unit tests for source spans.
- Unit tests for diagnostics.

## Dependencies

- Go module skeleton.

## 4. Implement Lexer Token Skeleton

## Goal

Implement first lexer pass with token and comment preservation.

## Scope

- Define token kinds for MVP syntax.
- Lex identifiers, keywords, punctuation, string literals, comments, and EOF.
- Preserve comment tokens.
- Attach source spans to every token.

## Out of Scope

- Full numeric literal support unless needed by examples.
- Parser implementation.

## Acceptance Criteria

- Lexer emits code tokens and comment tokens.
- Lexer tests cover mode declarations, classes, fields, methods, assignments,
  returns, calls, and comments.
- Invalid characters produce diagnostics.

## Tests

- Lexer token tests.
- Lexer diagnostic tests.

## Dependencies

- Source position and diagnostic model.

## 5. Implement Minimal Parser Infrastructure

## Goal

Parse a token stream into syntax AST with spans.

## Scope

- Define syntax AST interfaces and base node span behavior.
- Add parser entrypoint.
- Parse `mode:` declaration.
- Parse EOF and basic diagnostics.
- Skip comment tokens while preserving token stream for formatter use.

## Out of Scope

- Full class/method parsing.
- Type checking.
- Code generation.

## Acceptance Criteria

- Parser returns AST plus diagnostics.
- AST nodes carry source spans.
- Parser tests use golden snapshots.

## Tests

- Parser AST golden tests.
- Parser diagnostic tests.

## Dependencies

- Lexer token skeleton.

## 6. Decide First Backend Target

## Goal

Choose the first code generation backend for MVP.

## Scope

- Compare Ruby, TypeScript, and Go as first backend targets.
- Document the decision in `docs/decisions/`.

## Out of Scope

- Implementing the backend.

## Acceptance Criteria

- Decision record exists.
- Consequences and alternatives are documented.

## Tests

- Not applicable.

## Dependencies

- None.
