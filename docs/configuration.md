# Project configuration

`trbconfig.jsonc` is the source of truth for the project target, source and
output directories, and target dependencies. It accepts line and block
comments. Trailing commas are not allowed.

## Example

```jsonc
{
  // One target and package ecosystem for the whole project.
  "name": "my-app",
  "version": "0.1.0",
  "mode": "go",
  "sourceDir": ".",
  "outDir": "build",
  "copyFiles": true,
  "packageManagement": "managed",
  "dependencies": {
    "gorm.io/gorm": "v1.31.1"
  },
  "devDependencies": {},
  "go": {
    "module": "example.com/my-app",
    "version": "1.26",
    "rootPackage": "main"
  }
}
```

Use `trb init` to create a valid starting config:

```sh
trb init --mode go --module example.com/my-app .
trb init --mode ruby .
trb init --mode typescript .
```

## Modes

A project declares one mode: `go`, `ruby`, or `typescript`. The mode selects
the backend, target toolchain, and package ecosystem. It does not select a
grammar variant or loosen portable type checking. Target-specific capabilities
require an explicit `trb/platform/<mode>/*` import.

Go mode owns `go.mod`, Ruby mode owns `Gemfile`, and TypeScript mode owns
`package.json`. These manifests are deterministic views of
`trbconfig.jsonc`. Edit dependencies through the config or `trb add` and
`trb remove`, then run `trb sync`.

## Package management

The default `"packageManagement": "managed"` lets TypeRB generate and use the
target manifest.

When TypeRB is embedded in an application that already owns its manifest, set:

```jsonc
{
  "packageManagement": "external"
}
```

`trb build` then generates source without reading or modifying the host
manifest. `trb sync`, `add`, `remove`, and `install` are disabled in this mode.

## Project entrypoint

A runnable project defines exactly one top-level `def main()`. `main` is a
language convention rather than a configurable entrypoint, so the config has no
entrypoint field. A library project may omit `main`.

## Local packages

Map a portable source directory into the project import graph with
`localPackages`:

```jsonc
{
  "localPackages": {
    "todo/contracts": "../../packages/contracts/src"
  }
}
```

An `index.trb` file is the package entry module. Projects in different modes can
import the same package without copying its source.

## Rails loader

Ruby projects using Rails can select Zeitwerk loading:

```jsonc
{
  "ruby": {
    "loader": "zeitwerk"
  }
}
```

Project imports then remain compile-time dependencies without emitting Ruby
`require` calls.
