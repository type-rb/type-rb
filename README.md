# TypeRB

> [!WARNING]
> TypeRB is in alpha. The language, standard library, and tooling may change
> without notice.

TypeRB is a statically typed programming language that compiles to Go, Ruby,
and TypeScript.

## Try TypeRB

[A Tour of TypeRB](https://type-rb.github.io/tour/)

Learn the language through guided, executable lessons.

[TypeRB Playground](https://type-rb.github.io/play/)

Write, run, format, and transpile TypeRB while switching between Go, Ruby, and
TypeScript output.

With the compiler installed, launch the same tools locally:

```sh
trb tour
trb play
```

## Install

```sh
brew install type-rb/tap/trb
trb version
```

To build the compiler from source, use Go 1.26:

```sh
go install github.com/type-rb/type-rb/cmd/trb@latest
```

## REPL

For a terminal workflow, start a typed REPL from any directory. It uses Go
mode when no project configuration is present:

```console
$ trb
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

To start a portable JSON API with file-based routes and explicit request ID
and access-log middleware, generate the Web template:

```sh
trb init --mode go --module example.com/api --template web api
cd api
trb run
```

## Targets

- **Go** for native binaries and services
- **Ruby** for server and application development
- **TypeScript** for browser applications

Every target uses the same TypeRB grammar and portable semantics. A target mode
selects the backend, toolchain, and package ecosystem. Target-specific APIs
require explicit imports.

## Documentation

Read the [documentation](docs/README.md) for the CLI, configuration, language,
standard library, target guides, current status, and roadmap.

## License

TypeRB is available under the [MIT License](LICENSE).
