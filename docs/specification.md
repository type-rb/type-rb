# TypeRB Specification Draft v0.3

Last updated: 2026-08-24

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
or `typescript`. Source files do not contain mode declarations. A standalone
CLI or editor session without a discoverable project configuration may select
the same mode as session metadata; Go is the default. This selection does not
create a fourth configuration source or change the source language.

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

- TypeRB source files use UTF-8. An invalid byte sequence is a syntax error
  before tokenization, formatting, or semantic analysis.
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
- A function value that may return a recoverable error declares an ordinary
  `Result` return, such as `fn(): Result<User, LoadError> ... end`. Its type is
  written `() -> Result<User, LoadError>`. Prefix `try`, postfix `catch`, and
  exhaustive `case` use the same Result rules as named functions.
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
  implementation detail rather than TypeRB syntax. A Result-returning function
  value has an explicit portable boundary and may suspend with a non-Void
  success value. A pure suspending function value with another non-Void result
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
- A component may be a checked member of another imported component, such as
  `Table.Row` or `Sidebar.Item`. Package-owned declaration adapters declare
  these compound component members and their props; generated TSX retains the
  member expression unchanged.
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
- `trb fmt` prints returned JSX directly after `return` without adding
  parentheses. It recursively indents structure-only element and embedded
  expression children, while preserving text-bearing elements inline so
  formatting does not change provider-visible text whitespace.

### 3.2 Typing

- Parameter/type annotation form is `name: Type`.
- Local variable initialization uses `:=`.
- Type inference is enabled for `:=`.
- Explicit type with initialization is allowed: `x: T := expr`.
- Type names are case-sensitive. Portable built-in type names use exactly one
  source spelling: `Any`, `Boolean`, `Integer`, `Float`, `String`, `Bytes`,
  `StringBuilder`, `Array`, `Hash`, `Range`, and `Iterable`. Target-language
  spellings and shorthand aliases such as `bool`, `Int`, `int`, `number`, and
  `Map` are not TypeRB type names. Every other type reference must name a type
  parameter, a declaration in the project, or an explicitly imported type.
- Integer literals use the portable exact range
  `-9007199254740991..9007199254740991`. A literal outside that range is a
  compile error in every mode rather than a target-dependent rounded value.
- Every runtime `Integer` uses that same range. Checked arithmetic and
  compiler-owned ingress adapters reject an out-of-range result or value with
  the runtime failure `Integer is outside the portable range`. This includes
  String and JSON conversion and ORM reads. Handwritten native code is
  responsible for honoring the same contract when it constructs a typed
  `Integer`. A generated native adapter
  that exposes Integer values must apply the same boundary check; the current
  String-envelope runtime adapter does not expose Integer directly.
- `Float` is IEEE 754 binary64. A source Float literal must be finite and fit
  binary64; overflow is a compile error, while underflow rounds to signed zero.
  Runtime arithmetic may produce positive or negative infinity and NaN. These
  values are ordinary `Float` values, not `Integer` values, and have no source
  literal spelling in the current grammar.

#### Aliases and nominal newtypes

- `alias Name = Target` declares a transparent type shorthand. A generic alias
  uses `alias Name<T> = Target<T>`. Assignment and member checking expand the
  alias while diagnostics, completion, imports, and generated signatures may
  retain its authored name.
- The former `type Name = Target` spelling is not accepted as current syntax.
  It produces a migration diagnostic because an existing declaration must be
  classified as either a transparent `alias` or a nominal `newtype`.
- `newtype Name = Representation` declares a nominal type. Its representation
  may be any concrete, fully instantiated type, including
  `Array<ProductId>`, but the representation must not itself be nullable.
  Generic newtype declarations such as `newtype Id<T> = T` are not supported.
- Construction is explicit with `Name.new(value)`, and `value()` returns the
  representation. After ordinary argument type checking, `new()` is
  infallible and performs no user-defined domain validation. Newtype
  declarations cannot currently define custom members or replace these
  generated methods; fallible invariant checks use ordinary Result-returning
  functions. No other representation members are forwarded. Ordinary
  parameters, returns, assignments, collection elements, and record fields do
  not implicitly convert between a newtype, its representation, or a different
  newtype with the same representation.
- Optionality is applied outside the nominal type as `Name?`. Rejecting a
  nullable representation keeps an erased backend representation from
  conflating a wrapped `nil` with absence of the newtype value.
- `==` and `!=` accept two values of the same newtype only when the recursively
  expanded representation supports portable equality. Ordering, arithmetic,
  indexing, iteration, and other representation operations require explicit
  `value()` unless a future newtype API specifies them.
