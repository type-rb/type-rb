# Lint rule index

Built-in lint rules ship with TypeRB and use the `trb` release version. Every
rule has one stable ID and one documentation page.

| Rule | Default | Recommended | Fix | Since |
| --- | --- | --- | --- | --- |
| [`trb/prefer-conditional-transfer`](prefer-conditional-transfer.md) | warning | yes | safe cases | 0.3.25 |
| [`trb/omit-terminal-void-return`](omit-terminal-void-return.md) | warning | yes | safe cases | 0.3.48 |

Configure rule levels under `lint.rules` in `trbconfig.jsonc`. See the
[linting guide](../guides/linting.md) for presets, command status, fixes, and
versioning policy.
