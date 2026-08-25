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
trb update acme/contracts

# Manage a Go module, gem, or npm/Bun package in the target manifest.
trb add --native PACKAGE VERSION
trb add --native --dev PACKAGE VERSION
trb remove --native PACKAGE

# Resolve TypeRB packages, then install native target dependencies.
trb install
trb install --frozen
trb install --offline
trb install --config trbconfig.ruby.jsonc

# Validate a package-owned declaration adapter before publishing it.
trb adapter check
trb adapter check --format json
trb adapter check ../ui-types

# Build and run its explicitly declared native conformance project.
trb adapter test
trb adapter test --format json ../ui-types
```

TypeRB packages are ordinary typed source compiled through the same AST, typed
IR, and backend as application code. `trb.lock` pins their canonical identity,
transitive graph, Git revision, and content checksum. `--frozen` rejects config
drift, while `--offline` uses only local packages and the existing project
cache. `--config` selects an explicit project configuration, including when a
single source tree has separate target configurations. See the
[package guide](guides/packages.md).

`trb update` re-resolves the complete TypeRB package graph. Passing one or more
direct project aliases re-resolves only those packages and the dependencies
reached from their new manifests. Other direct package graphs remain pinned.
A selective update requires a current `trb.lock`; run `trb install` first after
editing project package requirements. If a selected graph requests a different
version of a canonical package still pinned by an unselected graph, resolution
reports the existing incompatible-requirements error instead of choosing a
version implicitly.

`trb adapter check` runs from a TypeRB package root by default, or accepts one
explicit package-root directory. It validates `trbpackage.json`, every
configured `declarationAdapters.<mode>` catalog and paired
`runtimeAdapters.<mode>` mapping, native-dependency ownership,
catalog-internal export/supporting-record conflicts, bridge kinds, runtime
coverage, and target symbol rules through the same selected ecosystem adapter
used by installation and compilation. Human output is the default. `--format
json` writes a versioned report to standard output on both success and failure
and returns a nonzero status when diagnostics contain errors.

The check command does not install native dependencies or compare the
projected contract with their declarations. An adapter package may declare one
`adapterTests.<mode>` conformance project with a project config and a structured
command argument vector. After installing that project's dependencies with a
frozen lock, `trb adapter test` runs catalog validation, a TypeRB project build,
and the declared native check in order. It verifies that the conformance
project installs the package under test from the current package root.

`adapter test` never runs during ordinary package installation, compilation,
or import. Invoking it explicitly authorizes the declared command. TypeRB
passes its argument vector directly to the executable instead of interpreting
a shell command string; the invoked tool may retain its own script, process,
and network behavior. TypeRB performs no implicit dependency installation or
network access before that command. Human output is concise; `--format json`
emits stable phase states and diagnostics while failed native-tool output
remains on standard error.

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

`trb fmt` is deterministic and idempotent. It emits one tab per nesting level,
preserves comments and opaque literal contents, and removes a terminal
`/index` from a project import when the shorter path resolves to the same
module. It retains `/index` when a direct file would otherwise win or when the
import cannot be resolved. Stdin formatting has no project snapshot and
therefore does not remove `/index`.

## Lint

```sh
# Run project checking followed by the recommended lint rules.
trb lint

# Apply fixes that a rule marks safe.
trb lint --fix

# Make remaining warnings fail CI.
trb lint --deny-warnings

# Emit a versioned machine-readable report.
trb lint --diagnostic-format json

# Lint a config-free file and its explicit imports.
trb lint report.trb
trb lint --mode ruby report.trb
```

`trb lint` is a style and maintainability layer over `trb check`: it first
requires the same project to parse, resolve, and type check, then runs the
configured lint rules on authored application source. Compiler-owned modules,
official packages, and external packages are not linted. `trb check` remains
the correctness-only command and its errors cannot be disabled by lint
configuration.

Warnings are reported without failing the command. A rule configured as
`error`, or any warning under `--deny-warnings`, returns a nonzero status.
`--fix` applies only source edits declared safe by the rule and reruns linting
on the changed files. JSON output uses the shared diagnostic schema, adds the
running `toolVersion`, and uses each lint rule ID as its stable diagnostic
code. See the [linting guide](guides/linting.md) and
[rule index](lint-rules/index.md).

## Check, build, and run

```sh
# Check the configured project without writing generated source.
trb check

