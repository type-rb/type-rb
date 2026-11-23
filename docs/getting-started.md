# Getting started

TypeRB is a statically typed programming language. The v0.5 alpha series
introduces the Native compiler as the default `trb` and retains the Go
implementation as `trbg`.

## Install

On Arm64 macOS or Linux:

```sh
brew install type-rb/tap/trb
trb --version
```

If you already installed `trb` with Homebrew, run
`brew upgrade type-rb/tap/trb`. Native executable builds use the system C
toolchain. See the [Native command guide](native-cli.md) for the available
commands and modes in this alpha.

The Go compiler provides the Go, Ruby, and TypeScript workflows used in the
rest of this guide. Install it on Arm64 or x86-64 macOS or Linux, then use
`trbg` wherever the examples invoke the Go compiler:

```sh
brew install type-rb/tap/trbg
trbg --version
```

Both commands can be installed together. See the [alpha release](https://github.com/type-rb/type-rb/releases/tag/v0.5.0-alpha.1)
for platform archives and known feature gaps.

## Run a standalone program

Create `hello.trb`:

<!-- trb-doc-test: getting-started-standalone kind=program -->
```trb
def main()
	name := "TypeRB"
	puts("Hello, " + name + "!")
end
```

TypeRB needs only the toolchain for the target you choose. The Go examples in
the rest of this guide require Go 1.27. Ruby is required only for the Ruby
command below, and Bun only for the TypeScript command.

Run the program in Go mode to follow the rest of this guide:

```sh
trb hello.trb
```

If you prefer another target, run only the corresponding command:

```sh
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

<!-- trb-doc-test: getting-started-project kind=program -->
```trb
def main()
	puts("Hello from TypeRB")
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
