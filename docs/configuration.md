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
    "example.com/acme/library": "v1.2.3"
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

Go mode owns `go.mod`, Ruby mode owns `Gemfile` and `.ruby-version`, and
TypeScript mode owns `package.json`. These files are deterministic views of
`trbconfig.jsonc`. Edit dependencies through the config or `trb add` and
`trb remove`, then run `trb sync`.

## TypeScript toolchain

New managed TypeScript projects use ESM, npm, and the latest TypeScript release:

```jsonc
{
  "devDependencies": {
    "typescript": "latest"
  },
  "typescript": {
    "packageManager": "npm",
    "moduleType": "module",
    "runtime": "node"
  }
}
```

TypeRB targets the current TypeScript release without legacy code-generation
branches. Browser applications are the primary TypeScript use case; APIs tied
to a particular runtime remain explicit platform packages.

`typescript.runtime` selects `browser`, `bun`, or `node`. Existing projects
that omit it retain the previous Node execution behavior. `browser` projects
compile source for a browser application or bundler and cannot use `trb run`
as a process entrypoint. Bun server projects can select both runtime and
package manager without additional manifest files:

```jsonc
{
  "mode": "typescript",
  "typescript": {
    "runtime": "bun",
    "packageManager": "bun",
    "moduleType": "module"
  }
}
```

When `packageManager` is omitted, Bun runtime projects default to `bun`; other
TypeScript projects default to `npm`. The two settings remain independent, so
an explicitly configured npm installation can still be executed by Bun.

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
    "acme/contracts": "../../packages/contracts/src"
  }
}
```

An `index.trb` file is the package entry module. Projects in different modes can
import the same package without copying its source.

## Ruby toolchain and loader

Ruby projects default to the current supported Ruby release and can select how
generated imports are loaded:

```jsonc
{
  "ruby": {
    "version": "4.0.6",
    "loader": "zeitwerk"
  }
}
```

For a managed Ruby project, `trb sync` writes the configured version to both
`Gemfile` and `.ruby-version`. Bundler itself uses the version included with
that Ruby release.

The `require_relative` loader emits explicit relative requires. The `zeitwerk`
loader keeps project imports as compile-time dependencies without emitting
Ruby `require` calls.