# Emit stable, machine-readable diagnostics for editors and agents.
trb check --diagnostic-format json

# Check a config-free file and its explicit imports, using Go by default.
trb check report.trb
trb check --mode ruby report.trb
trb check --mode typescript report.trb

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

# Include TypeRB source locations and unoptimized Go debug information.
trb build --compile --debug --outfile .trb/debug/api

# Build a config-free Go file-root program and its explicit imports.
trb build --compile report.trb
trb build --compile --debug --outfile /tmp/report-debug report.trb

# Build in a temporary directory and run the project's main().
trb run
trb run -- first-argument

# Run one file without a project, using Go by default.
trb test.trb
trb run test.trb
trb run --mode ruby test.trb
trb run --mode typescript --runtime node test.trb
trb run --mode typescript --runtime bun test.trb

# Retain the exact generated target tree for inspection.
trb run --keep-generated
```

`trb check` is the canonical validation command. It parses, resolves, type
checks, and validates source without writing generated files or starting a
target toolchain. Without a source path it validates the complete configured
project. With one config-free `.trb` path it validates that file-root program
and its transitive explicit local imports, ignores unrelated sibling files,
uses Go by default, and accepts `--mode ruby|go|typescript`. A standalone
library file does not need to define `main()` merely to be checked. When a
configuration is discoverable from the selected file, `trb check` validates
the complete project and the configured mode wins; passing `--mode` is an
error. Human-readable diagnostics are the default.
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

Passing a `.trb` file starts standalone execution only when no
`trbconfig.jsonc` can be discovered from that file's directory. The short form
`trb FILE.trb` is equivalent to `trb run FILE.trb`. Standalone execution uses
Go when `--mode` is omitted, accepts `ruby`, `go`, or `typescript`, and accepts
`--runtime node|bun` for TypeScript. The selected entry file must define
`main()`. Its explicit local imports are resolved transitively from the entry
directory. Unrelated siblings are ignored, and imports of `*_test.trb` modules
are excluded from the production closure. TypeRB package and native
dependencies still require `trbconfig.jsonc`.

If a configuration is discovered or passed through `--config`, its mode and
runtime always win. Supplying standalone `--mode` or `--runtime` options in
that case is an error instead of an implicit project override.

Project execution writes target source below `.trb/run`; standalone execution
uses an operating-system temporary directory. Both remove generated source
after the child process exits. TypeRB forwards interrupts and termination
signals to the child process group, waits for it to stop, and then removes the
workspace. A lease protects every active project workspace; the next run
removes abandoned project workspaces left by a forced process or system
shutdown without disturbing concurrent runs. `--keep-generated` moves the
completed target tree to `.trb/generated` beside the project or standalone
file and prints its path instead of deleting it.

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
resolved from the project root. With one config-free `.trb` path, `--compile`
builds that file-root program and its explicit import closure; its default
output is `<entry-directory>/bin/<source-stem>`, and a relative `--outfile` is
resolved from the entry directory. Both forms require a top-level `main()` and
cannot be combined with `--stdout`, `--copy`, or `--out-dir`.

`--debug` is available with `--compile` in Go mode. It disables Go compiler
optimizations and records the original `.trb` paths and lines for source
debuggers such as Delve. The VS Code extension prepares this executable
automatically when starting a Go debug session.

Ruby and TypeScript executable packaging is not implemented. `--compile`
reports an unsupported-mode error for configured projects and file-root
programs in those modes. The Go executable contains compiled code and linked
dependencies, but TypeRB does not embed application files, run
schema-management commands, or remove system-library requirements introduced
by a dependency.

Output names follow the project mode:

- `main.trb` becomes `main.go` in Go mode;
- `post.trb` becomes `post.rb` in Ruby mode;
- `greeter.trb` becomes `greeter.ts` in TypeScript mode; and
- `main.go.trb` becomes `main.go`, without duplicating an existing target
  suffix.

## Compiler tooling

```sh
# Emit a read-only, versioned JSON snapshot of the configured project.
trb compiler inspect

