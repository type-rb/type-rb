# Command-line reference

Run commands from a directory containing `trbconfig.jsonc` unless the command
explicitly supports a standalone file or scratch session.

## Projects and dependencies

```sh
# Create trbconfig.jsonc and the target manifest.
trb init --mode go --module example.com/acme/app .
trb init --mode ruby .
trb init --mode typescript .
trb init --mode typescript --runtime bun .

# Generate a portable JSON API project for any target.
trb init --mode go --module example.com/acme/api --template web .

# Generate go.mod, Gemfile, or package.json from trbconfig.jsonc.
trb sync

# Add, remove, or update portable TypeRB packages.
trb add acme/contracts v1.2.3
trb add --source gitlab.com/company/auth company/auth v2.0.0
trb add --path ../contracts local/contracts
trb remove acme/contracts
trb update

# Manage a Go module, gem, or npm/Bun package in the target manifest.
trb add --native PACKAGE VERSION
trb add --native --dev PACKAGE VERSION
trb remove --native PACKAGE

# Resolve TypeRB packages, then install native target dependencies.
trb install
trb install --frozen
trb install --offline
```

TypeRB packages are ordinary typed source compiled through the same AST, typed
IR, and backend as application code. `trb.lock` pins their canonical identity,
transitive graph, Git revision, and content checksum. `--frozen` rejects config
drift, while `--offline` uses only local packages and the existing project
cache. See the [package guide](guides/packages.md).

The `web` template uses `src` as the source root and creates `main.trb`, an
index route, and an explicit root middleware stack with request IDs and JSONL
access logging. It never replaces an existing source file.

## Format

```sh
# Format all .trb files below the current directory in place.
trb fmt

# Check formatting in CI without changing files.
trb fmt --check

# Format stdin to stdout.
trb fmt -

# Format selected paths.
trb fmt src test.trb
```

`trb fmt` is deterministic and idempotent. It emits one tab per nesting level
and preserves comments and opaque literal contents.

## Build and run

```sh
# Check the configured project without writing generated source.
trb check

# Emit stable, machine-readable diagnostics for editors and agents.
trb check --diagnostic-format json

# Compile a project to its configured output directory.
trb build

# Choose a project output directory.
trb build --out-dir dist .

# Compile one file to stdout.
trb build --stdout app/models/post.trb

# Build a Go project into bin/<project-name>.
trb build --compile

# Choose the executable output file.
trb build --compile --outfile bin/api

# Build in a temporary directory and run the project's main().
trb run
trb run -- first-argument

# Explicitly choose a source file for a one-off run.
trb run test.trb
```

`trb check` is the canonical validation command. It parses, resolves, type
checks, and validates the complete project without writing generated files or
starting a target toolchain. Human-readable diagnostics are the default.
`--diagnostic-format json` writes a versioned report to standard output and
returns a nonzero status when errors exist. Locations use one-based lines and
columns plus zero-based UTF-8 byte offsets. Each diagnostic has a stable
`TRBxxxx` code; messages may improve without changing that code. Related
locations and atomic source-edit suggestions are included when available.
The initial code families are `TRB1xxx` for syntax, `TRB2xxx` for resolution
and imports, `TRB3xxx` for types and flow, `TRB4xxx` for project integration,
and `TRB5xxx` for backend generation.

A runnable project defines exactly one top-level `def main()`. `trb run`
compiles before every execution, so a separate build step is unnecessary.
Library projects may omit `main`, but cannot be run directly.

TypeScript projects execute with the configured `typescript.runtime`. Node is
the compatibility default; Bun projects use `bun run`. Browser projects are
compiled for their application or bundler and report that `trb run` has no
process entrypoint.

`trb build` compiles every input before writing generated files. Directory
builds copy non-`.trb` files by default, producing a runnable project tree. Use
`--copy=false` when only generated source is wanted.

In Go mode, `trb build --compile` generates Go source in a temporary directory,
invokes `go build`, and keeps only the executable. The default output is
`bin/<project-name>` below the project root. A relative `--outfile` is also
resolved from the project root. `--compile` builds the complete configured
project, requires a top-level `main()`, and cannot be combined with source
paths, `--check`, `--stdout`, `--copy`, or `--out-dir`.

Ruby and TypeScript executable packaging is not implemented. `--compile`
reports an unsupported-mode error for those projects. The Go executable
contains compiled code and linked dependencies, but TypeRB does not embed
application files, run schema-management commands, or remove system-library
requirements introduced by a dependency.

Output names follow the project mode:

