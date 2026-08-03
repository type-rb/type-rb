# TypeRB v0.1

TypeRB is a class-based typed language implemented in Go. A `.trb` file is
formatted by `trb fmt` and transpiled by `trb build` to Ruby, TypeScript, or Go.
The target is selected once per project in `trbconfig.jsonc`.

v0.1 includes a real compiler pipeline:

```text
source -> lossless lexer -> syntax AST -> resolver/type checker
       -> typed IR -> Ruby / TypeScript / Go backend
```

The Ruby backend is designed for normal Rails applications. Portable TypeRB
syntax is type checked. Open-ended Rails DSL such as associations, validations,
routes, migrations, callbacks, scopes, jobs, mailers, and concerns is retained
as explicit Ruby-native AST/IR nodes and emitted without losing behavior.

## Install

Once the first tagged release is published, Homebrew releases are distributed
from the TypeRB tap:

```sh
brew install type-rb/tap/trb
```

The generated Formula also carries a `head` definition, so after the tap has
been initialized it can install the development build with
`brew install --HEAD type-rb/tap/trb`.

```sh
go install github.com/type-rb/type-rb/cmd/trb@latest
```

For a repository checkout, use the bootstrap launcher. It rebuilds the Go
bootstrap compiler internally when compiler sources change, then delegates to
the normal `trb` CLI:

```sh
./trb version
```

The compiler targets the current Go toolchain (Go 1.26). The minimum version is
advanced as new stable Go releases become the development baseline.

Maintainers publish a release by pushing a `v*` tag. The release workflow runs
the test suite and self-host bootstrap check, creates a deterministic source
archive, and updates `type-rb/homebrew-tap` when the repository secret
`HOMEBREW_TAP_TOKEN` is configured.

## Commands

```sh
# Create trbconfig.jsonc and the target manifest.
trb init --mode ruby .
trb init --mode go --module example.com/acme/app .
trb init --mode typescript .

# Format all .trb files below the current directory in place.
trb fmt

# CI check. Prints files that need formatting and exits non-zero.
trb fmt --check

# Format stdin to stdout.
trb fmt -

# Compile a project to build/. Non-.trb files are copied as well.
trb build

# Compile without writing output.
trb build --check

# Choose a project output directory.
trb build --out-dir dist .

# Compile one file to stdout.
trb build --stdout app/models/post.trb

# Compile and immediately run through Bundler, Go, or Node.
trb run test.trb
trb run test.trb -- first-argument

# Start the project-aware typed IR REPL. The mode, imports, local packages, and
# type providers come from trbconfig.jsonc.
trb repl
trb repl --config path/to/trbconfig.jsonc

# Generate Gemfile, go.mod, or package.json from trbconfig.jsonc.
trb sync

# Update dependencies in trbconfig.jsonc and regenerate the manifest.
trb add rails "~> 8.0"
trb add --dev rspec-rails "~> 7.0"
trb remove rspec-rails

# Run bundle install, go mod download, or npm install.
trb install
```

`trb repl` requires a project configuration. Each submission is parsed,
resolved, type checked, and lowered through the normal compiler pipeline before
evaluation, so platform packages are accepted or rejected according to the
configured mode. The initial evaluator supports portable expressions, state,
conditionals, loops, functions, classes, records, project imports, and portable
standard-library intrinsics. A mode-specific intrinsic without a REPL runtime
adapter produces an explicit runtime diagnostic.

REPL commands are `:type EXPRESSION`, `:load FILE`, `:reload`, `:help`, and
`:quit`. Multiline declarations and delimiter-balanced calls use a continuation
prompt automatically. Interactive terminals provide colored input/results, Tab
completion, cursor editing, Up/Down history, and Ctrl-R reverse search. Common
Readline/Emacs navigation is available: Ctrl-B/F moves by character, Ctrl-A/E
by line, Alt-B/F by word, and Ctrl-P/N moves vertically or through history.
History is retained per project in `.trb/repl_history`; Ctrl-C cancels the
current input or running evaluation without leaving the REPL, and Ctrl-D exits.

