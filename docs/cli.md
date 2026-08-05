# Command-line reference

Run commands from a directory containing `trbconfig.jsonc` unless the command
explicitly supports a standalone file or scratch session.

## Projects and dependencies

```sh
# Create trbconfig.jsonc and the target manifest.
trb init --mode go --module example.com/acme/app .
trb init --mode ruby .
trb init --mode typescript .

# Generate go.mod, Gemfile, or package.json from trbconfig.jsonc.
trb sync

# Update dependencies in trbconfig.jsonc and regenerate the manifest.
trb add rails "~> 8.0"
trb add --dev rspec-rails "~> 7.0"
trb remove rspec-rails

# Run go mod download, bundle install, or npm install.
trb install
```

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
# Compile a project to its configured output directory.
trb build

# Compile without writing output.
trb build --check

# Choose a project output directory.
trb build --out-dir dist .

# Compile one file to stdout.
trb build --stdout app/models/post.trb

# Build in a temporary directory and run the project's main().
trb run
trb run -- first-argument

# Explicitly choose a source file for a one-off run.
trb run test.trb
```

A runnable project defines exactly one top-level `def main()`. `trb run`
compiles before every execution, so a separate build step is unnecessary.
Library projects may omit `main`, but cannot be run directly.

`trb build` compiles every input before writing generated files. Directory
builds copy non-`.trb` files by default, producing a runnable project tree. Use
`--copy=false` when only generated source is wanted.

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
# Use the nearest project configuration, or a scratch Go session.
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

Each submission passes through the ordinary parser, resolver, type checker,
typed IR lowering, and evaluator. Platform packages are accepted or rejected
according to the active mode.

REPL commands are:

- `:type EXPRESSION`
- `:load FILE`
- `:reload`
- `:help`
- `:quit`

Interactive terminals provide multiline input, colors, Tab completion,
cursor editing, Up/Down history, and Ctrl-R reverse search. Ctrl-B/F moves by
character, Ctrl-A/E by line, Alt-B/F by word, and Ctrl-P/N moves vertically or
through history. Ctrl-C cancels current input or evaluation without exiting;
Ctrl-D exits.

Project history is stored in `.trb/repl_history`. Scratch history is stored in
the user cache and separated by mode.
