# TypeRB packages

TypeRB uses distributed Git repositories rather than a required central
registry. A package contains TypeRB source and a strict `trbpackage.json`
manifest. Application and package source share the same grammar, type checker,
typed IR, and three backends.

## Install a package

The short name `acme/contracts` defaults to
`github.com/acme/contracts`:

```sh
trb add acme/contracts v1.2.3
trb install
```

Source keeps the short explicit import:

```trb
import { Contract } from acme/contracts
```

Use a full repository name when no abbreviation is wanted, or select another
Git host explicitly:

```sh
trb add github.com/acme/contracts v1.2.3
trb add --source gitlab.com/company/auth company/auth v2.0.0
```

Git credentials and private-repository access remain the responsibility of the
installed Git client. Source URLs containing embedded passwords, query strings,
or fragments are rejected so credentials cannot leak into project config or
the lock. The initial resolver accepts exact semantic-version tags
such as `v1.2.3`, `latest`, and Git revisions. `latest` resolves only during
install or update and is then pinned.

## Package manifest

A repository publishes `trbpackage.json` at its root:

```json
{
  "formatVersion": 1,
  "name": "github.com/acme/contracts",
  "version": "1.2.3",
  "sourceDir": "src",
  "modes": ["go", "ruby", "typescript"],
  "packages": {
    "acme/validation": "v1.0.0"
  },
  "nativeDependencies": {
    "go": {
      "github.com/google/uuid": "v1.6.0"
    },
    "ruby": {
      "uuid7": "0.1.0"
    },
    "typescript": {
      "uuid": "11.1.0"
    }
  }
}
```

`formatVersion`, `name`, and a semantic `version` are required. `sourceDir`
defaults to `src`, and omitted `modes` support all three backends. Dependency
aliases are local to the declaring package. Native dependencies are selected
only for the application's active mode and merge into its generated target
manifest.

External compiler extensions are intentionally unavailable. Packages that
need syntax, type-provider, or code-generation integration must wait for a
versioned and sandboxed extension protocol rather than importing compiler
internals.

## Lock and cache

Commit `trb.lock`. It records direct import mappings, the complete dependency
graph, exact Git commits, and SHA-256 checksums. Remote content is stored by
checksum below `.trb/packages`, which is ignored by Git. Builds, checks, runs,
and the REPL read the lock and verify cached content without network access.

```sh
# Reject a missing or stale lock in CI.
trb install --frozen

# Never contact Git; fail if locked remote content is absent.
trb install --offline

# Re-resolve entries such as latest.
trb update
```

## Local development

Use the same package manifest through an editable path:

```sh
trb add --path ../contracts local/contracts
```

Local paths are recorded relative to the project and source content is not
locked, so code edits are visible immediately. The normalized manifest is
checksummed; run `trb install` after changing its identity, dependencies,
supported modes, or native dependencies. A local package's manifest name
remains its canonical identity; `local/contracts` is only the importing
project's alias.

`localPackages` remains available for older source-only workspaces, but new
reusable packages should use `trbpackage.json` and a path requirement.

The initial compiler still requires public user-defined type names to be
unique across one complete project graph. Namespace-stable type identities are
planned before the package system is considered production-compatible.

## Native dependencies

`dependencies` and `devDependencies` in `trbconfig.jsonc` describe target
language packages rather than TypeRB source packages. Manage them explicitly:

```sh
trb add --native PACKAGE VERSION
trb add --native --dev PACKAGE VERSION
trb remove --native PACKAGE
trb sync
```

Projects with `"packageManagement": "external"` may still resolve TypeRB
packages, but the host project remains responsible for installing all merged
native dependencies.
