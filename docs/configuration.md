# Project configuration

`trbconfig.jsonc` is the source of truth for the project target, source and
output directories, TypeRB packages, and native target dependencies. It
accepts line and block comments. Trailing commas are not allowed.

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
  "packages": {
    "acme/contracts": "v1.2.3"
  },
  "dependencies": {
    "example.com/acme/library": "v1.2.3"
  },
  "devDependencies": {},
  "go": {
    "module": "example.com/my-app",
    "version": "1.27",
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
the native `dependencies` and `devDependencies` in `trbconfig.jsonc`. Edit
native dependencies through the config or `trb add --native` and
`trb remove --native`, then run `trb sync`.

`go.version` must be Go 1.27 or later. Generated Go uses native generic
methods introduced in Go 1.27, so projects targeting Go 1.26 are rejected.

## TypeScript toolchain

New managed TypeScript projects use ESM, npm, and the latest compatible
TypeScript 6 patch:

```jsonc
{
  "devDependencies": {
    "typescript": "^6.0.0"
  },
  "typescript": {
    "packageManager": "npm",
    "moduleType": "module",
    "runtime": "node"
  }
}
```

TypeScript 6 is the supported toolchain while the TypeScript 7 programmatic API
and its surrounding package ecosystem stabilize. The range accepts compatible
6.x patch releases without moving a project to 7.x. Browser applications are
the primary TypeScript use case; APIs tied to a particular runtime remain
explicit platform packages.

After native dependencies are installed, `trb install` also indexes supported
`.d.ts` exports into `.trb/native-types.json`. TypeRB source can import those
configured packages directly; ordinary builds and completion use the cached
index and generated TypeScript keeps the original package specifier. Native
declaration indexing requires TypeScript 6.x and reports the installed version
when another major version is detected.

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

## TypeRB packages

`packages` declares portable TypeRB source packages. A short key defaults to a
GitHub repository, while the lock records its canonical manifest identity:

```jsonc
{
  "packages": {
    "acme/contracts": "v1.2.3",
    "company/auth": {
      "source": "gitlab.com/company/auth",
      "version": "v2.0.0"
    },
    "local/widgets": {
      "path": "../widgets"
    }
  }
}
```

`acme/contracts` resolves from `github.com/acme/contracts` by default and stays
the explicit source import. Full repository paths are also valid keys. Remote
packages accept an exact semantic-version tag, `latest`, or a Git revision.
`latest` is pinned until `trb update` is run. Local paths are development
inputs and are intentionally not content locked.

`trb.lock` records canonical package names, transitive dependencies, Git commit
IDs, and SHA-256 content checksums. Commit it to version control. Resolved
content lives below the ignored `.trb/packages` directory. Builds and the REPL
never contact a package source; run `trb install` after changing `packages`.
See the [package guide](guides/packages.md).

## Native package management

The default `"packageManagement": "managed"` lets TypeRB generate and use the
target manifest from `dependencies` and `devDependencies`.

When TypeRB is embedded in an application that already owns its manifest, set:

```jsonc
{
  "packageManagement": "external"
}
```

`trb build` then generates source without reading or modifying the host
manifest. Native `sync`, add, remove, and install operations are disabled in
this mode. TypeRB source packages remain available because their lock and
source cache are independent of the target package manager.

## Database schema workflow

The optional `db` section configures mode-independent schema commands:

```jsonc
{
  "db": {
    "adapter": "sqlite",
    "database": "db/development.sqlite3",
    "schema": "db/schema.sql",
    "lock": "db/schema.lock.json",
    "sqldef": {
      "command": "sqlite3def",
      "version": "3.11.19"
    }
  }
}
```

`database` may instead be `{ "environment": "DATABASE_URL" }`. Adapter
defaults select `sqlite3def`, `psqldef`, or `mysqldef` and the sqldef version
supported by the current TypeRB release. `sqldef.arguments` adds project-owned
command options. See the [database schema guide](guides/database.md).

## Job adapter composition

Projects using `trb/jobs` select an adapter through one typed composition
module:

```jsonc
{
  "jobs": {
    "configuration": "config/jobs"
  }
}
```

The module returns the portable `JobAdapter` contract. The official SQL
adapter keeps database and worker choices outside application Job definitions:

```trb
import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql
import { Duration } from trb/std/time

JOBS_ADAPTER: JobAdapter := SQLAdapter.new(
	dialect: SQLDialect::PostgreSQL,
	source_environment: "JOBS_DATABASE_URL",
	poll_interval: Duration.seconds(1),
)
```

`configuration` is relative to `sourceDir`; a trailing `.trb` is optional.
Normal Job modules import only `trb/jobs`. Importing `trb/jobs/sql` in the
composition module also selects the target-native database dependency. The
initial compiler integration accepts one explicitly typed `JOBS_ADAPTER`
constant initialized directly with `SQLAdapter.new(...)`; duration and scalar
options are compile-time values. The configuration module initializes this
application-scoped adapter once, and all generated enqueue wrappers reuse the
same instance. It is not a factory evaluated for each enqueue.
This keeps native dependency and generated SQL selection deterministic while
the external adapter protocol is still under development.

Derived Job enqueue methods call the configured `JobAdapter` through its
portable `enqueue` and `enqueue_at` methods. Portable generated TypeRB owns
payload serialization and relative scheduling validation; the adapter owns ID
generation, persistence, and native error mapping. Worker lifecycle remains a
separate bundled integration in this alpha contract.

## Project entrypoint

A runnable project defines exactly one top-level `def main()`. `main` is a
language convention rather than a configurable entrypoint, so the config has no
entrypoint field. A library project may omit `main`.

## Local packages

Map a portable source directory into the project import graph with
`localPackages` when maintaining an older source-only workspace:

```jsonc
{
  "localPackages": {
    "acme/contracts": "../../packages/contracts/src"
  }
}
```

An `index.trb` file is the package entry module. Projects in different modes can
import the same package without copying its source. New reusable packages
should instead contain `trbpackage.json` and use a `packages` path requirement,
which exercises the same manifest and dependency graph as a remote package.

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
