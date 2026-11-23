# 0033: Compiler budget for ordinary Hash support

Status: accepted on integration of the separately validated policy PR.

## Capability and evidence

[PR #401](https://github.com/type-rb/type-rb-native/pull/401) adds ordinary
Integer/String-keyed Hash values to compilation and the stateful REPL. The
checked MIR owns operation types, source boundaries, mutation permissions and
literal capacity; lowering retains receiver/key roots across allocating right
hand sides. Compact word arrays, deletion, shrinking and precise GC support
managed values and cycles. This is basic collection support, not a benchmark
special case. Compiler self-use still requires a separately accepted seed handoff.

The reviewed implementation is `bab51303dddc923053cdd8972ddf4bf670497193`.
The same-feature control is `7014e1daa1c4acd79301e9425a0361321503f751`;
the cumulative baseline remains `ac633935a7f248470c59d22666da14c819a131fa`.
The control lacks Hash, so there is no same-program pre-feature Hash speedup.
Neither baseline is reset by this decision.

The [complete local diagnostic cohorts](https://github.com/type-rb/type-rb-native/tree/bab51303dddc923053cdd8972ddf4bf670497193/results/2026-09-10-native-hash-diagnostic)
retain all warmups and measured observations, source and binary identities,
inputs and limitations. These uncontrolled prototype cohorts overlapped local
validation and are not final-source or formal cross-language acceptance.
The initial CLI source cannot be exactly reconstructed; later paired cohorts
identify their source commits. Their original diagnostic status is preserved.

The Darwin core grows from 365,816 to 398,888 bytes: 33,072 bytes (9.04%).
Against the frozen 349,240-byte compiler, retained growth is 49,648 bytes
(14.22%). Linux arm64 core growth is 36,496 bytes (10.98%) from the
332,440-byte control, or 55,688 bytes (17.78%) from the 313,248-byte frozen
baseline. The cumulative combined core grows from 662,488 to 767,824 bytes
(15.90%); the baseline remains fixed. Initial same-source self-build medians are 1.872 to 1.887 seconds
(0.8%); own-source medians are 1.716 to 1.887 seconds (10.0%), including the
new implementation source. Own-source RSS is 40,124,416 to 40,353,792 bytes
(0.6%). Three unchanged application build wall ratios are 1.006 to 1.017,
with no demonstrated application runtime gain. These remain diagnostics.

The 100,000-entry compiled insert/read/delete examples use about 10.4 MiB
RSS for Integer pairs and 18.0 MiB for String pairs. Final paired REPL medians
are about 2.27 s / 118 MiB and 3.14 s / 163 MiB, respectively. The interpreter
retains temporaries until submission completion; this cost is not hidden by
bucket reclamation. The reviewed CLI is 548,872 bytes, and QBE is a separate
406,656-byte dependency. Core, CLI and QBE total 1,354,416 bytes on the local
host, excluding the required platform linker and system libraries.

[Full CI attempt 34486513812](https://github.com/type-rb/type-rb-native/actions/runs/34486513812)
records the existing size failures below. Worker and ordinary fixed-point
outputs are separate artifacts; both must fit. Linux arm64 fixed-point B1-B4
are 368,936 bytes; its worker is 368,928 bytes. The amd64 bootstrap
has equal B2/B3/B4 binaries at 315,552 bytes before its size checkpoint fails.
The recovery preflight also exposes missing Hash modules in its staged source;
that correctness failure requires a separate implementation-PR repair. The
remaining dependent authorities are unrun, not accepted measurements.

## Decision and enforcement

Retain the measured implementation cost of ordinary Hash collections:

| Complete compiler | Observed candidate | Previous ceiling | Revised ceiling |
| --- | ---: | ---: | ---: |
| Darwin arm64 worker | 398,928 | 366,000 | 400,000 |
| Linux arm64 fixed point | 368,936 | 334,000 | 370,000 |
| Sum of largest arm64 artifacts | 767,864 | 700,000 | 770,000 |
| Linux amd64 fixed point | 315,552 | 310,000 | 316,000 |

The ceilings allow 1,072 Darwin bytes and 1,064 Linux arm64 bytes above the
largest observed artifacts; amd64 allows 448 bytes. The combined ceiling is their sum. Both bootstrap and the
amd64 controller consume the same shared target ceiling. This is retained
capability cost, not an investigation ceiling or general space for another pass.

A one-time comparison permits compiler-size ratio 1.12 and compiler self-build
wall ratio 1.15. The observed arm64 size increases are about 9.0% and 11.0%;
the size allowance is bounded further by the absolute ceilings. The self-build
allowance covers the measured 10.0% source-growth cost with five percentage
points of headroom; fresh hosted comparisons still must pass.
It applies only when both clean `compiler/src` trees match exactly:

- Candidate: `fd6f68e3647e0bca1b1f21d150164355bff92011`.
- Control: `0a328521aeb97e0035bb8ab4824598ec981708ac`.

Dirty or untracked source, another baseline, reversed roles, or later compiler
changes do not qualify. Policy/documentation-only merges preserve these trees.
Once Hash is the baseline, the allowance also stops automatically. Tests cover
these boundaries and retain the ordinary 1.05 ratios. Remove the expired
comparison hook at the next policy cleanup; do not broaden its identities to
admit further feature work without another reviewed decision.

RSS, application build/runtime/size checks, catastrophic limits, historical
markers, and all correctness, recovery, target, memory, process and cleanup
authorities remain unchanged. Text/QBE absolute limits belong to the historical
transitions that register them; ordinary mode observes those sizes. Correcting
the initial PR's mistaken description changes no enforcement. Published seed
assets and source-era evidence remain immutable.

Validate and integrate this policy separately, then require fresh full
acceptance of the corrected Hash PR under it. No failed or skipped job, cost
diagnostic, admin bypass or older binary authorizes the feature merge. A
substantive application regression requires reassessment. This decision makes
no Pure Go comparison, changes no Pages values and authorizes no seed release.
