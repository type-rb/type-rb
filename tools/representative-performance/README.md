# Representative application and input coverage

This opt-in diagnostic implements the [performance policy](../../docs/performance-policy.md).
It never modifies daily/weekly schedules, baselines, thresholds or published
victory counts. `suite.json` registers all input variants before measurement.

The single TypeRB executable contains four application workloads and four
sensitivity families. Runtime arguments select workload, count, variant and
batch count. Go uses ordinary standard-library facilities with matching output,
Unicode code-point counts and observable retained lifetimes. A separate Python
oracle checks every batch and final retained-data checksum. Setup is included
in process wall/CPU/RSS, while the `ready` marker separates it from batch-arrival
observations. `json-transform` scales the number of small documents. `json-document` instead
scales one document: ASCII, Unicode and escaped scalar strings, or wide arrays
and objects. Numeric variants separately cover integer arrays, fractional and
exponent arrays, and objects mixing these forms. A separate precise-fractional
array cycles through signed values and different scales with about 17 significant
digits, alongside the shorter fractional forms. Their source spellings and
decoded values match across implementations; the Python oracle independently
checks the canonical output length. Its count is the number of repeated text
units or container members;
each batch parses and serializes the entire document. Input generation precedes
`ready` but remains included in process costs. These are new input cases, not
replacements for the existing small-document history.

Input parsing operates in memory; file/network I/O is not measured.

## Run locally

Build the current Native CLI once using the repository's ordinary procedure.
Reuse the pinned reference CLI and a shared Go build cache if comparing controls.
The controller never builds a compiler and deletes all temporary application
binaries/generated Go projects when finished or failed.

```sh
python3 tools/representative-performance/measure.py \
  --native bin/trbn --smoke --evidence /tmp/representative-smoke

python3 tools/representative-performance/measure.py \
  --native bin/trbn --reference /path/to/pinned/trb --go /path/to/go \
  --gc-probes --evidence /tmp/representative-diagnostic

python3 -m unittest discover -s tools/representative-performance -p 'test_*.py'

python3 tools/representative-performance/report.py /tmp/representative-diagnostic/manifest.json
```

`--evidence` must be new. `--case ID` selects registered cases and is repeatable.
`--samples N` fixes 1..100 retained fresh processes before a run; the default is
three, after one first process/warmup per role. `--profile registered` keeps the
original batches (the default). `--profile sustained-v1` multiplies small, medium
and large batch counts by 200, 40 and 8 respectively; the already sustained
100-batch medium worker stays unchanged. Counts are fixed before execution,
without calibration, adaptive repetition or retries. Working-set dimensions
and algorithms stay the same; retained worker state evolves across the extra
batches. This profile reduces the fraction of startup costs but does not prove
stable timings. It has its own exact input and series identities. Existing
deadlines still apply; a slow run retains partial evidence. Smoke rejects a
non-default profile and remains unchanged. All observations remain in the
result. `--smoke` uses 23 items and two batches for each operation/variant,
without warmup or repetition. Smoke wall times do not establish speed claims.

`--output-mode summary` is an opt-in throughput diagnostic for long repetitions.
Both programs still execute every batch, accumulate its checksum modulo
1,000,000,007, and report the aggregate and final retained-data checksum. Output
stays bounded instead of emitting one progress line per batch. The independent
oracle checks both checksums. This mode does not measure batch-arrival latency
and cannot enter the streaming Pages series. The default `stream` mode,
registered inputs, output limit and scheduled CI remain unchanged. Freeze any
calibrated repetitions and qualify timing reproducibility before comparing a
candidate; the mode itself provides no reproducibility guarantee.

Each binary is built once and reused for the entire matrix. Compilation has a
120-second timeout per role; each application has a 30-second deadline and the
runtime matrix has a 15-minute deadline. Darwin uses `wait4` CPU/RSS accounting.
Linux requires GNU time at `/usr/bin/time`: its small launcher collects the
command's peak RSS, excluding the Python observer's inherited pre-exec memory.
Process wall and `wait4` CPU include that launcher, so small-input measurements
include its overhead. The manifest records the RSS method and tool identity;
changing collection methods starts a separate source-qualified series. Run
without other heavy work. `GOMAXPROCS=1`
and disabled Native instrumentation apply to ordinary observations; record the
machine and tool versions when interpreting local diagnostics. Controls are
optional for correctness smoke; missing controls cannot receive a win.

