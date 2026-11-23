# TypeRB Native capability map

This directory contains a dependency-free static capability map for the
TypeRB Native toolchain under development toward production use. It catalogs
capabilities before they become scheduled implementation work.

The same GitHub Pages artifact also contains a
[benchmark explorer](benchmarks/README.md). Its committed data is generated
from the formal runtime and build median TSV files under `results/`.

The [dependency register](dependencies/) is generated from
`docs/native-dependencies.json`, `tools/network-dependencies.json` and the QBE
manifest. It distinguishes dependencies in the current source from candidates,
and separates application runtime, system data, build and validation needs.
Update the records in the implementing PR, run
`python3 tools/dependency-pages.py --write`, and verify `--check` before publishing.
The deployment workflow checks the same generated view. Dependency qualification
still requires actual artifact imports, versions/configuration and license
receipts; this page does not replace per-artifact inventories.

The four evidence states have narrow meanings:

- `verified`: reproducible public evidence exists for the stated scope;
- `partial`: some behavior exists, but coverage or evidence remains incomplete;
- `open`: the capability is needed but not implemented or verified; and
- `unassessed`: the capability is cataloged, but its exact gap has not been
  inventoried.

These states report current implementation evidence. They do not establish
release support or compatibility guarantees; production readiness and full
coverage remain [project goals](../mir-consolidation.md). Public evidence links
point only to this repository.
The stated execution path is part of the scope: snapshot/recovery verification
must not be presented as ordinary `trbn` check, execution or REPL support.
See the [ordinary coverage plan](../native-language-coverage.md) for that separate
feature-level boundary.

The ordinary-language panel is generated from the same reviewed case registry
that the pinned reference and Native CLI jobs execute. It displays check, build,
execution and REPL separately, including negative cases, reference REPL
limitations and explicitly untested contracts. Its probe counts are not a
language-support percentage. The broad catalog above and these concrete cases
are complementary views; neither implies that a whole feature family passes
because one example works.

## Maintenance boundary

This catalog is the public, project-wide coverage view. It is not a task
tracker, roadmap, changelog, or record of work in progress.

Update `catalog.js` when a merged public result changes at least one
capability's status, scope, description, or best evidence link. Set
`catalog.updatedAt` to the date of that coverage review. A `verified` entry
must link to reproducible evidence already present in this public repository;
`partial` may omit a link when the remaining coverage is described accurately.
When ordinary behavior changes, review the relevant broad catalog entries as
well as the case expectations. Regenerate `ordinary-language.js` using the
commands in the ordinary coverage plan; never edit that data by hand. Its
registry hash, reference revision and exact generated contents are checked in
CI, so changing the contract without updating the public detail view fails.

Do not add an entry for every implementation or optimization. If a result
improves an already represented capability without changing its catalog state,
leave the catalog unchanged. Formal performance snapshots belong in the
benchmark explorer and should be regenerated only from complete committed
result sets. Planning, prioritization, and current task state remain outside
both public views.

Run the structural check from the repository root:

```sh
python3 tools/native-language-coverage.py --check-pages-data docs/capabilities/ordinary-language.js
node tools/capability-map-check.mjs
node tools/benchmark-pages-data.mjs --check
node tools/benchmark-pages-check.mjs
```

For local review, serve `docs/capabilities` with any static HTTP server. The
site has no package-manager or build dependency.
