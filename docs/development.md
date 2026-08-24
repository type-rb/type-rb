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

## Website boundary

The Pages build publishes the product landing page, user documentation, tour,
and playground as one static site. `docs/site.json` owns documentation
navigation and the explicit maintainer-only exclusions. All other Markdown
under `docs/` is rendered, so a lint rule page becomes public without adding a
second website-specific registry. The build rewrites links between published
Markdown pages to stable directory URLs and sends links to excluded or
repository-owned Markdown back to GitHub.

Keep website generation host-only and separate from the browser compiler.
`internal/site` owns the landing page and documentation rendering;
`internal/playground` continues to own only the executable tour and playground
assets. The host-only website build applies Shiki to fenced documentation code
before publication. TypeRB fences load the canonical grammar from `syntaxes/`,
while standard fences use Shiki's bundled grammars; published pages do not run
a syntax-highlighting script.

Install the two website tool dependencies and build the complete site with:

```sh
npm ci --prefix tools/textmate
npm ci --prefix tools/site
./scripts/build-site.sh dist/site
```

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

## Documentation ownership

- `language.md` teaches implemented syntax; `specification.md` defines its
  normative semantics.
- `learning.md` orders existing material; it does not redefine syntax or APIs.
- `standard-library.md`, `cli.md`, and `configuration.md` are the references for
  those public surfaces.
- `migrations/` records release-specific source and package upgrade steps; it
  does not redefine the current language contract.
- `status.md` records current capability and limitations; `roadmap.md` contains
  future outcomes only.
- Decision records preserve durable rationale. Scoped implementation work
  belongs in GitHub issues rather than progress logs in this directory.