- A package declaration may mark a typed serialization or persistence
  parameter as a representation boundary. At that boundary only, a newtype is
  checked through its recursively expanded representation. JSON codecs,
  `trb/web` binding, Jobs payload generation, and ORM column-value parameters
  use representation metadata rather than checker logic tied to those package
  names.
- Typed IR retains construction, unwrapping, nominal identity, and
  representation metadata. Go, Ruby, and TypeScript backends may erase the
  physical wrapper; target-native code is not a source-level nominality
  guarantee.

#### Fresh empty mutable collections

- An unannotated `mut values := []` or `mut values := {}` starts with a
  pending collection type. The first later statement in the binding's lexical
  statement sequence that constrains the collection is its inference region.
  A statement that only observes the collection, such as `values.empty?()`,
  does not end the pending state.
- All statically checked writes in that one statement contribute, including
  writes nested in its `if` or `case` branches, iteration blocks, call blocks,
  and literal `fn` bodies. Array elements and Hash values use the same common
  type and union normalization as non-empty literals. A fully typed assignment,
  argument, or return context instead supplies the exact collection type.
- The type is fixed after that statement. A later statement must conform and
  never widens the collection. Branches join into one collection type; they do
  not retain separate flow-sensitive Array or Hash types.
- A syntactically present callback body participates even when the callback may
  execute later. The implementation body of a separately named function or
  method never constrains a caller's pending collection; only its declared
  parameter or return signature can do so.
- A pending collection that reaches an untyped boundary, is aliased without a
  concrete collection context, or remains unresolved at the end of its lexical
  scope is an error and requires an explicit annotation. Interactive top-level
  REPL bindings may remain pending across submissions and are refined when a
  later submission supplies their first constraint.
- Hash keys do not use value-union inference. The first write fixes one
  homogeneous, non-nullable `String` or `Integer` key type for the entire Hash;
  another key type in the same inference region is an error. Hash values still
  use common-type and union inference.

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
- A direct nullable data field may also narrow when its receiver is a stable
  lexical binding and the field cannot change: record fields are always
  stable, and class fields must be declared `readonly`. Reassigning the
  receiver invalidates every field fact derived from it. Imported record and
  class fields retain the same stability metadata.
- Mutable class fields, indexes, calls, and chained member paths are not
  assumed to produce the same value when evaluated again. Typed IR records
  each nullable unwrap explicitly so Go, Ruby, TypeScript, and the REPL use the
  same checked flow facts.

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
- One literal branch may list multiple comma-separated values, such as
  `when "index", "show"`. The values share one body. Enum and union patterns,
  including payload bindings, use separate `when` branches.
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

alias CreateResponse = CreatedResponse | InvalidResponse

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

#### Indexes, ranges, and subsequences

- Array and String single-element access uses `value[index]`. Nonnegative
  indexes are zero-based from the start, while negative indexes count from the
  end (`-1` is the last element). An index that remains outside the collection
  after this normalization fails at runtime. String indexing counts Unicode
  code points rather than encoded bytes.
- The safe single-element form is `value.try_fetch(index)`, returning a
  structured `IndexLookupError` with the originally requested index. It uses
  the same negative-index normalization. Array and String do not provide a
  second strict `fetch` spelling.
- Subsequence access uses `value.slice(range)`. `value[range]`,
  `slice(start, length)`, and a one-argument tail slice are not part of the
  initial language. `try_slice(range)` is the safe counterpart and returns a
  structured `SliceRangeError`.
- Both `start..finish` and `start...finish` are accepted. Bounds must be
  nonnegative, ordered, and within the collection. The exclusive
  `size...size` range is a valid empty slice; an inclusive finish always names
  an existing element. Array slices return a new shallow Array.
- Range values retain their bounds and inclusivity through variables and
  function boundaries. Converting `Range<Integer>` to `Iterable<Integer>`
  enumerates the represented values without changing slice semantics.
  `range.to_a()` materializes those values as a new `Array<Integer>`; a
  reversed Range materializes an empty Array.
- Array `index(value)` returns the zero-based position of the first value that
  is equal under portable `==`, or `nil` when no value matches.
