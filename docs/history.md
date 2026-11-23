# Historical implementation and measurement records

Current architecture, ownership, language coverage and validation live in the
[documentation index](README.md). Completed experiment narratives and early
decisions are available in the immutable
[pre-consolidation documentation](https://github.com/type-rb/type-rb-native/tree/7726ff18e9230cd149e9f0c317577f6429f907fc/docs).
That snapshot includes the original commands, source names, links, assumptions,
failed observations and acceptance contracts. Use the exact source revision
recorded by an experiment when reproducing it.

The historical documentation is evidence of its own checkpoint, not a second
current implementation or an instruction to rerun retired controllers against
current source. Later policies explicitly supersede earlier scheduling and cost
rules; see [MIR consolidation](mir-consolidation.md), [CI validation](ci-validation.md)
and [seed handoffs](bootstrap-seed-updates.md).

A second consolidation removed per-feature coverage narratives, per-update
reference notes, accepted-slice reports and hand-maintained inventories from the
current documents. Their last versions are in the
[pre-consolidation documentation of 2026-09-24](https://github.com/type-rb/type-rb-native/tree/04c6ca7263c066fd13e83b3faa09c4e80d707c13/docs).
Current documents describe the present contract; PRs and issues record how each
change was delivered.

Registered `results/`, immutable release assets and their identities are unchanged.
The [retention policy](evidence-retention.md) continues to govern measured data.
