# Language guide

TypeRB uses one grammar and one set of portable semantics in every mode. The
mode selects code generation and package tooling, while explicit imports expose
target-specific APIs.

This guide summarizes the implemented language. The
[specification](specification.md) is the detailed source of truth.

## Program structure

A runnable project contains exactly one top-level `main()` function:

```trb
def main()
	puts("Hello from TypeRB")
	return
end
```

Functions and methods use `def` and `end`. Calls include parentheses. A
function that returns no value omits the return annotation and may either fall
through or use a bare `return`:

```trb
def print_name(name: String)
	puts(name)
	return
end
```

A function with an explicit return type must use a value-bearing `return` on
every path. Final expressions are not implicit returns. Complete `if` flow and
exhaustive enum or union `case` flow can satisfy this rule without a trailing
return after the construct:

```trb
def label(ready: Boolean): String
	if ready
		return "ready"
	else
		return "waiting"
	end
end
```

Writing `: Void` is an error. `Void` is an internal compiler type, not source
syntax.

Typed function values use `fn` and capture their lexical environment:

```trb
def apply(value: Integer, callable: (Integer) -> String): String
	return callable(value)
end

prefix := "item "
label := fn(value: Integer): String
	return prefix + value.to_s()
end
```

Each `fn` parameter has a type. A function value with no result omits its
return annotation, just like `def`. Its `return` exits the function value, not
the enclosing method.

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
import { Contract } from acme/contracts
import trb/std/strings
```

Portable libraries use `trb/std/*`. Target-specific APIs use
`trb/platform/<mode>/*` and are rejected when imported from another mode:

```trb
import trb/platform/go/http
```

Project package identities come from paths below `sourceDir`; source files do
not declare target packages. External TypeRB packages also use explicit
imports. A project may configure a short import such as `acme/contracts`; its
lock maps that name to the canonical package identity without changing source
syntax by target mode.

Ordinary imports must be used. Package imports require a member reference, and
each symbol named inside `{ ... }` must be referenced. Compiler integration
imports count as semantic uses when they activate their documented syntax or
type provider.

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

Local bindings declared inside methods must be used. Iterator and enum-pattern
bindings follow the same rule. The exact name `_` discards a value and cannot
be read. A descriptive name beginning with `_` remains readable but does not
produce an unused-binding error:

```trb
values.each do |_value|
	puts("tick")
end

values.each do |_value|
	puts(_value) # _value remains readable
end

case result
when Result::Ok(value)
	puts(value)
when Result::Err(_error)
	puts("failed")
end
```

Use `_` for a value that is intentionally inaccessible and `_name` when the
role should remain visible or the binding may be referenced later.
This does not make a local binding private. Leading `_` denotes privacy only on
class members; ordinary lexical scope still distinguishes a local `_value`
from a private `_value()` method.

Method parameters, fields, constants, and top-level bindings are not rejected
solely for being unused.

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

Numeric expressions may mix `Integer` and `Float`. The `Integer` operand is
widened to `Float`, and typed IR retains that conversion for every backend and
the REPL. The same safe widening is available in typed initialization,
assignment, arguments, and returns; narrowing to `Integer` remains explicit.
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
Classes explicitly marked with `implements` can be passed and returned through
that interface type. Subclasses inherit the declared conformance, while a class
with merely matching methods does not conform implicitly. Fresh literals such
as `values: Array<Named> := [User.new(...)]` use the expected interface element
type; existing mutable arrays remain invariant.

## Records

`record` declares a closed product type for data shared across targets:

```trb
record Message
	id: Integer
	text: String
	delivered: Boolean
end

message := Message.new(id: 1, text: "Hello", delivered: false)
```

Construction is keyword-only and checks the complete field set. Records cannot
inherit. Go emits a value struct, Ruby emits `Data`, and TypeScript emits an
interface.

## Enums, raw values, and sum types

TypeRB uses `enum` for two related but distinct models. An ordinary enum is a
closed set of named values:

```trb
enum TrafficLight
	Red
	Yellow
	Green
end
```

Every member may instead bind an explicit String or Integer representation for
storage and JSON boundaries. This is a raw-value enum: it extends the ordinary
enumeration model rather than introducing a different runtime type.

```trb
enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"

	def terminal?(): Boolean
		return self == OrderStatus::Completed
	end
end

status := OrderStatus::Completed
puts(status.terminal?())
puts(status.raw_value())
parsed := OrderStatus.from_raw("PENDING")
```

The enum remains a nominal `OrderStatus`; conversion is never implicit.
`from_raw()` returns `Result<OrderStatus, EnumValueError>`. Every member of a
raw-value enum must declare a distinct raw value of the same type.

A payload enum is TypeRB's closed sum-type model. Its alternatives may carry
different typed data, and it may mix payload-bearing and payloadless variants:

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
immutable bindings with types from the variant declaration. A payload enum
cannot also declare raw values. `Result<T, E>` is the standard generic payload
enum, with `Ok(value: T)` and `Err(error: E)` variants.

Ordinary, raw-value, and payload enums all remain nominal, use qualified member
names, support exhaustive `case`, and may define instance methods after their
members. TypeRB does not add a separate `sum` declaration.

The initial user-defined generics surface supports payload enums and top-level
functions with explicit type arguments:

```trb
def identity<T>(value: T): T
	return value
end

text := identity<String>("value")
```

## Control-flow expressions

Complete `if` and exhaustive `case` constructs can produce values. An `if`
expression always requires `else`; a `case` expression requires either an
`else` branch or exhaustive enum/union coverage:

```trb
label := if enabled
	"enabled"
else
	"disabled"
end

description := case token
when Token::Text(value)
	"text: " + value
when Token::Integer(value)
	value.to_s()
when Token::EOF
	"eof"
end
```

Each branch ends with its result expression. Earlier statements in that branch
run before the result is evaluated. Branch types must be equivalent or have a
safe common type such as `Float` for `Integer` and `Float`; TypeRB does not
silently fall back to `Any` or create a new union for incompatible branches.
A branch may instead leave its enclosing function or loop with `return`,
`break`, or `next`. Such a branch does not participate in the common result
type:

```trb
value := case result
when Result::Ok(found)
	found
when Result::Err(error)
	return fallback(error)
end
```

The ordinary placement rules still apply: `return` requires a function or
method, while `break` and `next` require a loop. The internal `Never` type used
to model these paths is not source syntax. A `return` inside the single-result
block of `map`, `select`, or `reduce` remains unsupported; use explicit `each`
when a transformation needs enclosing control flow. The statement forms remain
available when no value is needed.

## Arrays, hashes, and iteration

Arrays and hashes are homogeneous collections. Hash keys are non-nullable
`String` or `Integer` values in the current alpha:

```trb
numbers := [1, 2.5]                 # Array<Float>
values := [1, "two"]                # Array<Integer | String>
fields := {count: 1, name: "Ada"}  # Hash<String, Integer | String>
```

Literal inference retains one equivalent type, uses a safe common type such as
Float for mixed Integer and Float values, and otherwise constructs a union.
Narrow a scalar union with an exhaustive type case before using
alternative-specific operations:

```trb
case fields[:count]
when Integer(number)
	puts(number + 1)
when String(text)
	puts(text)
end
```

Ordinary homogeneous collection operations remain unchanged:

```trb
mut scores: Hash<String, Integer> := {ada: 1}
scores["grace"] = 2
puts(scores["ada"])

snapshot := scores.merge({linus: 3})
scores.update({ada: 10})
removed := scores.delete("grace")

scores.each do |name, score|
	puts(name + ": " + score.to_s())
end
```

Index and hash lookup are strict and fail at runtime when the value is absent.
Hash deletion is strict as well. Safe `Result`-returning lookup is available in
the standard library with `IndexLookupError` and `KeyLookupError` values from
`trb/std/errors`. `merge` is non-destructive; `update` and `delete` require a
`mut` receiver.
Destructive Array operations require a `mut` binding, while `reverse` returns
a new shallow Array:

```trb
mut values := [2, 3]
first := values.shift()
values.unshift(1)
reversed := values.reverse()
known := values.include?(2)
occurrences := values.count(3)
```

Membership and occurrence counting use portable `==` and are therefore
available for numeric, Boolean, String, and payloadless enum elements. They do
not implicitly enable target-native structural equality for nested values.

Arrays, integer ranges, and hashes support structured iteration:

```trb
[1, 2, 3].each do |value|
	puts(value)
end

(0...10).each { |index| puts(index) }

values.each_slice(2).with_index do |slice, index|
	puts(index)
end

scores.each do |name, score|
	puts(name)
	puts(score)
end
```

`break` exits the innermost loop and `next` skips to its next iteration.
`return` exits the enclosing function, including from an iterator block.
Hash iteration always binds key and value separately. Its enumeration order is
unspecified, and the entries are captured in a shallow snapshot before the
first iteration. Indexed and transforming Hash iteration is not part of the
current alpha.

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

These transformations currently operate on Arrays. The current alpha accepts
one result expression in transformation blocks.
Structured multi-statement blocks and first-class lambdas are planned.

## Result and fallible effects

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

`Result` is an ordinary value: it can be stored, returned, and handled
explicitly with exhaustive `case`. TypeRB also models operations that may fail
before they have been captured as a value. Such a function declares a `fails`
effect:

```trb
def recent_posts(): Array<Post> fails DbError
	return Post.order(created_at: :desc).limit(20).all()
end
```

A compatible enclosing `fails` function propagates the effect automatically.
A named function that neither declares nor captures it is rejected. `attempt`
captures the effect as an ordinary `Result<T, E>`:

```trb
posts_result := attempt recent_posts()

count_result := attempt do
	posts := recent_posts()
	posts.size()
end
```

`main()` cannot declare `fails`; it must use `attempt`. At the REPL top level,
a fallible expression prints its success or structured error and keeps the
session alive. There is no postfix `Result` propagation operator in the current
alpha; explicit `case`, `fails`, and `attempt` are the implemented choices.

## Formatting

The canonical indentation is one tab per nesting level. `trb fmt` preserves
comments, literal spelling, heredoc contents, and supported platform DSL
syntax. Formatting is deterministic and idempotent.
