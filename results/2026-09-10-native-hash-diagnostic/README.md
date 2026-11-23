# Hash implementation diagnostic

This is local diagnostic evidence for [#400](https://github.com/type-rb/type-rb-native/issues/400),
not an accepted optimization checkpoint. Ordinary cost limits were unchanged
throughout these diagnostic cohorts. The later
[Hash acceptance decision](../../docs/decisions/0033-hash-compiler-budget.md)
records the reviewed policy separately; it does not relabel these observations.

## Registration and conditions

- [Initial bounded cohort](https://github.com/type-rb/type-rb-native/issues/400#issuecomment-5616373433):
  [initial.json](initial.json), 162 observations including warmups and workload builds.
- [String byte-reader refinement](https://github.com/type-rb/type-rb-native/issues/400#issuecomment-5616628846):
  [byte-reader.json](byte-reader.json), 24 paired observations.
- [Final scalar-pool refinement](https://github.com/type-rb/type-rb-native/issues/400#issuecomment-5616813863):
  [scalar-pool.json](scalar-pool.json), 24 paired observations. No further tuning
  cohort is included or implied.

Hardware was an Apple M2 Pro with 32 GiB RAM, Darwin arm64, warm OS caches,
one warmup and five measured repetitions per case/role. Compiler recovery and
validation overlapped these runs on an uncontrolled host. Wall time, CPU time,
maximum RSS, output, status, input source, tool versions and binary digests are
retained in the JSON files. RSS is process high-water resident memory, not live
Hash storage. These measurements do not establish parity with other languages
or current Go performance headroom.

The initial comparison uses accepted control source
`7014e1daa1c4acd79301e9425a0361321503f751` and frozen cumulative source
`ac633935a7f248470c59d22666da14c819a131fa`. Its candidate was a prototype:
extract the `patch` field of [initial-core.json](initial-core.json) and apply it
to core source at
`7acb9dff192dd5a25ee0839bcecf2610f13bea14` to reproduce the recorded core source
digests. Initial CLI evidence retains the executable digest and inputs, but
exact CLI source reconstruction was not established. Do not treat that initial
CLI measurement as a reproducible final-source comparison.

The final scalar-pool pair has exact source identities: control
`7acb9dff192dd5a25ee0839bcecf2610f13bea14`, candidate
`0004b86928a47a49181066160c90717e8f481a7d`. Later MIR validation changes at
`2a5e2ac54795226deb1a62db8cba1358afaf3411` do not change the Hash runtime or
REPL pool representation, but have a different binary identity. Final source
and artifact sizes are recorded in [metadata.json](metadata.json); timings
remain attached to the measured revisions, rather than being relabeled.

## Application behavior and memory

Each workload inserts 100,000 distinct pairs, observes the size and final key,
deletes every entry, then observes size zero. String workloads construct decimal
String keys and values. All measured outputs are checked. Compiled application
time excludes compilation; REPL time includes starting and evaluating a session.

| Workload | Compiled time | Compiled maximum RSS | REPL time after final refinement | REPL maximum RSS |
| --- | ---: | ---: | ---: | ---: |
| Integer keys and values | 0.020 s | 10.4 MiB | 2.266 s | 118.0 MiB |
| String keys and values | 0.048 s | 18.0 MiB | 3.142 s | 163.1 MiB |

Compiled columns come from the initial cohort and REPL columns from the final
paired cohort; they are not a simultaneous cross-mode comparison. The compiled
runtime uses two words per bucket and no entry boxes. The REPL uses pooled value
identities and retains evaluation temporaries until a submission finishes, so a
large loop has materially higher peak memory than its live Hash table. This is
a known interpreter-pool limitation, separate from Hash bucket reclamation.

The byte reader removes per-character String allocation while hashing: paired
String REPL time fell from 11.336 s to 3.407 s. Sharing the read-only empty child
list of scalar pool entries subsequently reduced Integer maximum RSS from
218.0 MiB to 118.0 MiB and String maximum RSS from 273.5 MiB to 163.1 MiB.
Those final paired times were 2.585 s to 2.266 s and 3.435 s to 3.142 s.

Behavior tests separately verify cleared deleted cells, empty backing release,
sparse-table shrink, managed aliases and cyclic graphs. Automatic GC and final
live-byte accounting are asserted. Clearing references makes detached storage
collectable; it does not require an immediate decrease in OS RSS.

## Compiler cost and integration status

| Measured Darwin artifact | Bytes | Applicable ordinary limit | Result |
| --- | ---: | ---: | --- |
| Complete core executable | 398,888 | 366,000 | exceeds |
| Core text | 290,856 | observed in ordinary mode | diagnostic |
| Core QBE text | 1,328,312 | observed in ordinary mode | diagnostic |

Review correction: the 250,904-byte text and 1,120,000-byte QBE limits
apply to named historical transitions, not this ordinary comparison. Their
values and enforcement remain unchanged. The complete executable ceiling
and ordinary 1.05 compiler-size ratio still fail.

The accepted control core is 365,816 bytes: current growth is 33,072 bytes
(9.0%). The current CLI is 548,872 bytes versus a 482,744-byte accepted CLI.
Required QBE is another 406,656 bytes, with its digest retained in the initial
report. Core, CLI and QBE together are 1,354,416 bytes; the platform linker and
system libraries remain required host tooling and are not included in that
subtotal. Application executables use the embedded runtime and system libc;
their exact sizes and digests are recorded separately in the observations.

The diagnostic ceilings (core 430,000; CLI 590,000; text 310,000; QBE text
1,450,000 bytes) permit measurement only. They do not replace ordinary limits.
No current Linux or complete cross-platform acceptance result is claimed.
Integration therefore remains blocked on explicit cost review and full
acceptance, even where correctness and local runtime checks pass.

Initial same-source self-build medians were 1.872 s control and 1.887 s
candidate (about 0.8% overhead). Own-source builds were 1.601 s frozen,
1.716 s control and 1.887 s candidate; these include source growth and are
reported separately. The three unchanged application build cases and their
individual raw observations are retained in `initial.json`.

## Reproduction

Use [the measurement tools](../../tools/native-hash/README.md) with the
recorded sources, exact accepted seed, QBE and platform toolchain. A fresh run
produces new diagnostic evidence; it must not overwrite these historical files.
Correctness commands and supported boundaries are documented in
[native-hash.md](../../docs/native-hash.md).