# Select an explicit project configuration.
trb compiler inspect --config trbconfig.typescript.jsonc

# Inspect a config-free file and its explicit import closure.
trb compiler inspect --mode ruby report.trb
```

The snapshot contains the exact analyzed sources, module and authored-import
identities, flattened checked declarations and types, and diagnostics. It does
not generate target source or run a target toolchain. Compiler-generated
helpers and implicit runtime imports are excluded from the authored surface.

The command always writes the JSON snapshot to standard output after project
inputs have been loaded. A snapshot containing errors still includes source,
module, and diagnostic data and returns a nonzero status; checked declarations
are empty when semantic artifacts are unavailable. See the
[compiler tooling protocol guide](guides/compiler-tooling.md) for the version 2
schema, compatibility policy, and security considerations.

## Clean

```sh
# Remove abandoned run and test workspaces. Active workspaces are preserved.
trb clean

# Also remove output retained by trb run --keep-generated.
trb clean --generated

# Also remove the configured outDir.
trb clean --build

# Also remove downloaded TypeRB packages and the native declaration index.
trb clean --cache
```

Clean options may be combined. The default command removes only compiler-owned
temporary run and test workspaces whose leases are inactive. `--generated`
removes `.trb/generated`; `--build` removes the configured project output after
the same project-boundary safety check used by a full build; and `--cache`
removes `.trb/packages` plus `.trb/native-types.json`. A later `trb install`
restores those caches. Cleaning never removes `.trb/repl_history`.

## Test

```sh
# Discover and run colocated *_test.trb files.
trb test

# Run one suite or case by a substring of its full name.
trb test --filter "Calculator / adds numbers"

# Restrict discovery to one colocated test file.
trb test --file src/calculator_test.trb

# Emit JSON Lines events for editors and automation.
trb test --reporter json

