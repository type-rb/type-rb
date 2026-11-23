# Formal TypeRB backend-pair build controller

This directory owns the compiler/build/distribution layer registered in
[`docs/benchmarksgame.md`](../../docs/benchmarksgame.md) and
[Decision 0027](../../docs/decisions/0027-formal-build-distribution-controller.md).
It does not contain a performance result.

## Fixed policy

- cases: `fannkuch-redux`, `n-body`, and `spectral-norm`;
- candidates: the ordinary self-hosted Native compiler and pinned Go `trb`;
- byte-identical authored TypeRB source and the published small correctness
  oracle for each candidate;
- BenchExec `runexec` 3.35 on Linux arm64;
- the same four logical CPUs and a 4 GB process-tree memory limit;
- two warmup rounds followed by eleven retained alternating rounds;
- persistent, explicit Go build/module caches populated by the warmups;
- a deleted output and Linux page-cache reset before every observation; and
- read-only `/`, an isolated `/home`, BenchExec's default hidden writable
  temporary directory, and one writable measurement workspace.

Each observation retains the raw BenchExec log and metrics, build artifact
size/hash, and an untimed post-build correctness result. Summaries report the
artifact-size median and range, distinct byte-variant count, and whether all
retained artifacts are byte-identical. Artifact variation remains evidence; it
does not discard otherwise correct compiler measurements. A failed build,
timeout, signal, missing metric, missing artifact, or incorrect generated
program remains in `raw.tsv`. The complete schedule runs before the controller
returns failure.

## Distribution boundary

Preflight builds are process-traced outside timing. Evidence records the raw
trace, candidate-scoped successfully executed tools (including split
`unfinished`/`resumed` strace records), per-tool dynamic dependencies, tool
versions, raw and stripped compilers/applications, the complete pinned Go
`GOROOT` apparent size and file count, and payload totals. Native compiler plus
QBE and reference `trb` plus Go are reported separately. The platform C driver,
assembler, linker, and shared libraries remain explicit external host
prerequisites.

The workflow is dispatch-only. Merging it neither schedules a formal run nor
publishes a value. A formal invocation has this shape:

```text
/bin/sh tools/benchmarksgame-build-formal/build-controller.sh \
  formal RUNEXEC CASE NATIVE_COMPILER REFERENCE_TRB QBE CC GO \
  linux-arm64-v0 CPU0,CPU1,CPU2,CPU3 \
  tools/benchmarksgame-formal/cache-control-linux.sh WORKSPACE EVIDENCE
```

`test` mode exists only for the deterministic controller regression test.
