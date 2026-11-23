# Native MIR Array-loop recovery workload

This focused workload measures the exact verified `Array<Integer>` induction
and reduction plan registered by issue #232. It is not a language benchmark.

The program constructs 100,000 integers, calls the selected reduction 1,000
times, and must print the checked-in expected output. The controller runs each
direct executable twice for warmup, retains 21 alternating observations, uses
an in-process monotonic clock and per-process resource usage, and rejects output
or diagnostic differences.

Acceptance requires candidate/baseline wall and CPU medians at or below 0.75,
peak-RSS median at or below 1.05, and every retained wall, CPU, and RSS value for
both executables below 2.0 times the corresponding baseline median. Formal CI
also requires strict generated QBE and code-section shrinkage.
