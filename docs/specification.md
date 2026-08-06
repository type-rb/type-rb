# TypeRB Specification Draft v0.2

Last updated: 2026-08-06

## 1. Language Goals

TypeRB is a class-based, statically typed language. Its compiler is implemented
in Go and targets Go, Ruby, and TypeScript.

Design principles:

- Keep the source concise while removing implicit behavior.
- Prefer explicit, simple, Go-like rules.
- Keep syntax and semantics consistent across transpile targets.

## 2. Project Target Modes

A project declares exactly one output mode in `trbconfig.jsonc`: `go`, `ruby`,
or `typescript`. Source files do not contain mode declarations.

Mode selection controls transpilation output and package-manager/toolchain
integration:

- Go mode generates `go.mod` and uses the Go module toolchain.
- Ruby mode generates `Gemfile`, defaults to Ruby 4.0.6, and uses Bundler.
- TypeScript mode generates `package.json` and uses npm in v0.1.

Mode never changes TypeRB grammar or relaxes its portable type rules. The same
source syntax has the same meaning in every mode. Target-specific APIs,
framework integration, native escape syntax, and compatibility behavior are
available only through an explicit `trb/platform/<mode>/*` import; selecting a
mode by itself does not enable them.

## 3. Core Syntax and Semantics (Confirmed)

### 3.1 Declarations and Style

- Class-based language.
- Method declaration keyword: `def`.
- Block terminator: `end`.
- Parentheses are never omitted in ordinary calls. Portable iterator blocks
  use `values.each do |value| ... end` syntax.
- `return` is mandatory in method bodies.
- No explicit `void` type notation (Go-like). Methods with no return value omit return type.
- Outside `()`, `[]`, and `{}`, `;` is equivalent to a newline between
  complete statements. This is common syntax in every mode, so compact input
  such as `class Empty; end` has the same meaning as its multiline form.
- `trb fmt` expands top-level `;` separators into canonical lines. Semicolons
  inside opaque literals and the compact iterator form
  `values.each { |value| puts(value); puts(value) }` remain intact.

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

- `:=` introduces an immutable local binding. It cannot be rebound or used as
  the receiver of a destructive collection operation.
- `mut x := expr` introduces a mutable local binding. `mut` is required for
  later `=`/compound assignment and destructive operations such as
  `values.push(item)` or `arrays.push(values, item)`.
- A readonly reference cannot be made mutable merely by assigning it to a new
  `mut` binding.
- A non-nullable `Integer` is assignable to a non-nullable `Float`. The checker
  records this widening explicitly and typed IR lowers it in initializers,
  assignments, arguments, record and enum payloads, defaults, and returns.
  `Float` does not narrow implicitly to `Integer`.
- Method parameters and iterator block parameters are mutable bindings in
  v0.1. Class fields use their existing `readonly` modifier instead of `mut`.
- `@ivar := expr` is disallowed; instance variables use declared fields and `=` updates.

### 3.6 Constants

- An identifier beginning with an uppercase letter is a constant; no `const`
  or `let` keyword is added.
- Constants use declaration syntax such as `MAX_ITEMS := 100`.
- Constants may be declared only at top level or directly inside a `module` or
  `class`, never inside a method or control-flow block.
- Constant initializers are ordinary runtime expressions; a constant is not
  restricted to a target language's compile-time constant subset.
- Constants are always immutable and cannot be declared with `mut`, rebound,
  or passed to destructive APIs. For example,
  `DEFAULT_TAGS.push("work")` is a compile-time error.

### 3.7 Imports and Formatting

- Imports are explicit in every mode and are resolved before type checking.
- Every ordinary import must be used. A package import is used by referencing
  one of its members; every symbol in a named import list must be referenced.
  Imports that explicitly activate a compiler integration, such as a native
  syntax or type provider package, count as semantic uses.
- Project module identities come from paths below `sourceDir`; source files do
  not declare target packages.
- Portable packages use `trb/std/*`. Mode-specific APIs use mode-checked
  `trb/platform/<mode>/*` packages.
