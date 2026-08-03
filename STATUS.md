# TypeRB Status

Last updated: 2026-08-03

## Current State

TypeRB v0.1 is implemented as a Go compiler prototype.

Implemented:

- `trb fmt`, including comment, percent-literal, interpolation, and heredoc
  preservation.
- `trb build` with Ruby, TypeScript, and Go backends selected by the project
  `trbconfig.jsonc`.
- `trb run` for compiling a project in a temporary tree and executing its
  conventional top-level `main()`; an explicit `.trb` file remains available
  as a one-off override.
- Project-aware `trb repl`, with config-selected mode, persistent typed state,
  multiline cursor editing, per-project command history, reverse search,
  Readline/Emacs navigation, completion, colored output, project imports,
  interruptible portable IR evaluation, and `:type`, `:load`, and `:reload`
  commands.
- `trb init`, `sync`, `add`, `remove`, and `install`, with generated Gemfile,
  go.mod, or package.json ownership based on project mode.
- `packageManagement: "external"` for embedding generated source in an
  existing Rails/Go/TypeScript project without modifying its manifest.
- Project-tree builds which copy non-`.trb` files for runnable Rails output.
- Lossless lexer with byte/line/column spans.
- Handwritten recursive-descent and Pratt parser.
- Independent syntax AST and typed IR.
- Resolver/type checker with local inference, assignment/return checks, field
  initialization checks, duplicate detection, and private-member checks.
- Immutable `:=` bindings, explicit `mut` for reassignment and destructive
  Array updates, and runtime-initialized uppercase constants scoped to the top
  level, modules, or classes.
- Non-nullable `Boolean` conditions for portable `if`, `elsif`, and `while`,
  preventing target-specific truthiness differences.
- Checked unary, arithmetic, comparison, equality, logical, and compound
  operators, including target-independent Integer division/remainder and
  precedence-preserving backend output.
- Go-like no-value return syntax: the return annotation is omitted and explicit
  `: Void` return types are rejected while typed IR retains an internal Void.
- Explicit Ruby-native AST/IR nodes for open-ended Rails DSL.
- Compiler-owned Declaration IR and platform type-provider registry; the Rails
  provider automatically parses `db/schema.rb` into a Schema AST and derives
  controller, ActiveRecord model/column, relation, finder, and pagination types
  without application-maintained signature files.
- Project-wide compilation which parses every source once, builds and validates
  a deterministic import graph, and rejects import/inheritance cycles,
  duplicate exported types, and duplicate top-level `main` definitions.
- Cross-file checking of imported constructors, fields, methods, inheritance,
  and interface conformance, with target-relative backend imports.
- Portable `trb/std/io` and `trb/std/strings` packages lowered from resolved IR
  symbols, plus mode-checked Ruby, Go, and TypeScript platform packages.
- Closed `record` declarations as distinct AST/typed-IR nodes, with
  keyword-only construction, field checking, Go structs/JSON/GORM tags,
  TypeScript interfaces, and Ruby `Data` output.
- `localPackages` workspace imports, allowing one portable record package to be
  compiled into applications with different target modes.
- Explicit Go net/http and GORM/SQLite platform packages, React/browser
  TypeScript packages, portable arrays/numbers, and sqldef-aware `trb run`.
- A complete Todo vertical slice with a Go API, React client, shared records,
  sqldef schema, 1:N TodoList/TodoItem data, and N:M TodoItem/Tag data.
- Rails examples covering models, controllers, jobs, mailers, concerns, routes,
  and migrations.
- Unit/integration coverage for the compiler, formatter, three backends, and
  project build behavior.
- Stage-1 self-host sources for compiler value objects, with deterministic
  bootstrap regeneration checking.
- Tag-driven release packaging and a source-building Homebrew Formula for the
  `type-rb/tap` distribution path.

Verified locally:

- `go test ./...`
- generated Go parses, formats, and runs;
- generated TypeScript runs with Node type stripping;
- all generated Rails example files pass `ruby -c`.
- the TypeRB implementation of core's internal insurers JSON controller passes
  its existing request spec (index, show, and not-found behavior).
- the generated Todo API passes `go test ./...`; its live JSON create/get/update
  flow preserves both relation types after applying `schema.sql` with sqldef.
- the generated React client passes strict `tsc` checking and a Vite production
  build.

## v0.1 Scope Boundary

Portable v0.1 control flow includes conditionals, `while`, integer ranges, and
typed `each`/`each_slice`/`with_index` iteration. Ruby-only syntax is valid in
Ruby projects through explicit native nodes and is rejected in the portable
targets. Full generics inference, a complete Ruby semantic type model, and
source maps are later work.

## Next Work

The broader path to practical production use is tracked in `ROADMAP.md`.

1. Add portable enums/sum types, exhaustive type dispatch, fallible results,
   and filesystem/process APIs required to move the
   lexer/parser into the stage-1 self-host tree.
2. Generate runtime JSON decoders from record contracts and add wire-compatible
   optional/nullable semantics.
3. Add a concise React view/component syntax above the current explicit element
   builder.
4. Extend structured iterator IR with transformations such as `map`, `select`,
   and lazy pipelines; add first-class block/lambda values separately.
5. Add source maps from generated code back to `.trb` spans.
6. Add incremental project compilation and a persistent build cache.
7. Continue migrating small core endpoints and expand the Rails provider to
   associations, scopes, strong parameters, and more framework/gem APIs.
