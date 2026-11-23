# 0034: amd64 compiler budget for Hash index self-use

Status: accepted on integration of the separately validated policy PR.

## Measured implementation trade

[PR #406](https://github.com/type-rb/type-rb-native/pull/406), candidate
`53c7b131ac504ee595a6452b79ee1dadbe7ce8a4`, replaces bespoke module/function
indexes with ordinary Hash values. Parsing maintains function indexes without
rebuilding them; import-cycle traversal owns separate temporary storage. It
removes the former custom symbol index, nine index helpers and a redundant project name
vector. The verified seed and snapshot prerequisites were accepted in PR #405.

The same-feature control is `fcb9e610eb6478d545b47950b027f05ff919e2c2`;
the frozen cumulative baseline stays `ac633935a7f248470c59d22666da14c819a131fa`.
The [initial full CI](https://github.com/type-rb/type-rb-native/actions/runs/34499183321)
passes recovery, ordinary CLI, both arm64 targets and worker-memory checks.
amd64 produces identical B2/B3/B4 compilers at 316,096 bytes, but fails its
316,000-byte ceiling by 96 bytes. Dependent target comparison and performance
acceptance remain unrun in that attempt. This is a failed result, preserved as
such. No correctness failure is converted into a size decision.

Two bounded, uncommitted visit-state storage alternatives increased amd64
object text and were discarded. The [recorded reassessment](https://github.com/type-rb/type-rb-native/issues/403#issuecomment-5621914180)
then ran one independent [arm64 ordinary comparison](https://github.com/type-rb/type-rb-native/actions/runs/34501571525)
with two warmups and seven retained interleaved rounds for both fixed-point
adjacencies. Every existing limit passed:

| Metric | Control | Candidate | Change |
| --- | ---: | ---: | ---: |
| Darwin arm64 compiler | 398,888 bytes | 398,904 bytes | +16 bytes |
| Linux arm64 compiler | 368,936 bytes | 369,752 bytes | +816 bytes |
| Darwin self-build median | 3.020 s | 2.860 s | -5.3% |
| Linux arm64 self-build median | 1.555 s | 1.600 s | +2.9% |
| Darwin peak RSS median | 41,287,680 bytes | 41,312,256 bytes | +0.06% |
| Linux arm64 peak RSS median | 64,815,104 bytes | 64,849,920 bytes | +0.05% |
| Linux amd64 compiler | 315,552 bytes | 316,096 bytes | +544 bytes (+0.17%) |

Self-builds compile each role's own source. The mixed timing changes establish
ordinary acceptance bounds, not a general speedup. The retained benefit is
verified language self-use and removal of duplicated index storage/maintenance.
Local full recovery passes 107 tests and the compiler suite passes 193 tests.
The three benchmark application QBE files and same-basename Darwin executables
are identical to the control. No application runtime or Pure Go gain is claimed.

Against the frozen arm64 cores, retained growth is 49,664 Darwin bytes and
56,504 Linux bytes. Combined core size is 768,656 versus 662,488 bytes (+16.03%).
The largest worker/fixed-point pair is 768,696 bytes, within 770,000. The local
CLI is 548,888 bytes; core, CLI and the unchanged 406,656-byte QBE total
1,354,448 bytes, excluding platform linker/system libraries. No matched new Go
build/size comparison or frozen amd64 measurement is claimed by this decision.

## Narrow acceptance change

Raise only the complete Linux amd64 compiler ceiling from 316,000 to 316,500
bytes. This retains the measured 544-byte implementation increase and leaves
404 bytes above its fixed point, comparable to the previous 448-byte margin.
Darwin 400,000, Linux arm64 370,000 and combined 770,000 ceilings stay unchanged.
Every ordinary 1.05 time/size/RSS ratio, catastrophic bound, application check,
process boundary, recovery and target requirement remains unchanged. There is
no new feature-transition allowance, baseline reset or general relaxed ratio.
Historical decisions, seeds, manifests and measurements retain their source-era
identities and limits. This does not publish a seed or update benchmark Pages.

Validate this enforcement change separately before relying on it. The source
implementation must then pass fresh full acceptance under the approved policy,
including the previously blocked amd64/cross-target authorities. The independent
cost cohort is not merge acceptance. Subsequent compiler changes remain subject
to the same ordinary contract; further overruns require a new explicit review.