- Official formatter command: `trb fmt`.
- Canonical TypeRB indentation is one tab per nesting level. Formatter
  configuration is not part of v0.1; a future configuration surface may
  select a different indentation style without changing language semantics.

#### Unused bindings

- A local binding declared inside a method must be referenced. Iterator block
  parameters and payload-pattern bindings follow the same rule.
- The exact name `_` explicitly discards an iterator or pattern value and
  cannot be read as an expression. More than one binding with the same name,
  including `_`, remains a duplicate in one block or pattern.
- Method parameters, fields, constants, and top-level bindings are not subject
  to the local unused-binding rule. The REPL also permits an import-only
  submission because a later submission may use it; project builds still
  enforce ordinary import usage.

### 3.8 Program Entry

- A runnable project defines exactly one top-level `def main()`.
- `main` is a language convention and is not configurable in
  `trbconfig.jsonc`.
- `trb run` compiles the project before every execution; a preceding
  `trb build` is not required.
- `trb build` emits generated target source. In Go mode,
  `trb build --compile` instead invokes the Go toolchain and emits an
  executable, using temporary generated source.
- Projects intended only as libraries may omit `main`.

### 3.9 Boolean Conditions

- Conditions in `if`, `elsif`, and `while` must have the non-nullable
  `Boolean` type.
- TypeRB does not apply Ruby, JavaScript, or target-specific truthiness to
  portable conditions. Values such as `0`, `""`, `nil`, collections,
  `Boolean?`, and `Any` are rejected as conditions.
- An explicit Ruby-native project may use an `Any` condition as a compatibility
  escape hatch while compiler-owned library providers are still incomplete.

### 3.10 Operators

- `!` and `not` accept non-nullable `Boolean`; unary `+` and `-` accept a
  non-nullable `Integer` or `Float` and preserve that type.
- `+` accepts numeric values or two `String` values. `-`, `*`, `/`, and `**`
  accept numeric values. When one numeric operand is `Float`, an `Integer`
  operand is widened and the result is `Float`; two `Integer` operands retain
  an `Integer` result. `%` accepts two `Integer` values.
- `<`, `<=`, `>`, and `>=` compare numeric values with the same widening rule.
  Portable `==` and `!=` compare numeric values, matching non-nullable
  `Boolean` or `String` values, two values of the same payloadless enum type,
  or a nullable value with `nil`. Equality for payload-bearing enums is
  reserved until one structural rule can be implemented identically by every
  backend.
- `&&`, `||`, `and`, and `or` accept two non-nullable `Boolean` values and
  return `Boolean`. Compound assignments apply the corresponding binary rule
  before checking that the result remains assignable to the mutable target.
- Integer `/` truncates toward zero, `%` is its corresponding remainder, and a
  negative exponent is invalid for Integer `**`. Backends and the REPL preserve
  these semantics instead of inheriting different target-language behavior.
- Parenthesized TypeRB expressions retain their AST precedence in generated
  code. Ruby-specific matching, comparison, and bitwise operators (`=~`, `!~`,
  `<=>`, `~`, `|`, `&`, `^`, `<<`, and `>>`) require an explicit Ruby-native
  import until portable semantics are defined.

### 3.11 Loop Control

- `break` exits the innermost enclosing `while`, `each`, `each_slice`, or
  `each.with_index` loop.
- `next` skips the remainder of the current iteration of that innermost loop.
- Both are statements with no value form. They are compile errors outside a
  loop and have identical semantics in every mode.
- `return` remains distinct: it exits the enclosing method, including when it
  appears inside an iteration block.

### 3.12 Hashes

- A portable hash type is written `Hash<K, V>` with exactly two type
  arguments. Bare `Hash` is rejected outside the explicit Ruby-native
  compatibility surface.
- v0.1 portable keys are non-nullable `String` or `Integer` values. A literal
  must use one homogeneous key type. The label-style `name: value` literal
  spelling has a `String` key in portable TypeRB; it becomes a Ruby `Symbol`
  only under an explicit Ruby-native import.
