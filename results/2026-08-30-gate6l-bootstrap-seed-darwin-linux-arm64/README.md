# Gate 6L Durable Bootstrap Seed Results

Gate 6L passes every registered release-integrity, provenance, correctness,
fixed-point, process-boundary, size, elapsed-time, and peak-RSS criterion. The
first durable TypeRB Native seed is published as the immutable experimental
prerelease
[`bootstrap-seed-2026-08-30`](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-08-30).

A fresh Darwin arm64 or Linux arm64 job can now download its platform Native
compiler, the strict manifest, and `SHA256SUMS`; verify the immutable release,
GitHub asset digests, and exact artifact-attestation identity; and close an
ordinary B1-to-B2-to-B3-to-B4 Native chain. That ordinary graph downloads no
root QBE and does not install or execute Go, the reference `trb`, or a recovery
compiler.

This date-labelled artifact remains non-SemVer and experimental. It is a
bootstrap handoff, not a stable compiler release or target-support promise.

## Revisions and public evidence

- seed source and immutable tag revision:
  `0058818314977633c50393796ef9b9f8f1fda50f`
- final verification-tooling revision:
  `a7b296e1cf3415a0a435ad92a5c681a68bf9d593`
- pinned TypeRB semantic reference:
  `fa9e0503cc681bcaa691f6f11d2f1e19ca8e6453` (`0.3.49-dev`)
- registered fixed-point QBE: 658,639 bytes, SHA-256
  `62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a`
- QBE 1.3 source archive: 281,332 bytes, SHA-256
  `d587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320`
