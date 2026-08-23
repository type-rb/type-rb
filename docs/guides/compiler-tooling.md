# Compiler tooling protocol

`trb compiler inspect` exposes one checked project snapshot as versioned JSON.
It is intended for explicit tools, editor adapters, static analysis, and AI
agents that need compiler facts without importing TypeRB's internal Go
packages.

```sh
# Inspect the configured project discovered from the current directory.
trb compiler inspect

# Select a configuration explicitly.
trb compiler inspect --config trbconfig.typescript.jsonc

# Inspect one config-free file and its explicit local import closure.
trb compiler inspect --mode ruby app.trb
```

The command writes only the JSON report to standard output. It returns a
nonzero status when the report contains errors, while still returning the
available sources, modules, and diagnostics. Command-line usage errors remain
ordinary CLI errors on standard error.

## Version 2 snapshot

Every report contains these top-level fields:

| Field | Meaning |
| --- | --- |
| `protocolVersion` | Integer version of this JSON contract. Version 2 adds authored `newtype` declarations. |
| `compilerVersion` | Exact TypeRB compiler build that produced the snapshot. |
| `mode` | The configured or standalone `go`, `ruby`, or `typescript` mode. |
| `sources` | Exact source paths and contents analyzed by the compiler service. `encoding` is `utf-8` or `base64`; `content` uses that encoding. |
| `modules` | Module identities, source ownership flags, and authored imports. |
| `declarations` | Flattened, checked authored declarations with stable IDs, ownership, locations, and semantic types. |
| `diagnostics` | Stable diagnostic codes, messages, locations, related information, and fixes. |
| `summary` | Error and warning counts. |

Locations use zero-based UTF-8 byte offsets and one-based lines and columns,
matching `trb check --diagnostic-format json`. Source and diagnostic paths are
absolute so an invoking tool can open them without depending on its current
directory.

Declaration entries include public and private authored declarations. Nested
members refer to their owner through `ownerId`; `classMember` distinguishes a
class method from an instance method. Semantic types use a recursive
`kind`/`name`/`arguments` representation and retain nullability and readonly
facts. A `newtype` declaration has declaration kind `newtype` and exposes its
concrete representation through `type`; a transparent alias remains
`type_alias`. Declaration IDs are compiler-owned opaque identifiers within the
protocol: consumers should store and compare them, not parse their spelling.

Compiler-generated TypeRB helpers and implicit runtime imports are omitted.
They are implementation details rather than authored declarations. When a
project cannot produce checked artifacts, `sources`, `modules`, and
`diagnostics` remain available but `declarations` is empty.

## Version policy and scope

Consumers must check `protocolVersion` before interpreting a report and should
record `compilerVersion` when snapshots are persisted. An incompatible JSON
shape receives a new protocol version. TypeRB is currently alpha, so tooling
that requires reproducible output should pin the TypeRB compiler version even
when the protocol version is unchanged.

Version 2 remains a one-shot, read-only CLI boundary. It does not
expose mutable syntax trees, typed IR objects, backend hooks, target source,
an embedded Go SDK, or a long-lived JSON-RPC server. It is also separate from
the package extension protocol: importing a package never runs a compiler
tooling client.

The report contains complete application and package source text. Treat it as
source code when storing logs, attaching CI artifacts, or sending it to another
service.