- Non-empty literals infer their key and value types. Heterogeneous values
  currently widen to `Any`; a future union-type phase will infer types such as
  `Integer | String` instead. An empty `{}` receives its type from a declared
  variable, field, parameter, record field, assignment target, or return type.
- A fresh literal may be contextually widened when every entry is assignable,
  for example `Hash<String, Any> := {"count" => 1}`. Existing mutable Hash
  values are invariant in both arguments, preventing an alias from inserting
  a value that violates the original Hash type.
- Updating an entry requires a `mut` receiver. `hash[key] = value` may insert
  or replace an entry and checks both key and value types.
- `hash[key]` is a required lookup and raises a runtime error when the key is
  absent in every backend and the REPL. A future optional lookup API will
  return a nullable/optional value explicitly.
- Compound assignment to a Hash entry is reserved until its evaluate-once and
  missing-key behavior is represented directly in typed IR. Write
  `hash[key] = hash[key] + value` in v0.1.

Arrays and Hashes describe homogeneous collections. Future union types retain
the alternatives of heterogeneous collection values, while a Tuple retains
the type of each array-like position and a `record` retains the type of each
named field. `Array<Integer | String>[0]` therefore remains
`Integer | String`; exact constant-index inference belongs to Tuple.

### 3.13 Enums, payloads, and exhaustive case

- An enum is a closed nominal type declared with `enum Name`, uppercase
  members, and `end`. Enum declarations are allowed at top level or directly
  inside a module.
- A payloadless member is written `Ready`. A payload-bearing member is written
  `Value(value: String)` using one or more required, positional, typed fields.
  Default, keyword, rest, and untyped payload fields are rejected.
- Members are explicitly qualified with `EnumName::Member`. They infer the
  enum's nominal type and cannot be mixed with members of another enum even
  when the member names match.
- A payload value is constructed with `EnumName::Member(value, ...)`.
  Payloadless members are values and are not callable.
- Portable `case` dispatches on enum variants. A payload pattern such as
  `when Token::Text(value)` introduces immutable bindings whose types come from
  the variant declaration. v0.1 patterns bind every payload field positionally;
  partial patterns, guards, nested patterns, and wildcard syntax are reserved.
  Duplicate branches and duplicate bindings are errors.
- Without `else`, a case must list every member. With `else`, omitted members
  are handled by that branch. The selector is evaluated exactly once in every
  backend and the REPL.
- A separate `sum` declaration is not introduced: payload-bearing enum members
  provide the closed sum-type model.

```trb
enum Token
	Text(value: String)
	Integer(value: Integer)
	EOF
end

def describe(token: Token): String
	case token
	when Token::Text(value)
		return value
	when Token::Integer(value)
		return value.to_s()
	when Token::EOF
		return "eof"
	end
end
```

### 3.14 Class member model and deferred design

- Instance fields and instance methods are accessed through an instance. A
  method declared with `def self.name()` is a class member and is accessed
  through the class. Using either member kind through the wrong receiver is a
  compile-time error, including for imported and inherited classes.
- A `readonly` field may be assigned while its declaring object is being
  initialized, but external assignment is a compile-time error. This rule is
  retained across project imports.
- Class constants are runtime-initialized immutable bindings and are available
  to both instance and class methods.

The following class semantics remain deliberately unsettled rather than being
inferred from Go, Ruby, or TypeScript:

- portable `super(...)`, superclass constructor chaining, field initialization
  order, and the Go representation of an initialized superclass;
- method mutation effects and whether calling a mutating method requires a
  mutable receiver binding;
- generic classes and variance, which belong to the user-defined generics
  phase;
- override compatibility and whether an explicit `override` marker is useful;
- whether a field and method may share one source-level member name. Until a
  common backend-safe rule is chosen, portable code should use a private
  backing field such as `@_name` for a public `name()` accessor;
- whether abstract, final, or protected class members add enough value to
  justify new syntax. They are not part of v0.1.

## 4. Initial user-defined generics

- The first user-defined generic declarations are payload enums and top-level
  functions: `enum Result<T, E>` and `def identity<T>(value: T): T`.
