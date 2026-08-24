# Getting started

TypeRB is an alpha, statically typed programming language that compiles the
same portable source to Go, Ruby, and TypeScript.

## Try in the browser

Use [A Tour of TypeRB](https://type-rb.github.io/tour/) for guided executable
lessons, or open the [TypeRB Playground](https://type-rb.github.io/play/) to
evaluate, format, and transpile isolated scratch source in the browser.

## Install the compiler

Install the current stable release with Homebrew:

```sh
brew install type-rb/tap/trb
trb version
```

To build the compiler from source, use Go 1.27:

```sh
go install github.com/type-rb/type-rb/cmd/trb@latest
```

Container builds can copy the compiler from the matching release image:

```dockerfile
ARG TYPERB_VERSION=X.Y.Z
FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM golang:1.27-bookworm
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb
```

See [TypeRB in containers](containers.md) for complete build and runtime
layouts for each target.

## Run a standalone program

Create `hello.trb`:

```trb
def main()
	name := "TypeRB"
	puts("Hello, " + name + "!")
	return
end
```

Run it in Go mode, or select another target explicitly:

```sh
trb hello.trb
trb run --mode ruby hello.trb
trb run --mode typescript --runtime bun hello.trb
```

Start the typed REPL from any directory with `trb`. It uses Go mode when no
project configuration is present:

```console
$ trb
trb:go> 1 + 2
3 : Integer
```

## Create a project

Create a Go project:

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

Then format, lint, and run the project:

```sh
trb fmt
trb lint
trb run
```

Use `trb build` to keep generated target source. Go projects can also produce
an executable with `trb build --compile`.

## Choose a target

- Go is suitable for native binaries and services.
- Ruby is suitable for server and application development.
- TypeScript is suitable for browser and server applications.

A target mode selects the backend, toolchain, and package ecosystem. It does
not change TypeRB grammar or portable semantics. Continue with the
[learning path](learning.md), or use the [language guide](language.md) as a
practical reference.