`trb build` compiles every input before writing any generated file. When a
directory is built, non-`.trb` files are copied by default, producing a runnable
project tree. Use `--copy=false` when only generated source is wanted.

Output names follow the project mode:

- `post.trb` -> `post.rb` for Ruby mode
- `greeter.trb` -> `greeter.ts` for TypeScript mode
- `main.trb` -> `main.go` for Go mode
- `main.go.trb` -> `main.go` (an existing target suffix is not duplicated)

## Project configuration

`trbconfig.jsonc` accepts line comments, block comments, and trailing commas.
Mode belongs here, not in individual source files:

```jsonc
{
  // One target and package ecosystem for the whole project.
  "name": "my-app",
  "version": "0.1.0",
  "mode": "ruby",
  "sourceDir": ".",
  "outDir": "build",
  "entrypoint": "main",
  "copyFiles": true,
  "dependencies": {
    "rails": "~> 8.0",
  },
  "devDependencies": {
    "rspec-rails": "~> 7.0",
  },
  "ruby": {
    "loader": "zeitwerk",
  },
}
```

Ruby mode owns `Gemfile`, Go mode owns `go.mod`, and TypeScript mode owns
`package.json`. These files are deterministic generated views of
`trbconfig.jsonc`; edit dependencies through the config or `trb add/remove`,
then run `trb sync`.

When TypeRB is embedded in an existing application that already owns its
Gemfile, go.mod, or package.json, set `"packageManagement": "external"`.
`trb build` then generates source without reading or modifying the host
manifest; `sync`, `add`, `remove`, and `install` remain intentionally disabled.

To run `test.trb`, place it below a directory containing `trbconfig.jsonc`, then
run:

```sh
trb fmt test.trb
trb run test.trb
```

From this repository checkout, the executable example is:

```sh
./trb run examples/ruby/test.trb
```

## Core syntax

```trb
import trb/std/io
import { Logger } from "./logger"

interface Named
  name(): String
end

class User implements Named
  readonly @id: Integer
  @name: String
  @_token: String?

  def initialize(id: Integer, name: String)
    @id = id
    @name = name
    @_token = nil
    return
  end

  def name(): String
    return @name
  end
end

def main()
  user := User.new(1, "Alice")
  io.puts(user.name())
  return
end

main()
```

v0.1 portable syntax includes:

- explicit imports shared by every target; Go packages are derived from config
  and source paths
- classes, inheritance, interfaces, and modules
- typed fields, `readonly`, methods, parameters, and return types
- immutable local inference with `:=`; use `mut value := ...` when the binding
  will be reassigned or destructively updated
- uppercase runtime constants declared with `:=` at top level or directly in a
  module/class
- literals, arrays, hashes, calls, members, indexes, unary/binary expressions
- `if`/`elsif`/`else`, `while`, `return`, integer ranges, and Ruby-shaped
  `each`/`each_slice`/`each.with_index` iteration
- `_private` methods and `@_private` fields

Types are written as `name: Type`. Generics, arrays, and nullable types use
`Array<String>`, `String[]`, and `String?`.

Collection updates follow the same explicit rule:

```trb
values := [1, 2]       # immutable
mut output := [1, 2]   # mutable
output.push(3)
```

`values.push(3)`, `arrays.push(values, 3)`, or later assignment to `values`
is rejected. Constants are uppercase immutable bindings and may use runtime
initializers:

```trb
API_NAME := strings.uppercase("typerb")
```

A function that returns no value omits the return annotation, as in
`def save()`. Writing `def save(): Void` is an error; `Void` exists only as the
compiler's internal representation of no returned value.

For Ruby method definitions, ordinary keyword arguments remain available:

```trb
def page(limit: Integer, cache: false, required:)
end
```

A typed Ruby keyword parameter uses a double colon in v0.1:

```trb
def page(cache:: Boolean = false)
end
```

It transpiles to `def page(cache: false)` in Ruby.

## Rails workflow

Keep normal Rails files that do not need TypeRB as `.rb`. Rename files being
typed to `.trb`, and select `"mode": "ruby"` in the project config. Rails DSL
calls do not need to be rewritten:

```trb
import trb/platform/ruby/rails

class Post < ApplicationRecord
  belongs_to :author
  validates :title, presence: true
  scope :published, -> { where.not(published_at: nil) }

  def summary(limit: Integer = 80): String
    return body.to_s().truncate(limit)
  end
end
```

Build and run the generated Rails tree:

```sh
trb fmt --check
trb build --out-dir build .
cd build
bundle exec rails test
```

See [`examples/rails`](examples/rails) for controllers, models, concerns, jobs,
mailers, routes, and migrations.

[`examples/core-api`](examples/core-api) demonstrates the external-package
layout used to compile a TypeRB controller into an existing Rails application.

[`examples/todo`](examples/todo) is the first complete v0.1 vertical slice. A
single portable `record` package is compiled into a Go/GORM/net/http API and a
React/TypeScript client. Its schema is managed by sqldef, and the running API
exercises TodoList-to-TodoItem (1:N) and TodoItem-to-Tag (N:M) relations.

The Ruby interoperability nodes are intentionally rejected in TypeScript and
Go projects; portable targets never silently receive Ruby-only semantics.

## Imports and standard packages

Imports are resolved before type checking and retained as resolved package and
symbol identities in typed IR. A project compile parses every `.trb` file once,
builds a deterministic import graph, and checks constructor, field, method,
inheritance, and interface signatures across file boundaries. Import and
inheritance cycles, duplicate exported types, and duplicate entrypoints are
compile errors. Project imports use source-root paths:

```trb
import app/models/user
import { UserRepo } from app/repos/user_repo
```

Most portable standard packages are explicit and compile to target APIs. The
small portable prelude includes Ruby-like `puts`, and the same function remains
available through `trb/std/io` for namespaced code:

```trb
import trb/std/io
import trb/std/strings

puts(1 + 2)
io.puts(strings.uppercase("Hello"))
```

v0.1 includes `trb/std/io` and `trb/std/strings`. Platform APIs use mode-checked
packages such as `trb/platform/ruby/rails`, `trb/platform/go/context`, and
`trb/platform/typescript/node`. Importing a platform package from a mismatched
mode is a compile error. Rails projects use `ruby.loader: "zeitwerk"`, making
project imports compile-time dependencies without emitting Ruby `require`s;
standalone Ruby uses `require_relative`.

`trb/platform/ruby/rails` also activates an automatic Rails type provider. It
reads the host project's `db/schema.rb` and derives ActiveRecord models and
column types without application-authored signature files. Controller
inheritance, `params`, `render`, `ActiveRecord::Relation<T>`, `all`, `find_by!`,
`as_json`, and the pagination helper used by the core API example participate
in normal type checking. Library providers share a Declaration IR so future
RBS, `.d.ts`, and Go export-data frontends do not require checker-specific
paths.

## Formatter guarantees

`trb fmt` is deterministic and idempotent. It parses before printing and uses
the lossless lexer token stream to retain:

- standalone and trailing `#` comments;
- quoted/interpolated strings and percent literals;
- heredoc bodies and their internal whitespace;
- Rails symbol/keyword syntax and block parameters.

Indentation is two spaces and trailing whitespace is removed.

## Development

```sh
go test ./...
go vet ./...
```

Compiler phase boundaries live under `internal/ast`, `internal/checker`,
`internal/ir`, `internal/lower`, and `internal/codegen`. The source language
draft is in [`SPEC.md`](SPEC.md).