- Calls use explicit type arguments in this phase. Examples are
  `Result<Integer, String>::Ok(1)` and `identity<String>("value")`. The checker
  substitutes the arguments through parameters, return types, enum payloads,
  case bindings, and cross-file signatures before producing typed IR.
- Generic enum patterns omit repeated type arguments because the selector
  supplies them: a `case` over `Result<Integer, String>` uses
  `when Result::Ok(value)` and binds `value` as `Integer`.
- User-defined generic arguments are invariant. Missing, extra, and mismatched
  arguments are compile-time errors in every mode.
- Payloadless variants of generic enums are reserved until typed singleton
  construction has a portable representation. Generic records, classes,
  instance/class methods, constraints, variance declarations, and type-argument
  inference are staged work rather than implicit target-language behavior.

### 4.1 Standard Result

- `trb/std/result` exports the portable `Result<T, E>` payload enum with
  `Ok(value: T)` and `Err(error: E)` variants. It is imported explicitly with
  `import { Result } from trb/std/result`.
- Construction and handling use the ordinary generic enum rules. v0.1 keeps
  both control-flow paths visible with explicit constructors and exhaustive
  pattern matching:

```trb
import { Result } from trb/std/result

def unwrap(result: Result<Integer, String>): Integer
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return 0
	end
end
```

- The declaration is compiler-owned TypeRB source and passes through the same
  AST, checker, typed IR, and backend pipeline as application enums. Project
  builds emit its runtime module into the generated target tree.
- A standard-library operation may infer a `Result<T, E>` without a source
  import, for example `missing := values.try_fetch(9)`. The checker records the
  compiler-owned runtime dependency and project compilation emits and imports
  the Result runtime automatically. Source references to the declaration
  itself—including type annotations, constructors, and case patterns—still
  require `import { Result } from trb/std/result`.
- Concise propagation syntax such as postfix `?`, postfix `!`, or prefix `try`
  is deliberately not selected in v0.1. It may be added later as syntax sugar
  over explicit Result matching and early return after real application usage
  establishes which form is worth reserving.

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

Project execution policy is tracked in [project-plan.md](project-plan.md).

Next discussion should define:

1. Concrete token definitions.
2. Concrete syntax AST node definitions.
3. Parse diagnostic schema.
4. Type-checking phases and symbol-table structure.
5. Incremental rollout plan for generics and function-call inference.
6. Code generation architecture for Go/Ruby/TypeScript backends.
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

### 8.2 Ruby interoperability

The Ruby mode accepts explicitly imported Ruby syntax outside the portable
grammar through native syntax nodes:

- `NativeStatement`
- `NativeBlock`
- `NativeExpression`

Native nodes are preserved through AST and IR and emitted only by the Ruby
backend. Go and TypeScript report fallback nodes as unsupported portable
syntax rather than exposing Ruby-native AST terminology in user diagnostics.
This is the v0.1
extension mechanism for library-provided DSLs; it is not a text-rewrite compiler
path.

Ruby projects may mix `.rb` and `.trb` files. Project builds copy non-`.trb`
files and replace each `.trb` file with the extension selected by the project
mode in `trbconfig.jsonc`.

Platform packages may activate compiler-owned type providers. Providers produce
a target-independent Declaration IR so application authors do not maintain
parallel signature files. Provider-specific inference belongs to its platform
integration rather than the portable language specification.

### 8.3 Project and Package Configuration

`trbconfig.jsonc` is the source of truth for target selection, source/output
directories, and target dependencies. It accepts JSONC comments but rejects
trailing commas. `trb sync` derives `Gemfile` plus `.ruby-version`, `go.mod`, or
`package.json`; `trb install` delegates installation to Bundler, the Go
toolchain, or npm. Managed Ruby projects use the Bundler version included with
their configured Ruby release.

`trb build` emits generated source into `outDir`. Go projects may instead use
`trb build --compile` to compile the complete configured project with
`go build`. Generated Go source lives in a temporary directory and only the
executable is retained. Its default path is `bin/<project-name>`; `--outfile`
overrides the file path relative to the project root. The operation requires
one top-level `main()` and does not embed application files or run project
setup commands. Ruby and TypeScript executable packaging is not part of this
phase.

