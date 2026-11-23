# 0030: Complete-compiler budget for named record Arrays

Status: accepted on integration of the separately validated policy PR.

## Problem and evidence

[PR #352](https://github.com/type-rb/type-rb-native/pull/352) adds the ordinary
homogeneous named-record Array subset. Exact declaration types, nested arrays,
mutable value capabilities, readonly fields, retained assignment positions and
managed roots use the existing language and runtime contracts. This enables
ordinary structured data and is a prerequisite for replacing positional MIR
value rows; compiler self-use still needs separate recovery and verified seeds.

Candidate `4315d6f375179ca52df56f7401538d236e731974` includes the readonly
correction accepted as `f1d65b96d659f68376c7ec937be98ef95e02a838` in PR #351.
Keep that same-feature control and the frozen cumulative baseline
`ac633935a7f248470c59d22666da14c819a131fa`; the policy changes no
compiler source and does not reset either comparison.

The [first hosted cohort](https://github.com/type-rb/type-rb-native/actions/runs/34234062970)
passes both ordinary CLI targets and Linux worker size checks. The Darwin
worker fails the existing 350,000-byte ceiling at 365,808 bytes. Its worker
runtime measurements and the dependent combined and comparative authorities
are not accepted evidence. The source-era failure remains a failure.

| Complete compiler | Cumulative baseline | Candidate worker | Previous limit | New limit |
| --- | ---: | ---: | ---: | ---: |
| Darwin arm64 | 349,240 | 365,808 | 350,000 | 366,000 |
| Linux arm64 | 313,248 | 327,736 | 328,000 | 328,000 |
| Sum | 662,488 | 693,544 | 678,000 | 694,000 |

The initial local Darwin prototype grows its text section from 259,136 to
260,804 bytes, crossing a 16 KiB segment-alignment boundary. Its complete
executable grows from 349,256 to 365,768 bytes. Including the readonly repair
retains that file size with 260,964 text bytes. The hosted worker's additional
40 bytes are retained in the budget rather than equated to the local artifact.
The Darwin ceiling has 192 bytes above the observed worker and the
combined ceiling is exactly the sum of the target ceilings. Linux and amd64
ceilings remain unchanged; this is not general headroom for later features.

Local root 99/99 and compiler 133/133 tests pass with recovery and QBE enabled.
Ordinary fixed points, full CLI/REPL/terminal/automatic-GC tests, the 15-case
language registry and 423 deterministic check/QBE observations pass. Existing
cases preserve the readonly control's generated QBE. New record Array
execution matches the reference. The recorded same-named imported-record
reference-check discrepancy is not counted as a matching observation.
These correctness results do not establish runtime benefit or Pure Go parity.

The [registered diagnostic assessment](https://github.com/type-rb/type-rb-native/pull/353#issuecomment-5586627118)
retains all 105 observations. Control and candidate application artifacts are
identical for spectral-norm and n-body. Candidate/control compiler self-build
wall ratio is 1.002047; candidate/cumulative is 1.040143. Spectral-norm runtime
wall ratios are 0.992287 and 0.997435, respectively. No speedup is established.
The cohort also exposes cumulative n-body build and runtime wall ratios of
1.054824 and 1.742429; the accepted control already has the runtime regression.
[Issue #354](https://github.com/type-rb/type-rb-native/issues/354) retains that
unresolved regression. This budget neither approves that regression nor resets
its cumulative baseline, and the completed diagnostic is not renewed.

## Decision and enforcement

Change only ordinary Darwin and combined complete-compiler ceilings to
366,000 and 694,000 bytes. Retain Linux 328,000 and amd64 310,000, all ordinary
1.05 ratios, catastrophic limits, historical markers, text/QBE requirements,
inputs and every correctness, recovery, memory, target and process authority.
The shared policy and its exact-limit tests carry the change to consumers.
Historical seed manifests and their source-era budgets remain immutable.

Retain this bounded compiler cost for useful ordinary structured data. The
policy PR must pass its own selected checks and be integrated separately.
After acceptance, the feature still needs a fresh exact-head cohort under
that policy, including currently unrun Darwin worker and comparative checks.
No admin bypass, diagnostic success or skipped check can authorize its merge.
Keep the original failed cohort and the later policy/source identities distinct.

Reassess further compiler growth or a material application regression through
another reviewed decision. Continue useful MIR representation cleanup after
its ordinary/recovery/seed prerequisites pass. This decision neither accepts
#307 nor renews expired diagnostic budgets, authorizes seed publication, changes
Pages performance values or makes a runtime-performance claim.
