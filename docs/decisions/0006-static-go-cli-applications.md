# 0006: Static Go command-line applications

## Context

TypeRB should be able to produce a distributable command-line program as one
native executable. Established tools point to two useful models: Go's `flag`
and Cobra build a parser through calls, while Rust clap and Swift
ArgumentParser derive a closed schema from typed declarations. Python Typer
uses function signatures for a similarly concise surface.

TypeRB already has records for product types, payload enums for closed choices,
postfix declaration metadata, project-wide type information, and Go executable
compilation. Generating Ruby or TypeScript launchers would weaken the
single-binary goal without adding capability to that goal.

## Decision

`trb/platform/go/cli` is an official platform package available only in `mode: "go"`.
Applications parse one root record with an explicit generic call:

```trb
args := run<AppArgs>(name: "server", version: "1.0.0", about: "Serve a directory")
```

The canonical schema model is:

- a record is a set of arguments;
- an unannotated field is positional;
- `@cli(:option)` makes a field an option;
- `@cli(:subcommand)` selects one payload enum;
- each enum member is a subcommand;
- a subcommand is payloadless or carries exactly one argument record.

Field metadata initially supports `name` (or `long`), `short`, `about`, and
`value_name`. Enum-member metadata supports `name` and `about`. Metadata values
are static string literals. CLI names default to kebab case.

The initial value boundary accepts `String`, `Integer`, `Float`, and `Boolean`.
Record defaults and nullable fields make a CLI field omittable. The parser
supports long options, `--option=value`, one-character short options, Boolean
flags, `--`, generated root and command help, and generated `--version` when a
version is supplied. Global options precede the selected subcommand. Usage
errors are written to standard error and exit with status 2.

Project analysis converts declarations into a closed, target-independent CLI
schema. The Go backend generates the parser, conversions, payload-enum
construction, and record construction using only the Go standard library. It
does not use runtime reflection, dynamic command registration, or an external
CLI framework. `trb build --compile` therefore produces the intended single
binary.

The record/payload-enum model is canonical. A later function-based shorthand
may lower into the same schema, but it must not create a second parser model or
different naming and error rules.

## Initial exclusions

The following are designed as later schema extensions rather than alternate
models: repeated and collection values, environment-variable fallback,
aliases, mutually exclusive groups, value constraints, completion generation,
nested subcommands, and options after a subcommand that target the root
record. Each requires explicit metadata and deterministic conflict rules.

Ruby and TypeScript CLI generation is deliberately outside this package's
direction, not merely missing from the first implementation. Dynamic plugins
and runtime-discovered subcommands are also excluded because they conflict
with a closed schema and single executable. Applications needing those models
can use a target-specific interoperability boundary instead.

## Consequences

- Command handlers receive ordinary typed records and payload enums; there is
  no stringly typed result map in application source.
- Help, parsing, validation, and construction cannot drift from the checked
  schema.
- The initial surface is smaller than Cobra, clap, or Typer, but its omitted
  capabilities can be added without changing the canonical data model.
- CLI source is portable TypeRB syntax, while importing `trb/platform/go/cli` intentionally
  selects a Go-only application capability.