Imports pass through a dedicated resolver between parsing and checking. The
resolver validates project files and exports, rejects platform packages in the
wrong mode, and records package/symbol identities for lowering. Standard calls
therefore reach IR as resolved intrinsics rather than target-language text.

The portable prelude contains `puts(value: Any)` and lowers it to the
appropriate output primitive in each mode. An explicit `import trb/std/io`
exposes the identical intrinsic as `io.puts`. A statically typed `Float`
argument uses the same portable decimal spelling as `Float#to_s`.

Portable built-in and standard types may expose receiver methods.
Receiver syntax is not target-native escape syntax: the resolver maps it to a
compiler-owned standard-library contract, and typed IR records that contract.
The package and receiver forms consequently share argument checking, return
types, REPL semantics, and backend lowering. The initial surface includes
`Integer#to_s`, `Integer#to_f`, `Float#to_s`, `Float#to_i`, `String#to_i`,
`String#try_to_i`, and
Unicode-code-point-based `String#size`, corresponding to `trb/std/numbers` and
`trb/std/strings` operations. Unknown members on portable built-in types are
compile errors in every mode. Integer parsing accepts only a complete ASCII
decimal spelling matching `[+-]?[0-9]+`; values outside
`-9007199254740991..9007199254740991` are rejected so every target has the
same exact result. `to_i` raises a runtime error, while `try_to_i` and package
`try_parse_integer` return `Result<Integer, String>` with a stable error
message.

`Integer#to_f` is exact over the portable Integer range. `Float#to_i`
truncates toward zero and raises for a non-finite value or a result outside the
portable Integer range. `Float#to_s` emits the shortest fixed decimal spelling
used by the compiler runtime, never exponent notation; integral values retain
`.0`, negative zero is normalized to `0.0`, and non-finite values use `NaN`,
`Infinity`, or `-Infinity`.

`Bytes` is the portable immutable binary-sequence type; it is not an alias for
`Array<Integer>` or `String`. `trb/std/bytes` provides UTF-8 `from_string` and
`to_string`, byte `length` and zero-based `at`, non-mutating `concat`, and
`valid_utf8`. `String#to_bytes`, plus `Bytes#to_s`, `#size`, `#at`, `#concat`,
and `#valid_utf8`, resolve to those same contracts. Encoding a String always
produces UTF-8. Decoding replaces invalid input with U+FFFD; `valid_utf8`
allows callers to reject it first. An out-of-range `at` is a runtime error.
Consequently `String#size` counts Unicode code points while `Bytes#size`
counts encoded bytes.

`StringBuilder` is the portable mutable text accumulator from
`trb/std/string_builder`. `new` and `from_string` construct it; `append`,
`append_codepoint`, and `clear` are destructive operations and require a `mut`
binding in both package and receiver forms. `to_string`/`#to_s` returns an
immutable String snapshot, `length`/`#size` counts Unicode code points, and
`empty`/`#empty?` tests whether no text has been appended.
`append_codepoint` accepts a Unicode scalar value as an Integer and raises a
runtime error for negative values, surrogate code points, or values above
U+10FFFF.

`trb/std/unicode` owns portable Unicode scalar classification. Its
`valid_scalar`, `letter`, `digit`, `uppercase`, `lowercase`, `whitespace`,
`identifier_start`, and `identifier_part` functions accept an Integer code
point; `from_codepoint` performs the checked reverse conversion and `version`
reports the data version. v0.1 uses Unicode 15.0.0 tables pinned by the Go 1.26
compiler toolchain. The compiler emits those range tables as compiler-owned
TypeRB source and all three backends execute that same generated library;
classification never delegates to independently versioned Ruby or JavaScript
Unicode databases.

