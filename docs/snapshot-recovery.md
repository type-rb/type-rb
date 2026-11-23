# Snapshot recovery

The reference compiler supplies a versioned, data-only snapshot for recovery and
differential testing. This path is separate from ordinary file/project compilation
and does not establish ordinary Native coverage for every snapshot feature.

`recovery/src/snapshot/validation.trb` validates the interchange before lowering.
`recovery_scalar_*` owns scalar control flow and checked arithmetic;
`recovery_aggregate_*` adds heap-free records, tagged values and Result propagation;
`recovery_managed_*` handles managed Strings, Arrays, closures and exact-root tracing.
Shared arithmetic and QBE formatting live in `recovery/src/scalar/qbe.trb`. Distinct recovery models
use those shared operations without merging their representation contracts.

The ordinary compiler's recovery input is derived from its canonical source
closure by `recovery/src/compiler/source.trb`. It is temporary, validated and never
substituted for the file-root source used in ordinary self-hosting. Required suites
cover reconstruction, repeated generations, differential behavior and workspace
cleanup; see [validation](ci-validation.md) and [architecture](architecture.md).

Original feature contracts and target observations are preserved in the
[historical documentation](history.md). Current ordinary support is recorded in
[language coverage](native-language-coverage.md), including explicit gaps for
features supported only by the recovery path.
