# Native application time services

Native implements the existing `trb/std/time` API using checked TypeRB calendar,
TZif and POSIX-footer code. The host supplies clock values, the `TZ` and
`ZONEINFO` environment inputs, and file byte ranges. It does not select time
zones or decode ZIP/TZif data. No new public API or timezone library is added.

## Local time and named zones

`TimeZone.get("Local")` uses the local zone. Compiled programs retain the first
use of local zone data for the process; replacing the source file does not
change an already initialized local zone. A fresh process starts independently.
The retained REPL keeps that module state within the evaluation session.
Explicit REPL reset/replay behavior is outside these retained-session checks
and remains part of the separate replay qualification (#1127).

- With `TZ` absent, Native reads `/etc/localtime`.
- An empty `TZ`, `UTC`, or `:UTC` selects UTC.
- A zone name such as `Asia/Tokyo`, optionally prefixed by `:`, selects system
  zone data. An absolute path, also optionally prefixed by `:`, reads a TZif file.
- If local-zone configuration cannot be loaded, the reference fallback is UTC.

For other named locations, Native reads `ZONEINFO` once, at the first named
lookup. A nonempty value identifies either a directory or an uncompressed ZIP
archive. That source precedes the system locations `/usr/share/zoneinfo`,
`/usr/share/lib/zoneinfo`, `/usr/lib/locale/TZ`, and `/etc/zoneinfo`.
Missing entries fall through. Malformed configured TZif data also falls through,
following the pinned reference; configured read and ZIP errors take precedence
only when no system source succeeds. Unknown locations remain typed errors.

ZIP lookup reads the central directory and selected stored member without
extracting files or decompressing data. It checks names and byte extents before
indexing. A compressed matching entry is rejected; its method is not silently
accepted. Ordinary TZif files use the reference's 10 MiB read ceiling. Selected
ZIP ranges are checked against the file's size, so unrelated archive members
need not be loaded. TZif v1, v2 and v3 are supported; v4 is also rejected by the
selected reference.

## Ownership and verification

Only loader-owned declarations with complete checked signatures can acquire the
host adapters. MIR independently verifies parameter types, mutable output arrays,
result types and conservative call effects. Authored function names cannot
select an intrinsic. Output arrays remain rooted while strings are allocated;
file paths are checked for NUL, files close on success and failure, and failed
reads never publish partially initialized values.

`tools/native-time-services.json` freezes the initial twenty M3a scenario groups:
eight existing Bytes/JSON/calendar/clock cases and twelve environment groups
with twenty-two variants. Expectations were captured from the pinned reference
before implementation; they are not generated from Native results.

```sh
python3 tools/native-time-service-test.py --native bin/trbn \
  --reference /path/to/pinned/trb --report /tmp/time-services.json --gc-stress
python3 tools/native-time-host-test.py --qbe bin/qbe \
  --receipt /tmp/time-host.json
python3 tools/native-time-test.py bin/trbn --reference /path/to/pinned/trb
```

The service harness also accepts either compiler separately. A Native-only
`local-unset` observation records an independent OS-localtime comparison, not a
same-host reference replay. Configured cases use deterministic synthetic data.
Every run retains check/build and ordinary/REPL outcomes, including failures;
`complete` becomes true only after all selected groups pass. The mechanical
adapter test executes the actual emitted QBE against libc, including binary
bytes, exact ranges, absent/empty environment values, error statuses, clock ABI,
root-frame balance and descriptor cleanup under a low descriptor limit.
The service GC mode additionally collects at actual owned host allocations and
ordinary managed allocations, checking unchanged values, cached roots and full
reclamation at process shutdown.

## Remaining qualification

Passing this initial slice does not establish complete time or v0.5 parity.
Native currently relies on available system/configured data; the reference's
embedded timezone database is not yet a Native distribution artifact. Record
OS tzdata and configured sources when qualifying an application image. Embedded
data distribution remains part of dependency-artifact qualification.
Monotonic/scheduling services and the separate constructor/parser decisions
remain explicit work under the owning initiatives. The four-host mechanical
ABI checks do not establish four-host ordinary compiler or release readiness.