The built-in receiver surface also includes `String#codepoints`, `#empty?`,
`#include?`, `#start_with?`, `#end_with?`, `#split`, `#upcase`, and
`#downcase`, plus collection operations described below. String splitting uses
an exact String separator, preserves empty fields including a trailing field,
and raises a runtime error for an empty separator. As with the earlier
conversion and size methods, each resolves to its corresponding portable
package contract rather than a target-native method by name.

Compiler-owned package contracts may declare internal type parameters. They
are inferred from call arguments and receivers; this is library-contract
inference, not a mode-specific relaxation or user-defined implicit generic
call syntax. `trb/std/arrays` provides `length`, `empty`, strict zero-based
`fetch`, safe `try_fetch`, strict `first`/`last`, shallow `copy`, and mutable
`push` for `Array<T>`. Receiver spellings are `size`, `empty?`, `fetch`,
`try_fetch`, `first`, `last`, `dup`, and `push`. `trb/std/hashes` provides
`length`, `empty`, strict `fetch`, safe `try_fetch`, `contains_key`, `keys`,
`values`, and shallow `copy` for `Hash<K, V>`; receiver spellings are `size`,
`empty?`, `fetch`, `try_fetch`, `key?`, `keys`, `values`, and `dup`. Missing
strict fetch keys, out-of-range strict Array indexes, and first/last on an
empty Array are runtime errors in every mode. Safe fetch returns
`Result<T, String>` for Arrays and `Result<V, String>` for Hashes, with stable
missing-value messages. Hash key/value enumeration order is unspecified.

`Array<String>` additionally provides package/receiver `join`, and generic
`Array<T>` provides mutable, strict `pop`. `pop` requires `mut` and raises on
an empty Array. Receiver lookup checks the complete collection type, so a
method constrained to `Array<String>` is not exposed on `Array<Integer>`.

`map`, `select`, and `reduce(initial)` are structured, value-producing
collection expressions rather than target-native callback calls. `map` returns
`Array<U>` inferred from its block result, `select` requires a Boolean block
result and retains `Array<T>`, and `reduce` requires its block result to remain
assignable to the initial accumulator type. `map.with_index` and
`select.with_index` bind a zero-based Integer index; `reduce.with_index` is not
part of v0.1. Transformation blocks use their final expression as the value.
The v0.1 parser/checker accepts exactly one expression in these blocks; this is
an explicit staging boundary for later structured multi-statement block and
first-class lambda semantics. The AST and typed IR retain the source,
operation, bindings, item type, accumulator, and result expression until
backend lowering and REPL evaluation.

`trb/std/path` defines portable logical paths as `/`-separated Strings. It is
compiler-owned TypeRB source, so `clean`, two-part `join`, `absolute`,
`components`, `base`, `directory`, and `separator` execute the same lexical
algorithm in every backend. `clean` removes repeated separators and `.`
segments, resolves `..` without moving above an absolute root, and returns `.`
for an empty relative result. This package does not inspect the process working
directory, access the filesystem, recognize target-native drive letters or UNC
paths, or silently select target OS path rules.

`trb/std/filesystem` is the explicit host-filesystem bridge. It is
compiler-owned TypeRB source and exports `FileError`, `exists`, `read_text`,
`read_bytes`, `write_text`, `write_bytes`, `create_directory`, and `list`.
Every operation returns `Result<T, FileError>`; filesystem failures are values,
not target-specific exceptions. `FileError` records the portable `operation`,
the requested `path`, and a human-readable target message. The compiler-owned
implementation delegates only through `trb/internal/filesystem`, which cannot
be imported by application code.

Filesystem paths are host paths rather than `trb/std/path` logical paths.
`exists` returns `Ok(false)` only for a missing path and returns `Err` for other
inspection failures. Text is written as UTF-8 and invalid UTF-8 is replaced by
U+FFFD when read. Byte operations preserve raw bytes. `create_directory` is
recursive, writes do not implicitly create parent directories, and `list`
returns immediate child names sorted by UTF-8 byte sequence rather than
recursive or absolute paths. Successful writes and directory creation return
`Ok(Unit.new())`, typed as `Result<Unit, FileError>`. `Unit` is a portable
zero-field value from `trb/std/unit`; unlike `Void`, it can be stored in a
generic value while carrying no application data. `Void` remains a function
return category whose annotation is omitted. The TypeScript backend uses the
current Node host filesystem; a
runtime without that host API returns `Err` rather than exposing
browser-specific behavior.

