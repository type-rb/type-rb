# Formal Benchmarks Game runtime controller

This directory owns the preregistered fresh-process runtime measurement core
for the language benchmark plan in [`docs/benchmarksgame.md`](../../docs/benchmarksgame.md).
It does not contain a published performance result.

## Fixed policy

- BenchExec [`runexec` version `3.35`](https://github.com/sosy-lab/benchexec/releases/tag/3.35);
- release wheel SHA-256:
  `af196e0fa5715038a81c4ac25e7427cf312560da8664dc40e5da97454af33a91`;
- release Debian package SHA-256:
  `b6e42daf63a0284b597f1d05f0daf7690381f7ca9ebe21d0b5083495d01bbfe0`;
- candidates, in rotation order: TypeRB through Native, TypeRB through Go,
  C, C++, Go, Rust, and Java;
- two warmup rounds followed by eleven retained rounds;
- one-core and four-core lanes with explicit logical CPU identifiers;
- `4GB` process-tree memory limit;
- wall-time limits of `300s` for `fannkuch-redux` and `n-body`, and `600s`
  for `spectral-norm`;
- default BenchExec container networking disabled, `/` visible read-only, and
  `/home` isolated through a non-persistent overlay; and
- swap disabled and Linux page cache dropped before every measured process.

Candidate order rotates by one position per round. Every candidate therefore
occupies every order position before any position is repeated. Warmup rows are
retained in `raw.tsv` but excluded from `medians.tsv`.

## Catalog contract

The controller accepts one eight-column TSV catalog:

```text
case  candidate  command  arg1  arg2  arg3  arg4  expected
```

Fields are tab-separated. `-` means an omitted argument. Each case must contain
the seven fixed candidates in the fixed order. Commands are executed directly,
without `eval` or a timing wrapper process. This allows ordinary executables
and Java's command/classpath form while keeping the measured command explicit.

Before timing, every catalog row is executed once with separate stdout and
stderr. Status must be zero, stderr empty, and stdout byte-identical to the
published expected output. Any failure stops the run before a timing claim.

[`prepare-runtime.sh`](prepare-runtime.sh) composes the existing identical-
TypeRB and pinned-context verifiers. It builds all 21 artifacts, emits the
catalog, checks either the small smoke oracle or registered performance oracle
for every selected catalog row, and records source, toolchain, and artifact
identities. `--case CASE` emits one seven-candidate case for an isolated formal
job; its default `all` emits all three cases for local capability checks. Formal
preparation requires Linux arm64 and `linux-arm64-v0`; smoke mode exists for
capability testing on either registered arm64 target. The formal controller
additionally rejects a catalog whose performance input shape or expected-output
SHA differs from the registered case.

## Evidence and failures

Each BenchExec observation retains:

- cache state before and after the requested drop;
- the raw BenchExec result and diagnostics;
- the raw command log and extracted program payload;
- process return or termination reason;
- wall time, CPU time, and process-tree peak memory; and
- warmup/retained round, rotation position, lane, case, and candidate.

A measured timeout, signal, nonzero return, missing metric, or output mismatch
does not stop later candidates. The controller finishes the complete schedule,
marks that candidate incomplete instead of calculating a passing median, then
returns failure with all raw evidence still present.

`test` mode exists only for the deterministic fake-`runexec` regression test.
`formal` mode additionally requires Linux arm64, the exact BenchExec version,
and CPU identifiers allowed by the current cgroup. A formal invocation is:

```text
/bin/sh tools/benchmarksgame-formal/prepare-runtime.sh formal \
  --native-compiler COMPILER --reference-trb TRB --qbe QBE --cc CC \
  --target linux-arm64-v0 --source-archive ARCHIVE --cxx CXX --go GO \
  --rustc RUSTC --javac JAVAC --java JAVA --unzip UNZIP \
  --workspace PREP_WORKSPACE --evidence PREP_EVIDENCE --catalog CATALOG \
  --case CASE

/bin/sh tools/benchmarksgame-formal/runtime-controller.sh \
  formal RUNEXEC CATALOG CASE one-core CORES \
  tools/benchmarksgame-formal/cache-control-linux.sh WORKSPACE EVIDENCE
```

The controller measures whole fresh processes, including runtime startup and
JIT activity. It does not represent steady-state service latency. Compiler
time, build RSS, raw/stripped artifacts, and distribution inventory remain a
separate formal controller so a build failure cannot contaminate runtime data.

The [measurement-host procedure](../../docs/benchmarksgame.md#measurement-host-procedure)
requires reviewing the host/setup context and complete sample spread before a
formal claim. A fresh hosted runner and serial controller prevent competing
project commands in that job; they cannot prove that the physical host is
otherwise idle. Clear interference makes the cohort inconclusive, and a new
complete run must retain the earlier evidence.

The dispatch-only
[`benchmarksgame-formal.yml`](../../.github/workflows/benchmarksgame-formal.yml)
workflow assigns each case to a fresh `ubuntu-24.04-arm` runner. It closes the
current self-hosted Native compiler chain from the immutable published seed,
enables the current systemd user service's `cpuset` delegation when the hosted
runner omits it, prepares one seven-candidate catalog, probes BenchExec and
cgroup isolation, runs both lanes on that same host, and uploads all raw
evidence even when a measurement fails. The delegation helper records the
exact cgroup path and controller state before and after the narrowly scoped
change. A non-measured discovery probe lets BenchExec create its transient
`benchexec.slice`; the helper then verifies and, when necessary, enables the
same controller on that nested boundary before the strict probe. Merging the
controller does not itself publish or schedule a performance claim.