- String `index(substring)` and `rindex(substring)` search literal substrings
  and return a code-point position as `Integer?`. They return `nil` when the
  substring is absent. An empty substring is found at position zero for
  `index` and at the String size for `rindex`.

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
- The REPL appends ` [mut]` to the displayed value and type when a submission
  directly declares, assigns, or evaluates a mutable binding. The marker is
  REPL metadata about the binding rather than part of its value or type, and
  it does not propagate through a derived expression.

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
- A package provider may define a declaration-only reference position for a
  declarative API. The initial case is the first model argument of
  `trb/orm`'s `belongs_to`, `has_many`, and `has_one`: model declarations in
  the same source directory resolve there without an import. The name is not
  made visible to ordinary expressions or type annotations, and a subdirectory
  begins a different ORM model group.
- Every ordinary import must be used. A package import is used by referencing
  one of its members; every symbol in a named import list must be referenced.
  Imports that explicitly activate a compiler integration, such as a native
  syntax or type provider package, and imports that activate an external fixed
  declaration provider count as semantic uses.
- Project module identities come from paths below `sourceDir`; source files do
  not declare target packages.
- A project import may omit a terminal `/index`. The omitted form is the
  canonical authored spelling when it resolves uniquely to the directory's
  `index.trb`; an explicit `/index` remains accepted. If both `name.trb` and
  `name/index.trb` exist, `from name` resolves `name.trb`, so tooling retains
  `/index` when it is needed to select the directory entry. Project-aware
  formatting removes an explicit `/index` only when resolving the shortened
  path selects the same module. Unresolved paths and source-only formatting
  without a project snapshot retain an explicit `/index`.
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
  `Any`. A declarative adapter from an installed TypeRB package may replace
  indexed exports and records without changing the application import. An
  adapter may describe generic functions, classes with distinct instance and
  class members, non-constructible interfaces with readonly properties and
  instance methods, records, and transparent type aliases. Calls continue to
  use TypeRB's explicit type arguments, and
  generated TypeScript imports any transitive target types required by the
  selected contracts without exposing those helper names to TypeRB source. An
  adapter may mark a Result-returning function field or parameter with the
  `result_to_promise_rejection` bridge. The TypeScript backend then unwraps the
  callback's `Result`, resolves `Ok(value)`, and rejects the exact `Err(error)`
  payload only at that native boundary. A native function or instance member
  may instead use the call-level `promise_rejection_to_result` bridge and
  declare `Result<T, String>`. The call becomes a TypeScript suspension root;
  resolution produces `Ok`, and a synchronous throw or Promise rejection
  produces `Err` from `Error.message` or `String(value)`. `Promise<void>` maps
  to `Result<Unit, String>`. If String conversion throws, the error is
  `"Unknown native rejection"`. Other error projections are not yet accepted.
- A package-owned declaration adapter uses a versioned, mode-independent
  semantic catalog selected for one native ecosystem by
  `declarationAdapters.<mode>` in the package manifest. The common host
  strictly decodes the catalog and validates its protocol shape and checksum.
  The selected ecosystem adapter validates native-dependency ownership, name
  conflicts, and adapter-specific bridge kinds before import. Direct import
  from an installed native declaration system is currently available only for
  TypeScript. A declaration adapter cannot execute compiler code, access
  compiler AST or typed IR, or emit target source.
- A Ruby TypeRB package may select a fixed Declaration Protocol version 2
  catalog with `declarationProviders.ruby`. The package must declare at least
  one Ruby native dependency and provide a root `index.trb`; importing that
  root activates the catalog and retains the root module's ordinary runtime
  loading. The catalog provider identity must equal the package's canonical
  name. It may declare fixed external types, modules, properties, methods,
  generics, overloads, and literal-dependent signatures. It cannot execute
  compiler code, emit source, claim project source declarations, supply
  project rules or runtime types, select runtime operations or call
  specializers, declare compiler-controlled block behavior, or weaken nominal
  representation boundaries. Strict decoding rejects unknown fields, trailing
  data, empty catalogs, symlinked catalogs, unsafe `Any` or invalid signature
  types, and compiler-derived representation metadata. A declaration name that
  conflicts with another active provider or a project declaration is an error.
  Other modes and project-aware external providers are not supported by this
  capability.
- A package may pair `declarationAdapters.<mode>` with
  `runtimeAdapters.<mode>` in Go, Ruby, or TypeScript mode. Native Runtime
  Adapter Protocol version 1 maps a canonical `module#export` identity to a
  declared native dependency, target module, and top-level function symbol.
  Every export in a runtime-backed declaration module must have the exact
  non-generic `(String) -> String` signature; direct and runtime-backed exports
  cannot share a module. The runtime binding may declare that the call suspends
  and that the generated backend passes its hidden execution scope before the
  string argument. Runtime adapter data is strictly decoded and validated but
  never executed by package resolution or checking. Target shims and ordinary
  TypeRB package source own JSON envelopes, SDK error normalization, and
  conversion to domain `Result` values.
