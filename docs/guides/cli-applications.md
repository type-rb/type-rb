# Build a command-line application

`trb/platform/go/cli` generates a typed command-line parser for a Go project. The result
can be compiled as one native executable; Ruby and TypeScript CLI backends are
not provided.

## Define the schema

Use records for argument groups and a payload enum for subcommands:

```trb
import { run } from trb/platform/go/cli

record ServeArgs
	directory: String
	port: Integer = 8080 @cli(:option, short: "p", about: "Port to listen on")
	verbose: Boolean = false @cli(:option, short: "v", about: "Enable verbose output")
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
short option. `trb check` reports these names instead of generating help for an
option or command that the parser cannot select.

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
	return
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

## Compile one executable

Configure the project for Go, then compile the entrypoint:

```sh
trb check
trb build --compile
./bin/fileserver --help
```

The generated parser uses only the Go standard library. It does not require a
Ruby or JavaScript runtime, runtime reflection, or a separate schema file.

## Initial contract

The first implementation supports scalar `String`, `Integer`, `Float`, and
`Boolean` fields, one root subcommand field, payloadless commands, and commands
with one record payload. Root options must appear before the subcommand.

Repeated values and Arrays, environment fallback, option aliases, validation
constraints, mutually exclusive groups, shell completion, nested subcommands,
and combined short flags are not yet supported. These are reserved as
extensions to the same record and payload-enum schema. Dynamic plugins and
Ruby or TypeScript CLI generation are outside the package's single-binary
model.
