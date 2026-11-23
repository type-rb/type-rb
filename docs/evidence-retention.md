# Evidence lifecycle and retention

Keep a bounded working set with an explicit purpose, not every past experiment.
Storage cleanup must not change a measurement, hide a failed acceptance decision,
or move the fixed baseline of an experiment that is still running.

## What belongs in main

`results/active.json` is the complete registry. Each dated result occupies one
named slot and states its current purpose and retirement condition. The slots
cover published runtime/build measurements, compatibility, the bootstrap seed,
the amd64 target, persistent memory, the accepted compiler, one candidate, its
comparison baseline, and one runtime investigation. Unused slots stay empty.

When a result supersedes another in the same slot, update consumers and remove
the predecessor directory **in the same PR**. If the old result still has an
independent active purpose, explicitly assign that purpose's available slot.
Do not invent permanent slots for individual dates or implementation attempts.

CI enforces **512 files / 4 MiB across all of results/**, including the registry
and indexes. Every directory must have exactly one slot and an existing README.
Unknown slots, duplicate entries, missing reasons/retirement conditions, orphan
directories and unregistered root files fail. Budget changes require policy
review; a larger artifact import is not sufficient justification.

New results also have the existing **100 files / 2 MiB** limit; new or changed
files must be nonempty text, at most **256 KiB**, without generated executables,
QBE/assembly or stdout/stderr/status sidecars. Consolidate observations and
statuses in tables. Existing oversized per-result inventories may shrink but
not grow; they are **not exempt from the global budget**.

Current published values retain all required observations, statuses, revisions,
measurement contracts and digests, not just favorable medians. Keep any
machine-consumed seed manifests unchanged. Retained reports may link detailed
source-era inventories to an exact Git revision or an existing verified archive.

## Completed and intermediate work

Keep significant conclusions, important numbers and rejected-approach reasons in
the [development history](https://github.com/type-rb/type-rb-native/blob/7726ff18e9230cd149e9f0c317577f6429f907fc/docs/development-history.md) or the relevant decision/PR.
Retain useful regression cases in their maintained test owner. Do not keep a
dated folder merely because it passed a checkpoint, was once a baseline, or is linked
from historical prose: historical links should use the exact archived revision.

During an experiment, keep successful and failed observations until the decision
and review are complete. Afterwards, discard intermediate logs and generated
payloads that have no remaining reproduction, published-evidence or diagnostic
purpose. Routine CI artifacts are temporary; do not promote all of them into
permanent release assets. Archive only evidence with a documented durable use.
Never cherry-pick observations inside a still-active/public measurement cohort.

The [pre-retirement snapshot](https://github.com/type-rb/type-rb-native/tree/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results)
preserves existing historical reports and their original inventories. The
previously published evidence archive and immutable bootstrap release remain
available. No new full archive is necessary just to remove already-versioned,
superseded results from main. This policy does not rewrite Git history; history
still grows through ordinary source changes.

## Safe retirement

1. Identify current Pages, compatibility, seed, comparison and capability
   consumers; assign only results with a concrete current use to active slots.
2. Record significant conclusions/rejection reasons and a pinned historical
   link before removing a superseded record. For new evidence, publish any
   required durable payload before Git import; never commit raw payloads
   temporarily and delete them later.
3. Check exact targets against committed public evidence, update historical
   links and current consumers, and remove only those tracked targets.
4. Run `python3 tools/result_archive.py check BASE HEAD`, archival/lifecycle
   tests, link checks, Pages and compatibility validation. Review and merge
   through a PR. Preserve frozen benchmark contracts and published seed assets.

For evidence that actually needs an archive, the existing `pack`, `verify`
and `compact` commands remain available. `pack` accepts already-committed
historical evidence, not a requirement to commit new raw files first. Publish a
new evidence-only asset without overwriting one, freshly download it and verify
its whole-archive and per-member SHA-256 before removing the archived payload.
Extract verified archives only into a new empty directory.

Lifecycle checks stay in the lightweight documentation CI. Planning-only
maintenance uses unconditional routing tests; compiler/execution changes retain
their normal authority. See [CI validation stages](ci-validation.md).

## Formal measurement bundles

New formal runtime/build jobs upload one `formal-evidence.jsonl.gz` file each,
instead of thousands of small logs. The bundle contains one record per original
file with its relative path, permission bits, byte length, SHA-256 and base64
content. Empty logs, failed observations and binary artifacts remain exact;
missing evidence roots are recorded explicitly. Bundling runs after measurement
and changes no input, repetition, timing, cache control or acceptance rule.
If bundling fails, the job remains failed and uploads its original files.
The explicit evidence roots match the previous uploads. Symlinks and hidden
files are rejected rather than expanding the publication boundary.

After checking the downloaded GitHub artifact digest, inspect or restore a bundle:

```sh
python3 tools/evidence_bundle.py verify formal-evidence.jsonl.gz
python3 tools/evidence_bundle.py extract formal-evidence.jsonl.gz restored-evidence
```

Extraction validates the full inventory and hashes before creating a new output
directory. It preserves the original layout for existing correctness, median and
provenance checks. Storage verification does not imply benchmark acceptance:
failed or incomplete measurements can also have intact bundles.

For future formal publication, retain these verified per-job bundles directly
as evidence-only assets when durable payloads are needed. Record each bundle's
whole-file digest and original-file count, freshly download and verify it, and
keep compact observation tables in Git under the existing limits. A complete
three-case runtime/build cohort needs six payload files. Logical record counts
remain explicit; aggregation does not discard observations or claim to reduce
their information volume. Historical `.tar.gz` assets and their existing
`ARCHIVE.json` references remain immutable and verifiable by `result_archive.py`.
This representation change does not alter the independent daily data format.

This reduces small-file handling; compressed byte size can increase because
binary content uses base64. It is not a measurement-count or storage-byte cut.
