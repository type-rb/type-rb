# TypeRB

TypeRB is a statically typed language with a TypeRB-authored Native compiler.
The v0.5 alpha series introduces Native as the default implementation.

| Command | Homebrew | Platforms in alpha.1 |
| --- | --- | --- |
| `trb` (Native) | `brew install type-rb/tap/trb` | Arm64 macOS and Linux |
| `trbg` (Go) | `brew install type-rb/tap/trbg` | Arm64 and x86-64 macOS and Linux |

Use `brew upgrade type-rb/tap/trb` to update an existing Homebrew installation
on a supported Native host. The Go implementation provides Go, Ruby, and
TypeScript source workflows and is maintained in
[type-rb/type-rb-go](https://github.com/type-rb/type-rb-go).

- [Getting started](docs/getting-started.md)
- [Language guide](docs/language.md)
- [Language specification](docs/specification.md)
- [Native commands and build modes](docs/native-cli.md)
- [Native performance results](https://type-rb.github.io/type-rb-native/benchmarks/)
- [Native capability map and coverage](https://type-rb.github.io/type-rb-native/)
- [Changelog](CHANGELOG.md)
- [v0.5 transition and shared contract](docs/native-transition.md)
- [Contributing to the Native compiler](CONTRIBUTING.md)

The [GitHub Releases](https://github.com/type-rb/type-rb/releases) page provides
the Native alpha archives, checksums, and the tagged v0.4 releases.