## Read results

`manifest.json` lists exact input cases, executable/source/tool hashes, per-role
summaries and Native/control median wall ratios (below 1 means shorter wall).
Raw `observation.json`, stdout and stderr remain for every process, including
failure; a failed run retains its partial manifest and exits nonzero. No rerun
or outlier removal happens automatically. Temporary binaries are not evidence.

Process p95 needs at least 20 observations and p99 at least 100; otherwise the
field is null. These are empirical nearest-rank summaries, not significance or
confidence claims. Wall, CPU, RSS and throughput also retain minimum, median, maximum and sample
count. Ranges use all retained processes, with no trimming. The first process
remains separate in the raw evidence and its wall summary. Overlapping ranges
warn that a median difference is uncertain; non-overlap is not a significance
test. Historical medians without ranges stay labelled as such. Median screening
thresholds and adoption criteria are unchanged. Peak RSS is per process, not a
sampled live-heap curve.
Throughput is input items across all batches divided by full process wall time.
It includes preparation and output and is not a maximum service throughput.

Batch completion intervals are observed by the parent when stdout lines arrive.
They include pipes and scheduling, and exclude initial input setup after the
`ready` marker. If multiple lines arrive together, interval summaries are null
with a reason. The medium worker has 100 batches to expose sustained behavior;
its interval p99 is still not request latency or a GC-pause measurement. Request
queueing, cross-language allocation counters, live RSS trajectories, file/network input and
parallelism need dedicated future probes or existing resource-soak harnesses.

`--gc-probes` separately runs medium worker/allocation cases with the existing
Native runtime counters enabled. It retains allocated/reclaimed/live managed
bytes and other available collector counters. These instrumented observations
never enter wall-time comparisons, and Native managed allocations are not
presented as equivalent to Go total heap allocations. Live RSS trajectories and
service pause times still need the dedicated resource/load harnesses.

## CI

The existing Native CLI job reuses its built CLI for `--smoke`, with a two-minute
step deadline. Controller tests run in the existing tooling job; the existing
Linux CLI job also checks resource attribution against an inflated observer and
a larger child allocation. This is a small correctness check, not a speed gate.
No additional
compiler bootstrap, comparative timing gate or scheduled matrix runs in PR CI.
Changing the workload/controller still uses the planner's applicable authorities;
this does not waive full validation for this change itself.

## Public application coverage

The Pages application view always lists all registered cases, with independent
dates, source revisions and environments. `representative-performance.yml` is a
manual, main-only workflow with an explicit fixed-profile choice for the initial public matrix and later relevant-change
or milestone measurements. It builds the current CLI through the ordinary pinned
seed/fixed-point path and the exact reference compiler, then measures Native,
TypeRB Go and Pure Go together on one Linux arm64 logical CPU. It retains the
registered warmup/sample counts, full matrix and runtime deadlines, plus a
25-minute workflow limit. It has no daily/weekly schedule or PR trigger.

`publication.py` validates and reduces the manifest to public fields, excluding
machine-local tool paths. It rejects smoke data, partial input selections, changed
sources and missing comparison identities. A failed attempt retains partial
observations and the previous complete snapshot. Pages displays failures and
unmeasured inputs explicitly; it never imports these results into the daily score.
Linux RSS observations without the corrected collector identity remain in raw
evidence but are unavailable in Pages and text reports. Their other metrics
remain visible.

Raw evidence and state artifacts last 90 days. Compact state retains twelve
complete snapshots and the latest attempt; Pages falls back to its previous
public state when artifacts expire, preserving the original measurement dates.
Main pushes update freshness metadata without running a benchmark. Public
measurement happens only on explicit dispatch; local diagnostics are separate.