- A package may declare one explicit declaration-adapter conformance project
  per mode with `adapterTests.<mode>`. Its config path stays below the package
  root and its native check is a structured argument vector rather than a
  shell string. Only `trb adapter test` executes that command, after validating
  the adapter, verifying that the project installs the current package, and
  building the project. Package resolution, installation, compilation, and
  import never execute adapter tests, and the test command does not install
  dependencies implicitly.
- Official formatter command: `trb fmt`.
- Formatting must preserve program meaning. Before removing whitespace between
  ordinary TypeRB tokens, the formatter verifies that re-lexing does not fuse
  them into a different token or operator. It retains a separating space when
  the boundary would change.
- `NativeStatement`, `NativeExpression`, and `NativeBlock` are opaque
  formatting islands. Their internal bytes, including whitespace, newlines,
  and comments, are preserved. Only their shared leading indentation is moved
  to match the surrounding TypeRB block. Ordinary TypeRB outside an island is
  still formatted canonically.
- Canonical TypeRB indentation is one tab per nesting level. Formatter
  configuration is not part of the current language; a future configuration
  surface may select a different indentation style without changing language
  semantics.
- Interactive multiline REPL input displays each indentation level as two
  spaces while it is open and after the accepted input remains on screen.
  Enter reindents the open submission and inserts the indentation for the next
  line; accepting a complete submission converts the value passed to the
  compiler and stored in history to canonical tab indentation without changing
  the displayed width.
- REPL diagnostics identify the accumulated interactive source as `(trb)` and
  retain one-based line and column positions. Locations inside the active
  project use paths relative to the project root, while locations outside it
  retain absolute paths. Runtime evaluation failures use the innermost typed-IR
  source location when one is available; REPL operation errors without a source
  location retain the `trb: repl:` prefix.

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

- A runnable project defines exactly one top-level `def main()`. The runnable
  declaration takes no parameters or type parameters and has no return
  annotation. A class, enum, interface, or module member named `main` is an
  ordinary non-entrypoint method and follows the usual method rules.
- A standalone file-root program also starts from one top-level `def main()`
  in its selected entry file. The entry and the transitive closure of its
  explicit project imports form the program; unrelated sibling files are not
  discovered implicitly.
- Project and standalone entrypoints have the same signature and
  startup rules. Selecting a file never turns top-level statements into a
  second script execution model or makes a function named `main` ordinary.
- `main` is a language convention and is not configurable in
  `trbconfig.jsonc`.
- Projects intended only as libraries may omit `main`.

Build and execution behavior belongs to the [CLI reference](cli.md).

### 3.9 Boolean Conditions

- Conditions in `if`, `elsif`, `while`, a conditional expression, and a
  conditional control-transfer statement must have the non-nullable `Boolean`
  type.
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
- Integer `+`, `-`, `*`, `/`, `%`, `**`, unary `-`, and their supported
  compound assignments are checked against the portable Integer range. An
  out-of-range result is a runtime failure rather than wraparound, rounding, or
  a recoverable `Result`.
- Float arithmetic follows binary64 edge behavior in every backend and the
  REPL. Division by positive or negative zero produces the corresponding
  infinity, `0.0 / 0.0` produces NaN, overflow produces infinity, and a
  negative base raised to a non-integral Float exponent produces NaN rather
  than a target-specific complex value.
- Parenthesized TypeRB expressions retain their AST precedence in generated
  code. Ruby-specific matching, comparison, and bitwise operators (`=~`, `!~`,
  `<=>`, `~`, `|`, `&`, `^`, `<<`, and `>>`) require an explicit Ruby-native
  import until portable semantics are defined.

#### Conditional expression

- `condition ? then_expression : else_expression` is the conditional
  expression. It is the only ternary operator and has lower precedence than
  every binary operator.
- The condition is evaluated once. Exactly one branch is then evaluated, so
  side effects and failing operations in the unselected branch do not run.
- Its branch types use the same safe common-type rule as a value-producing
  `if`. The compiler does not choose `Any` or synthesize a union merely to
  combine incompatible alternatives.
- An unparenthesized conditional expression cannot contain another conditional
  expression. Parentheses make the intended grouping explicit; the official
  linter may still discourage nesting when a complete `if` is clearer.
