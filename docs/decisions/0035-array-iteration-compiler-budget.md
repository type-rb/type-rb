# 0035: Compiler budget for checked Array iteration

Status: accepted through [PR #420](https://github.com/type-rb/type-rb-native/pull/420).

## Capability and observed cost

[PR #418](https://github.com/type-rb/type-rb-native/pull/418) adds ordinary
Array `each` and `each.with_index` through checked `MirIteration` plans, including
live mutation, retained receivers, managed-element roots and nested loop
transfers. It shares sparse origin lookup with Hash and Array assignment and
replaces source-scanning parameter reassignment boundaries with checked function
boundaries. This is basic language capability, not a benchmark-specific
optimization. Compiler adoption of authored iteration remains a later slice
with separate recovery and immutable-seed prerequisites in issue #410.

The implementation is `185a0a122feae889cd7a17a1efc846e8b33afb06`, based on
accepted main `c1a3c2d2c05517dbed094d07ed351e53b010d258`. The control cohort is
[the completed PR #411 run](https://github.com/type-rb/type-rb-native/actions/runs/34577478494)
(head `3c4fdf9d838a91d55a9bb5fc0c1269d7dfa51c2d`); its compiler source is
unchanged by the merge. The [initial candidate run](https://github.com/type-rb/type-rb-native/actions/runs/34581808596)
retains these measurements and the failed absolute-size checks:

| Complete compiler | Control | Candidate | Increase | Old ceiling | Proposed ceiling |
| --- | ---: | ---: | ---: | ---: | ---: |
| Darwin arm64 worker | 398,944 | 415,472 | 16,528 (4.14%) | 400,000 | 417,000 |
| Linux arm64 fixed point | 369,752 | 387,128 | 17,376 (4.70%) | 370,000 | 388,000 |
| Sum of largest arm64 artifacts | 768,696 | 802,600 | 33,904 (4.41%) | 770,000 | 805,000 |
| Linux amd64 fixed point | 316,096 | 330,560 | 14,464 (4.58%) | 316,500 | 331,000 |

Worker and ordinary fixed-point artifacts are distinct. The Linux arm64 worker
is 387,120 bytes; the Darwin ordinary core is 415,432 bytes. Both Linux targets
close identical B2/B3/B4 binaries before the existing size check rejects them.
Their target-neutral compiler QBE is 1,390,875 bytes versus 1,331,041 (4.50%).
The initial worker jobs stop before executing their workload; remaining
acceptance cannot be inferred from this size evidence.

The frozen cumulative baseline remains
`ac633935a7f248470c59d22666da14c819a131fa`: 349,240 Darwin bytes and 313,248
Linux arm64 bytes. Ordinary core growth is 66,192 and 73,880 bytes respectively;
the combined ordinary core is 802,560 versus 662,488 bytes (21.14%). Neither
baseline is reset. The local CLI is 565,432 bytes; core, CLI and the unchanged
406,656-byte QBE dependency total 1,387,520 bytes, excluding platform linker and
system libraries. There is no matched new Go build/size measurement.

Local compiler recovery passes through B0, ordinary regeneration, project and
module controls, and conformance. The compiler suite passed 198 tests during
implementation, with focused final-source tests also passing. Hosted ordinary
CLI/REPL checks pass on Darwin and Linux arm64. Existing spectral-norm, n-body
and fannkuch-redux QBE and same-basename Darwin application binaries are
unchanged. This establishes no new runtime gain or Pure Go comparison.

## Enforcement and remaining acceptance

Retain the implementation cost with only the four complete-compiler ceilings
above. Headroom above the largest observed artifacts is 1,528 Darwin bytes,
872 Linux arm64 bytes and 440 amd64 bytes; the paired ceiling is exactly the
sum of the arm64 ceilings. These small margins cover the current capability,
not an allowance for another optimization or syntax slice.

All ordinary 1.05 compiler size, self-build wall and RSS ratios remain intact,
as do application checks, catastrophic limits, MIR ownership, recovery,
process, target and memory requirements. No transition marker or comparison
exception is added. Historical decisions, failed runs, seed manifests and
source-era limits stay unchanged. This decision neither publishes a bootstrap
seed nor changes benchmark Pages.

Validate this policy independently on accepted compiler source. Then integrate
it into the feature PR and require fresh full acceptance, including the
previously blocked worker, target and performance jobs. A substantive cost or
application regression requires reassessment; this policy alone never makes
the feature eligible to merge.
