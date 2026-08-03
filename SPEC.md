# TypeRB Specification Draft v0.2

Last updated: 2026-08-03

## 1. Language Goals

TypeRB is a class-based, statically typed language implemented in Go and transpiled to Ruby, TypeScript, and Go.

Design principles:

- Keep Ruby ergonomics while removing implicit behavior.
- Prefer explicit, simple, Go-like rules.
- Keep syntax and semantics consistent across transpile targets.

## 2. Project Target Modes

A project declares exactly one output mode in `trbconfig.jsonc`: `ruby`, `go`,
or `typescript`. Source files do not contain mode declarations.

Mode selection controls transpilation output and package-manager/toolchain
integration:

- Ruby mode generates `Gemfile` and uses Bundler.
- Go mode generates `go.mod` and uses the Go module toolchain.
- TypeScript mode generates `package.json` and uses npm in v0.1.

## 3. Core Syntax and Semantics (Confirmed)

### 3.1 Declarations and Style

- Class-based language.
- Method declaration keyword: `def`.
- Block terminator: `end`.
- Parentheses are never omitted in ordinary calls. Portable iterator blocks
  use the Ruby-shaped `values.each do |value| ... end` syntax.
- `return` is mandatory in method bodies.
- No explicit `void` type notation (Go-like). Methods with no return value omit return type.

### 3.2 Typing

- Parameter/type annotation form is `name: Type`.
- Local variable initialization uses `:=`.
- Type inference is enabled for `:=`.
- Explicit type with initialization is allowed: `x: T := expr`.

### 3.3 Access Rules (Private)

- Private class/method names must start with `_`.
- External access to private members is forbidden and must be a compile-time error.

### 3.4 Instance Variables

- Instance variables are declared at class scope (TypeScript-like), not implicitly introduced inside `initialize`.
- Instance variable names use `@name`.
- Private instance variables use `@_name`.
- Zero-value initialization is not adopted initially.
  - Therefore, initialization must be explicit (e.g., in `initialize`).

### 3.5 Assignment Rules

- `:=` is for local variable first definition.
- `=` is for reassignment/update.
- `@ivar := expr` is disallowed; instance variables use declared fields and `=` updates.

### 3.6 Imports and Formatting

- Imports are explicit in every mode and are resolved before type checking.
- Project module identities come from paths below `sourceDir`; source files do
  not declare target packages.
- Portable packages use `trb/std/*`. Mode-specific APIs use mode-checked
  `trb/platform/<mode>/*` packages.
- Official formatter command: `trb fmt`.

## 4. Open Point: Initial Generic Inference Scope

Current decision:

- `x: T := expr` is allowed.
- Inference from `:=` is enabled.

Pending implementation-scope decision:

- Whether to support full inference for function calls and generics in the earliest implementation stage.
- If implementation cost is comparable, adopt support early; otherwise phase it in.

## 5. Example (Current Direction)

```trb
import app/user_repo

class User
  @name: String
  @_token: String

  def initialize(name: String, token: String)
    @name = name
    @_token = token
    return
  end

  def name(): String
    return @name
  end

  def _token(): String
    return @_token
  end
end

u := User.new("Alice", "secret")
```

## 6. Implementation Tooling and Algorithms

### 6.1 Parser Strategy

Initial implementation should use a handwritten recursive descent parser in Go.

This is an implementation choice, not a long-term architecture constraint. The
parser must be isolated behind a stable boundary:

```text
source
  -> lexer/token stream
  -> parser
  -> syntax AST
  -> resolver/typechecker
  -> typed AST or IR
  -> codegen
```

To keep a future parser-generator migration practical, the following interfaces
must be stable from the first implementation:

- Token kinds and token source spans.
- Syntax AST node shapes.
- Parse diagnostic structure.
- Golden parser tests.

Parser implementation details must not leak into resolver, typechecker, or
codegen phases.

### 6.2 Comment and Source Span Policy

Formatter comment placement may be implemented after MVP, but the lexer must
preserve comment tokens and all AST nodes must carry source spans from the first
implementation.

