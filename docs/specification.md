# TypeRB Specification Draft v0.2

Last updated: 2026-08-13

## 1. Language Goals

TypeRB is a class-based, statically typed language. Its compiler is implemented
in Go and targets Go, Ruby, and TypeScript.

Design principles:

- Keep the source concise while removing implicit behavior.
- Prefer explicit, simple, Go-like rules.
- Prefer one canonical spelling for each common operation. Add alternatives
  only when they express a meaningful semantic distinction or application
  experience demonstrates substantial value.
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

Portable packages may expose cancellable operations without adding a context
or `async` parameter to TypeRB source. Whole-project analysis forwards a hidden
execution scope through the generated call graph, including interface methods
and function values. Request disconnects and middleware deadlines cancel that
scope; generated control-flow checkpoints and supported native APIs observe it.
The scope belongs to backend lowering, is absent from the source type system
and typed IR signatures, and must not create mode-dependent source semantics.

## 3. Core Syntax and Semantics (Confirmed)

### 3.1 Declarations and Style

- Class-based language.
- Method declaration keyword: `def`.
- Block terminator: `end`.
- Parentheses are never omitted in ordinary calls. Portable iterator blocks
  use `values.each do |value| ... end` syntax.
- Functions and methods with an explicit return type must return a value on
  every reachable path. Final expressions are not implicit returns.
- Complete `if` flow and exhaustive enum or union `case` flow can satisfy the
  return rule. A return inside a loop alone does not prove that the function
  returns because initial loop analysis is conservative.
- A terminal unresolved expression enabled by an explicit Ruby-native import
  delegates its return behavior to Ruby and is outside this portable check.
  Known TypeRB expressions do not gain implicit return behavior from that
  import.
- No explicit `void` type notation (Go-like). Methods with no return value omit
  the return type and may fall through or use a bare `return`.
- A trailing positional parameter may declare a default with
  `name: Type = expression`. All following positional parameters must also
  have defaults. Defaults are evaluated when the function or method is called,
  may reference earlier parameters, and are used only for omitted arguments.
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
- A callable suffix is a naming convention only. `?` does not imply a Boolean return
  type, and `!` does not imply mutation, failure, exception behavior, or
  `Result` propagation. Those properties remain explicit in the signature and
  implementation.
- Ruby output retains the source spelling. Backends whose ordinary identifiers
  cannot contain the suffix encode the complete UTF-8 source name in a
  compiler-reserved namespace. This makes the lowering deterministic and keeps
  distinct TypeRB names from silently colliding. Encoded target names are a
  compiler implementation detail rather than a target-language API contract.

#### Function values

- `fn(parameters) ... end` creates a first-class, lexically scoped function
  value. Every parameter has an explicit type. An absent return annotation
  means no return value; a non-Void function uses `: Type` and must return that
  type on every reachable path.
- Function types are written `(ParameterType, ...) -> ReturnType`. `Void` is
  permitted in a function type, for example `(String) -> Void`, but remains
  omitted from the corresponding `fn` declaration.
- A function value may declare the same fallible effect as a named function:
  `fn(): User fails LoadError ... end`. Its type is written
  `() -> User fails LoadError`. Calling that value propagates or captures the
  effect through the ordinary `fails` and `attempt` rules. A pure function is
  assignable to a compatible fallible function type; a fallible function is
  not assignable to a pure function type.
- A function value owns `return` statements in its body. It may capture outer
  lexical bindings, while ordinary immutability and `mut` assignment rules
  continue to apply to captured values.
- Function values take required positional parameters in the initial syntax;
  defaults, keyword parameters, rest parameters, call blocks, and generic
  lambda parameters are not accepted.
- The compact spelling uses the ordinary statement separator rather than a
  second lambda syntax: `double := fn(value: Integer): Integer; return value *
  2; end`. `trb fmt` expands it to the canonical multiline form.
- TypeScript alone may lower a function value to an `async` callback when its
  body reaches a Promise-based platform API. Suspension remains a backend
  implementation detail rather than TypeRB syntax. A fallible function value
  has an explicit `Result` runtime boundary and may suspend with a non-Void
  success value. A pure suspending function value with a non-Void result
  remains rejected until a higher-order suspension contract is available.

#### JSX expressions

- JSX is structured shared grammar represented in the syntax AST and typed IR.
  Its element types and runtime meaning come from an explicitly imported JSX
  provider; JSX without a provider is rejected.
