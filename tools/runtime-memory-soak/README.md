# Runtime memory stability harness

This harness verifies the ordinary self-hosted compiler's managed runtime under
sustained allocation. It implements the frozen criteria in
[issue #104](https://github.com/type-rb/type-rb-native/issues/104) without
changing portable TypeRB behavior.

`workload.trb` is the single authored workload. Its registered invocation is
the 60-phase, 300,000,000-iteration formal soak. The harness derives the CI
smoke and reduced memory-oracle inputs by replacing that one invocation with an
exact checked variant; the workload body is never duplicated or generated.
Every allocation phase creates dynamic Strings, four-element Integer Arrays,
and managed records across nested calls, then retains only an Integer checksum.
An allocation-idle Integer loop and one fixed marker follow every phase.

The harness has four modes:

- `smoke` runs one 5,000,000-iteration phase, checks the 4.25-second ceiling,
  and enforces the registered collector invariants;
- `formal` runs 60 phases on Linux arm64, samples production-binary RSS every
  250 ms, and enforces the absolute, temporal-quartile, and fitted-slope bounds;
- `asan` links a dedicated Linux arm64 QBE output with Clang ASan/LSan and runs
  50,000 iterations; and
- `valgrind` runs the 50,000-iteration input under Memcheck with every leak
  class visible and no broad suppression. The full log must enumerate any
  still-reachable storage; the harness permits only the collector's one bounded
  512-byte temporary-root buffer and records that justification explicitly.

All modes require a current self-hosted Native compiler, QBE 1.3, a system C
driver, a target profile, a fresh workspace, and a fresh evidence directory:

```text
/bin/sh tools/runtime-memory-soak/runtime-memory-soak.sh \
  smoke COMPILER QBE CC darwin-arm64-v0 WORKSPACE EVIDENCE
```

The output evidence includes the exact derived source, QBE IL, tool and artifact
hashes, compiler size, runtime output, collector statistics, and environment.
Formal mode additionally retains every RSS sample and both raw trend values.
ASan/LSan intercept allocator and linked-toolchain behavior; QBE instructions
themselves are not described as sanitizer-instrumented.