- The canonical formatter writes one space on each side of `?` and `:`. A
  callable suffix remains part of its identifier, so
  `ready?() ? "ready" : "waiting"` is unambiguous.

### 3.11 Loop Control

- `break` exits the innermost enclosing `while`, `each`, `each_slice`, or
  `each.with_index` loop.
- `next` skips the remainder of the current iteration of that innermost loop.
- Both are statements with no value form. They are compile errors outside a
  loop and have identical semantics in every mode.
- `return` remains distinct: it exits the enclosing method, including when it
  appears inside an iteration block.
- A simple transfer may add a trailing `if` condition:
  `return value if condition`, `return if condition`, `break if condition`, or
  `next if condition`. The formal construct is a conditional control-transfer
  statement, shortened to conditional transfer. Its value, when present, is
  evaluated only when the condition is true.
- Trailing `if` is not a general statement modifier. Calls, assignments, and
  other statements such as `notify() if condition` are syntax errors.
  `unless` is not part of the portable grammar.
- Because `return if condition` is the bare conditional-return form, a complete
  value-producing `if` should be assigned and returned separately or replaced
  by a conditional expression.

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
An ordinary structured iteration retains the same `return`, `break`, and
`next` owners as a portable iteration. A Result-boundary structured block is a
cleanup boundary: authored `return` cannot cross it, and `break` or `next`
cannot cross it to an outer loop. A loop or function value nested inside the
block still owns its local transfers. A Result-boundary structured iteration
also rejects authored `return`, while `break` and `next` retain their local
iteration meaning. Prefix `try` may return an Err to the structured boundary;
the operation completes its rollback or cleanup before an outer `catch`
handler runs. Ordinary call blocks are not value-producing unless their
package declaration explicitly provides structured lowering.

Portable Array transformations `map`, `select`, and `reduce` use the same
typed-IR boundary. The short-circuit predicates `any?`, `all?`, and `none?`
and searches `find` and `find_index` require one non-nullable Boolean result
expression at the end of their block. Transformation blocks may contain
ordinary statements before that final expression; their locals are scoped to
one element evaluation. They evaluate from left to right and stop when the
result is known.
Empty Arrays produce `false`, `true`, and `true` for the predicates;
`find` and `find_index` return a nullable element and nullable `Integer`, with
`nil` for no match. `return`, `break`, and `next` are not accepted inside a
value-producing transformation block; use `each` when control must leave or
skip the enclosing iteration. Indexed predicate blocks are not currently
enabled.

Array sorting is stable and non-destructive. `sort()` and
`sort_descending()` use the element's portable natural order. `sort_by` and
`sort_by_descending` use their block's final expression as a key evaluated
exactly once per element; no statement in the block may use an operation that
may fail. Equal
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
`index(value)` uses the same equality contract and returns the first matching
position as `Integer?` without a block.

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
  assignment target, or return type, or from the first constraining statement
  of a fresh unannotated mutable binding as defined in section 3.2.
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

The complete public collection receiver API belongs to the
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
- A conditional expression is the compact two-value form of an `if`
  expression. It shares Boolean checking, branch narrowing, lazy evaluation,
  common-type selection, typed IR, and backend behavior with the complete
  form.
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
- Divergence is supported in ordinary expression positions. Value-producing
  collection transformations accept statements followed by one final result
  expression, but still reject an enclosing `return`; explicit `each` remains
  the portable alternative when control must leave the transformation.
  This rule does not make loops into expressions, add values to `break` or
  `next`, or introduce additional unreachable-code diagnostics.

