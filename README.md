# TypeRB

> [!WARNING]
> TypeRB is in alpha. The language, standard library, and tooling may change
> without notice.

TypeRB is a statically typed programming language that compiles to Go, Ruby,
and TypeScript.

## Install

```sh
brew install type-rb/tap/trb
trb version
```

Homebrew installs a prebuilt compiler on macOS or Linux. Install the target
toolchain separately when you want to run generated code or manage its
packages.

To build the compiler from source, use Go 1.26:

```sh
go install github.com/type-rb/type-rb/cmd/trb@latest
```

## Try TypeRB

Open the local browser playground or follow the guided language tour:

```sh
trb play
trb tour
```

Both commands run locally and open a browser. The playground can execute,
format, and transpile scratch TypeRB while switching between Go, Ruby, and
TypeScript output.

For a terminal workflow, start a typed REPL from any directory. It uses Go
mode when no project configuration is present:

```console
$ trb repl
trb:go> puts("Hello, TypeRB!")
Hello, TypeRB!
trb:go> 1 + 2
3 : Integer
```

Select another target for a one-off session:

```sh
trb repl --mode ruby
trb repl --mode typescript
```

## Create a project

Create and run a Go project:

```sh
trb init --mode go --module example.com/hello hello
cd hello
```

Create `main.trb`:

```trb
def main()
	puts("Hello from TypeRB")
	return
end
```

Then format and run it:

```sh
trb fmt
trb run
```

`trb run` builds in a temporary directory before executing the program. Use
`trb build` when you want to keep the generated source.

Go projects can also produce an executable:

```sh
trb build --compile
./bin/hello
```

## Targets

- **Go** for native binaries and services
- **Ruby** for server and application development
- **TypeScript** for server and browser applications

Every target uses the same TypeRB grammar and portable semantics. A target mode
selects the backend, toolchain, and package ecosystem. Target-specific APIs
require explicit imports.

## Documentation

Read the [documentation](docs/README.md) for the CLI, configuration, language,
standard library, target guides, current status, and roadmap.

## License

TypeRB is available under the [MIT License](LICENSE).
