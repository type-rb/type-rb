# Build a command-line application

`trb/cli` generates a typed command-line parser and compiles the application as
one native executable. Its records, payload enums, and metadata are TypeRB
APIs; users do not program against Go APIs. The current native executable
backend requires `mode: "go"` and the Go toolchain. Ruby and TypeScript launcher
generation is not provided.

## Define the schema

Use records for argument groups and a payload enum for subcommands:

```trb
import { run } from trb/cli

record ServeArgs
	directory: String
	port: Integer = 8080 @cli(:option, short: "p", about: "Port to listen on")
	verbose: Boolean = false @cli(:option, short: "v", about: "Enable verbose output")
	header: Array<String> = [] @cli(:option, value_name: "HEADER", about: "Add a header")
end

enum Command
	Serve(args: ServeArgs) @cli(about: "Start the server")
	Version @cli(about: "Print version details")
end

record AppArgs
	command: Command @cli(:subcommand)
end
```

An unannotated field such as `directory` is positional. An option uses its
kebab-cased field name by default, so `verbose` becomes `--verbose` and
`output_path` becomes `--output-path`. `name` or `long` overrides the long
name; `short` must contain one character. Defaults belong to the record and are
also used by ordinary non-CLI construction. Subcommand names cannot begin with
`-`, long option names cannot contain `=`, and `-` is reserved from use as a
short option. U+0000 is also reserved in subcommand and option names because
it cannot appear in an operating-system argument; other Unicode names are
supported. `trb check` reports reserved names instead of generating help for
an option or command that the parser cannot select.

Use `about` and `value_name` to describe a positional field in generated help.
Options accept those keys plus `name` (or `long`) and `short`. The root
subcommand selector itself has no naming metadata; put `name` and `about` on
its enum members.

The root `@cli(:subcommand)` field is required and non-nullable and cannot have
a record default in the initial contract. Defaults and nullable types make
ordinary scalar positional or option fields omittable.

The record passed to `run<...>` must itself be non-nullable and non-generic in
the initial contract. Transparent non-generic aliases are accepted, but forms
such as `run<AppArgs?>` and `run<AppArgs<Integer>>` are reported before native
executable generation.

A transparent alias of the root record uses the same schema:

```trb
alias Arguments = AppArgs

args := run<Arguments>(name: "fileserver")
```

## Parse and dispatch

Call `run` once from the application and match the returned payload enum:

```trb
def main()
	args := run<AppArgs>(name: "fileserver", version: "1.0.0", about: "Serve a local directory")

	case args.command
	when Command::Serve(serve)
		puts("serving " + serve.directory)
		puts("port " + serve.port.to_s())
	when Command::Version
		puts("extended version details")
	end
end
```

Typical calls are:

```sh
fileserver serve public
fileserver serve public --port 9000 --verbose
fileserver serve public -p 9000 -v
fileserver serve --help
fileserver --version
```

Long values may use `--port=9000`. `--` ends option parsing. Boolean options
are flags and also accept an explicit long value such as `--verbose=false`.
Generated usage errors go to standard error and exit with status 2.

An option field may use `Array<String>`, `Array<Integer>`, `Array<Float>`, or
`Array<Boolean>`. Every occurrence appends one converted scalar in command-line
order, for example `--header one --header=two`. Arrays are not positional
arguments. A required Array option must occur at least once; a record default
such as `[]` makes it optional. If a non-empty default is declared, the first
explicit occurrence replaces the default and later occurrences append to that
parsed value. Repeating a non-Array scalar option retains its existing
last-occurrence-wins behavior.

## Compile one executable

The current native executable backend uses the Go toolchain. Configure the
project with `mode: "go"`, then compile the entrypoint:

```sh
trb check
trb build --compile
./bin/fileserver --help
```

The generated parser uses only the Go standard library internally. The
resulting program does not require a Go, Ruby, or JavaScript runtime, runtime
reflection, or a separate schema file.

## Initial contract

The current implementation supports scalar `String`, `Integer`, `Float`, and
`Boolean` fields, repeated scalar option Arrays, one required root subcommand
field, payloadless commands, and commands with one record payload. Root options
must appear before the selected subcommand.

Environment fallback, option aliases, validation constraints, mutually
exclusive groups, shell completion, nested subcommands, optional or default
root commands, variadic positionals, and combined short flags are not yet
supported. These are reserved as extensions to the same record and payload-enum
schema. Dynamic plugins and Ruby or TypeScript launcher generation are outside
the package's single-binary model.

`trb/platform/go/cli` was published in TypeRB 0.3.44 and remains accepted as a
compatibility import. Use `trb/cli` in new source.