# Build a Go test executable with TypeRB source-debug information.
trb test --compile --debug --outfile .trb/debug/tests
```

`trb test` compiles the complete project together with its test files and a
temporary test entrypoint. An application `main()` is not started during a
test build. The selected target backend executes each case, preserves `.trb`
assertion locations, and returns a nonzero status when any case fails.
`--compile` produces a Go test executable instead of running it; `--debug`
retains source mappings for debuggers such as Delve.

The temporary target tree lives below `.trb/test`, uses the same process lease
and signal handling as `trb run`, and is recovered by the next test or
`trb clean` after an unclean shutdown.

Tests use the explicit portable API from `trb/std/test`; see the
[testing guide](guides/testing.md). TypeScript browser projects report that a
process test host is unavailable. Select Bun or Node for `trb test` while
browser-hosted test execution remains staged.

To format and run an explicitly selected entry below a configured project:

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
source continues to require explicit imports. Completion offers types such as
`Date` and `Result`, package namespaces such as `math`, and functions such as
`md5` in a scratch session. Accepting an import candidate inserts its ordinary
import visibly into the input. If more than one package exports the same name,
the menu shows each origin and accepting one inserts the selected import.
When the input consists only of that candidate, Tab keeps even a unique match
as a cancellable selection. Escape, Backspace, or Delete restores the original
input without confirming the candidate. Enter confirms the selection by
replacing the editable input with only the import; a later Enter submits that
import. Typing `.` for a selected package namespace or `(` for a selected
parameterized function instead confirms the candidate as an expression: the
visible import is prepended, the character is inserted after the candidate,
and editing continues. Other ordinary characters cancel the selection and edit
the original input. Completion inside a larger expression keeps the expression
and prepends its import.

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
them. Completion edits, including an inserted import, become part of the
visible input and saved history, so copied submissions remain ordinary TypeRB
source. Enter reindents a multiline submission with a visual width of two
spaces per level and indents the next line while a block or delimiter is open.
The accepted input retains that visual width on screen, while the value used
for evaluation and history storage is converted to canonical tab-based
indentation. Terminals also provide
cursor editing, Up/Down history, and Ctrl-R
reverse search. Ctrl-B/F moves by
character, Ctrl-A/E by line, Alt-B/F by word, and Ctrl-P/N moves vertically or
through history. Ctrl-C cancels current input or evaluation without exiting;
Ctrl-D exits.

Ordinary results retain the `value : Type` form. A result directly associated
with a mutable REPL binding includes a trailing `[mut]`; arbitrary expressions
do not inherit the marker:

```console
trb:go> mut count := 123
123 : Integer [mut]
trb:go> count
123 : Integer [mut]
trb:go> count + 1
124 : Integer
```

Project history is stored in `.trb/repl_history`. Scratch history is stored in
the user cache and separated by mode.

With no arguments, `trb` starts the REPL only when both standard input and
standard output are terminals. In a non-interactive environment it prints the
command usage instead of waiting for input. `trb repl` remains the explicit
form for scripts and sessions that pass options.

## Language server

```sh
# Serve the configured project over LSP standard input/output.
trb lsp

# Select a project explicitly.
trb lsp --config path/to/trbconfig.jsonc

# Serve one file without project configuration, using Go by default.
trb lsp hello.trb
trb lsp --mode ruby hello.trb
trb lsp --mode typescript --runtime bun hello.trb
```

Standalone LSP sessions compile the selected entry and its explicit local
import closure while ignoring unrelated sibling `.trb` files. Open-document
overlays and workspace create, change, and delete events rebuild that closure,
so adding or removing an import and editing an imported file update the same
compiler snapshot. Standalone sessions use the same mode and runtime selection
as standalone execution. If `trbconfig.jsonc` is discoverable, the language
server serves that complete project instead and standalone mode or runtime
overrides are rejected.

The language server provides project-wide live diagnostics, completion, checked
hover information, signature help, document formatting, and quick fixes backed
by structured compiler suggestions. It applies ordered incremental edits using
UTF-16 LSP positions at the protocol boundary, then gives the compiler a
complete UTF-8 source snapshot. Dependency-level recompilation remains a
performance follow-up. Definition, reference, document highlight, and rename
queries follow checked project declarations, receiver members, parameters, and
local bindings rather than textual name matches.
Project-wide symbol search and the document outline follow the lossless syntax
tree, so classes, records, enums, modules, interfaces, fields, and methods
remain visible while a file has type errors. Structural folding ranges use the
same syntax tree for declarations and expression blocks, while selection ranges
expand from tokens through lines and enclosing blocks. Full-document semantic tokens
classify TypeRB types, constants, functions, methods, literals, comments, and
keywords using the same shared language service as the REPL and browser tools.
Workspace file notifications update the compiler snapshot when `.trb` files
are created, changed, or deleted outside an open editor buffer.
Non-browser projects expose a run CodeLens for each valid top-level `main()`.
The LSP command identifies the runnable declaration; an editor client owns the
process lifecycle and presentation.

The thin Visual Studio Code client in `editors/vscode` starts this command and
packages the canonical TypeRB TextMate grammar. It defaults to `trb` on `PATH`;
`typerb.server.path` overrides the executable and `typerb.server.config` adds
an explicit project configuration when needed. It discovers ordinary
`trbconfig.jsonc` files below the workspace and starts one server per project,
so declarations from different applications are never combined. An open
`.trb` file outside every discovered project receives an independent
file-root language-server session. `typerb.standalone.mode` selects its mode
and `typerb.standalone.typescript.runtime` selects Node or Bun. Its CodeLens
starts `trb run` for the owning project or file through Visual Studio Code's
Run and Debug lifecycle and changes from `▶ Run` to `↻ Restart` while that
program is active. Imported files share live editor overlays with every open
standalone entry that depends on them. Output appears in the Debug Console.
Go-mode standalone entries also support source debugging; the extension builds
a private executable for each debug session and removes it when the session
ends.

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