Initial formatter behavior may be limited to comment-free files or to a small
safe subset of comment placement. However, the architecture must allow a later
formatter to attach comments using both syntax AST spans and the original
token/comment stream.

Required first-stage behavior:

- Lexer can emit both code tokens and comment tokens.
- Parser can skip comment tokens for grammar parsing.
- Syntax AST nodes carry start/end source positions.
- Formatter entrypoint can receive both syntax AST and token/comment stream.

### 6.3 Type Checking Rollout

MVP type inference should support local variable inference from `:=`.

Full inference for function calls and generics is not required for the first
implementation unless the implementation cost is comparable to the minimal
inference path. Generic syntax and semantics may be reserved and staged.

### 6.4 TDD Implementation Plan

Implementation should proceed test-first around compiler phase boundaries.

Recommended first test layers:

1. Lexer token tests.
2. Parser AST golden tests.
3. Parser diagnostic tests.
4. Resolver and symbol-table tests.
5. Type-checking tests for declarations, assignment, returns, and private access.
6. Codegen golden tests per target mode.
7. Formatter golden tests.

Each phase should have tests that lock down its public output instead of its
internal implementation strategy. This keeps later parser or formatter changes
possible without rewriting the rest of the compiler.

## 7. Next Phase: Detailed Design

Project execution policy is tracked in `PROJECT_PLAN.md`.

Next discussion should define:

1. Concrete token definitions.
2. Concrete syntax AST node definitions.
3. Parse diagnostic schema.
4. Type-checking phases and symbol-table structure.
5. Incremental rollout plan for generics and function-call inference.
6. Code generation architecture for Ruby/TypeScript/Go backends.
7. Formatter stability guarantees and initial unsupported cases.

## 8. v0.1 Implementation Profile

The v0.1 prototype fixes the previously open implementation decisions as
follows.

### 8.1 Compiler Pipeline

All targets use the same phase pipeline:

```text
lossless tokens -> syntax AST -> resolver/type checker -> typed IR -> backend
```

Code generators consume IR and do not inspect parser state or source text.
Expressions in checked portable syntax have a semantic type in the checker
result and IR.

### 8.2 Ruby and Rails Interoperability

The Ruby mode accepts normal Ruby/Rails DSL outside the portable grammar through
explicit native syntax nodes:

- `NativeStatement`
- `NativeBlock`
- `NativeExpression`

Native nodes are preserved through AST and IR and emitted only by the Ruby
backend. TypeScript and Go report them as compile errors. This is the v0.1
extension mechanism for gem-provided Rails DSL; it is not a text-rewrite
compiler path.

Rails projects may mix `.rb` and `.trb` files. Project builds copy non-`.trb`
files and replace each `.trb` file with the extension selected by the project
mode in `trbconfig.jsonc`.

Importing `trb/platform/ruby/rails` also activates the compiler-owned Rails type
provider. Providers produce a target-independent Declaration IR; application
authors do not maintain parallel signature files. The Rails provider parses
`db/schema.rb` into a dedicated Schema AST and derives ActiveRecord model,
column, finder, relation, controller, and included-helper types. Known provider
types reject unknown members rather than silently degrading to `Any`.

### 8.3 Project and Package Configuration

`trbconfig.jsonc` is the source of truth for target selection, source/output
directories, and target dependencies. It accepts JSONC comments and trailing
commas. `trb sync` derives `Gemfile`, `go.mod`, or `package.json`; `trb install`
delegates installation to Bundler, the Go toolchain, or npm.

Imports pass through a dedicated resolver between parsing and checking. The
resolver validates project files and exports, rejects platform packages in the
wrong mode, and records package/symbol identities for lowering. Standard calls
therefore reach IR as resolved intrinsics rather than target-language text.

The portable prelude contains `puts(value: Any)`, preserving Ruby-like source
while lowering to the appropriate output primitive in each mode. An
explicit `import trb/std/io` exposes the identical intrinsic as `io.puts`.

