# Linting

TypeRB keeps correctness, canonical formatting, and maintainability checks as
three explicit surfaces:

| Command | Owns | Configuration |
| --- | --- | --- |
| `trb check` | Syntax, resolution, types, and portable semantics | Not suppressible |
| `trb fmt` | Deterministic source layout | Intentionally minimal |
| `trb lint` | Optional style and maintainability guidance | Presets and per-rule levels |

`trb lint` runs the complete `trb check` pipeline first. Lint rules never make
invalid source valid and cannot hide compiler diagnostics. Once the project is
valid, lint analyzes authored project files and excludes compiler-owned,
official-package, and external-package source.

## Running lint

```sh
trb lint
trb lint --fix
trb lint --deny-warnings
trb lint --diagnostic-format json
```

The default `recommended` preset reports warnings without failing the command.
`--deny-warnings` is the CI switch for treating every remaining warning as a
failure. A rule explicitly configured as `error` always fails. `--fix` applies
only edits the rule declares safe, preserves file permissions, and reports any
remaining findings afterward.

Standalone operation follows the same file-root model as `trb check`:

```sh
trb lint app.trb
trb lint --mode typescript app.trb
```

When `trbconfig.jsonc` is discoverable, the configured project and mode win.

## Configuration

```jsonc
{
  "lint": {
    "preset": "recommended",
    "rules": {
      "trb/prefer-conditional-transfer": "error"
    }
  }
}
```

The available presets are `recommended` and `none`. Rule levels are `off`,
`warning`, and `error`. Unknown IDs are errors so a typo or removed rule cannot
silently weaken CI. The [rule index](../lint-rules/index.md) records each rule's
default, first TypeRB release, fix support, and full rationale.

## Versions and compatibility

Built-in rules are part of TypeRB and share the `trb` version. Patch releases
may fix false positives, crashes, messages, or unsafe fixes without intending
to add new findings. New rules, a broader rule scope, or a change to the
recommended preset belongs to a minor release.

JSON output uses the shared diagnostic envelope. `schemaVersion` versions the
report shape, `toolVersion` identifies the running TypeRB release, and a lint
diagnostic's `code` is its rule ID. This separation lets integrations track a
stable protocol without inventing a second built-in-linter SemVer.

## Extension boundary

The initial linter intentionally contains only a built-in registry and
project configuration. It does not execute third-party rules during checking
or compilation. A future external rule protocol must define versioning,
resource limits, package trust, deterministic inputs, and editor behavior
before it becomes public. Independent tools can consume
`trb compiler inspect` today, but that tooling protocol does not grant a rule
access to compiler internals or raw mutable AST state.
