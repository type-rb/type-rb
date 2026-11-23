# Accepted loop-local Array headers

[PR #389](https://github.com/type-rb/type-rb-native/pull/389) retains ordered
Array effects and proves header stability for individual while regions in MIR.
A loop can reuse a header even when an unrelated opaque effect prevents the
function-wide proof. The adapter consumes the verified placement and restores
its header scope on exit. Bounds checks, negative-index normalization,
evaluation order and owner roots remain intact. No benchmark names, inputs or
source-text recognition occur in the compiler.

The accepted source is `8980b5978f0e8519964d200b7f7799b741796478`.
Its tree `39433c4030b974548f233cf2e64c086ce336845f` is identical to the
reviewed candidate `b230d34c1a415eab9f8eaa98c7ed418b57b5bafa` and tested merge
`db0310c8f381ecf3957afc76d31be2ce167cd065`. The formal runtime/build cohorts
measure the accepted source itself; they are distinct from the local diagnostic.

## Correctness and compiler costs

All 17 authorities pass in [CI run 34423530952](https://github.com/type-rb/type-rb-native/actions/runs/34423530952),
attempt 1. These include recovery, ordinary fixed points, CLI/REPL and terminal
controls, reference/language checks, Linux arm64/amd64, target-neutral QBE,
persistent-worker memory and both platform compiler comparisons.
`ci-authorities.json` retains the exact outcomes. Local recovery-enabled suites
also pass 103 root and 172 compiler tests and all 13 recovery stages, followed
by owned workspace cleanup. Test-suite durations are not performance evidence.

The root QBE recovery path now resolves a block parameter once and redirects
all operand aliases and roots to its replacement, iterating to convergence.
A generic managed-String backedge test covers the previous undefined-operand
failure. Ordered region tests cover safe nested loops, opaque effects,
branch-local bindings and malformed plans. The shared lexical resolver and
invalid-plan constructor remove duplicated implementation work.

| Compiler cost | Darwin control | Darwin candidate | Linux control | Linux candidate |
| --- | ---: | ---: | ---: | ---: |
| Executable bytes | 365800 | 365816 | 330384 | 332440 |
| Text bytes | 263348 | 265248 | 266032 | 268064 |
| Self-build wall median, seconds | 1.895 | 1.930 | 1.420 | 1.420 |
| Self-build peak RSS median, bytes | 41320448 | 41353216 | 64737280 | 64708608 |

All ordinary 1.05 relative limits and per-observation catastrophic bounds pass.
The two interleaved comparisons retain all 72 observations: 16 warmups and 56
retained rows. The combined executable size is 698256 bytes, 2072 bytes above
the immediate control. [PR #390](https://github.com/type-rb/type-rb-native/pull/390)
and [Decision 0032](../../docs/decisions/0032-loop-local-header-compiler-budget.md)
independently set Darwin/Linux/combined ceilings to 366000/334000/700000 bytes.
Build/RSS, application, QBE and other acceptance authorities remain unchanged.
The compiler QBE is 1195739 bytes, SHA-256
`ab9c8729308823f7c12e26c3616587032569e19e8eddc2e710b59cd1cf35de7a`.
Darwin compiler SHA-256 is
`3ef84618311efbb580c7fddbea1f405358e9c820dc59f47f501ea017a7d99c4f`;
Linux is `d5a68818d61afded4a976b1f86e953507b7c85b37882d0db492f60cef737fe61`.

The compact CI inventories retain source identities, original interleaved rows,
limits and comparison results. Routine CI payloads are not promoted to a new
permanent archive. Formal published observations have their own durable archive.

## Local application comparison

The registered Apple M2 Pro / macOS 26.6.2 diagnostic uses 13 rotating rounds,
two warmups and eleven retained per role/case: all 117 observations are retained
in `local-runtime-raw.json`. The observer records fresh-process monotonic wall,
wait4 CPU and peak RSS on an uncontrolled host, with no affinity, cache
manipulation or observation filtering. This is not a Pure Go comparison.

| Case and input | Control wall | Candidate wall | Candidate/control wall | Candidate/control CPU | Candidate/cumulative wall |
| --- | ---: | ---: | ---: | ---: | ---: |
| n-body 50000000 | 3.677527250 | 3.677012125 | 0.999860 | 1.000783 | 0.729410 |
| fannkuch-redux 10 | 0.349091125 | 0.308360042 | 0.883322 | 0.881077 | 0.883123 |
| spectral-norm 5500 | 1.620519000 | 1.620372042 | 0.999909 | 1.000183 | 0.730016 |

Fannkuch uses 11.67% less wall time than the same-feature control. N-body and
spectral stay within their registered control limits; the recovered cumulative
improvements remain intact. All status, exact output, CPU, RSS and catastrophic
checks pass. The same-feature source is `4be54adfdf01dae89675982fa64da333526d0af4` (see the public registration
and exact compiler digests in `local-runtime-context.json`); the frozen
cumulative source remains `ac633935a7f248470c59d22666da14c819a131fa`.

The measured candidate `4ad461eb0d21b73179bacff5345eff1f4c1cedf6` predates
constructor-cost refinement. All three application QBE files and executables
remain byte-identical after that refinement; the compiler identity changes are
separate. No replacement runtime observations were substituted.
The [complete registered result](https://github.com/type-rb/type-rb-native/issues/388#issuecomment-5610946529)
retains exact controls, commands, identities and all observations.

The independent [application build cohort](https://github.com/type-rb/type-rb-native/issues/388#issuecomment-5610946774)
retains all 45 observations (18 warmups, 27 retained), separately from compiler
self-build and program execution. Candidate/control wall ratios are 0.985405
for n-body, 0.974585 for fannkuch and 1.021487 for spectral; CPU/RSS limits and
all outputs pass. Application sizes are unchanged. Its complete local context,
raw rows and summary are retained alongside the runtime records.

## Rejected attempts and eventual acceptance

Earlier region tests expected the superseded sticky-effect stream; the tests
were corrected to check ordered effects. An initial snapshot Boolean comparison
was replaced by recovery-supported operations. Root recovery then exposed an
undefined operand; the first alias-anchor repair still failed. The final
all-alias redirect above resolves it. Failed suites are not passing evidence.

[CI 34421338987](https://github.com/type-rb/type-rb-native/actions/runs/34421338987)
rejected the first 333776-byte Linux compiler against the 331000-byte ceiling.
The shared-constructor refinement reduced it to 332440 bytes, still rejected
in [run 34422165569](https://github.com/type-rb/type-rb-native/actions/runs/34422165569).
The separate budget review and fresh full feature CI establish acceptance;
the older failures and skipped dependencies are not relabelled as passing.

The budget PR's first two Darwin comparison attempts stopped on GitHub seed
attestation HTTP 503/502 before any measurement. Cross-target/aggregate failures
followed the missing artifact. The unchanged-head failed-job retry passed on
attempt 3; [the complete result](https://github.com/type-rb/type-rb-native/pull/390#issuecomment-5611990031)
retains those failures and the accepted outcomes. These API errors did not
occur in the subsequently registered formal cohort.

## Published checkpoint and next organization boundary

The [formal runtime cohort](../2026-09-10-benchmarksgame-runtime-loop-local-headers-linux-arm64/README.md)
and [formal build cohort](../2026-09-10-benchmarksgame-build-loop-local-headers-linux-arm64/README.md)
retain the complete accepted-source schedules. The preceding compiler report,
including the original scalar-guard regressions and subsequent assignment
recovery, remains at its [exact historical revision](https://github.com/type-rb/type-rb-native/blob/8980b5978f0e8519964d200b7f7799b741796478/results/2026-09-09-scalar-range-guards-accepted-darwin-linux-arm64/README.md).

This checkpoint removes duplicated lexical resolution and invalid-plan
construction. Loop placement still uses three Integer fields per entry; a
named placement carrier is the next bounded organization candidate. That
source change requires its own recovery/fixed-point and cost gate. It is
outside this frozen publication cohort and is not claimed as completed.
