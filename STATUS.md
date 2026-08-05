# TypeRB Status

Last updated: 2026-08-04

## Current State

TypeRB v0.1 is implemented as a Go compiler prototype.

Implemented:

- `trb fmt`, with canonical tab indentation plus comment, percent-literal,
  interpolation, and heredoc preservation.
- `trb build` with Go, Ruby, and TypeScript backends selected by the project
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
  existing Go, Rails, or TypeScript project without modifying its manifest.
- Project-tree builds which copy non-`.trb` files for runnable Rails output.
- Lossless lexer with byte/line/column spans.
- Handwritten recursive-descent and Pratt parser.
- Mode-independent `;` statement separators, including compact declarations
  such as `class Empty; end`, with canonical newline expansion by `trb fmt`.
- Independent syntax AST and typed IR.
- Resolver/type checker with local inference, assignment/return checks, field
  initialization checks, duplicate detection, and private-member checks.
- Checked separation of class and instance members, imported/inherited
  `readonly` field enforcement, and REPL evaluation of class constants.
- Immutable `:=` bindings, explicit `mut` for reassignment and destructive
  Array updates, and runtime-initialized uppercase constants scoped to the top
  level, modules, or classes.
- Non-nullable `Boolean` conditions for portable `if`, `elsif`, and `while`,
  preventing target-specific truthiness differences.
- Checked unary, arithmetic, comparison, equality, logical, and compound
  operators, including target-independent Integer division/remainder and
  precedence-preserving backend output.
- Structured `break` and `next` statements for `while` and collection
  iteration, checked outside loops and executed by all backends and the REPL.
- Generic `Hash<K, V>` values with String/Integer keys, contextual empty
  literals, invariant mutable aliases, checked index updates, and required
  missing-key lookup semantics shared by all backends and the REPL.
- Closed nominal `enum` declarations with typed payload variants, dedicated
  construction IR, positionally typed pattern bindings, and enum-only `case`
  dispatch. Duplicate/wrong-member diagnostics, required exhaustiveness when
  `else` is omitted, cross-file signatures, three backend representations, and
  REPL execution are covered.
- Explicit user-defined generics for payload enums and top-level functions,
  with invariant type arguments, checker-owned substitution, imported generic
  signatures, generic case narrowing, three backend outputs, formatter support,
  and typed-IR REPL execution.
- Portable `Result<T, E>` from `trb/std/result`, implemented as compiler-owned
  TypeRB source with checked `Ok`/`Err` payloads, exhaustive pattern matching,
  generated runtime modules for all three backends, and typed-IR REPL execution.
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
- Portable `trb/std/io`, `trb/std/strings`, `trb/std/arrays`, `trb/std/hashes`,
  `trb/std/bytes`, `trb/std/string_builder`, `trb/std/unicode`,
  `trb/std/path`, `trb/std/filesystem`, `trb/std/process`, `trb/std/numbers`,
  and `trb/std/result` packages, plus portable `Unit` values and mode-checked
  Go, Ruby, and TypeScript platform packages.
- Compiler-owned receiver-method contracts shared with portable package APIs,
  including strict and Result-returning integer conversion and Unicode-aware
  `String#size` across the checker, typed IR, all backends, and the REPL.
- A distinct portable `Bytes` type with UTF-8 conversion, byte length/indexing,
  non-mutating concatenation, validity checks, package and receiver APIs, three
  backend representations, and typed-IR REPL values.
- A portable mutable `StringBuilder` with checked `mut` receiver/package
  operations, Unicode scalar appends, String snapshots, code-point length,
  emptiness, clear, three backend representations, and typed-IR REPL execution.
- Compiler-owned Unicode 15.0.0 range tables emitted as TypeRB source, with
  scalar/category and identifier classification, code-point construction,
  default/named imports, three runnable backends, and REPL execution.
- Compiler-owned generic collection contracts inferred from `Array<T>` and
  `Hash<K, V>`, with strict and Result-returning lookup, strict edge access,
  emptiness and size queries, shallow copies, typed key/value extraction,
  checked Array mutation, all receiver forms, three backends, and typed-IR REPL
  execution.
- Expanded compiler-owned receiver methods for String code points, emptiness,
  containment, prefix/suffix checks, exact splitting and case conversion, plus
  constrained `Array<String>` joining and generic mutable Array pop.
- Structured, value-producing `map`, `select`, and `reduce(initial)` collection
  expressions with typed item/result/accumulator IR, optional index binding for
  map/select, formatter support, three runnable backends, and matching REPL
  execution. v0.1 transformation blocks contain one result expression.
- A `/`-based portable lexical path library written in compiler-owned TypeRB,
  with normalization, joining, inspection and decomposition shared by all
  generated targets and the REPL.
- A compiler-owned portable filesystem facade with typed `FileError` values,
  UTF-8 and raw-byte I/O, existence checks, recursive directory creation,
  sorted listing, meaningful `Result<Unit, FileError>` mutation results, an
  inaccessible internal intrinsic boundary, runnable Go, Ruby, and TypeScript
  output, and matching typed-IR REPL execution.
- A compiler-owned portable process facade with argv/environment/working
  directory access, shell-free captured execution, typed non-zero exits versus
  launch errors, runnable Go/Ruby/TypeScript output, and REPL execution.
- A compiler-owned portable JSON/JSONC value model with typed syntax, decode,
  and encode errors, JSON Pointer paths, comment-aware JSONC parsing, strict
  trailing-comma rejection, safe cross-target number semantics, runnable Go,
  Ruby, and TypeScript output, and typed-IR REPL execution.
- Checked JSON record codecs retained as typed-IR schemas and generated for all
  backends without target reflection, including wire-name attributes,
  nullable and nested fields, typed arrays and String-keyed Hashes, and REPL
  execution.
- Closed `record` declarations as distinct AST/typed-IR nodes, with
  keyword-only construction, field checking, Go structs/JSON/GORM tags, Ruby
  `Data` output, and TypeScript interfaces.
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
typed `each`/`each_slice`/`with_index` iteration. Portable collections include
typed Arrays and `Hash<K, V>` with strict lookup. Ruby-only syntax is valid in
Ruby projects through explicit native nodes and is rejected in the portable
targets. Union/Tuple inference, full generics inference, a complete Ruby
semantic type model, and source maps are later work.

## Next Work

The broader path to practical production use is tracked in `ROADMAP.md`.

1. Extend collection transformation blocks beyond the v0.1 single-expression
   baseline and add the remaining small receiver APIs needed by real
   standard-library code; safe conversion and lookup Results are available as
   the explicit failure baseline.
2. Exercise the process APIs while moving the lexer/parser into the stage-1
   self-host tree. Revisit concise Result propagation syntax only after
   explicit Result handling has been exercised in real application code.
3. Add a concise React view/component syntax above the current explicit element
   builder.
4. Extend structured iterator IR with transformations such as `map`, `select`,
   and lazy pipelines; add first-class block/lambda values separately.
5. Add source maps from generated code back to `.trb` spans.
6. Add incremental project compilation and a persistent build cache.
7. Continue migrating small core endpoints and expand the Rails provider to
   associations, scopes, strong parameters, and more framework/gem APIs.
