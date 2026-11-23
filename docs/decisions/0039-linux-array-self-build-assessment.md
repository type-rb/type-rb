# 0039: Retain the assessed Linux Array self-build cost

Status: policy accepted through [PR #428](https://github.com/type-rb/type-rb-native/pull/428).
The one registered implementation cohort failed; this allowance is exhausted.
The subsequent String-index runtime candidate uses ordinary limits.

## Measured capability and remaining failure

The current Array candidate implements checked ordinary `each` / `each.with_index`,
live receivers, lexical bindings, nearest-loop transfers and managed roots. It
also retains the measured stack-vector String output and per-module declaration
index improvements. These are general compiler/language changes. Compiler-source
Array self-use still requires accepted recovery and verified immutable seed handoff.

The [complete dbe2 run](https://github.com/type-rb/type-rb-native/actions/runs/34613696617)
fails ordinary Linux arm64 self-build elapsed time: 1.550 to 1.640 s, ratio
1.058065 over 1.05. Its size ratio passes (369,752 to 387,936 bytes, 1.049179).
Darwin passes at 2.965 to 2.975 s (1.003373), RSS ratio 1.000198. Functional,
recovery, target, CLI and memory authorities pass; checks following the Linux
time failure remain unaccepted. The earlier [standalone run](https://github.com/type-rb/type-rb-native/actions/runs/34613777947)
passes Linux at 1.590 to 1.660 s (1.044025). That observation does not replace
the failed full run or establish that the failure is noise.

Each role compiles its own source. Following the ordinary entry's explicit
imports, accepted-main contains 16 modules / 435,481 source bytes / 12,318 lines;
the candidate contains 17 modules / 455,282 bytes / 12,759 lines. Source bytes
increase by 4.55%. That is useful context for retaining a basic capability, not
an exact attribution of the Linux elapsed increase to source growth. Matched-input
Linux throughput has not been isolated and is not claimed here.

The [final declaration-index cohort](https://github.com/type-rb/type-rb-native/issues/414#issuecomment-5636699141)
records local Darwin own-source ratio 1.014311 against accepted main, unchanged
representative same-feature controls, and 32.09% less time for 6,000-function
emission. Its extra 2,441,216 bytes of scale-case peak RSS failed the original
5% diagnostic condition; [Decision 0038](0038-declaration-index-memory-tradeoff.md)
assesses that exact trade separately. The [String output cohort](https://github.com/type-rb/type-rb-native/issues/421#issuecomment-5634609933)
measures 3.55% less identical-source compilation time. Neither establishes a
general application runtime gain.

The [subsequent token-classification cleanup](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5637086419)
was rejected: its single 72-observation cohort showed no useful identical-source
gain, and Linux grew by 352 bytes. Reverting it restores the dbe2 tree exactly.
There is no variant sweep or size-budget increase for that rejected source.

## Candidate-limited decision

Retain the remaining own-source Linux self-build cost with a ratio of **1.07**,
only for `linux-arm64-v0` and this exact clean `compiler/src` pair:

- Candidate: `bb47cf20daf6a3c2903f75098021ab163ad72638` (dbe2, also restored by b2d5).
- Control: `47e078f2d8b38429e52935db8a46c00ccb5a6efb` (accepted a2ab source).

At the failed control median this permits 1.6585 s, leaving 18.5 ms above the
observed candidate and 1.1935 percentage points above its measured increase.
This explicitly retains the cost of the current checked language capability
and measured general compiler improvements after bounded refinements. It does
not declare the failed 1.05 run successful or assume further code will be free.
The already exhausted [Decision 0036](0036-array-iteration-self-build-budget.md)
concerned a different source and both arm64 profiles; its failed 1.08 cohort
remains failed, and that old tree no longer qualifies for any exception.

Darwin, amd64, unknown/missing profiles, future source, reversed roles and an
accepted candidate used as the control retain ordinary 1.05. Cleanliness checks
reject unstaged/staged changes and untracked/ignored compiler source; no marker
or environment value can nominate a different tree. Compiler size/RSS ratios,
all absolute caps, application build/runtime/size, catastrophic bounds and
correctness/target/process/recovery requirements remain unchanged. Seed-generation
bounds are separate and unchanged.

Cumulative compiler cost remains 415,480 Darwin + 387,936 Linux = 803,416 bytes,
21.27% above the frozen `ac633935a7f248470c59d22666da14c819a131fa` baseline of
662,488 bytes. Local core/CLI/QBE total 1,387,616 bytes, excluding platform linker
and system libraries. Current matched Go cost and general Pure Go parity remain
unmeasured. No cumulative baseline is reset.

## Validation, expiry and next action

First validate and merge this policy separately on accepted main, with rejection
tests for old/rejected/changed source and every unqualified profile. Then integrate
it into the restored Array implementation and run **one fresh full acceptance
cohort**. All authorities must pass at its exact reviewed head; this decision is
not a merge bypass. Any further miss requires reassessment, not an unchanged
retry, renewed cohort or automatically larger allowance. Preserve all earlier
failed results and raw observations.

After the implementation becomes the baseline, the selector stops matching.
Remove it, the workflow argument and dedicated tests in the next recovery cleanup;
keep this decision as history. Complete recovery and immutable seed verification,
then adopt Array iteration in the already registered bounded compiler traversal
slice and proceed with Range/MIR/organization work. A seed or Pages publication
cannot follow from this policy decision alone.

## Following recovery cleanup

The recovery integration removes the source selector, its Git-cleanliness helper,
the workflow profile argument and their dedicated tests. It may merge only after
the Array parent has full acceptance and is on main; its comparison then uses
the same-feature Array baseline and ordinary 1.05. Historical observations and
this decision remain intact. This cleanup does not publish a seed.

## Final result and subsequent source

The [one registered final 1b cohort](https://github.com/type-rb/type-rb-native/actions/runs/34622882942)
failed Linux own-source time at 1.730 to 1.870 seconds, ratio 1.080925 over the
actual 1.07 bound. Darwin passed at 1.044177. This decision grants no repeat or
larger allowance. Its complete observations remain linked from
[the result and profiling record](https://github.com/type-rb/type-rb-native/issues/410#issuecomment-5637946459).

The later [bounded String-index improvement](https://github.com/type-rb/type-rb-native/issues/429)
changes compiler source to tree `2c5449437f0b1b216bdc9aa810c00804a1f76cae`.
It cannot match this selector. Its early target/cost checks and single registered
local benefit/control cohort pass under ordinary limits. Its separate
[full final cohort](https://github.com/type-rb/type-rb-native/actions/runs/34630185576)
passes all 17 authorities under ordinary 1.05 before PR #418 merges. PR #423
then retires the unused selector with its own complete same-feature validation.
This preserves the failed historical outcome and every ordinary contract.