- `main.trb` becomes `main.go` in Go mode;
- `post.trb` becomes `post.rb` in Ruby mode;
- `greeter.trb` becomes `greeter.ts` in TypeScript mode; and
- `main.go.trb` becomes `main.go`, without duplicating an existing target
  suffix.

To run a standalone `test.trb` below a configured project:

```sh
trb fmt test.trb
trb run test.trb
```

## REPL

```sh
# Start a REPL from an interactive terminal. It uses the nearest project
# configuration, or a scratch Go session.
trb

# Start the same REPL explicitly.
trb repl

# Select the mode for this session.
trb repl --mode go
trb repl --mode ruby
trb repl --mode typescript

# Select a project configuration explicitly.
trb repl --config path/to/trbconfig.jsonc
```

When a project config is available, the REPL loads its imports, local packages,
and type providers. Without a config, it starts an isolated Go-mode scratch
session. `--mode` takes precedence over a discovered project mode for that
session.

Public declarations with a name unique across the project and public types
from portable standard packages are available without typing an import in the
REPL. The session adds deterministic hidden imports while ordinary project
source continues to require explicit imports. Completion therefore offers
types such as `Date` and `Result` in a scratch session. If more than one module
exports the same name, import the intended declaration explicitly before using
it.

Each submission passes through the ordinary parser, resolver, type checker,
typed IR lowering, and evaluator. Platform packages are accepted or rejected
according to the active mode.

REPL commands are:

- `:type EXPRESSION`
- `:load FILE`
- `:reload`
- `:help`
- `:quit`

Interactive terminals provide multiline input, colors, and a Tab completion
menu with checked signatures and return types. A single candidate completes
directly; multiple candidates are shown before Tab or Shift-Tab cycles through
them. Terminals also provide cursor editing, Up/Down history, and Ctrl-R reverse
search. Ctrl-B/F moves by
character, Ctrl-A/E by line, Alt-B/F by word, and Ctrl-P/N moves vertically or
through history. Ctrl-C cancels current input or evaluation without exiting;
Ctrl-D exits.

Project history is stored in `.trb/repl_history`. Scratch history is stored in
the user cache and separated by mode.

With no arguments, `trb` starts the REPL only when both standard input and
standard output are terminals. In a non-interactive environment it prints the
command usage instead of waiting for input. `trb repl` remains the explicit
form for scripts and sessions that pass options.

## Database schema

```sh
trb db plan
trb db apply [--allow-destructive]
trb db export [--output PATH]
trb db lock [--from-db]
trb db check [--from-db]
```

`plan`, `apply`, and `export` adapt the configured external sqldef executable.
`apply` refreshes the deterministic schema lock only after a successful schema
change. DROP operations remain disabled unless `--allow-destructive` is
explicit. `lock` and `check` operate offline by default; `--from-db` selects
live introspection. See the [database schema guide](guides/database.md).

## Background Jobs

```sh
trb jobs start [--once] [--queue NAME]
trb jobs list
trb jobs retry JOB_ID
trb jobs discard JOB_ID
```

These commands run the adapter selected by `jobs.configuration`. `start`
claims and performs durable Jobs; `--once` stops after at most one claim and
`--queue` limits claims to one queue. `list`, `retry`, and `discard` operate on
persisted adapter state. See the [`trb/jobs` guide](guides/jobs.md).

## Playground and tour

```sh
# Open an isolated scratch playground in the default browser.
trb play

# Choose the initial target and a fixed local port.
trb play --mode typescript --port 3000

# Print the URL without opening a browser.
trb play --no-open

# Open the guided language tour.
trb tour
```

`trb play` and `trb tour` bind only to `127.0.0.1`. A zero or omitted port
selects an available port. Without `--mode`, the nearest project mode becomes
the initial target; when there is no project, the initial target is Go. The
target can be changed inside the browser.

The editor provides dependency-free TypeRB syntax highlighting and shortcuts
for tab indentation and Cmd/Ctrl-Enter execution. Run compiles through the
ordinary parser, resolver, type checker, and typed IR evaluator. Transpile
shows the generated source for the selected backend, and Format uses the same
comment-preserving formatter as `trb fmt`.

Browser runs are isolated scratch evaluations. They do not load project files,
invoke `main()`, or start a target toolchain. Top-level expressions are
evaluated from a fresh state on each run. Host filesystem, process, and
platform packages are rejected during browser execution; they may still be
type checked and displayed with Transpile.

The tour stores lesson edits and completion progress in the local browser.
The repository test suite compiles and evaluates every lesson in Go, Ruby, and
TypeScript modes before Pages is published.
