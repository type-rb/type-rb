# 0036: Source-bound self-build budget for checked Array iteration

Status: accepted through [PR #422](https://github.com/type-rb/type-rb-native/pull/422).

The registered final cohort failed both targets under the correctly selected
1.08 bound. The following output-runtime candidate has a different compiler
tree and was measured under ordinary 1.05 comparisons. The old source pair no
longer qualifies. [Decision 0039](0039-linux-array-self-build-assessment.md)
separately assesses the later Linux-only comparison; the accepted decision and
failed observations below remain intact.

## Capability, controls and failed acceptance

[PR #418](https://github.com/type-rb/type-rb-native/pull/418) adds ordinary
Array `each` and `each.with_index` using checked MIR plans. Receiver evaluation,
live traversal, independent cursors, managed roots and nearest loop transfers
are language functionality. They do not establish an application speedup.
Compiler adoption still requires matching recovery and an immutable seed handoff.

[Decision 0035](0035-array-iteration-compiler-budget.md) retained this capability's
complete-compiler size cost while leaving the ordinary self-build ratio at 1.05.
The [final-source run](https://github.com/type-rb/type-rb-native/actions/runs/34589592031)
for `f4b81fe7615f0fc3e1e0b49cf232e65a90ab5e10`, compared with accepted
`76ddcea21fce8a231eed36df3a4a9a8fda2c07bc`, still fails that ratio:

| Target | Control self-build | Candidate self-build | Ratio | Compiler bytes, control to candidate |
| --- | ---: | ---: | ---: | ---: |
| Darwin arm64 | 2.075 s | 2.210 s | 1.065060 | 398,904 to 415,432 (1.041434) |
| Linux arm64 | 1.580 s | 1.690 s | 1.069620 | 369,752 to 387,192 (1.047167) |

These are the workflow's two-warmup, seven-retained alternating adjacent pairs,
pooling byte-identical B2/B3 and B3/B4 builds. Each role compiles its own source;
the control does not implement Array iteration. All functional CI jobs passed.
The performance steps stop at the wall-time failure, so later RSS, catastrophic,
application and combined-evidence checks are not accepted by this run.

The preregistered [local source-control diagnostic](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5633154759)
retains all 36 observations, warmups, commands, toolchain and source/binary
identities. With two warmups and seven retained alternating pairs per workload,
own-source wall medians are 1.87 to 1.97 s (1.053476); CPU ratio is 1.048649
and RSS ratio 1.001616. Compiling the identical control source gives 1.87 to
1.86 s (0.994652), CPU ratio 1.0, RSS ratio 1.000403 and identical output bytes.
This supports source-growth cost as the main explanation on that local host;
it does not establish a universal decomposition or replace hosted acceptance.
The first attempted control warmup failed because OS timing statistics were
unavailable; that aborted attempt and its explicit environment correction remain
recorded with the diagnostic. No candidate or retained observation preceded it.

The earlier [absolute-size failure](https://github.com/type-rb/type-rb-native/actions/runs/34581808596),
[self-build failure](https://github.com/type-rb/type-rb-native/actions/runs/34586317274),
and [initial](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5632389906)
and [suffix-guard](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5632704256)
local cohorts retain their original status. Reusing the consumed member name
and sharing block-end checks removed repeated token work without adding emitter
semantics; two bounded refinements have now reached the trade-off checkpoint.
Unchanged reruns or further size-only tuning are not the chosen next step.

## Retained cost and enforcement

Retain this basic capability with a self-build wall ratio of **1.08**, only for
`darwin-arm64-v0` and `linux-arm64-v0` and this exact clean `compiler/src` pair:

- Candidate: `bc4cdfd82560401970e07ef7ef965d8bdb22b612`.
- Control: `47e078f2d8b38429e52935db8a46c00ccb5a6efb`.

The eight-percent bound is about 1.49 and 1.04 percentage points above the two
observed increases. At these control medians it allows 2.241 s and 1.7064 s,
leaving 31 ms and 16.4 ms above the candidate observations. This is limited
headroom for the measured capability, not a new general percentage or the
larger local investigation budget converted to acceptance.

Missing/unknown profiles, dirty or staged source, extra untracked or ignored
source, reversed roles, changed trees and a later baseline do not qualify.
Documentation and policy merges do not change the compiler input identity.
All other ordinary self-build comparisons retain 1.05; compiler size and RSS ratios,
absolute caps, application build/runtime/size, catastrophic limits, historical
transitions and every correctness/target/recovery authority remain unchanged.
No mutable marker or environment variable can supply the approved tree pair.

The frozen cumulative baseline remains `ac633935a7f248470c59d22666da14c819a131fa`:
349,240 Darwin bytes and 313,248 Linux arm64 bytes. The candidate cores total
802,624 versus 662,488 bytes, up 140,136 bytes (21.15%). The local core, CLI
(565,432 bytes) and QBE (406,656 bytes) total 1,387,520 bytes, excluding required
platform linker and system libraries. Cumulative matched build time and a
current matched Go build/size comparison remain unmeasured; neither baseline
is reset and old Go measurements confer no headroom.

The three existing Benchmarks Game programs produce byte-identical QBE and
same-basename Darwin application binaries with the control and candidate.
This is artifact stability, not a measured application runtime improvement.
The shared MIR plan adds necessary verification; emitter-only discovery is
not introduced. Compiler-source iteration self-use and further MIR cleanup
remain issue #410, while bounded output buffering is an independent measured
follow-up candidate in issue #421, not an assumed benefit of this decision.

## Validation, expiry and next action

Validate and integrate this policy separately on accepted compiler source.
Then run one fresh full feature-PR acceptance cohort under it. All checks must
pass at the exact reviewed head before integration. Any cost miss or substantive
application regression requires another assessment, not automatic expansion
or an unchanged rerun. Failed historical cohorts remain failed.

Once the candidate is the baseline, this allowance stops automatically. Remove
the expired selector, workflow argument and its dedicated tests in the following
recovery/policy cleanup; do not carry the exception into compiler self-use or
another syntax slice. The decision record remains. This changes no seed release,
Pages result, supported-language contract or Pure Go performance claim.

## Exhausted allowance and output follow-up

The one [final cohort](https://github.com/type-rb/type-rb-native/actions/runs/34595389857)
at `eb7ef16cd742fb7442e5d6fd029858ea2a7d5c05` applied 1.08 correctly but failed
Linux wall time 1.955 to 2.140 s (1.094629) and Darwin 2.195 to 2.385 s
(1.086560). Compiler byte ratios still passed. Later performance checks remain
unaccepted. This cohort is not retried or reclassified by removing its selector.

[Issue #421](https://github.com/type-rb/type-rb-native/issues/421) investigates
output cost instead. Its first bounded String-copy buffer improved identical-source
wall time but failed RSS and was reverted. The next implementation uses stack
vectors for ordinary String output, including positive short-write progress,
without a heap buffer or new MIR semantics. It requires normal 1.05 acceptance
and every correctness/recovery/target authority; no seed or Pages update follows
from a local diagnostic. The failed prototype, interrupted diagnostic expectation
and its correction remain recorded in that issue.