- The initial provider is `trb/platform/typescript/react`. It is a TypeScript
  platform package and emits ordinary TSX for the React toolchain. This target
  boundary does not make the grammar itself React- or TypeScript-specific.
- Within JSX only, a lowercase name is an intrinsic element and an uppercase
  name is a component reference. This rule does not change ordinary TypeRB
  function or constant naming.
- JSX attribute names retain their source spelling. The compiler does not
  translate names such as `className` or `onClick` to or from snake case.
- A JSX provider may declare checked intrinsic attributes. The initial React
  provider distinguishes mouse, change, form, and keyboard callbacks and maps
  their event objects to the corresponding React and DOM types.
- A TypeRB component returns the provider's node type. It accepts either no
  parameters or one record parameter; that record defines its checked props.
  Unknown, missing, duplicate, and incompatible component props are errors.
- Text, numbers, Booleans, nullable values, provider nodes, and recursively
  renderable arrays and unions may be JSX children. Other values are rejected.

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

#### Nullable narrowing

- Comparing a nullable lexical binding directly with `nil` narrows the
  non-`nil` path. `value != nil` narrows the matching `if`, `elsif`, or `while`
  body; `value == nil` narrows the unmatched `elsif` and `else` paths. The
  operands may be written in either order.
- Short-circuit Boolean evaluation carries the same fact into the right-hand
  side: `value != nil and use(value)` and `value == nil or use(value)` may use
  `value` as non-nullable in `use(value)`.
- A guard without `else` narrows the following statements when every matching
  branch returns. For example, after `if value == nil; return fallback; end`,
  `value` has its non-nullable type.
- Reassignment invalidates the narrowing immediately. The assignment itself is
  checked against the binding's declared nullable type, and subsequent uses
  must narrow again.
- Initial narrowing is deliberately limited to stable lexical identifiers.
  Member, index, and call expressions are not assumed to produce the same
  value when evaluated again. Typed IR records each nullable unwrap explicitly
  so Go, Ruby, TypeScript, and the REPL use the same checked flow facts.

#### Literal types and discriminated unions

- An explicit Integer or String literal may appear in a type position, for
  example `status: 201` or `kind: "created"`. It constrains that value to the
  one written literal. A literal value widens safely to its ordinary
  `Integer` or `String` type; an arbitrary scalar does not narrow implicitly to
  a literal type.
- Literal types may form unions such as `200 | 404`. A scalar alternative
  subsumes its literals, so `200 | Integer` normalizes to `Integer`. Nullable,
  array, and generic modifiers are not accepted on a literal type.
- `case` accepts explicit Integer and String values. A case over an ordinary
  scalar may be open and normally uses `else`; a case over a literal union is
  exhaustive and may omit `else` after handling every alternative.
- A union of records or classes exposes a data member only when every
  alternative exposes that member through the same storage model. The member
  type is the normalized union of the alternative field types. This rule does
  not select methods from a union.
- When such a common member has a literal type in every alternative, a case on
  `binding.member` narrows the complete lexical `binding` in each branch.
  Record fields are immutable discriminants. A class field must be declared
  `readonly`; imported fields retain their exported readonly flag.
- Alternatives may share a discriminant. That branch retains their union
  rather than choosing one arbitrarily. An `else` branch, when present, narrows
  to the unhandled alternatives.

```trb
record CreatedResponse
	status: 201
	body: String
end

record InvalidResponse
	status: 422
	body: Array<String>
end

type CreateResponse = CreatedResponse | InvalidResponse

def body_text(response: CreateResponse): String
	case response.status
	when 201
		return response.body
	when 422
		return response.body[0]
	end
end
```

Literal types are compile-time constraints rather than new runtime scalar
representations. TypeScript output retains them in generated types. Go and
Ruby erase them to the corresponding scalar while typed IR preserves the
discriminant and Go lowers erased-union field access through checked type
switches.

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
- A non-nullable value is assignable to the corresponding nullable type. The
  typed IR records this conversion explicitly so pointer-based backends do not
  expose their representation through portable semantics.
- A non-nullable `Integer` is assignable to a non-nullable `Float`. The checker
  records this widening explicitly and typed IR lowers it in initializers,
  assignments, arguments, record and enum payloads, defaults, and returns.
  `Float` does not narrow implicitly to `Integer`.
