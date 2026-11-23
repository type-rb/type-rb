TypeRB v0.5.0-alpha.1 introduces Native as the default compiler on Arm64 macOS
and Linux. The archives contain `trb`, QBE, and the libraries used to build
Native applications. Building Native executables uses the system C toolchain.
Keep the libraries and QBE alongside the binary when extracting an archive.

After the Homebrew tap update, use `brew install type-rb/tap/trb` or
`brew upgrade type-rb/tap/trb`. Install `type-rb/tap/trbg` alongside it for
Go, Ruby, and TypeScript source workflows. The
[same-version Go companion](https://github.com/type-rb/type-rb-go/releases/tag/v0.5.0-alpha.1)
supports Arm64 and x86-64 on macOS and Linux.

This alpha supports Native executable builds, a typed REPL, Native project
tests, and editor language services. Source build and test workflows, source
debugging, tooling, and official-package coverage have gaps compared with the
Go compiler. Use the Go companion for those workflows. See the
[v0.5 transition contract](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md)
for the full release criteria.
