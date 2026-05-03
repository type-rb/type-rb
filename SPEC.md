# TypeRB Specification Draft v0.2

Last updated: 2026-05-03

## 1. Language Goals

TypeRB is a class-based, statically typed language implemented in Go and transpiled to Ruby, TypeScript, and Go.

Design principles:

- Keep Ruby ergonomics while removing implicit behavior.
- Prefer explicit, simple, Go-like rules.
- Keep syntax and semantics consistent across transpile targets.

## 2. Target Modes

A source file declares output mode explicitly:

- `mode: ruby`
- `mode: typescript`
- `mode: go`

Mode selection controls transpilation output and package-manager/toolchain integration.

## 3. Core Syntax and Semantics (Confirmed)

### 3.1 Declarations and Style

- Class-based language.
- Method declaration keyword: `def`.
- Block terminator: `end`.
- Parentheses are never omitted in calls.
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

- Imports are explicit (exact grammar to be finalized).
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
mode: typescript

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

## 6. Next Phase: Implementation Tooling and Algorithms

Next discussion should define:

1. Parser strategy (handwritten recursive descent vs parser generator).
2. AST design for multi-target transpilation.
3. Type-checking phases and symbol-table structure.
4. Incremental rollout plan (MVP grammar vs staged generics/inference).
5. Code generation architecture for Ruby/TypeScript/Go backends.
6. Formatter (`trb fmt`) strategy and formatting stability guarantees.