- Method parameters and iterator block parameters are mutable bindings in the
  current language. Class fields use their existing `readonly` modifier
  instead of `mut`.
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
- External TypeRB packages declare a canonical identity in
  `trbpackage.json`. A project lock may map an explicit short import to that
  identity. Alias mappings are scoped to the application or declaring package,
  so two dependencies may use the same short name for different packages.
  Package source passes through the ordinary parser, checker, typed IR, and
  selected backend.
- A REPL may add hidden imports for public declarations whose names resolve to
  exactly one project module, and for public types exported by portable
  standard packages. Ambiguous declarations still require an explicit import.
  This is an interactive convenience only; project source keeps the same
  explicit-import rule.
- Portable packages use `trb/std/*`. Mode-specific APIs use mode-checked
  `trb/platform/<mode>/*` packages.
- In TypeScript mode, a named import whose path belongs to a configured native
  dependency resolves through the declaration index produced by `trb install`.
  Its generated import keeps the original package specifier. Declaration
  shapes that cannot be represented safely are errors and never fall back to
  `Any`. A declarative provider from an installed TypeRB package may replace
  indexed exports and records without changing the application import. A
  provider may describe generic functions, classes, records, and transparent
  type aliases. Calls continue to use TypeRB's explicit type arguments, and
  generated TypeScript imports any transitive target types required by the
  selected contracts without exposing those helper names to TypeRB source. A
  provider may mark a fallible function field or parameter with the
  `promise_rejection` effect bridge. The TypeScript backend then unwraps the
  callback's `Result`, resolves `Ok(value)`, and rejects `Err(error)` only at
  that native boundary.
- Official formatter command: `trb fmt`.
- Canonical TypeRB indentation is one tab per nesting level. Formatter
  configuration is not part of the current language; a future configuration
  surface may select a different indentation style without changing language
  semantics.

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

Compiler-owned package declarations may mark a block operation as structured.
Unlike a target-language callback, a structured block remains in typed IR and
may produce the declared call result without changing lexical control flow. A
whole structured block call can be used as an initializer, assignment value,
or return value:

```trb
result := records.process_each() do |record|
	puts(record)
end
```

The result must be assigned or returned; silently discarding it is an error.
`return`, `break`, and `next` inside the block retain the same owners as an
ordinary portable iteration. Ordinary call blocks are not value-producing
unless their package declaration explicitly provides structured lowering.

Portable Array transformations `map`, `select`, and `reduce` use the same
typed-IR boundary. The short-circuit predicates `any?`, `all?`, and `none?`
and searches `find` and `find_index` require one non-nullable Boolean result
expression. They evaluate from left to right and stop when the result is known.
Empty Arrays produce `false`, `true`, and `true` for the predicates;
`find` and `find_index` return a nullable element and nullable `Integer`, with
`nil` for no match. Indexed predicate blocks are not currently enabled.

Array sorting is stable and non-destructive. `sort()` and
`sort_descending()` use the element's portable natural order. `sort_by` and
`sort_by_descending` accept one expression whose key is evaluated exactly once
per element; the key expression cannot use an operation that may fail. Equal
elements or keys retain their input order in both directions. Portable natural
order currently covers non-nullable `Integer`, `Float`, and `String`: numeric
values use numeric order, Strings use Unicode code point order without locale
collation, and Float `NaN` values follow all ordinary numbers in both
directions. Negative and positive zero compare as equal. Arbitrary comparators
and mixed-direction multi-key ordering are not part of the current language.

`uniq()` returns a new Array containing the first occurrence of each value in
input order. It uses portable `==` and is therefore available only when that
equality is defined for the element type. `concat(other)` returns a new Array
with the receiver's elements followed by `other`; neither input is mutated.
Unlike Ruby's destructive `Array#concat`, TypeRB `concat()` follows the
language's non-destructive collection default and does not require `mut`.

### 3.12 Hashes

- A portable hash type is written `Hash<K, V>` with exactly two type
  arguments. Bare `Hash` is rejected outside the explicit Ruby-native
  compatibility surface.
- Portable keys are currently non-nullable `String` or `Integer` values. A
  literal must use one homogeneous key type. The label-style `name: value` literal
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
  block call, while enumeration order is unspecified.
- `hash[key]` is a required lookup and raises a runtime error when the key is
  absent in every backend and the REPL.
- Compound assignment to a Hash entry is reserved until its evaluate-once and
  missing-key behavior is represented directly in typed IR. Write
  `hash[key] = hash[key] + value` in the current language.