`trb/std/process` is the explicit host-process bridge. It exports `argv()`,
`environment(name)`, `working_directory()`, and `run(command, args)`, plus
`ProcessResult` and `ProcessError`. `argv` excludes the executable name.
Missing environment variables return `nil` through `String?`, while
`working_directory` returns `Result<String, ProcessError>`.

`run` accepts a command String and a separate `Array<String>` of arguments and
never inserts a shell. It captures stdout and stderr as UTF-8 Strings, replacing
invalid input with U+FFFD. A process that starts and exits with any status
returns `Ok(ProcessResult)`; the record contains `status`, `stdout`, `stderr`,
and `success`. Launch or host failures return `Err(ProcessError)` with
`operation`, `command`, and `message`. Working-directory selection, stdin,
streaming, signals, timeouts, and explicit process exit are outside the v0.1
API.

`trb/std/json` provides the portable JSON value model. `JsonValue` is a
payload enum with `Null`, `Boolean`, `Integer`, `Float`, `String`, `Array`, and
`Object` variants. `parse` and `stringify` return `Result` values carrying a
`JsonError`; typed accessors such as `as_string`, `as_array`, and `field`
report decode errors instead of exposing target casts or unchecked dynamic
values. Error paths use JSON Pointer escaping. Line and column are one-based
when the target parser provides a stable source location and are otherwise
`nil`.

JSON numbers that are integral are represented as `Integer` only within the
portable exact range -9007199254740991 through 9007199254740991. Fractional
finite numbers are `Float`; non-finite or out-of-range values are errors. JSON
object serialization order and spelling details such as escaping are not part
of the portable contract. Applications must compare the decoded value rather
than serialized bytes when canonical output is required.

`trb/std/jsonc` shares the same value and error types and exposes `parse` with
line (`//`) and block (`/* ... */`) comments enabled. Removing comments
preserves source offsets. JSONC does not enable trailing commas: both JSON and
JSONC reject them. Application code cannot import the compiler-only
`trb/internal/json` lowering boundary. Go output currently uses the stable
`encoding/json` API; Go 1.26's `encoding/json/v2` remains an opt-in experiment
and is not required by generated projects.

Typed codecs use `decode<T>(source)` and `encode(value)`. The decoder requires
an explicit target type; the encoder infers it from its argument, while
`encode<T>(value)` is also valid. The checker constructs a closed codec schema
and retains it in typed IR. Backends generate conversions from that schema and
do not discover record fields through target reflection or accept unchecked
target casts in TypeRB source.

Codec schemas support Boolean, Integer, Float, String, nullable forms, arrays,
`Hash<String, V>`, and nested records. A record field's source name is its wire
name unless `@json("wire_name")` overrides it. Missing nullable fields decode
as `nil`; missing non-nullable fields and values of the wrong shape return a
Decode error at the field's JSON Pointer path. Unknown object fields are
ignored for forward compatibility. An integral JSON number may decode as
Float, but a fractional number cannot decode as Integer. Bytes, enums,
classes, untyped collections, non-String Hash keys, and recursive record
graphs are rejected at compile time in v0.1.

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
contents therefore survive repeated formatting. It emits one tab per nesting
level while leaving indentation inside opaque literal bodies unchanged.
Top-level semicolon statement separators are canonicalized to physical
newlines; nested and literal semicolons are preserved.

### 8.6 v0.1 Portable Control Flow

Dedicated AST and IR nodes exist for `if`/`elsif`/`else`, exhaustive enum
`case`, `while`, `return`, integer ranges, and structured collection iteration. Inclusive `start..end`
and exclusive `start...end` ranges have type `Range<Integer>`.

