# 0032: Complete-compiler budget for loop-local Array headers

Status: accepted on integration of the separately validated policy PR.

## Problem and evidence

[Issue #388](https://github.com/type-rb/type-rb-native/issues/388) and
[PR #389](https://github.com/type-rb/type-rb-native/pull/389) separate effects
before a loop from effects within it. Verified MIR can retain a local Array
header across a clean loop after initialization has grown the Array. The
adapter consumes the proof and restores its lexical lifetime at loop exit;
normal index normalization, bounds failures and GC roots remain in place.
This is a general loop-effect facility with one semantic owner.

The same-feature baseline is `4be54adfdf01dae89675982fa64da333526d0af4`;
the frozen cumulative baseline remains `ac633935a7f248470c59d22666da14c819a131fa`.
The [registered local runtime cohort](https://github.com/type-rb/type-rb-native/issues/388#issuecomment-5610946529)
at `4ad461eb0d21b73179bacff5345eff1f4c1cedf6` retains all 117 observations:
18 warmups and 99 measured runs across three programs and three roles.
Fannkuch-redux(10) improves from 0.349091125 to 0.308360042 seconds median
wall time (ratio 0.883322); its CPU ratio is 0.881077. N-body(50000000)
and spectral-norm(5500) wall ratios are 0.999860 and 0.999909. All outputs,
control bounds and catastrophic checks pass. The cumulative n-body and
spectral wall ratios are 0.729410 and 0.730016. These uncontrolled Darwin
measurements are not current Linux or Pure Go comparisons.

The separate [application build cohort](https://github.com/type-rb/type-rb-native/issues/388#issuecomment-5610946774)
retains 45 observations, including 18 warmups. All same-feature wall/CPU/RSS
ratios pass 1.05; complete application sizes do not grow. Application build
cost is separate from compiler self-build cost.

[CI run 34421338987](https://github.com/type-rb/type-rb-native/actions/runs/34421338987)
fails the Linux complete-compiler ceiling at 333,776 bytes against 331,000.
Native recovery and the other completed correctness jobs pass; dependent
combined and performance authorities are skipped, not passing evidence.
Earlier recovery failures and their subsequent alias-resolution correction
remain recorded in the issue. No size decision waives correctness.

One bounded refinement shares identical invalid-plan construction in MIR.
At `880a2a5fe6173b3d6c08e8102df9d09ce02b0f54`, all three complete application
QBE and executable identities match the measured candidate. Its ordinary
fixed points pass. The separate [cost workflow 34422165569](https://github.com/type-rb/type-rb-native/actions/runs/34422165569)
has [complete retained observations](https://github.com/type-rb/type-rb-native/issues/388#issuecomment-5611035010)
and reports Darwin compiler size 365,816 bytes, median self-build ratio 1.020349,
and RSS ratio 1.000396. Linux still fails at 332,440 bytes, so its comparative
build/RSS measurements remain unrun. Combined size is 698,256 bytes versus
696,184 for the accepted baseline: 2,072 bytes, about 0.30%, of incremental
complete-compiler cost. The cumulative Darwin compiler is 349,240 bytes;
the current 365,816 bytes represent 4.75% retained growth from that frozen
baseline. No cumulative baseline is reset by this decision.

## Decision and enforcement

| Complete compiler ceiling | Previous | Revised |
| --- | ---: | ---: |
| Darwin arm64 | 366,000 | 366,000 |
| Linux arm64 | 331,000 | 334,000 |
| Combined arm64 | 697,000 | 700,000 |
| Linux amd64 | 310,000 | 310,000 |

Retain the bounded proof cost of this loop-local header facility. The Linux
ceiling adds 3,000 bytes (0.906%), leaving 1,560 bytes above the refined
observed compiler. The combined ceiling remains the sum of the target ceilings.
The measured runtime gain warrants review without another size-only rewrite;
this supplies no general allowance for new facts or additional passes.

All ordinary 1.05 compiler/build/RSS ratios, catastrophic limits, historical
markers, text/QBE rules, baseline pins and correctness/recovery/target/memory/
process authorities remain unchanged. The shared enforcement and exact-value,
unknown-target and transition tests are validated in this separate PR.
Published seed assets and source-era limits remain immutable. Neither original
failed run is relabelled as passing, and no diagnostic budget grants acceptance.

The refined feature must pass fresh exact-head full acceptance under this
policy, including the previously unrun Linux comparative build/RSS and combined
cost checks. Continue accounting for cumulative cost and the remaining direct
emitter migration before proposing another fact family. Preserve the restored
n-body and fannkuch behavior and the spectral improvement. Current formal
cross-language runtime and build evidence is required before refreshing Pages;
this policy does not establish a new Pure Go win or authorize a seed release.