Functions that return no value omit the return annotation: `def save()` is
valid, while `def save(): Void` is a syntax error. `Void` remains an internal
semantic/IR type, but is not a TypeRB return-type spelling.

### 8.4 Ruby Keyword Parameters

The ordinary Ruby forms `name: default` and required `name:` remain keyword
parameters. A typed positional parameter is `name: Type`. To disambiguate a
typed Ruby keyword parameter in v0.1, use:

```trb
def find(active:: Boolean = true)
end
```

The Ruby backend emits `def find(active: true)`.

### 8.5 Formatter Guarantees

The v0.1 formatter is deterministic and idempotent. It uses the parsed program
and lossless token stream, preserves standalone/trailing comments, and treats
heredoc and percent-literal contents as opaque. Comment positions and literal
contents therefore survive repeated formatting.

### 8.6 v0.1 Portable Control Flow

Dedicated AST and IR nodes exist for `if`/`elsif`/`else`, `while`, `return`,
integer ranges, and structured collection iteration. Inclusive `start..end`
and exclusive `start...end` ranges have type `Range<Integer>`.

Arrays and ranges support both iterator block delimiters. The delimiters have
identical precedence and semantics in TypeRB:

```trb
values.each do |value|
  puts(value)
end

values.each { |value| puts(value) }
```

`each_slice(size)` yields `Array<T>`. Appending `.with_index` adds a zero-based
`Integer` block parameter:

```trb
values.each_slice(2).with_index do |slice, index|
  puts(index)
end
```

These forms lower to structured iteration IR, not target callbacks. A `return`
inside the block therefore returns from the enclosing TypeRB method in every
mode. `each_slice` sizes must be positive.

### 8.7 Records and cross-mode contracts

`record` is a closed product type, not class syntax sugar:

```trb
record TodoItem
  id: Integer
  title: String
  completed: Boolean
  tags: Array<Tag>
end
```

Record construction is keyword-only and checked against the complete field
set. Records cannot inherit. The syntax AST and typed IR have dedicated record
and record-field nodes. Go lowers a record to a value struct with JSON tags,
TypeScript lowers it to an interface, and Ruby lowers it to `Data`.

Projects may map a portable source directory into the import graph with
`localPackages`. An `index.trb` file is the package entrypoint, so two projects
with different modes can import the same source without copying it:

```jsonc
"localPackages": {
  "todo/contracts": "../../packages/contracts/src",
}
```

### 8.8 Explicit platform application APIs

The first full v0.1 target uses `trb/platform/go/http` with the current Go 1.26
toolchain and ServeMux
patterns, `trb/platform/go/gorm` for typed GORM operations, and
`trb/platform/typescript/react`/`web` for React and Fetch. These are resolved
platform imports and typed intrinsics; application source does not contain raw
target-language fragments.

For Go projects, `go.sqldef` may declare a command, arguments, database, and
schema. `trb run` applies the schema before starting the generated program and
passes the selected database path explicitly.

### 8.9 Project-aware REPL

`trb repl` resolves `trbconfig.jsonc` in the same way as build and run. There is
no mode-less REPL in v0.1: the configured mode determines available platform
packages, type providers, project imports, and runtime adapters. Every accepted
submission passes through the ordinary lexer, parser, resolver, checker, and
typed IR lowering pipeline before evaluation.

The REPL is a language evaluator rather than an application console. It loads
project declarations and constants but does not invoke the configured
entrypoint. A future `trb console` may load a running application's framework
and resources.

On an interactive terminal, the REPL provides multiline cursor editing,
persistent per-project history, reverse history search, completion, and colored
prompts/results/diagnostics. Cursor editing includes the common Readline/Emacs
Ctrl-B/F, Ctrl-A/E, Ctrl-P/N, and Alt-B/F navigation bindings. Piped input
remains deterministic and free of terminal control sequences. Ctrl-C interrupts
the current input or IR evaluation and returns to the prompt; it does not leave
the REPL.
