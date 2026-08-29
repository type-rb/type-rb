# TypeRB

> [!WARNING]
> TypeRB is in alpha. The language, standard library, and tooling may change
> without notice.

TypeRB is a statically typed programming language that compiles to Go, Ruby,
and TypeScript.

## Try TypeRB

[TypeRB documentation](https://type-rb.github.io/docs/)

Install the compiler, follow the learning path, and browse application guides,
lint rules, and language references.

[A Tour of TypeRB](https://type-rb.github.io/tour/)

Learn the language through guided, executable lessons.

[TypeRB Playground](https://type-rb.github.io/play/)

Write, evaluate, format, and transpile TypeRB scratch source while switching
between Go, Ruby, and TypeScript output.

[Docker quickstart](examples/docker/README.md)

Run TypeRB locally without installing the compiler or a target toolchain.

With the compiler installed, launch the same tools locally:

```sh
trb tour
trb play
```

## GitHub syntax highlighting

GitHub's built-in syntax highlighting depends on
[GitHub Linguist](https://github.com/github-linguist/linguist/blob/main/CONTRIBUTING.md#language-extension-and-filename-usage-requirements),
which requires widespread real-world usage before accepting a new language or
file extension. Until TypeRB becomes eligible for native highlighting, install
[TypeRB Syntax Highlighting for GitHub](https://chromewebstore.google.com/detail/typerb-syntax-highlightin/icogpeecnhfgfdbdjihfhmngpengkcni).

The extension highlights `.trb` file views, pull request diffs, and explicit
`trb` and `typerb` Markdown code blocks on github.com.

## Install

```sh
brew install type-rb/tap/trb
trb version
```

To build the compiler from source, use Go 1.27:

```sh
go install github.com/type-rb/type-rb/cmd/trb@latest
```

Container builds can copy the compiler from the matching release image without
adding a Go, Ruby, Node, or Bun version to TypeRB's distribution contract:

```dockerfile
ARG TYPERB_VERSION=X.Y.Z
FROM ghcr.io/type-rb/trb:${TYPERB_VERSION} AS typerb

FROM golang:1.27-bookworm
COPY --from=typerb /usr/local/bin/trb /usr/local/bin/trb
```

See [TypeRB in containers](docs/containers.md) for Go, Ruby, and TypeScript
build and runtime layouts, supported platforms, and version pinning.

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

Run a single file without creating `trbconfig.jsonc`. Standalone execution uses
Go by default; select Ruby or TypeScript explicitly when needed:

```trb
def main()
	puts("Hello from TypeRB")
	return
end
```

```sh
trb hello.trb
trb run --mode ruby hello.trb
trb run --mode typescript --runtime bun hello.trb
```

When a configuration exists above the file, TypeRB follows that project
instead. Standalone execution follows the selected file's explicit local
imports without compiling unrelated siblings and leaves no generated files
beside it. A config-free Go entry can also be built for distribution or source
debugging:

```sh
trb build --compile --debug --outfile ./hello-debug hello.trb
```

Inside a configured project, omit the file argument and use
`trb build --compile` to build the project entrypoint.

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
trb lint
trb run
```

Add `calculator_test.trb` beside application source and run portable tests:

```trb
import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(1 + 2).to_equal(3)
	end
end
```

```sh
trb test
```

The VS Code extension discovers the same suites in Test Explorer. See the
[testing guide](docs/guides/testing.md) for filtering and assertion APIs.

`trb run` builds in a temporary directory before executing the program. Use
`trb build` when you want to keep the generated source.

Go projects can also produce an executable:

```sh
trb build --compile
./bin/hello
```

For a typed single-binary command-line application, `trb/cli` derives
arguments, options, and subcommands from records and payload enums. Building
currently requires the Go toolchain, but application source does not use Go
APIs. See the
[CLI application guide](docs/guides/cli-applications.md).

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
- **TypeScript** for browser and server applications

Every target uses the same TypeRB grammar and portable semantics. A target mode
selects the backend, toolchain, and package ecosystem. Target-specific APIs
require explicit imports.

## Documentation

Follow the [learning path](https://type-rb.github.io/docs/learning/), or browse
the [documentation](https://type-rb.github.io/docs/) for application guides,
lint rules, references, current status, and the roadmap.

## License

TypeRB is available under the [MIT License](LICENSE).