Portable `if`, `elsif`, and `while` conditions are checked as non-nullable
`Boolean`; generated targets never choose their own truthiness semantics.

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
record Message
	id: Integer
	text: String
	delivered: Boolean
end
```

Record construction is keyword-only and checked against the complete field
set. Records cannot inherit. The syntax AST and typed IR have dedicated record
and record-field nodes. Go lowers a record to a value struct with JSON tags,
Ruby lowers it to `Data`, and TypeScript lowers it to an interface.

Projects may map a portable source directory into the import graph with
`localPackages`. An `index.trb` file is the package entry module, so two projects
with different modes can import the same source without copying it:

```jsonc
{
  "localPackages": {
    "acme/contracts": "../../packages/contracts/src"
  }
}
```

### 8.8 Explicit platform APIs

Target-specific capabilities use resolved `trb/platform/<mode>/*` imports and
typed intrinsics. Application source does not contain raw target-language
fragments, and selecting a mode does not enable a platform API implicitly.

Platform integrations may add explicit toolchain or lifecycle configuration.
Their concrete contracts are documented with the integration and do not change
portable grammar or semantics.

### 8.9 Project-aware REPL

`trb repl` resolves `trbconfig.jsonc` in the same way as build and run. There is
no mode-less REPL in v0.1: the configured mode determines available platform
packages, type providers, project imports, and runtime adapters. Every accepted
submission passes through the ordinary lexer, parser, resolver, checker, and
typed IR lowering pipeline before evaluation.

The REPL is a language evaluator rather than an application console. It loads
project declarations and constants but does not invoke `main`. A future
`trb console` may load a running application's framework
and resources.

On an interactive terminal, the REPL provides multiline cursor editing,
persistent per-project history, reverse history search, completion, and colored
prompts/results/diagnostics. Cursor editing includes the common Readline/Emacs
Ctrl-B/F, Ctrl-A/E, Ctrl-P/N, and Alt-B/F navigation bindings. Piped input
remains deterministic and free of terminal control sequences. Ctrl-C interrupts
the current input or IR evaluation and returns to the prompt; it does not leave
the REPL.

Completion and live syntax highlighting use a presentation-independent
language-service layer. Completion combines the active module's checked typed
IR with tolerant lexical analysis of the submission being edited. It offers
keywords, built-in types and functions, visible bindings, declarations,
explicit imports, receiver methods, and enum or module members. Compiler-added
runtime dependencies do not become source-visible completion candidates. The
service returns semantic candidate kinds and UTF-8 byte replacement ranges;
the Readline adapter owns Tab menus and terminal rendering.

Syntax highlighting likewise returns classified source spans rather than ANSI
text. The REPL adapter colors keywords, types, constants, literals, comments,
functions, methods, and lexically invalid input while preserving the original
source bytes. The same completion and classification results are intended for
future browser and LSP adapters; neither terminal colors nor Readline types are
part of the language-service API.

### 8.10 Local browser playground and tour

`trb play` serves an isolated scratch playground on `127.0.0.1` and opens it in
the default browser. `trb tour` serves the same compiler surface with ordered
language lessons, local progress, and editable source. Both choose Go when no
project is present; a discovered project mode or explicit `--mode` selects the
initial target, and the browser can switch among Go, Ruby, and TypeScript.

Run requests use the ordinary parser, resolver, checker, typed IR lowering, and
REPL evaluator. Each request starts with fresh scratch state and evaluates
top-level statements rather than invoking `main()` or a target toolchain.
Evaluation has a time limit. Filesystem, process, and platform imports are
rejected for browser execution, while Transpile may still compile them and
display selected-target source. Format uses the ordinary lossless formatter.

The browser editor's TypeRB syntax highlighting is a presentation feature and
does not define lexical or semantic behavior. The compiler remains the only
source of diagnostics and accepted syntax. Compile diagnostics retain their
source range, which the browser presents as a source line and caret marker.
Runtime errors remain message-only until runtime source mapping and stack
translation are implemented. `trb tour --check` compiles and
evaluates each shipped lesson in all three modes as an explicit release or
language-change checkpoint; ordinary project builds do not validate the tour.