- successful initial ceremony:
  [Actions run 33279638760](https://github.com/type-rb/type-rb-native/actions/runs/33279638760)
- successful post-publication verification:
  [Actions run 33280882464](https://github.com/type-rb/type-rb-native/actions/runs/33280882464)
- preregistered scope:
  [issue #90](https://github.com/type-rb/type-rb-native/issues/90)

The published tag directly targets the seed source revision. The final
post-publication workflow records the separate verifier revision while reading
all compiler source and fixtures from that immutable tag.

## Immutable release inventory

The release has `draft: false`, `prerelease: true`, `immutable: true`, and
exactly five assets:

| Asset | Bytes | SHA-256 |
| --- | ---: | --- |
| `type-rb-native-bootstrap-root-qbe-v1.ssa` | 658,639 | `62db3c31527a670c3050051a9fa27bf142b6c5deaab81ef8234104bd467aa95a` |
| `type-rb-native-bootstrap-darwin-arm64` | 259,032 | `ef438d13598c534766334b408a39715c56ff1b69db528910ebf7d90ec7720b65` |
| `type-rb-native-bootstrap-linux-arm64` | 241,488 | `b4307c244edc9e4da620f2a7c1b03a733e575da032efefae615f9edf75048a37` |
| `type-rb-native-bootstrap-manifest-v1.json` | 1,957 | `a46d8c789f661a96aa38b1d4b9fd9ee21e46ccb3f4f2303a4f4d43caae1701b0` |
| `SHA256SUMS` | 422 | `c301b72a1f6c2c844c6498868cd6d23a754f1082cd8051eb6fbd0b332cc6e189` |

The two platform compilers total 500,520 bytes, below the registered 620,000
byte ceiling; each is below its 310,000 byte ceiling. The complete five-asset
set is 1,161,538 bytes. An ordinary target downloads only 261,411 bytes on
Darwin or 243,867 bytes on Linux, excluding network protocol overhead and the
separately built QBE tool.

The checked-in [`release`](release) directory retains the exact manifest,
checksum index, and target metadata without committing the compiler binaries
or root QBE to Git history.

## Bootstrap and correctness result

The initial ceremony translated the registered root QBE with QBE 1.3 and the
target system CC. On each fresh runner, root-built B1 and generated B2, B3, and
B4 were byte-identical. Every generation emitted the exact registered
fixed-point QBE.

The post-publication jobs downloaded only the target compiler, manifest, and
checksum index. On both targets the downloaded B1 and generated B2, B3, and B4
were byte-identical:

| Target | B1/B2/B3/B4 bytes | B1/B2/B3/B4 SHA-256 |
| --- | ---: | --- |
| Darwin arm64 | 259,032 | `ef438d13598c534766334b408a39715c56ff1b69db528910ebf7d90ec7720b65` |
| Linux arm64 | 241,488 | `b4307c244edc9e4da620f2a7c1b03a733e575da032efefae615f9edf75048a37` |

B4 checked the canonical three-file compiler closure, emitted the 658,639-byte
fixed point, and checked every retained valid, invalid, mutation, and
runtime-invalid fixture. QBE and CC failure probes preserved existing outputs;
unsupported targets launched no tool; success and failure left no Native
intermediate. The configured Gate 6K project built and printed exactly
`configured-project-ok`, including from paths containing spaces.

The Linux ordinary process trace contains 18 `execve` records for the Native
compiler, QBE, CC, assembler, `collect2`, and linker boundary. It contains no
Go, reference `trb`, recovery compiler, or shell-mediated Native compiler
child. Darwin records the same explicit QBE and `/usr/bin/cc` child boundary;
the compiler invokes both directly.

## Integrity and provenance result

Before execution, both post-publication jobs required all of the following:

- an immutable, non-draft experimental prerelease with the exact tag, source
  revision, five asset names, byte sizes, and GitHub asset digests;
- strict manifest version 1 with no missing or unknown members;
- exact manifest and target entries matching `SHA256SUMS` and the downloaded
  compiler bytes;
- GitHub SLSA provenance attestations for the compiler, manifest, and checksum
  file;
- signer workflow
  `.github/workflows/bootstrap-seed-initial.yml`, source ref
  `refs/heads/main`, source commit
  `0058818314977633c50393796ef9b9f8f1fda50f`, event
  `workflow_dispatch`, and a GitHub-hosted runner.

Executable mode was restored only after those checks. The retained attestation
JSON includes the transparency-backed verification result and invocation id
for the initial ceremony.

QBE remains an explicit external dependency. Its source archive is exact on
all runs. The Darwin QBE executable built from that source had SHA-256
`8e6a433091aa7c85e6ecab54c3f53495a3ee5962c58f1e49b1f4367828af17ae`
during the initial ceremony and
`5d459adb0f49d7ff655cc06c6f161324db1506716231ac7ef67b93a196984391`
during final verification, while both runs produced the exact same compiler
bytes and fixed-point QBE. Linux reproduced QBE executable SHA-256
`510f15a1c724c204141d0b7531fe1641b983fe41dd008f5806470709a79a746c`.
Gate 6L therefore pins and verifies QBE source plus compiler output; it does not
claim cross-runner byte reproducibility for the externally built QBE executable.

## Measurements

Every series uses two warmups and seven retained observations. The final
post-publication measurement interleaves B1-to-B2, B2-to-B3, and B3-to-B4 in
each observation round. Elapsed values are seconds and RSS values are bytes.

Initial root ceremony:

| Target and metric | B1-to-B2 | B2-to-B3 | B3-to-B4 | Largest adjacent spread |
| --- | ---: | ---: | ---: | ---: |
| Darwin elapsed | 0.94 | 0.91 | 0.91 | 3.30% |
| Darwin peak RSS | 36,077,568 | 36,126,720 | 35,995,648 | 0.36% |
| Linux elapsed | 0.37 | 0.37 | 0.37 | 0.00% |
| Linux peak RSS | 19,963,904 | 19,963,904 | 19,963,904 | 0.00% |

Final published-seed verification:

| Target and metric | B1-to-B2 | B2-to-B3 | B3-to-B4 | Largest adjacent spread |
| --- | ---: | ---: | ---: | ---: |
| Darwin elapsed | 1.10 | 1.13 | 1.08 | 4.63% |
| Darwin peak RSS | 35,930,112 | 36,175,872 | 36,061,184 | 0.68% |
| Linux elapsed | 0.37 | 0.37 | 0.37 | 0.00% |
| Linux peak RSS | 19,963,904 | 19,963,904 | 19,963,904 | 0.00% |

All values pass the 25% adjacent-median bound with substantial headroom and
are far below the catastrophic 2x threshold. The published asset download took
one second on Darwin and two seconds on Linux; manifest, release, digest, and
three attestation checks took 11 seconds on each target. Those network-backed
latencies remain advisory.

## Recorded corrections

The first initial workflow run stopped safely before compiler execution because
its read-only workflow token could not list a draft release. GitHub documents
draft listings as visible only to callers with push access. PR
[#93](https://github.com/type-rb/type-rb-native/pull/93) limited the required
contents-write permission to the root-reading jobs and strengthened the exact
draft target checks; the registered root asset did not change.

The first post-publication run verified the release and attestations on both
targets and completed Linux, but Darwin's grouped timing series exceeded the
elapsed bound.
It also exposed that failed medians were not retained. PR
[#95](https://github.com/type-rb/type-rb-native/pull/95) kept the bound, two
warmups, and seven observations unchanged, interleaved generations to avoid
assigning shared-runner drift to one series, and made failed evidence upload
unconditional. The final fresh run then passed on both targets. The failed
[run 33280216317](https://github.com/type-rb/type-rb-native/actions/runs/33280216317)
remains part of the public history rather than being omitted from the result.

## Raw evidence

- [`initial/darwin-arm64`](initial/darwin-arm64) and
  [`initial/linux-arm64`](initial/linux-arm64) retain the initial fixed-point,
  measurements, environment, executable identity, dependencies, and process
  inventory.
- [`published/darwin-arm64`](published/darwin-arm64) and
  [`published/linux-arm64`](published/linux-arm64) additionally retain the
  immutable release response, exact attestation verification JSON, download
  and verification time, and verifier revision.
- [`EVIDENCE_SHA256SUMS`](EVIDENCE_SHA256SUMS) covers all 58 raw evidence and
  release-metadata files committed with this result.

## Conclusion and deferred scope

Gate 6L establishes a durable, verifiable, Go-free ordinary bootstrap handoff
for the current experimental Darwin and Linux arm64 profiles. The one-time root
remains visible as setup provenance, while normal consumers need only the
previous Native compiler plus external QBE and system toolchain.

This result does not define Native SemVer, TypeRB compatibility ranges, stable
target support, installation, signing-key custody, bundled external tools,
static linking, x86-64, production support, or recovery guarantees. Updating
the pinned TypeRB revision or any seed artifact requires independent
revalidation; an immutable release is never changed in place.
