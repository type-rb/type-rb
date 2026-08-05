# Language guide

TypeRB uses one grammar and one set of portable semantics in every mode. The
mode selects code generation and package tooling, while explicit imports expose
target-specific APIs.

This guide summarizes the implemented language. The
[specification](../SPEC.md) is the detailed source of truth.

## Program structure

A runnable project contains exactly one top-level `main()` function:

```trb
def main()
	puts("Hello from TypeRB")
	return
end
```

Functions and methods use `def` and `end`. Calls include parentheses. A
function that returns no value omits the return annotation, but still ends its
executed path with `return`:

```trb
def print_name(name: String)
	puts(name)
	return
end
```

Writing `: Void` is an error. `Void` is an internal compiler type, not source
syntax.

Outside delimiters, `;` can separate complete statements:

```trb
class Empty; end
```

`trb fmt` expands separators into canonical lines.

## Imports and modes

Imports are explicit and resolved before type checking:

```trb
import app/models/user
import { UserRepo } from app/repos/user_repo
import trb/std/strings
```

Portable libraries use `trb/std/*`. Target-specific APIs use
`trb/platform/<mode>/*` and are rejected when imported from another mode:

```trb
import trb/platform/go/http
```

Project package identities come from paths below `sourceDir`; source files do
not declare target packages.

## Types and bindings

Type annotations use `name: Type`. `:=` declares a local binding and infers its
type when no annotation is present:

```trb
name := "Ada"
count: Integer := 3
nickname: String? := nil
names: Array<String> := ["Ada", "Grace"]
scores: Hash<String, Integer> := {ada: 10}
```

Bindings declared with `:=` are immutable. Add `mut` at declaration time when
the binding will be reassigned or used by a destructive collection operation:

```trb
mut count := 0
count = count + 1

mut names := ["Ada"]
names.push("Grace")
```

An immutable reference cannot become mutable by assigning it to a new `mut`
binding.

Identifiers beginning with an uppercase letter are immutable constants. They
are allowed at top level or directly inside a module or class:

```trb
API_NAME := "TypeRB"
DEFAULT_LIMIT := 100
```

Constant initializers may be runtime expressions, but constants cannot be
rebound or passed to destructive APIs.

## Conditions and operators

Conditions must have the non-nullable `Boolean` type. TypeRB does not inherit
truthiness rules from a target:

```trb
if count > 0
	puts("non-empty")
else
	puts("empty")
end
```

`Integer` and `Float` operands are not mixed implicitly. Arithmetic,
comparison, equality, and Boolean operators are checked before lowering.
Integer division truncates toward zero in every target.

## Classes, interfaces, and modules

```trb
interface Named
	name(): String
end

class User implements Named
	readonly @id: Integer
	@_name: String

	def initialize(id: Integer, name: String)
		@id = id
		@_name = name
		return
	end

	def name(): String
		return @_name
	end
end
```

Instance fields are declared at class scope. Names beginning with `_` and
`@_` are private. `readonly` fields can be assigned during initialization but
not externally. Class methods use `def self.name()`.

Classes support inheritance, interfaces, modules, class constants, and checked
instance/class member access. Superclass construction, override rules, generic
classes, and a final field/method collision rule remain alpha design work.

## Records

`record` declares a closed product type for data shared across targets:

```trb
record TodoItem
	id: Integer
	title: String
	completed: Boolean
end

item := TodoItem.new(id: 1, title: "Try TypeRB", completed: false)
```

Construction is keyword-only and checks the complete field set. Records cannot
inherit. Go emits a value struct, Ruby emits `Data`, and TypeScript emits an
interface.

## Enums and exhaustive case

Enums are closed nominal types and may carry typed payloads:

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

A `case` without `else` must handle every member. Payload patterns introduce
immutable bindings with types from the enum declaration.

The initial user-defined generics surface supports payload enums and top-level
functions with explicit type arguments:

```trb
def identity<T>(value: T): T
	return value
end

text := identity<String>("value")
```

## Arrays, hashes, and iteration

Arrays and hashes are homogeneous collections. Hash keys are non-nullable
`String` or `Integer` values in the current alpha:

```trb
mut scores: Hash<String, Integer> := {ada: 1}
scores["grace"] = 2
puts(scores["ada"])
```

Index and hash lookup are strict and fail at runtime when the value is absent.
Safe `Result`-returning operations are available in the standard library.

Arrays and integer ranges support structured iteration:

```trb
[1, 2, 3].each do |value|
	puts(value)
end

(0...10).each { |index| puts(index) }

values.each_slice(2).with_index do |slice, index|
	puts(index)
end
```

`break` exits the innermost loop and `next` skips to its next iteration.
`return` exits the enclosing function, including from an iterator block.

Value-producing collection blocks are part of the typed IR:

```trb
labels := [1, 2, 3].map do |value|
	"item-" + value.to_s()
end

visible := labels.select.with_index do |label, index|
	!label.empty?() and index < 2
end

total := [1, 2, 3].reduce(0) do |sum, value|
	sum + value
end
```

The current alpha accepts one result expression in transformation blocks.
Structured multi-statement blocks and first-class lambdas are planned.

## Result

`trb/std/result` provides `Result<T, E>` with `Ok` and `Err` variants:

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

There is no propagation operator in the current alpha. Explicit exhaustive
handling is the baseline while concise syntax is evaluated through application
usage.

## Formatting

The canonical indentation is one tab per nesting level. `trb fmt` preserves
comments, literal spelling, heredoc contents, and supported platform DSL
syntax. Formatting is deterministic and idempotent.
