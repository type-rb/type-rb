# Development and compiler architecture

TypeRB is implemented in Go 1.27. Clone the repository and run:

```sh
go test ./...
go vet ./...
```

## Compiler pipeline

Every target uses the same phase pipeline:

```text
lossless tokens
  -> syntax AST
  -> resolver and type checker
  -> typed IR
  -> Go, Ruby, or TypeScript backend
```

Compiler phase boundaries live under `internal/ast`, `internal/checker`,
`internal/ir`, `internal/lower`, and `internal/codegen`. Backends consume typed
IR and do not inspect parser state or rewrite source text.

Read the [language specification](specification.md) for semantics and the
[architecture decisions](decisions/) for long-term implementation choices.

## Formatter guarantees

`trb fmt` parses source and uses the lossless token stream when printing. It is
deterministic and idempotent and preserves:

- standalone and trailing `#` comments;
- quoted and interpolated strings;
- percent literals;
- heredoc bodies and their internal whitespace; and
- platform DSL block parameters and literal syntax.

The canonical indentation is one tab per nesting level. Indentation is not
configurable in the current alpha.

## Linter boundary

`trb lint` invokes the compiler pipeline before running source-oriented rules
from `internal/lint`. Correctness diagnostics remain owned by compiler phases
and cannot be suppressed by lint configuration. Each built-in rule registers
a stable ID, default level, recommended-preset membership, first release, fix
capability, and documentation slug. Tests require the shared rule index and a
dedicated rule page to match that registry metadata.

The initial linter does not load package code or expose compiler AST state to
third parties. Add an external rule protocol only after deterministic inputs,
resource limits, version negotiation, and package trust are specified.

## Development workflow

Keep one grammar and portable semantics across modes. Target-specific APIs and
compatibility behavior belong behind explicit `trb/platform/<mode>/*` imports.
Language changes must pass through syntax AST, checked types, typed IR, and each
affected backend.

Create an issue before work that requires a language or architectural decision.
Tasks with settled behavior can proceed as focused pull requests. Keep each PR
narrow enough to review as one outcome, and include invalid-input diagnostics
when the change introduces a new rule.

Test public phase boundaries rather than private helper structure. Depending on
the change, this includes parsed AST shape, checker diagnostics and inferred
types, typed IR, generated source for every affected backend, formatter
idempotence and comment preservation, and REPL evaluation. Run the full Go test
suite and any relevant target-toolchain checks before merging.

For current gaps, see [status](status.md) and the [roadmap](roadmap.md).

Releases are described in [releasing.md](releasing.md).
