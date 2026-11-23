# 0037: Compiler budget for stack-vector String output

Status: accepted through [PR #424](https://github.com/type-rb/type-rb-native/pull/424).

## Capability, benefit and retained failures

[Issue #421](https://github.com/type-rb/type-rb-native/issues/421) investigates
compiler output cost while #418 adds checked ordinary Array iteration. The
String `puts` implementation writes content and its newline using two stack
vectors and one successful `writev` call. Positive short writes resume at the
remaining bytes; zero/error stops without spinning. It introduces no heap buffer,
new I/O API, compiler-only intrinsic or MIR semantic owner, and preserves the
current best-effort Void boundary. REPL output remains immediate.

The implementation is `c62ace1754b2509883dbd76990a8fb02984a79d1`, compiler tree
`96c279672a6ab0dd4279643ccd439661cd05c907`. The same implementation was tested at
`a66bb1f7853df36967603686908aa5d8705ac6c5` after removal of Decision 0036's
exhausted self-build exception. The heap-copy prototype was rejected for RSS
regression; the [complete diagnostic record](https://github.com/type-rb/type-rb-native/issues/421#issuecomment-5634609933)
retains both the failure and the later 54-observation stack-vector cohort.
The latter uses two warmups and seven retained adjacent pairs for each workload:

| Local Darwin workload | Control wall | Candidate wall | Wall ratio | CPU ratio | RSS ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Each compiler building its own source | 1.87 s | 1.90 s | 1.016043 | 1.016216 | 1.002433 |
| Identical pre-output Array compiler source | 1.97 s | 1.90 s | 0.964467 | 0.963918 | 0.996769 |
| n-body application build | 0.13 s | 0.13 s | 1.000000 | 0.916667 | 0.999585 |

The first comparison uses accepted main `752338fbfe869b0500840dd1a6263db611ee5800`;
the second holds the pre-output source at `eb7ef16cd742fb7442e5d6fd029858ea2a7d5c05`.
The registered benefit is about 3.55% for identical-source compilation, with no
RSS loss. It is not a generated-application runtime gain. An incorrect diagnostic
binary expectation interrupted the cohort; its public correction preserves all
20 completed observations and runs only the 34 missing observations.

All 40 actual-QBE syscall cases pass, including empty/embedded-NUL/multiline/large
Strings, call counts, partial writes and no progress after a prefix. Local
recovery-enabled suites pass 107 root and 198 compiler tests, with fixed points,
snapshot export and CLI/REPL checks. The three existing application probes have
matching output; their only QBE change is the String puts routine, and Darwin
binary growth is 48 / 64 / 48 bytes. These claims do not imply runtime speedup.

The [a66 hosted run](https://github.com/type-rb/type-rb-native/actions/runs/34600582569)
passes functional/recovery/CLI and arm64 memory authorities. Linux amd64 fails
its absolute 331,000-byte ceiling at a byte-identical B2/B3/B4 size of 331,272
(SHA-256 `29478741d3a06f6e1db0d1eff74f6299ba0cf2f3f2901733282412961d12d2cb`).
Performance authorities are skipped, not accepted. Its same-feature pre-output
[eb7 fixed point](https://github.com/type-rb/type-rb-native/actions/runs/34595389857)
is 330,528 bytes: the output change adds 744 bytes, approximately 0.225%.

One naming-only size refinement at `a932d6ae9de6cb6df3c14663133f851755074ce8`
[also failed](https://github.com/type-rb/type-rb-native/actions/runs/34602825836):
331,336 bytes. It reduced an embedded QBE String from 8,307 to 8,000 raw bytes,
but the existing static-String encoder increased its stored representation from
3,824 to 3,888 bytes. The routine's assembly and the ELF executable segment were
unchanged; initialized data grew by exactly 64 bytes. Revert that refinement and
retain the readable original names. Do not search names or compression settings
for a favorable sample. Both failed candidates remain failed. The
[failure and reassessment record](https://github.com/type-rb/type-rb-native/issues/421#issuecomment-5635116121)
keeps the exact artifacts and encoder observations together.

## Narrow enforcement change and cumulative accounting

Raise only `NATIVE_MIR_LINUX_AMD64_COMPILER_LIMIT` from 331,000 to 332,000 bytes.
The increase is 1,000 bytes (0.302%); headroom above the retained implementation
is 728 bytes. This retains a small measured output-runtime cost with a local
same-source compiler-time benefit. No arm64, combined, application, text, QBE,
RSS or ordinary 1.05 ratio limit changes. There is no new transition marker or
self-build exception, and historical decisions/seed manifests remain unchanged.

The a66 arm64 memory authority observes a 415,480-byte Darwin compiler and a
387,936-byte Linux compiler; its post-strip accounting is 415,520 / 387,928 bytes.
The latter pair totals 803,448, within the existing 805,000 ceiling. Keep these
artifact roles separate. Against the frozen `ac633935a7f248470c59d22666da14c819a131fa`
ordinary baseline of 349,240 / 313,248 bytes, the observed unstripped pair totals
803,416 versus 662,488 (21.27% cumulative growth). The baseline is not reset.
Local core/CLI/QBE sizes are 415,480 / 565,480 / 406,656 bytes, totaling 1,387,616,
excluding the platform linker/system libraries. No new matched Go build/size
comparison exists.

Validate this policy independently on accepted compiler source. Then integrate
it into #418 and require fresh complete acceptance under ordinary 1.05, including
the previously skipped performance authorities. Retire the exhausted Decision
0036 selector before measuring that candidate. A further cost failure requires
reassessment, not another allowance or unchanged retry. This policy alone does
not accept Array iteration, publish a seed, update Pages or establish Pure Go
parity.
