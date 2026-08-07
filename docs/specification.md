# TypeRB Specification Draft v0.2

Last updated: 2026-08-07

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

Mode selection controls transpilation output and target package/toolchain
integration. The concrete manifest and toolchain settings belong to the
[project configuration reference](configuration.md).

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

#### Callable names ending in `?` or `!`

- A function or method name may end in one `?` or `!`. The suffix is part of
  the source-level name and is preserved in the AST, typed IR, diagnostics,
  imports, completion, interface checks, and inheritance.
- The suffix is a naming convention only. `?` does not imply a Boolean return
  type, and `!` does not imply mutation, failure, exception behavior, or
  `Result` propagation. Those properties remain explicit in the signature and
  implementation.
- Ruby output retains the source spelling. Backends whose ordinary identifiers
  cannot contain the suffix encode the complete UTF-8 source name in a
  compiler-reserved namespace. This makes the lowering deterministic and keeps
  distinct TypeRB names from silently colliding. Encoded target names are a
  compiler implementation detail rather than a target-language API contract.

### 3.2 Typing

- Parameter/type annotation form is `name: Type`.
- Local variable initialization uses `:=`.
- Type inference is enabled for `:=`.
- Explicit type with initialization is allowed: `x: T := expr`.

#### Union types

- `A | B` is a portable union type. Unions are flattened, duplicate
  alternatives are removed, and their displayed order is deterministic.
- Collection inference first retains an equivalent type, then selects the most
  specific type available through safe implicit conversion, and otherwise
  constructs a union. Because Integer widens safely to Float,
  `Integer | Float` normalizes to `Float` rather than remaining a union.
- A value is assignable to a union when it is assignable to one of the union's
  alternatives. A union is assignable to another union only when every source
  alternative is accepted by the target. A union does not implicitly narrow
  to one alternative.
- Direct operators and receiver methods are not selected from a union. Code
  must first narrow the value with an exhaustive type case. The initial
  portable type patterns are non-nullable Boolean, Integer, Float, and String:

```trb
def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end
```

- A type pattern binds exactly one narrowed value. The exact `_` discards it,
  while `_name` follows the ordinary readable unused-binding opt-out rule.
  Without `else`, every union alternative must be handled exactly once. An
  `else` handles the remaining alternatives.
- More complex values may be retained in a union and passed through compatible
  annotations or `Any`, but type patterns for nullable, collection, enum,
  record, and class alternatives are staged until one runtime test model is
  specified across all backends.

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
  `values.push(item)`, `values.shift()`, or their package forms.
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
  cannot be read as an expression or used as a local declaration name. A name
  that begins with `_`, such as `_value`, remains a normal readable binding but
  is exempt from the unused-binding error. This opt-out applies consistently to
  method-local, iterator, and payload-pattern bindings. More than one binding
  with the same name, including `_`, remains a duplicate in one block or
  pattern.
- Leading `_` therefore remains context-sensitive but unambiguous: it marks a
  class member private in a member declaration and opts a lexical binding out
  of required-use checking in a local, iterator, or pattern declaration. A
  lexical binding keeps ordinary scope precedence over a same-named member.
- Method parameters, fields, constants, and top-level bindings are not subject
  to the local unused-binding rule. The REPL also permits an import-only
  submission because a later submission may use it; project builds still
  enforce ordinary import usage.

### 3.8 Program Entry

- A runnable project defines exactly one top-level `def main()`.
- `main` is a language convention and is not configurable in
  `trbconfig.jsonc`.
- Projects intended only as libraries may omit `main`.

Build and execution behavior belongs to the [CLI reference](cli.md).

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
- Non-empty literals infer their key and value types. Equivalent values retain
  their type; values with a safe most-specific common type use that type and
  receive the required implicit conversions. For example, Integer and Float
  values infer Float. Values without a safe common type retain their
  alternatives in a normalized union such as `Integer | String`. An empty `{}`
  receives its type from a declared variable, field, parameter, record field,
  assignment target, or return type.
- A fresh literal may be contextually widened when every entry is assignable,
  for example `Hash<String, Any> := {"count" => 1}`. Existing mutable Hash
  values are invariant in both arguments, preventing an alias from inserting
  a value that violates the original Hash type.
- Updating an entry requires a `mut` receiver. `hash[key] = value` may insert
  or replace an entry and checks both key and value types.
- `hash.each do |key, value| ... end` binds a `K` key and its `V` value through
  structured iteration IR. It requires exactly two block parameters and has
  the same `break`, `next`, and enclosing-method `return` behavior as Array
  iteration. Iteration uses a shallow entry snapshot captured before the first
  block call, while enumeration order is unspecified. Hash `each.with_index`,
  `each_slice`, `map`, `select`, and `reduce` are not enabled in v0.1.
- `hash[key]` is a required lookup and raises a runtime error when the key is
  absent in every backend and the REPL.
- Compound assignment to a Hash entry is reserved until its evaluate-once and
  missing-key behavior is represented directly in typed IR. Write
  `hash[key] = hash[key] + value` in v0.1.

Arrays and Hashes describe homogeneous collections. A union element type
retains the alternatives of heterogeneous collection values, while a future
Tuple retains the type of each array-like position and a `record` retains the
type of each named field. `Array<Integer | String>[0]` therefore remains
`Integer | String`; exact constant-index inference belongs to Tuple.

The complete collection receiver and package API belongs to the
[standard-library reference](standard-library.md).

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

### 3.14 Value-producing control flow

- `if` and `case` retain their statement forms and may also appear wherever an
  expression is accepted.
- An `if` expression must contain an `else`, even when it has one or more
  `elsif` branches. A `case` expression must be exhaustive under the ordinary
  enum or union rules; an `else` may cover the remaining alternatives.
- A branch must end in a result expression or transfer control with `return`,
  `break`, or `next`. Blank lines and comments after the final expression or
  transfer do not change the branch. Earlier statements execute within the
  branch scope first. The ordinary function and loop placement rules for each
  transfer still apply.
- Branch result types must be equivalent or admit one safe common type. The
  checker applies the same explicit Integer-to-Float widening used by ordinary
  assignment. It does not choose `Any` or synthesize a new union merely to
  combine incompatible branch results. Diverging branches are excluded from
  this common-type calculation.
- The checker represents divergence with the internal bottom type `Never`.
  When every branch diverges, the control-flow expression also has `Never`;
  otherwise its type is the common type of the value-producing branches.
  `Never` is not a source-level type name in v0.1.
- Typed IR records each branch result and whether that branch diverges. Go and
  TypeScript hoist expressions containing enclosing transfers into structured
  statements and compiler-owned temporaries, preserving the function or loop
  that owns `return`, `break`, or `next`. Ruby uses native value-producing
  control flow, and the REPL propagates the same transfer directly.
- Divergence is supported in ordinary expression positions. The single-result
  blocks of `map`, `select`, and `reduce` are still lowered as target callbacks
  and therefore reject an enclosing `return`; explicit `each` remains the
  portable alternative until multi-statement transformations are designed.
  This rule does not make loops into expressions, add values to `break` or
  `next`, or introduce additional unreachable-code diagnostics.

```trb
label := if ready
	"ready"
else
	"waiting"
end

text := case result
when Result::Ok(value)
	value.to_s()
when Result::Err(error)
	return fallback(error)
end
```

### 3.15 Class member model and deferred design

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
