# 0029: Complete-compiler budget for safe Array assignment

Status: accepted on 2026-09-08.

## Problem and measured cost

[PR #343](https://github.com/type-rb/type-rb-native/pull/343) repairs an indexed
assignment that can write through freed storage after RHS growth. Its checked
target/store plan retains the Array owner and validated position, protects
managed values and resolves the final write in current storage. Removing unused
emitter mutability and a redundant carrier field recovers 704 Linux bytes.

The cumulative source baseline is `ac633935a7f248470c59d22666da14c819a131fa`.
The first same-feature control is `90435186fc5c06b662c06cb80584575e28d9d064`;
the compact candidate is `885414012e0aa891d1af99f30069e60da300a6cb`.

| Complete compiler | Cumulative baseline | Compact candidate | Previous limit | New limit |
| --- | ---: | ---: | ---: | ---: |
| Darwin arm64 | 349,240 | 349,256 | 350,000 | 350,000 |
| Linux arm64 worker | 313,248 | 325,600 | 317,000 | 328,000 |
| Sum | 662,488 | 674,856 | 667,000 | 678,000 |

The Linux target chain independently produces 325,608-byte identical
generations. Both variants fit the new limit. The remaining worker increase is
12,352 bytes (3.94%) against the cumulative baseline. The 328,000-byte ceiling
provides 2,400 bytes above the worker measurement; it is a rounded budget, not
an inferred minimum implementation cost. The combined ceiling is the sum of
the two target ceilings. Linux amd64 remains at 310,000 bytes.

The first [full cohort](https://github.com/type-rb/type-rb-native/actions/runs/34199705462)
and the [compact cohort](https://github.com/type-rb/type-rb-native/actions/runs/34202210192)
failed the previous Linux size limits. Those failures remain failures. The
compact cohort passed recovery, both CLI targets, Darwin worker memory and
Linux amd64; the combined and comparative-performance authorities were skipped.

One preregistered local Darwin arm64 diagnostic compared all three compilers,
with two warmups and seven retained observations per case in rotating order.
All 216 observations retained the expected program outputs and fixed-point
identities. Spectral-norm(5500) candidate/baseline wall medians were
2.258403/2.233271 seconds (1.0113); CPU medians were 2.21/2.20 seconds.
The 50,984-byte application was unchanged. Self-build wall medians were
1.651388/1.617943 seconds (1.0207); Integer/Float Array reduction runtime ratios
were 1.0027/0.9917. Application build wall ratios ranged from 0.9611 to 1.0145.
The small spectral-norm(100) CPU readings rounded to zero and provide no ratio.
These local observations do not replace hosted acceptance or establish current
Go competitiveness. The complete registration and assessment remain in PR #343.

## Decision and enforcement

Retain the measured complete-compiler cost of the correctness repair and use
the new Linux and combined ceilings for ordinary validation. Do not require
another size-only refinement before measuring the complete acceptance contract.
This revision introduces no structural transition marker or relative exception.

The shared policy supplies target, persistent-worker, combined and comparative
checks. Unknown targets and malformed historical markers still fail closed;
existing tests assert the new exact limits and unchanged ordinary ratios.
Historical marker contents, compiler text/QBE requirements, Darwin/amd64 caps,
all 1.05 ordinary ratios, catastrophic limits, workloads and correctness,
recovery, reproducibility, GC and cleanup requirements remain unchanged.

The budget PR must be accepted separately before the correctness repair can
merge. The repair still requires every selected authority at its exact head
under this policy. A skipped or failed authority is not acceptance, and this
decision does not grant an unchanged-head performance retry.

## Follow-up and historical boundaries

Keep the cumulative baseline above and the same-feature control when assessing
the repair; a policy-only baseline update has identical compiler sources and
must not reset the measured cumulative cost. Once accepted, the repair becomes
ordinary implementation history while this budget remains explicit. Reassess
at the next compiler-budget decision or a material application regression;
further increases require another reviewed decision.

Continue removing superseded emitter ownership through verified MIR slices.
This budget does not excuse semantic duplication, renew a deferred experiment,
change a published seed's source-era manifest limits, authorize a release, or
provide evidence for a Pages benchmark update.
