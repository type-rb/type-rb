# 0006: Single-binary command-line applications

## Context

TypeRB should be able to produce a distributable command-line program as one
native executable. Established tools point to two useful models: Go's `flag`
and Cobra build a parser through calls, while Rust clap and Swift
ArgumentParser derive a closed schema from typed declarations. Python Typer
uses function signatures for a similarly concise surface.

TypeRB already has records for product types, payload enums for closed choices,
postfix declaration metadata, project-wide type information, and native
executable compilation through the Go toolchain. Generating Ruby or TypeScript
launchers would weaken the single-binary goal without adding capability to
that goal. The current backend choice is not part of the CLI application model
exposed to users.

## Decision

`trb/cli` is the official compiler-integrated package for building
single-binary command-line applications. Applications parse one root record
with an explicit generic call:

```trb
args := run<AppArgs>(name: "server", version: "1.0.0", about: "Serve a directory")
```

The type argument resolves through transparent aliases to one non-nullable,
non-generic root record. Nullable and instantiated generic root records are
rejected before native executable generation in the initial contract.

The canonical schema model is:

- a record is a set of arguments;
- an unannotated field is positional;
- `@cli(:option)` makes a field an option;
- `@cli(:subcommand)` selects one payload enum;
- each enum member is a subcommand;
- a subcommand is payloadless or carries exactly one argument record.

Option metadata initially supports `name` (or `long`), `short`, `about`, and
`value_name`; positional fields support `about` and `value_name`.
Enum-member metadata supports `name` and `about`. The root subcommand selector
uses only `@cli(:subcommand)`. Metadata values are static string literals. CLI
names default to kebab case. A subcommand name cannot begin with `-`, a long
option name cannot contain `=`, and `-` cannot be used as a short option because
those spellings conflict with the parser grammar. U+0000 is rejected in
subcommand and option names because an operating-system argument cannot contain
NUL. Other Unicode names remain valid. The schema analyzer rejects reserved
names before generation.

The value boundary accepts `String`, `Integer`, `Float`, and `Boolean`.
An option may wrap one of those scalar types in `Array`; each occurrence
appends one converted value in argument order. Repeated Arrays are not
positional arguments. Required and default behavior continues to come from the
record field.
Record defaults and nullable types make scalar argument fields omittable. The
root subcommand selector is required, non-nullable, and has no default in the
initial contract. The parser supports long options, `--option=value`,
one-character short options, Boolean flags, `--`, generated root and command
help, and generated `--version` when a version is supplied. Global options
precede the selected subcommand. Usage errors are written to standard error
and exit with status 2.

Project analysis converts declarations into a closed, target-independent CLI
schema. The current native executable backend generates the parser,
conversions, payload-enum construction, and record construction using only the
Go standard library internally. It does not use runtime reflection, dynamic
command registration, or an external CLI framework. `trb build --compile`
therefore produces the intended single binary.

A transparent, non-generic type alias of the root record resolves to that
record before schema analysis. Aliases do not create a second CLI schema
identity.

The record/payload-enum model is canonical. A later function-based shorthand
may lower into the same schema, but it must not create a second parser model or
different naming and error rules.

## Initial exclusions

The following are designed as later schema extensions rather than alternate
models: non-scalar collection values, variadic positionals,
environment-variable fallback, aliases, mutually exclusive groups, value
constraints, completion generation, nested subcommands, optional or default
root commands, and options after a subcommand that target the root record.
Each requires explicit metadata and deterministic conflict rules.

The initial compiler implementation requires `mode: "go"` and the Go
toolchain to produce the native executable. This is a build requirement, not a
Go API exposed by `trb/cli`. Ruby and TypeScript launcher generation is
deliberately outside this package's direction, not merely missing from the
first implementation. A future native executable backend may implement the
same schema without changing application imports.

Dynamic plugins and runtime-discovered subcommands are excluded because they
conflict with a closed schema and single executable. Applications needing
those models can use a target-specific interoperability boundary instead.

## Consequences

- Command handlers receive ordinary typed records and payload enums; there is
  no stringly typed result map in application source.
- Help, parsing, validation, and construction cannot drift from the checked
  schema.
- The initial surface is smaller than Cobra, clap, or Typer, but its omitted
  capabilities can be added without changing the canonical data model.
- Users select the CLI application capability with `trb/cli`; they do not
  select or program against Go APIs.
- Building currently requires the Go target and toolchain, while the schema,
  diagnostics, and application-facing API remain target-independent.
- `trb/platform/go/cli`, published in 0.3.44, remains a compatibility import;
  new source and documentation use `trb/cli`.
