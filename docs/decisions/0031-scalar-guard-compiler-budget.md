# 0031: Complete-compiler budget for guarded scalar regions

Status: accepted on integration of the separately validated policy PR.

## Problem and evidence

[Issue #371](https://github.com/type-rb/type-rb-native/issues/371) and
[PR #372](https://github.com/type-rb/type-rb-native/pull/372) introduce a bounded
MIR range proof for complete pure scalar regions. One guarded input range can
replace multiple checked Integer operations while every rejected input retains
the original operations and failure behavior. This is a general scalar MIR
facility, not a benchmark-specific source rewrite.

The same-feature baseline remains `5cb44a7ad8d0802368a2fbdf4c433e0c5acf5c16`;
the cumulative baseline remains `ac633935a7f248470c59d22666da14c819a131fa`.
The [registered first local cohort](https://github.com/type-rb/type-rb-native/issues/371#issuecomment-5598355290)
at `854d79ecd9e47447f267a27b3c8c3af684b158b3` retains all 81 observations:
18 warmups and 63 retained executions across three programs and three roles.
Spectral-norm(5500) wall and CPU ratios are 0.730960 and 0.728420 against the
immediate control. Both other numeric controls, memory and catastrophic checks
pass. This uncontrolled Darwin preflight is not a formal Pure Go comparison.

[CI run 34326478178](https://github.com/type-rb/type-rb-native/actions/runs/34326478178)
rejects the 330,496-byte Linux worker compiler against 328,000 bytes. Darwin's
worker passes its current ceiling. The dependent combined and performance
authorities are skipped; they are not passing evidence. The first source also
fails recovery because its String conversion is outside snapshot v4. Correcting
that source and completing all authorities remain prerequisites for adoption.
No budget can waive that failure. Formal attempts 34326755971 and 34326758994
were stopped before measurements; some jobs had already failed bootstrap size
checks. Their failures remain part of the experiment.

Named range records reduce repeated Array accesses and avoid duplicating lower
and upper bound carriers. A further attempt to merge checked/fast emitter
control flow increased local text by 632 bytes and was discarded. The selected
implementation keeps one proof owner and one reusable scalar body emitter. The
retained proof cost is useful optimization code; further size-only restructuring
is not a prerequisite for testing its measured runtime benefit.

## Decision and enforcement

| Complete compiler ceiling | Previous | Revised |
| --- | ---: | ---: |
| Darwin arm64 | 366,000 | 366,000 |
| Linux arm64 | 328,000 | 331,000 |
| Combined arm64 | 694,000 | 697,000 |
| Linux amd64 | 310,000 | 310,000 |

Retain only the bounded complete-compiler cost of this scalar range facility.
The Linux ceiling adds 3,000 bytes (0.915%), with 504 bytes above the first
observed worker; the combined ceiling remains exactly the sum of the target
ceilings. This decision supplies no general budget for additional fact families.

All ordinary 1.05 compiler/build/RSS ratios, catastrophic limits, historical
markers, text/QBE rules, inputs, context pins and correctness/recovery/target/
memory/process authorities remain unchanged. Update the shared policy and its
exact-value tests in this separate PR. Existing published seed assets and their
source-era limits remain immutable. No default or historical comparison baseline
moves, and no failed observation is relabelled.

The corrected feature must pass a fresh exact-head cohort under this policy,
including previously unrun comparative build and combined-cost authorities.
Current formal cross-language runtime and build evidence is required before
publishing Pages. A spectral-norm Pure Go win must come from that same-host
formal comparison, not the preflight or an older published result.

The current cumulative n-body and fannkuch regressions already exist in the
immediate control and remain unresolved. Their local candidate/cumulative wall
ratios are 2.809720 and 2.244872, respectively. This budget does not accept or
hide those regressions. It also does not revive deferred PR #307, authorize a
new backend or seed release, or excuse further growth without another review.
