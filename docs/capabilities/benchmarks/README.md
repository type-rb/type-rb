# TypeRB Native performance pages

The default benchmark page shows daily performance. `trends.html` shows up to
60 snapshots from the same measurement series. `comparison.html` preserves the
complete dated Benchmarks Game runtime and build comparison.

## Daily updates

`daily-performance.yml` checks main once daily and measures only when relevant
inputs changed; manual dispatch can force a refresh. It is independent of PR
acceptance. The Pages workflow reads the verified main-branch daily state
artifact and generates `daily-state.json` only in the deployment workspace.
The committed JSON is an explicit empty state, never invented measurements.
Source changes use PRs; data updates require no generated-data commits.

See [the strategy and measurement policy](../../daily-performance.md) for
coverage, failure states, time budgets, comparisons and evidence retention.

## Detailed comparison

Generate and check the committed formal comparison from the repository root:

```sh
node tools/benchmark-pages-data.mjs
node tools/benchmark-pages-data.mjs --check
node tools/benchmark-pages-check.mjs
python3 -m unittest discover -s tools/daily-performance -p 'test_*.py'
```

Refresh the detailed comparison only from a complete committed formal result
set that passes its registered correctness and measurement boundaries. Update
result roots and metadata in `tools/benchmark-pages-data.mjs`, regenerate
`data.js`, and review the detailed page copy. Focused or daily results never
replace individual rows in this cross-language snapshot.

Daily data can update without another formal run. The detailed comparison has
no mandatory calendar refresh: rerun it when a concrete cross-language or
large-input question warrants the cost. Historical evidence stays immutable.

`weekly.html` shows the latest small-input Native/Pure Go/C/C++/Rust/Java
comparison. `weekly-state.json` is composed from the independent weekly workflow
artifact; source commits retain an explicit empty placeholder. It updates on
changed-input weeks or manual dispatch, independently of daily measurements.
