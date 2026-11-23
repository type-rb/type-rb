# Loop-local header runtime results on Linux arm64

All 462 retained observations and 84 warmups pass. Native retains its lead over
the pinned Pure Go spectral-norm implementation: one-core wall medians are
2.25684 versus 3.21985 seconds,
29.91% less time. N-body and fannkuch recover the earlier published
regressions and improve further, but remain slower than Pure Go. These three
programs do not establish general language parity.

## Exact scope and bootstrap

[Formal run 34431896353](https://github.com/type-rb/type-rb-native/actions/runs/34431896353),
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

## Runtime medians

Upstream Benchmarks Game revision is
`40296663ed350d5fe4a6ab5e367bab61cb77c219`. Inputs are fannkuch-redux 12,
n-body 50000000 and spectral-norm 5500. Seven implementations rotate through
two warmup and eleven retained rounds in each of the one-core and four-core
lanes. Complete fresh processes include startup and Java JIT activity.
Swap is disabled; cache control precedes each observation. Every process tree
has a 4 GB memory ceiling and fixed lane CPU allocation.

| Case | Lane | Native wall | TypeRB Go wall | Pure Go wall | Native/Go |
| --- | --- | --- | --- | --- | --- |
| fannkuch-redux | one-core | 61.8302 | 53.4575 | 24.6204 | 2.5113x |
| n-body | one-core | 7.87414 | 7.54048 | 3.59973 | 2.1874x |
| spectral-norm | one-core | 2.25684 | 5.63669 | 3.21985 | 0.7009x |
| fannkuch-redux | four-core | 61.8198 | 53.8846 | 24.653 | 2.5076x |
| n-body | four-core | 7.87101 | 7.53896 | 3.59885 | 2.1871x |
| spectral-norm | four-core | 2.2566 | 5.63765 | 3.21972 | 0.7009x |

| Case | Lane | Native CPU | Pure Go CPU | Native memory | Pure Go memory |
| --- | --- | --- | --- | --- | --- |
| fannkuch-redux | one-core | 61.8258 | 24.6114 | 524288 | 2637824 |
| n-body | one-core | 7.8713 | 3.59351 | 524288 | 2899968 |
| spectral-norm | one-core | 2.25478 | 3.21286 | 1478656 | 4173824 |
| fannkuch-redux | four-core | 61.8193 | 24.7081 | 524288 | 3502080 |
| n-body | four-core | 7.86875 | 3.60331 | 524288 | 3694592 |
| spectral-norm | four-core | 2.25469 | 3.22165 | 1486848 | 4984832 |

All times are median seconds and memory values bytes. The seven implementations
remain individual context rows, not a composite language score. Only the two
TypeRB backends hold authored source and portable semantics constant. Four-core
allocation does not parallelize single-threaded programs; selected upstream
implementations do use multiple cores. Keep the lanes separate.

## Regression recovery and attribution

| Case | Pre-regression published | Previous published | Current | Less time than previous | Less time than pre-regression |
| --- | --- | --- | --- | --- | --- |
| n-body | 10.2644 | 26.2869 | 7.87414 | 70.05% | 23.29% |
| fannkuch-redux | 77.1176 | 202.325 | 61.8302 | 69.44% | 19.82% |

These publication-date comparisons expose the recovered behavior but are not
controlled causal A/B measurements. Compiler revisions and hosts differ even
within the same named runner class. The assignment-effect repair restored the
older programs; subsequent MIR Array-header work improves the accepted baseline.
The [accepted checkpoint](../2026-09-10-loop-local-headers-accepted-darwin-linux-arm64/README.md)
records the separate controlled local comparison: the loop-local slice uses
11.67% less fannkuch wall time than its immediate control and preserves n-body
and spectral within their registered limits. No local timing replaces these
formal observations, and the local fannkuch input is 10 rather than 12.

Independent checks verify all 546 rows, rotating order, original process
metrics, exit status, cache controls and exact output, including untimed
controls. The canonical summarizer reproduces all six median tables byte for
byte. No earlier compiler result or favorable subset is mixed into this cohort.

## Retained evidence

| Artifact | ID | ZIP bytes | GitHub SHA-256 |
| --- | --- | --- | --- |
| benchmarksgame-runtime-fannkuch-redux | 10137349942 | 393058 | `e984ba60313c1d6980ddca77031cce6ef316b3501aac36523b613ff1012b75a7` |
| benchmarksgame-runtime-n-body | 10135254844 | 388739 | `a722a0ce5c1b886f1856da98cd34fb76bf1266f69d50fb45482f0b3248c528c3` |
| benchmarksgame-runtime-spectral-norm | 10135068350 | 386780 | `c34fb28a48748596f0cd3408c069036971cc46909c2a44ad18fc5c260156e11c` |

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

See the [same-revision build cohort](../2026-09-10-benchmarksgame-build-loop-local-headers-linux-arm64/README.md) for compiler costs and distributions.