Arrays and Hashes describe homogeneous collections. A union element type
retains the alternatives of heterogeneous collection values, while a future
Tuple retains the type of each array-like position and a `record` retains the
type of each named field. `Array<Integer | String>[0]` therefore remains
`Integer | String`; exact constant-index inference belongs to Tuple.

The complete collection receiver and package API belongs to the
[standard-library reference](standard-library.md).

### 3.13 Enums, raw values, and sum types

An `enum` declaration defines a closed nominal set of uppercase variants. It
is allowed at top level or directly inside a module. TypeRB uses the shared
variant and exhaustive-case model for two related but distinct purposes.

#### 3.13.1 Enumerated values and raw values

- An ordinary member is a payloadless value such as `Ready`.
- A raw-value enum assigns every payloadless member an explicit String or
  Integer literal, such as `Pending = "PENDING"` or `Unknown = -1`. This adds
  an external representation to the ordinary enumeration model.
- Once one member has a raw value, every member must have one of exactly the
  same type. Raw expressions, duplicate raw values, payload fields, implicit
  numbering, and generic raw-value enums are rejected.
- A raw-value enum remains nominal. It does not implicitly become its raw
  String or Integer type. `value.raw_value()` performs the outward conversion;
  `EnumName.from_raw(raw)` returns
  `Result<EnumName, EnumValueError>`. These generated names are reserved only
  inside enum declarations.

```trb
enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"

	def terminal?(): Boolean
		return self == OrderStatus::Completed
	end
end

status := OrderStatus::Completed
raw := status.raw_value()
parsed := OrderStatus.from_raw("PENDING")
```

Typed JSON codecs encode a raw-value enum as its raw String or Integer and
decode only declared raw values. Unknown input produces `JsonError`; the
target-language object representation never determines the wire value.

#### 3.13.2 Payload enums as sum types

- A payload enum has at least one payload-bearing variant such as
  `Value(value: String)`. It may also contain payloadless variants. Payload
  variants use one or more required, positional, typed fields; default,
  keyword, rest, and untyped fields are rejected.
- A payload enum cannot also be a raw-value enum. The payload is variant data,
  not the external representation of an enumerated constant.
- A payload value is constructed with `EnumName::Member(value, ...)`.
  Payloadless members are values and are not callable.
- A separate `sum` declaration is not introduced. Payload-bearing enum
  variants provide TypeRB's closed sum-type model.

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

#### 3.13.3 Shared enum behavior

- Members are explicitly qualified with `EnumName::Member`. They infer the
  enum's nominal type and cannot be mixed with members of another enum even
  when the member names match.
- Enums may declare instance methods after all members. `self` has the enum's
  nominal type, and the same method surface applies to ordinary, raw-value,
  and payload enums. Enum class methods are not yet user-definable.
- Portable `case` dispatches on variants. A payload pattern such as
  `when Token::Text(value)` introduces immutable bindings whose types come from
  the variant declaration. Current patterns bind every payload field
  positionally; partial patterns, guards, nested patterns, and wildcard syntax
  are reserved. Duplicate branches and duplicate bindings are errors.
- Without `else`, a case must list every member. With `else`, omitted members
  are handled by that branch. The selector is evaluated exactly once in every
  backend and the REPL.

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
  `Never` is not a source-level type name.
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
- A class value is assignable to an interface only when the class explicitly
  names that interface with `implements`, or inherits that declaration from a
  superclass. Matching members alone do not create structural conformance.
  The rule applies to parameters, returns, bindings, and imported classes.
- A fresh collection literal is contextually typed when an interface element
  type is expected, so `Array<Named> := [User.new()]` is valid when `User`
  implements `Named`. Existing mutable collection values remain invariant;
  an `Array<User>` binding is not assignable to `Array<Named>`.

The following class semantics remain deliberately unsettled rather than being
inferred from Go, Ruby, or TypeScript:

- portable `super(...)`, superclass constructor chaining, field initialization
  order, and the Go representation of an initialized superclass;
- method mutation effects and whether calling a mutating method requires a
  mutable receiver binding;
- variance and generic class methods;
- override compatibility and whether an explicit `override` marker is useful;
- whether a field and method may share one source-level member name. Until a
  common backend-safe rule is chosen, portable code should use a private
  backing field such as `@_name` for a public `name()` accessor;
- whether abstract, final, or protected class members add enough value to
  justify new syntax. They are not part of the current language.

## 4. User-defined generics

