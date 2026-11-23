# 0038: Retain the measured declaration-index memory trade

Status: trade-off assessment accepted through [PR #427](https://github.com/type-rb/type-rb-native/pull/427).
Implementation accepted through [PR #418](https://github.com/type-rb/type-rb-native/pull/418)
after its [complete final CI](https://github.com/type-rb/type-rb-native/actions/runs/34630185576).
The original diagnostic RSS failure and the assessed trade below remain unchanged.

## Problem and scope

Function declaration validation compares each declaration with every preceding
function in its module, even when all names are unique. The existing function
lookup index serves a different purpose and must preserve last-match lookup.
The bounded replacement uses a transient `Hash<String, Integer>` of names already
visited in the current module, reuses stored name Strings, and keeps a constant
membership marker. Module boundaries reset that map. Declaration vectors,
first-diagnostic order and the persistent function index remain unchanged.

This is ordinary declaration processing, with no source-size special case,
benchmark recognition, new MIR fact, emitter analysis or Hash representation.
The candidate is `dbe2fc2f1ad7bddd00dfc4b86b1f542e68db29ee`, compiler tree
`bb47cf20daf6a3c2903f75098021ab163ad72638`. Its same-feature control is
`e903949c554296d3813c232da2d10e05490fe6a3`, tree
`96c279672a6ab0dd4279643ccd439661cd05c907`.

## Evidence and failed diagnostic condition

The [first registered cohort](https://github.com/type-rb/type-rb-native/issues/414#issuecomment-5636385621)
reduced 6,000-function QBE emission by 32.40%, but raised peak RSS by 7.15% and
exceeded the Linux executable cap by 48 bytes. One preregistered refinement
removed composite-key allocation and unused stored IDs. It resolved the size
failure but did not materially lower the scale case's peak RSS.

The [final cohort and all 90 observations](https://github.com/type-rb/type-rb-native/issues/414#issuecomment-5636699141)
retain two warmup pairs and seven alternating retained pairs per workload.
Each n-body observation averages eight builds; the cohort contains 216 commands.
Both cohorts' original 5% scale-RSS conditions fail and remain failed.

| Final local Darwin workload | Control wall | Candidate wall | Wall ratio | CPU ratio | RSS ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Same-feature, each compiling its own source | 1.929674333 s | 1.923237875 s | 0.996664 | 1.005319 | 1.000000 |
| Identical e903 source | 1.927290500 s | 1.908196708 s | 0.990093 | 0.994709 | 1.002018 |
| Accepted-main and candidate, each own source | 1.887287458 s | 1.914297084 s | 1.014311 | 1.016216 | 0.999595 |
| 6,000-function QBE emission | 1.389440541 s | 0.943544375 s | 0.679082 | 0.674074 | 1.070516 |
| n-body build | 0.140622469 s | 0.140650365 s | 1.000198 | 0.989130 | 0.997124 |

The scale case rises from 34,619,392 to 37,060,608 bytes of peak RSS:
2,441,216 additional bytes for about 32.09% less elapsed time. This measures
process peak RSS, not a leak or retained heap size. The normal compiler and
application controls remain within 5%; all output identities and registered
variation checks pass. No general application runtime gain is measured.

## Decision and remaining authorities

Retain this exact large-case time/space trade if the implementation passes all
ordinary acceptance authorities. A transient linear-space map replacing a
quadratic scan is useful for growing source modules; approximately 2.44 MB of
extra peak RSS in this registered scale case is justified by the measured time
reduction and unchanged representative controls. This is an explicit reassessment
of the failed diagnostic retention condition, not a passing interpretation of
either original cohort. The refinement sequence is finished; no further run,
representation sweep or broader memory allowance is registered here.

The decision grants no CI exception. Keep the existing ordinary 1.05 cost
contracts, absolute compiler ceilings, target/process/corpus requirements and
catastrophic bound unchanged. There is no source selector or enforcement change.
The [ordinary cost preflight](https://github.com/type-rb/type-rb-native/actions/runs/34613777947)
and [target preflight](https://github.com/type-rb/type-rb-native/actions/runs/34613781049)
pass; full exact-head parent and recovery PR validation is still required.
Local recovery passes 107 root and 200 compiler tests, alongside actual diagnostic,
module-scope, scale-output and application identity controls. A later ordinary
cost failure still requires reassessment and cannot use this decision as a bypass.

This reassessment applies only to the recorded candidate/control and scale
workload. It neither predicts another platform's scale RSS nor grants the next
index or workload another 7% increase. Future changes retain the ordinary cost
policy and account for this retained memory cost in their same-feature control.
[Issue #414](https://github.com/type-rb/type-rb-native/issues/414) remains open for
the other lookup/scaling questions; no additional lookup is converted here.

The compiler remains 415,480 Darwin / 387,936 Linux arm64 bytes, equal to the
pre-index e903 implementation. Their 803,416-byte total is 21.27% above the frozen
`ac633935a7f248470c59d22666da14c819a131fa` baseline of 662,488 bytes; the baseline
does not move. Local core/CLI/QBE remain 415,480 / 565,480 / 406,656 bytes, excluding
the platform linker and system libraries. These numbers establish no current
Go headroom or Pure Go parity. No seed publication or Pages result follows from
this decision alone.
