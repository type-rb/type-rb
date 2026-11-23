# Decision 0027: Formal Build and Distribution Controller

## Status

Accepted for the initial formal TypeRB backend-pair build measurements.

## Context

The first formal Benchmarks Game result measures fresh application processes.
It cannot answer how much compiler work each TypeRB backend performs or what a
user must install to build and deploy those applications. Reusing runtime
observations for either answer would omit the Native QBE/C toolchain boundary,
the Go compiler distribution, and build-process memory.

Compiler measurements also have a different write and cache boundary from
runtime measurements. Both backends create a fresh output, but the Go toolchain
uses a persistent compiler cache. Resetting that cache for every observation
would measure toolchain installation repeatedly; allowing an uncontrolled home
directory would make the result irreproducible.

## Decision

Formal build measurement is a separate dispatch-only Linux arm64 workflow. One
fresh runner measures one admitted benchmark case. The Native and Go backends
compile the byte-identical checked-in TypeRB source with the same portable
semantics. Each case performs two warmup rounds and eleven retained rounds,
alternating which backend builds first. The output is deleted before every
observation. The Go build and module caches are explicit directories shared by
the warmups and retained observations; the Linux page cache is dropped before
every measured process.

BenchExec `runexec` 3.35 measures the complete compiler process tree on the same
four logical CPUs with a 4 GB memory limit and a 300 second wall limit. `/` is
read-only, `/home` is an isolated overlay, `/tmp` uses BenchExec's default hidden
writable temporary-directory mode, and only the registered measurement
workspace is otherwise writable. Every observation records status or signal,
termination reason, wall time, CPU time, peak process-tree memory, compiler log,
artifact size and SHA-256, and a post-build small-input correctness check.
Failures remain in the raw schedule and prevent a passing median.

Every retained artifact hash remains in raw evidence. The summary reports its
size median and range, the number of distinct size/hash variants, and whether
all retained artifacts are byte-identical. A byte-varying but correct compiler
result is reported as non-reproducible instead of being reclassified as a
missing timing result.

Before timing, both backends must check, build, and execute the case correctly.
Those representative builds are process-traced outside the measured schedule.
The trace parser joins concurrent `unfinished` and successful `resumed`
`execve` records before producing a candidate-scoped resolved executable
inventory. Tool versions and a dynamic-library record for every resolved
observed executable describe the build closure.

Distribution evidence uses three explicit scopes rather than one ambiguous
number:

- the TypeRB-controlled build payload records raw and stripped Native compiler,
  QBE, and reference `trb` binaries, plus the pinned Go `GOROOT` tree required
  by the reference build;
- external host prerequisites record the C driver, assembler, linker, every
  successfully executed tool, and their dynamic libraries without implying
  that TypeRB owns or bundles the host toolchain; and
- deploy artifacts record raw and stripped applications and their dynamic
  libraries for each backend.

Directory and file totals retain their component rows. Shared host files are
not folded into a cross-backend composite score, and runtime values are not
mixed with build values.

## Consequences

The result can attribute compiler wall time, CPU, RSS, and generated artifact
size to two builds of the same TypeRB program. It also makes the current reason
the Go backend needs a Go installation, and the current Native dependence on
QBE and a platform C/linker toolchain, independently auditable.

The Go cache is warm after the registered warmups while the requested
application output is always clean. This is a deliberate compiler-throughput
measurement, not first-install latency. Cross-language compiler speed remains
outside the authoritative backend pair because those candidates use different
authored programs and build systems. Persistent-service, Web, Job, allocation,
and I/O performance remain separate workloads.
