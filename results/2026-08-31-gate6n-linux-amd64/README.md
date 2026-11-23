> Lifecycle: [complete source-era record](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-08-31-gate6n-linux-amd64/README.md).
> Historical checksum inventories refer to that full record, not this compact working set.

# Gate 6N Linux amd64 Target-chain Results

Gate 6N passes every preregistered fixed-point, correctness, process,
ELF, cleanup, cross-target, measurement, and size condition on the exact
merged `main` revision. The self-hosted compiler now closes an ordinary
Go-free Linux amd64 chain behind the internal `linux-amd64-v0` profile
without changing the shared TypeRB frontend, target-neutral QBE, managed
runtime semantics, or existing arm64 behavior.

On the identical portable TypeRB application, the amd64 Native path builds
77.60% faster with 21.00% less peak RSS than the pinned optimized-Go path.
The resulting application starts and completes 53.22% faster with 73.84% less
peak RSS, and its stripped executable is 99.26% smaller. This sub-millisecond
runtime case is deliberately startup-dominated; it proves the registered
portable-entry boundary rather than a broad language-performance ranking.

The Native-owned compiler build is effectively level with the equivalent
external emit-QBE/QBE/CC recipe: its median time is 0.02% lower and peak RSS
is 0.01% higher. Adjacent B2-to-B3 and B3-to-B4 medians differ by 1.33% in
time and 0.11% in RSS. The exact 240,888-byte compiler retains 69,112 bytes
of headroom under the frozen 310,000-byte limit.

## Revisions and public evidence

- pre-implementation Native baseline:
  `266c996668a4c3e0ad6eb833ca646b73ca7e56e1`
- exact merged and measured Native revision:
  `f7e6b02b38c77d0ea6f7da210e91575a4fa1cdf9`
- pinned TypeRB semantic reference:
  `2cf63e95b4fc1a92f6094e2c89c47fb75262adae` (`0.4.3-dev`)
- QBE 1.3 source SHA-256:
  `d587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320`
- immutable recovery release:
  [`bootstrap-seed-2026-08-30`](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-08-30)