- Generic declarations include payload enums, transparent type aliases,
  records, classes, top-level functions, and instance methods: `enum Result<T,
  E>`, `type DbResult<T> = Result<T, DbError>`, `record Pair<T, U>`, `class
  Response<T>`, `def identity<T>(value: T): T`, and `response.json<T>()`.
- `type Alias<T> = Target<T, ...>` creates a transparent alias rather than a
  nominal type. Assignment and member checking use the expanded target, while
  diagnostics, completion, imports, and generated signatures retain the alias.
  An alias of an enum also qualifies its variants, such as
  `DbResult<Integer>::Ok(1)` and `when DbResult::Err(error)`.
- Calls, construction, and generic method selection use explicit type
  arguments in this phase. Examples are
  `Result<Integer, String>::Ok(1)` and `identity<String>("value")`. The checker
  also accepts `Box<Integer>.new(1)`, `Pair<Integer, String>.new(...)`, and
  `response.json<Todo>()`. It substitutes arguments through parameters,
  returns, fields, enum payloads, case bindings, and cross-file signatures
  before producing typed IR.
- A compiler-owned package function may define an argument-inferred type
  contract where the package API would otherwise expose target-only machinery.
  For example, `use_state(0)` from the explicit React platform package returns
  `ReactState<Integer>`. This does not add general type-argument inference to
  user-defined or native-package functions.
- Generic enum patterns omit repeated type arguments because the selector
  supplies them: a `case` over `Result<Integer, String>` uses
  `when Result::Ok(value)` and binds `value` as `Integer`.
- User-defined generic arguments are invariant. Missing, extra, and mismatched
  arguments are compile-time errors in every mode. A method type parameter may
  not duplicate one owned by its class.
- Payloadless variants of generic enums are reserved until typed singleton
  construction has a portable representation. Generic class methods, generic
  interfaces, constraints, variance declarations, and type-argument inference
  are staged work rather than implicit target-language behavior. A portable
  generic instance method remains callable even when a target lacks native
  generic methods; that backend lowers the checked operation through an
  equivalent generated helper.

```trb
record Pair<T, U>
	left: T
	right: U
end

class Box<T>
	@value: T

	def initialize(value: T)
		@value = value
		return
	end

	def pair<U>(other: U): Pair<T, U>
		return Pair<T, U>.new(left: @value, right: other)
	end
end

pair := Box<Integer>.new(1).pair<String>("one")
```

### 4.1 Standard Result

- `trb/std/result` exports the portable `Result<T, E>` payload enum with
  `Ok(value: T)` and `Err(error: E)` variants. It is imported explicitly with
  `import { Result } from trb/std/result`.
- Construction and handling use the ordinary generic enum rules. The baseline
  keeps both control-flow paths visible with explicit constructors and
  exhaustive pattern matching:

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
  compiler-owned runtime dependencies and project compilation emits and
  imports `Result` and structured error declarations automatically. Members of
  an inferred error value remain typed without an error-package import. Source
  references to a declaration itself—including type annotations, constructors,
  and case patterns—still require an explicit import such as
  `import { Result } from trb/std/result` or
  `import { IndexLookupError } from trb/std/errors`.
- Postfix propagation with `?` or `!`, prefix `try`, implicit exceptions, and
  `unwrap` are not part of the current language. Callable names may still end
  in `?` or `!`; those suffixes are ordinary naming conventions. Fallible
  operations use the effect rules below.

### 4.2 Fallible effects

- A function declares one fallible effect after its success type:
  `def find_user(id: Integer): User fails DbError`. A function without a return
  value may write `def save() fails DbError`.
- A call with an error effect propagates automatically through an enclosing
  function that declares a compatible `fails` type. The compiler reports an
  error when a named function neither declares nor captures the effect.
- Function values retain their declared failure type. Invoking an effectful
  function value follows the same propagation and capture rules as invoking a
  named function.
- `attempt expression` captures an effect as `Result<T, E>`. `attempt do ... end`
  captures every compatible effect in the block and uses the block's final
  expression as `T`. A block without a final value produces `Result<Unit, E>`.
- `main()` cannot declare `fails`; it must capture fallible work explicitly.
  The REPL top level is the deliberate exception: it executes a fallible call,
  prints its success or error value, and keeps the session alive. Functions
  defined in the REPL follow the ordinary declaration rule.
- Typed IR represents propagation and capture explicitly. Backends may use a
  native result representation internally, but TypeRB source has identical
  control flow in Go, Ruby, and TypeScript modes.
