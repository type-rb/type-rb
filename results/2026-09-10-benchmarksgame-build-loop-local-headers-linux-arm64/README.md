# Loop-local header build results on Linux arm64

All 66 retained builds and 12 warmups pass. Native uses
38.2% to 39.7% of TypeRB Go's build wall time and
16.0% to 16.5% of its compiler CPU time for these three
identical authored TypeRB sources. This compares backends, not compilation of
handwritten Pure Go programs or application execution speed.

## Exact scope and bootstrap

[Formal run 34431898588](https://github.com/type-rb/type-rb-native/actions/runs/34431898588),
attempt 1, measures accepted Native source `8980b5978f0e8519964d200b7f7799b741796478` from
[PR #389](https://github.com/type-rb/type-rb-native/pull/389). All three jobs
succeed. Each case uses a fresh Ubuntu 24.04 Linux arm64 runner with four
Neoverse-N2 logical CPUs. The reference is
`0.4.6-dev@6cbd4025545d44a1de211335f9197772077bb478`.
QBE 1.3, GCC 13.3.0, LLD 18.1.3, Go 1.27.1 and BenchExec 3.35 remain pinned;
per-case `context.json` retains the complete tool and host inventories.

The immutable `bootstrap-seed-2026-09-09-record-arrays` supplies setup provenance.
After the setup-only transition, ordinary B2/B3/B4 generations reproduce the
same 332440-byte compiler, SHA-256
`d5a68818d61afded4a976b1f86e953507b7c85b37882d0db492f60cef737fe61`.
The 1195739-byte target-neutral compiler QBE has SHA-256
`ab9c8729308823f7c12e26c3616587032569e19e8eddc2e710b59cd1cf35de7a`.
All 42 generation observations for these three jobs pass. The reference Go
compiler is a comparison control, not an ordinary regeneration dependency.

## Measurements

Times are median seconds; memory and artifacts are bytes. Both backends rotate
through two warmup and eleven retained rounds with warm Go compiler caches,
clean outputs, cache reset, four fixed CPUs, a 4 GB process-tree memory ceiling
and process isolation. Output checks use fannkuch-redux 7, n-body 1000 and
spectral-norm 100. Swap remains disabled.

| Case | Native wall | TypeRB Go wall | Native CPU | TypeRB Go CPU | Native memory | TypeRB Go memory |
| --- | --- | --- | --- | --- | --- | --- |
| fannkuch-redux | 0.151237 | 0.393193 | 0.054545 | 0.331042 | 78381056 | 160550912 |
| n-body | 0.181254 | 0.474386 | 0.069446 | 0.434182 | 78598144 | 162955264 |
| spectral-norm | 0.147503 | 0.371779 | 0.052581 | 0.326499 | 78049280 | 160047104 |

All 78 raw rows agree with original process metrics, successful status,
empty diagnostics and exact output. The canonical summarizer reproduces every
median table byte for byte. Warmups remain in the raw tables but not medians.
Cross-date changes do not isolate an optimization's effect.

| Case | Native raw median | TypeRB Go raw median | Native stripped | TypeRB Go stripped |
| --- | --- | --- | --- | --- |
| fannkuch-redux | 18648 | 2810092 | 18640 | 1887832 |
| n-body | 21920 | 2817378 | 21912 | 1887832 |
| spectral-norm | 19552 | 2810658 | 19544 | 1887832 |

Each Native case has one byte-identical program across its eleven retained
builds; each TypeRB Go case has eleven correct variants, all counted. All twelve
archived representative raw/stripped programs match their recorded size and hash.
These application sizes are distinct from complete compiler distributions.

## Distribution boundary

| Controlled payload | Form | Bytes |
| --- | --- | --- |
| native-controlled | raw | 1047080 |
| native-controlled | stripped | 679664 |
| go-controlled | raw | 276002107 |
| go-controlled | stripped-compiler | 265589917 |

Native-controlled includes the compiler plus QBE. Go-controlled includes the
reference compiler plus the complete Go root; its alternate form strips only
the reference compiler. Native's C driver, assembler, linker, dynamic loader
and shared libraries remain explicit platform prerequisites. Process and
dependency inventories retain their roles. These payloads do not include a
complete operating-system or SDK distribution.

## Retained evidence

| Artifact | ID | ZIP bytes | GitHub SHA-256 |
| --- | --- | --- | --- |
| benchmarksgame-build-fannkuch-redux | 10134842338 | 2507566 | `01699f624f7936da5ae9d8352a9d3cfe6646541c231a8b1dffa8a810fa70bb62` |
| benchmarksgame-build-n-body | 10134843520 | 2524135 | `ad00052e9d8517159afa9744a661a053d087cddda8afdc5972b0e427943c2a73` |
| benchmarksgame-build-spectral-norm | 10134837311 | 2508058 | `b23590d16df58ba7359b41235f16227e8fa11aa5e4107eb8de98c4ea5ce00fe7` |

Each downloaded ZIP matches its GitHub size and digest. Complete raw tables,
statuses, source/artifact identities, contracts and applicable distribution
tables remain in Git. Text inventories are consolidated into per-case
`context.json` without changing their contents. `EVIDENCE_SHA256SUMS`
inventories the retained machine-readable working set.

[ARCHIVE.json](ARCHIVE.json) identifies the freshly downloaded and verified
public archive with every original extracted file, including original process
metrics/output/status, cache controls, traces and application artifacts.
Its manifest verifies every member's bytes and SHA-256. The archive is built
directly from the CI artifacts before Git import; `SOURCE.json` records that
provenance. No raw payload is temporarily committed, no observation is filtered,
and no earlier/partial cohort is substituted. Superseded reports remain at the
[exact pre-retirement revision](https://github.com/type-rb/type-rb-native/tree/8980b5978f0e8519964d200b7f7799b741796478/results).

See the [same-revision runtime cohort](../2026-09-10-benchmarksgame-runtime-loop-local-headers-linux-arm64/README.md) for execution.
