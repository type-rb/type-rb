> Lifecycle: [complete source-era record](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-09-01-persistent-worker-memory-darwin-linux-arm64/README.md).
> Historical checksum inventories refer to that full record, not this compact working set.

# Persistent Worker Memory Lifecycle Result on Darwin and Linux arm64

The registered single-threaded persistent worker lifecycle passes every
correctness, managed-memory, process-stability, independent-oracle,
fixed-point, build-regression, and compiler-size criterion.

The formal Linux arm64 run processes 460,800,000 original jobs and 489,600,000
attempts while exercising success, retry, terminal failure, cancellation,
bounded retained state, phase drains, and graceful shutdown. It allocates and
reclaims exactly 33,926,400,576 managed bytes, ends with zero live managed
bytes, keeps its sampled live set below 11 KiB, records zero quartile-median
RSS growth, and keeps descriptor and thread counts fixed.

This is evidence for the exact synthetic worker lifecycle below. It is not a
Web-server, concurrent-worker, socket, connection, timeout, thread-pool,
finalizer, weak-reference, or arbitrary external-resource cleanup result.

## Exact scope and identities

- preregistered scope and thresholds:
  [issue #150](https://github.com/type-rb/type-rb-native/issues/150);
- implementation and review:
  [PR #151](https://github.com/type-rb/type-rb-native/pull/151);
- exact current-main baseline:
  `caedf3082959337bf40b892a31e8a2d400f2f20b`;
- measured Native revision:
  `f16635e9297e1933786c18586b45d1d1cf8986c5`;
- pinned TypeRB source and semantic oracle:
  `5dc09070cf7f88a569279f5e63982a6de59d692c` (`0.4.4-dev`);
- immutable predecessor seed:
  [`bootstrap-seed-2026-08-30`](https://github.com/type-rb/type-rb-native/releases/tag/bootstrap-seed-2026-08-30);
- QBE 1.3 source archive SHA-256:
  `d587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320`;
- successful formal lifecycle workflow:
  [33412872678](https://github.com/type-rb/type-rb-native/actions/runs/33412872678);
- successful current-main build/RSS regression workflow:
  [33412703306](https://github.com/type-rb/type-rb-native/actions/runs/33412703306); and
- targets: `darwin-arm64-v0` on `macos-15` and `linux-arm64-v0` on
  `ubuntu-24.04-arm`.

One authored TypeRB source is compiled by both Native and the pinned optimized
Go backend. Each batch starts with 128 jobs and deterministically produces 112
successes, eight retry transitions, eight terminal failures, eight
cancellations, and 136 processed attempts. A 64-entry recent-payload cache is
the only state retained across batches. Native and Go emit the exact same 60
phase markers and final success line.

## Managed-memory result

The formal run reports 32,662 collections, of which 32,661 are automatic. The
trace retains every 64th automatic collection and the final manual collection.

| Metric | Result | Registered condition |
| --- | ---: | ---: |
| Original jobs | 460,800,000 | exact |
| Processed attempts | 489,600,000 | exact |
| Allocated bytes | 33,926,400,576 | at least 30 GiB |
| Reclaimed bytes | 33,926,400,576 | `allocated = reclaimed + live` |
| Final live bytes | 0 | 0 |
| Peak managed heap | 1,048,574 B | at most 4 MiB |
| Complete GC observations | 511 | at least 400 |
| Automatic observations | 510 | positive |
| Maximum sampled live bytes | 11,212 B | at most 128 KiB |
| Maximum logical roots | 9 | reported |
| Root capacity | 64 words | fixed at 64 |

Both 5,120,000-job target smokes allocate and reclaim 376,960,576 bytes, end
at zero live bytes, keep a 1,048,566-byte peak, and observe the same 11,212-byte
maximum sampled live set and 64-word root capacity. Darwin completes the smoke
in 0.88 seconds and Linux in 0.51 seconds on their respective hosted runners.

## Process stability and Go context

The Linux formal controller samples process state every 250 ms and excludes
the first six authored phases from trend decisions.

| Native metric | Result | Registered limit |
| --- | ---: | ---: |
| Raw samples | 177 | retain all complete samples |
| Post-warmup samples | 159 | positive sample set |
| Maximum RSS | 2,338,816 B | at most 64 MiB |
| First temporal-quartile median | 2,338,816 B | reported independently |
| Last temporal-quartile median | 2,338,816 B | at most first + 8 MiB |
| Quartile-median growth | 0 B | at most 8 MiB |
| Fitted RSS slope | 4,967.557339 B/min | at most 1 MiB/min |
| Descriptor range | 5 to 5 | no post-warmup growth |
| Thread range | 1 to 1 | no post-warmup growth |

Native completes in 46.37 seconds; the exact optimized-Go control completes in
14.67 seconds. Native is therefore about 3.16 times slower for this
allocation-heavy synthetic workload. The Go result is context rather than a
memory acceptance baseline: it reaches 10,670,080 bytes maximum RSS, retains
eight descriptors, and varies between seven and eight threads while emitting
the exact Native output.

## Independent memory oracles

The reduced Linux ASan/LSan mode allocates and reclaims 3,770,176 managed
bytes, ends at zero live bytes, and reports no sanitizer or leak finding. This
covers allocator interception and instrumented linked code, not QBE
instructions.

Valgrind 3.22.0 observes 61,208 allocations and 61,207 frees across 5,383,968
bytes. It reports zero definitely, indirectly, or possibly lost bytes and zero
errors. The one still-reachable 512-byte block is enumerated without a
suppression: it is the bounded 64-word temporary-root buffer, which remains
globally reachable until process exit.

## Fixed point, process boundary, and compactness

After two compatibility transitions, B2, B3, and B4 compiler executables are
byte-identical on each target. Darwin's 283,080-byte compiler has SHA-256
`03ee5b3dd5fcee9889741bd29cc700e768f4156f7499b5d3c2ae0312ac11ee34`;
Linux's 253,632-byte compiler has SHA-256
`0a440ae99831a41049e72b13a4c54773374fa2743c719df4fce5f8206ed4d0c2`.
Both targets repeatedly emit the same 874,387-byte target-neutral compiler QBE
with SHA-256
`5ec2521a18af002333ecfeb1e16d754e102a342846a965186ba847e19ab5b74e`.

Every Linux setup transition and ordinary generation has an independent
process trace. The setup transitions use the published Native seed or its
successor, QBE, `/usr/bin/cc`, `/usr/bin/as`, `collect2`, and `/usr/bin/ld`.
B2, B3, and B4 use only the corresponding Native compiler, the same QBE/CC/
assembler/`collect2` paths, and `/usr/bin/ld.lld`. The verifier rejects all
unregistered successful executables; no Go, reference `trb`, shell, recovery
generator, or hidden source-content path occurs in any trace.

The formal workflow strips copies before enforcing the registered size limits:

| Target | Stripped compiler | Limit | Headroom |
| --- | ---: | ---: | ---: |
| Darwin arm64 | 283,120 B | 290,000 B | 6,880 B |
| Linux arm64 | 253,624 B | 260,000 B | 6,376 B |
| Combined | 536,744 B | 550,000 B | 13,256 B |

The separate interleaved current-main regression uses two warmups and fourteen
pooled retained builds for the byte-identical B2-to-B3 and B3-to-B4
adjacencies. It passes the stricter existing compiler-size limits and the
registered 1.05 build/RSS ratios:

| Target | Compiler bytes baseline/candidate | Build-time ratio | Peak-RSS ratio |
| --- | ---: | ---: | ---: |
| Darwin arm64 | 283,080 / 283,080 | 0.993846 | 1.002592 |
| Linux arm64 | 252,224 / 253,632 | 1.000000 | 0.999873 |

Every retained adjacent observation also remains below the unchanged 2x
catastrophic boundary. The current-main comparison's raw compiler pair is
536,712 bytes, below its stricter 540,000-byte combined guardrail.

## Scope boundary

This result establishes bounded managed heap and stable baseline descriptors
and threads for one single-threaded, non-I/O persistent worker whose retained
state and shutdown behavior are expressed entirely in the existing TypeRB
subset.

It does not establish production Web or Job APIs, concurrent mutation,
long-lived sockets, connection churn, timeout ownership, thread pools,
finalizers, weak references, closure-cycle collection, or cleanup of external
resources. Those require separate portable adapter ownership and shutdown
contracts before representative service evidence is meaningful.

## Retained evidence

[`ARTIFACTS.tsv`](ARTIFACTS.tsv) identifies all six GitHub artifact archives
from the formal lifecycle and current-main regression workflows. Extracted
evidence is retained without compiler or workload executables:

- [`darwin-arm64`](darwin-arm64) contains fixed-point QBE and identities,
  direct build inventory, smoke workload/output, GC trace, collector totals,
  and workflow context;
- [`linux-arm64`](linux-arm64) adds all five generation process traces, the
  complete 3,066-line formal GC trace, Native and Go raw process series,
  ASan/LSan output, and complete Memcheck output;
- [`cross-target.txt`](https://github.com/type-rb/type-rb-native/blob/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results/2026-09-01-persistent-worker-memory-darwin-linux-arm64/cross-target.txt) records the independently stripped
  compiler pair; and
- [`build-regression`](build-regression) contains all interleaved current-main
  observations, decisions, catastrophic checks, bootstrap identities, and
  target-neutral QBE and size evidence.

[`EVIDENCE_SHA256SUMS`](EVIDENCE_SHA256SUMS) covers every retained evidence
file and `ARTIFACTS.tsv`, excluding this README and the checksum file itself.