```trb
label := if ready
	"ready"
else
	"waiting"
end

short_label := ready ? "ready" : "waiting"

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
- Interfaces may declare invariant type parameters, such as
  `interface Repository<T>`. Classes implement a concrete application such as
  `class MemoryRepository implements Repository<User>`, and every interface
  method signature is checked after substituting those arguments. A generic
  class may implement an interface using its own parameter. Generic interface
  methods, interface inheritance, constraints, and variance are not implied by
  this declaration model.
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
  records, classes, interfaces, top-level functions, and instance methods:
  `enum Result<T,
  E>`, `alias DbResult<T> = Result<T, DbError>`, `record Pair<T, U>`, `class
  Response<T>`, `interface Repository<T>`, `def identity<T>(value: T): T`, and
  `response.json<T>()`.
- `alias Alias<T> = Target<T, ...>` creates a transparent alias rather than a
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
  construction has a portable representation. Generic class and interface
  methods, constraints, variance declarations, and type-argument inference are
  staged work rather than implicit target-language behavior. Go output requires
  Go 1.27 and emits a generic class instance method as a native generic method.
  A target or receiver representation without native generic methods lowers
  the same checked operation through an equivalent representation.

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
- Postfix propagation with `?` or `!`, implicit exceptions, and unchecked
  `unwrap` are not part of the language. Callable names may still end in `?`
  or `!`; those suffixes are ordinary naming conventions. Recoverable failure
  remains an ordinary `Result` value.

### 4.2 Result control flow

- The exact lowercase words `try` and `catch` are reserved for Result control
  flow in authored declarations and bindings. Longer callable names such as
  `try_fetch` and `catch?` remain ordinary names.
- Prefix `try expression` requires the compiler-owned `Result<T, E>` or a
  transparent alias. It evaluates the operand once. `Ok(value)` makes the
  expression produce `value: T`; `Err(error)` returns a new Err from the
  nearest compatible Result-returning function or compiler-declared structured
  Result boundary.
- The operand error type must be assignable to the enclosing Result error type
  using the ordinary safe assignment conversions. TypeRB has no implicit
  error-mapping protocol. Use `catch` and construct the outer error explicitly
  when the types are incompatible.
- Prefix `try` binds to its immediate postfix expression chain before binary
  operators. It is not permitted at module or REPL top level, in a non-Result
  function, or inside a value-producing collection transformation. Ordinary
  `each` remains transparent to the enclosing function boundary.
- `expression catch |error| ... end` also evaluates its Result operand once.
  `Ok(value)` produces `value: T`. On `Err(error)`, the immutable binding has
  type `E` and the handler must either produce a value assignable to `T` or
  transfer control with `return`, `break`, or `next` to a valid lexical owner.
  `catch` handles only `Result::Err`; it does not intercept a target-language
  exception, Promise rejection, or panic.
- In the initial grammar, `catch` wraps a complete statement value or the
  complete result of a compiler-declared call block. A structured Result call,
  and its optional direct `try` or `catch` wrapper, must be the direct value of
  a variable declaration or return. Arbitrary argument, collection-element,
  and member-chain composition is reserved until application evidence requires
  it.
- A discarded standard Result is a compile error. This required-use rule
  covers bare Result expression statements and unread local Result bindings,
  including names beginning with `_`. `try`, `catch`, exhaustive `case`,
  `return`, passing, and storing the value are explicit handling or ownership
  transfer. The rule is intentionally shallow: it does not recursively inspect
  containers or perform whole-program liveness analysis. The REPL top level is
  exempt because it displays the Result value.
- An operation with no success value uses `Result<Unit, E>`. TypeRB does not
  add an implicit Unit success, a one-argument Result shorthand, or a
  zero-argument `Ok()` constructor.
- Typed IR represents Result branching and structured boundaries explicitly.
  Backends may choose a native representation internally, but source control
  flow is identical in Go, Ruby, and TypeScript modes.

### 4.3 Test declarations

- A project test file has the suffix `_test.trb`. Ordinary builds and runs do
  not emit or execute these files; configured-project `trb check`, `trb lsp`,
  and `trb test` validate them with the complete project. A config-free
  file-root closure excludes imported test modules, while a test file selected
  as the language-server entry is still analyzed.
- The portable `trb/std/test` package exports `describe`, `test`, and `expect`.
  Test DSL functions are available only through explicit named imports.
- A test file has one or more top-level `describe("literal") do ... end`
  declarations. A suite contains only nested `describe` and `test`
  declarations. A `test` must be nested inside a suite.
- Suite and test names are nonempty String literals. Their slash-separated
  nesting path is the stable full test name used by filters and tools.
  Duplicate full names in one file are an error.
- Suite and test blocks take no parameters. Test bodies otherwise use ordinary
  statements, helpers, Results, and imports. Expected errors are inspected with
  exhaustive `case`; the test package does not add an implicit Result boundary,
  change language scoping, or introduce implicit setup state.
- `return`, `break`, and `next` cannot transfer control across a `describe()` or
  `test()` block boundary. A nested function still owns its own `return`, and
  a `while` or iteration inside a test body still owns its local `break` and
  `next` statements.
- `expect<T>(actual)` preserves `T` in `Expectation<T>`. An assertion failure
  aborts the current case, records the assertion's `.trb` location, and does
  not abort subsequent cases.
- `trb test` creates a temporary entrypoint, invokes each test module in
  deterministic module order, and returns a nonzero status when a case fails.
  It suppresses application `main()` entrypoints for this compilation only.