- implementation:
  [PR #129](https://github.com/type-rb/type-rb-native/pull/129)
- successful exact-main evidence:
  [Actions run 33346867401](https://github.com/type-rb/type-rb-native/actions/runs/33346867401)
- successful exact-main repository validation:
  [Actions run 33346832658](https://github.com/type-rb/type-rb-native/actions/runs/33346832658)
- preregistered scope:
  [issue #128](https://github.com/type-rb/type-rb-native/issues/128)

The formal run used a fresh GitHub-hosted Ubuntu 24.04 x64 runner with four
virtual AMD EPYC 9V74 CPUs, Linux 6.17, GCC 13.3.0, binutils 2.42, Ubuntu LLD
18.1.3, Python 3.12.3, Go 1.27.0, and QBE 1.3. The QBE executable had SHA-256
`2bc09f15ceef535dcfaf8dfd0a55abcbf25cd514edc150ed3533d64c29077ee9`
and was built from the pinned 281,332-byte source archive. The compiler, QBE,
CC, assembler, `collect2`, LLD, dynamic libraries, optimized-Go toolchain,
and measurement controller remain separately identified in the raw evidence;
they are not collapsed into the 240,888-byte Native compiler claim.

## Recovery, fixed points, and target-neutral output

The amd64 setup verified the immutable 658,639-byte root QBE with SHA-256
`62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a`,
translated it with QBE `amd64_sysv`, and performed two separately identified
Go-free current-source transitions. Setup compilers were excluded from
candidate measurements.

The ordinary current-runtime B1-to-B2, B2-to-B3, and B3-to-B4 builds each
executed the preceding Native compiler as the complete compiler driver. B2,
B3, and B4 were byte-identical:

| Target | B2/B3/B4 bytes | B2/B3/B4 SHA-256 |
| --- | ---: | --- |
| Linux amd64 | 240,888 | `cb6002b84dcfcb05d6c1336a7d5affa4eaa16a216f2fcb776d48c01bbe605a0b` |
| Linux arm64 regression | 268,176 | `749c4aa6313c13054b459be382cb95e546cbbdd9ffa04067c3c92547db5d1904` |

Both architectures emitted the exact 932,584-byte target-neutral compiler QBE
with SHA-256
`b6a488c51d4a1c7ba0729172cf6d3f9af1b9954579746d2e032831e44ef6add9`.
The portable application QBE was also identical across amd64 and arm64, with
SHA-256
`2206fd67e432b7e620a1621058da9ebf09181d3c284bc84b4922a5bfd4977371`.
The same workflow attempt produced both architecture artifacts before the
cross-target comparison ran.

## Correctness and external boundaries

The complete registered valid, mutation, invalid, runtime-failure,
tool-failure, atomic-publication, configured-project, portable-entry, and
managed-runtime corpus passed. Native and optimized Go produced exact
successful output and the same status and failure class for all six runtime
failures:

| Case | Native status | Go status | Failure class |
| --- | ---: | ---: | --- |
| invalid decimal format | 2 | 2 | `panic: invalid Integer` |
| sign without digits | 2 | 2 | `panic: invalid Integer` |
| parsed Integer overflow | 2 | 2 | `panic: Integer is outside the portable range` |
| NaN to Integer | 2 | 2 | `panic: Float cannot be converted to Integer` |
| infinity to Integer | 2 | 2 | `panic: Float cannot be converted to Integer` |
| finite Float overflow | 2 | 2 | `panic: Integer is outside the portable range` |

Every ordinary amd64 compiler generation and the traced Native application
build expose exactly the Native compiler, pinned QBE, `/usr/bin/cc`,
`/usr/bin/as`, GCC `collect2`, and `/usr/bin/ld.lld`. The closed process
checks reject Go, the reference compiler, recovery generators, shell-mediated
compiler children, unapproved paths, and hidden source-content input.

The produced compiler and portable application are stripped x86-64 System V
PIE executables with the expected dynamic interpreter, RELRO, `BIND_NOW`,
non-executable `GNU_STACK`, and no Go build metadata. The compiler depends
only on `libc.so.6`; the portable application records `libm.so.6`,
`libc.so.6`, and its dynamic `sqrt` boundary. Repeated successful and
failed paths left no Native intermediate or temporary publication directory.

## Measurement method

Compiler measurements used two warmups and seven interleaved retained
observations. Application build and runtime measurements used two warmups and
eleven interleaved retained observations. Every one of the 88 warmup and
retained rows reports zero elapsed-command, RSS-command, and observer status.

Elapsed time and peak RSS were collected by independent process executions.
The elapsed controller launched the requested process directly and used
`clock_gettime(CLOCK_MONOTONIC)` through Python's nanosecond monotonic clock;
GNU time separately launched the same direct command for orchestration-root
peak RSS. Correctness, hashing, stripping, and target inspection remained
outside the measured intervals.

The raw catastrophic-regression audit also passed. The largest retained
time multiple against its applicable median baseline was 1.1761x for one
Native compiler build; the largest retained RSS multiple was 1.0380x for one
Native runtime observation. Both remain far below the frozen 2x boundary.

## Native-owned compiler versus external recipe

Elapsed values are seconds and RSS values are bytes. Medians exclude both
warmups.

| Metric | Native-owned build | External recipe | Native change | Limit |
| --- | ---: | ---: | ---: | ---: |
| compiler build time | 1.011471 s | 1.011697 s | -0.02% | +25% |
| compiler build peak RSS | 61,280,256 B | 61,272,064 B | +0.01% | +25% |

The external recipe performs the equivalent Native emit-QBE, QBE translation,
and CC/LLD link outside the compiler-owned orchestration path. The result shows
that the convenience of the ordinary Native build driver adds no material
cost for this full compiler build on the registered runner.

| Adjacent metric | B2 to B3 | B3 to B4 | Spread | Limit |
| --- | ---: | ---: | ---: | ---: |
| build time | 1.070266 s | 1.056263 s | 1.33% | 10% |
| build peak RSS | 61,272,064 B | 61,206,528 B | 0.11% | 10% |

The raw B2/B3/B4 compiler size is 240,888 bytes, 22.29% below the per-target
310,000-byte ceiling.

## Native versus optimized Go

| Metric | Native | Optimized Go | Native change | Limit |
| --- | ---: | ---: | ---: | ---: |
| application build time | 0.041964 s | 0.187373 s | -77.60% | +25% |
| application build peak RSS | 60,723,200 B | 76,865,536 B | -21.00% | +25% |
| application runtime | 0.000892 s | 0.001907 s | -53.22% | +25% |
| application runtime peak RSS | 1,617,920 B | 6,184,960 B | -73.84% | +25% |

Native is below optimized Go on all four registered primary medians.

| Artifact | Raw bytes | Stripped bytes |
| --- | ---: | ---: |
| Native portable application | 14,056 | 14,048 |
| optimized-Go portable application | 2,876,901 | 1,900,152 |

The stripped Native executable is 99.26% smaller, exceeding the required 80%
reduction by 19.26 percentage points. The independently regressed Linux arm64
Native application is 16,224 raw bytes; cross-architecture absolute size and
speed values are context only and were not used as direct comparisons.

## Recorded evidence corrections

The public history distinguishes implementation results from evidence-tool
corrections:

- [run 33345874321](https://github.com/type-rb/type-rb-native/actions/runs/33345874321)
  passed the amd64 verifier but exposed an overstrict arm64 setup assertion.
  Both setup transition compilers built successfully through the immutable
  older runtime's GCC linker path; the workflow incorrectly required LLD one
  generation before the current runtime became active.
- Commit `0310fdfa14a37a54aa84b3dfe7d3283bcb462896` removed only those two
  setup-transition linker assertions. The current fixed-point and amd64
  ordinary-process LLD requirements were unchanged.
- [run 33346233651](https://github.com/type-rb/type-rb-native/actions/runs/33346233651)
  then passed the complete PR revision.
- [run 33346867401](https://github.com/type-rb/type-rb-native/actions/runs/33346867401)
  is the fresh successful merged-`main` run recorded here.

No registered threshold, candidate behavior, or semantic criterion was
weakened after formal measurement.

## Raw evidence

This result retains all 1,365 extracted files from the three exact-main
workflow artifacts:

- [`linux-amd64/measurements.csv`](linux-amd64/measurements.csv) contains
  every warmup and retained measurement row.
- [`linux-amd64/medians.csv`](linux-amd64/medians.csv) records the enforced
  primary medians and frozen percentage limits.
- [`linux-amd64/ordinary-chain`](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-08-31-gate6n-linux-amd64/linux-amd64/ordinary-chain) and
  [`linux-amd64/applications`](linux-amd64/applications) retain process
  traces, exact executable inventories, outputs, failures, and cleanup
  evidence.
- [`linux-amd64/elf`](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-08-31-gate6n-linux-amd64/linux-amd64/elf) retains compiler and application ELF,
  dependency, symbol, note, stack, and absent-Go-metadata inspection.
- [`linux-arm64`](linux-arm64) retains immutable-seed verification,
  current-revision fixed points, portable behavior, process traces, target
  inspection, and identities.
- [`cross-target/observed-identities.txt`](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-08-31-gate6n-linux-amd64/cross-target/observed-identities.txt)
  records the same-attempt amd64/arm64 artifact and QBE identities.
- `EVIDENCE_SHA256SUMS` covers every retained raw evidence file; it excludes
  this explanatory README and itself.

## Conclusion and deferred scope

Gate 6N establishes that the TypeRB-authored self-hosted compiler can add a
second CPU architecture and a third execution environment while preserving
one shared frontend, target-neutral QBE, runtime semantics, exact fixed
points, a closed ordinary tool graph, and strict same-runner performance and
size bounds. For the bounded identical-source application, the Native result
is faster, lighter, and substantially smaller than optimized Go on every
registered primary metric.

`linux-amd64-v0` remains an experimental internal profile. This result does
not publish an amd64 seed, declare target support, freeze the CLI or runtime
ABI, promise cross compilation, add musl/static linking, or select a general
distribution toolchain. Broader Benchmarks Game comparisons remain tracked in
[issue #103](https://github.com/type-rb/type-rb-native/issues/103), and
persistent Web/Job resource stability remains tracked in
[issue #104](https://github.com/type-rb/type-rb-native/issues/104).
