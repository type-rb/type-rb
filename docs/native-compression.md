# Native Web response compression

`trb/web/middleware/compression` runs the pinned whole-buffer gzip contract in
compiled Native applications. `Compression.call` and `Compression.middleware`
retain the public package implementation; the internal `Bytes -> Bytes` operation
uses libdeflate 1.26. No language or public API change is needed.

The default minimum is 1,024 bytes. Negotiation, wildcard and quality handling,
compressible content types, HEAD/status/range exclusions, `no-transform`, existing
encodings and `Vary` are the pinned package's responsibility. Compression removes
the previous `Content-Length` and `ETag`; the transport computes the actual wire
length. Compare decoded bytes and response metadata with Go, not gzip byte
identity. The pinned Go REPL has no compression runtime adapter; Native reports
that boundary and remains usable after the diagnostic.

## Optional static dependency

`tools/compression-dependencies.json` pins the upstream release archive, commit,
SHA-256 and MIT license. CMake builds only a static, position-independent gzip
compressor: decompression, zlib wrappers, tools, tests and install targets are
disabled. The runtime requests the library's default level (`-1`, currently 6).
The C compiler verifies the LP64 pointer/size and C `int` ABI.

The compiler selects the archive from verified compression MIR. Importing the
compression middleware currently includes its internal operation even if a
particular application does not call it. Ordinary applications and the compiler
itself do not acquire libdeflate. The middleware also imports `trb/web`, so its
applications need the [HTTP network archives](native-network.md).

Development packages place `compression/` beside QBE. An explicit installation
can be selected at **build time**:

```sh
python3 tools/build-compression-dependencies.py \
  --archives /path/to/shared/source-archives --output /path/to/compression
TRBN_COMPRESSION_LIB_DIR=/path/to/compression ./trbn build --compile
python3 tools/compression_artifacts.py inspect ./bin/application
```

Building the archive requires Python, CMake and a platform C toolchain. Installing
a qualified prebuilt archive requires Python, without CMake:

```sh
python3 tools/compression_artifacts.py pack /path/to/compression compression.tar.gz
python3 tools/compression_artifacts.py install compression.tar.gz /path/to/new/compression \
  --sha256 EXPECTED_ARTIFACT_SHA256
```

The shared artifact verifier checks the trusted artifact hash, host, source pins,
bounded file inventory, archive hashes and embedded build identity. Obtain the
expected hash from a trusted qualification or release receipt. Installations are
new directories; the builder does not replace an active installation. Missing or
corrupt build inputs preserve the previous application output. Paths containing
spaces are ordinary arguments, without shell evaluation.

Generated applications need no libdeflate sidecar or library-directory environment
variable. Their inventory records the source/build identities and configuration.
Distribute the MIT notice with the executable. Keep one shared source archive and
installation; temporary source and object trees are removed on exit. A 128 MiB
budget covers this dependency's source and installation.

## Ownership and verification

Only the loader-owned internal declaration can acquire the gzip intrinsic. Its
full signature and conservative effects are verified by MIR; a matching authored
name or forged declaration cannot create the foreign call.

The synchronous adapter holds the input in the caller's managed roots. It creates
one foreign compressor and output buffer, frees the compressor after compression,
copies the result into managed Bytes and frees the raw buffer. No foreign code
retains a managed address. Compressor creation, bound validation, raw allocation
and compression failures release acquired resources before the terminal
allocation diagnostic. Allocation exhaustion follows the existing process-fatal
runtime contract; this is not a new recoverable compression error.

`tools/native-web-compression-test.py` checks the fixed 20-case negotiation corpus
against the exact reference revision and Native check/build/run paths, binary
Bytes and invalid options, the REPL boundary, forced collection, raw allocation
accounting and four injected external failure paths. Real HTTP tests exercise
default middleware, gzip rejection and HEAD using an executable relocated away
from the compiler and archives. An ablation records executable size with and
without the compression import; build and startup observations are descriptive.

CI also qualifies archive installation and ABI on Darwin/Linux arm64/amd64. Full
application/runtime checks run on the existing Native compiler targets. Archive
qualification alone does not establish a darwin-amd64 compiler or bootstrap seed.
CI bundles are finite-retention evidence. Immutable release publication and
advisory/update ownership remain under #1257 before supported distribution.

## Library selection observation

One bounded local diagnostic compared libdeflate 1.26 with zlib 1.3.2 on Darwin
arm64, at each library's default level, allocating a compressor per response.
Seven fixed synthetic inputs had three adjacent observations per library, with
decoded-byte checks. This was a dependency selection diagnostic, not a formal
performance qualification or an application throughput result. No parameter sweep
or repeat cohort was used.

| Input | CPU microseconds/body, libdeflate / zlib (median) | gzip bytes, libdeflate / zlib |
| --- | ---: | ---: |
| Empty | 3.96 / 3.72 | 23 / 20 |
| Binary, 256 bytes | 17.30 / 18.54 | 279 / 279 |
| Text, 66 bytes | 10.10 / 2.89 | 45 / 45 |
| Text, 8 KiB | 24.04 / 14.54 | 99 / 99 |
| JSON, 64 KiB | 104.64 / 97.07 | 286 / 286 |
| High-entropy bytes, 1 MiB | 8,650.88 / 16,461.12 | 1,048,684 / 1,048,914 |
| Repeated text, 1 MiB | 1,625.75 / 1,558.75 | 2,650 / 2,610 |

The isolated probe executable was 87,824 bytes with libdeflate and 70,504 with
zlib, a 17,320-byte increase. Median process peak RSS varied by input: the 8 KiB
case used 2,310,144 / 2,080,768 bytes; the high-entropy case used
5,505,024 / 5,472,256 bytes. These include the probe and are not allocator-only
measurements. Small-body CPU and size costs remain visible; libdeflate is not a
universal improvement. The selected MIT whole-buffer library provides a small
integration API and a useful large high-entropy result. Reconsider it using
application evidence, or consider streaming libraries if that contract changes.

A separate fixed three-pair HTTP application diagnostic on Darwin arm64 alternated
the same server with and without the compression import/call. It includes the
middleware's code as well as the external library, using the shared warm cache.

| Dimension | With compression | Without compression |
| --- | ---: | ---: |
| Executable bytes | 410,072 | 359,088 |
| Build seconds, median | 1.999 | 1.969 |
| Startup seconds, all three observations | 0.261 / 0.069 / 0.194 | 0.497 / 0.064 / 0.059 |

The size cost is 50,984 bytes (14.2%); median build cost is about 30 milliseconds.
Startup varied widely, with a higher compressed median (0.194 versus 0.064
seconds). The cause and stable increment are unqualified; retain that result
rather than inferring a speedup or attributing it to the operating system.
This optional feature cost is accepted for functional coverage, with release-host
qualification still required. No further cohort was run. Only libSystem was
dynamically imported. In the separate negotiation/GC acceptance application,
forced collection reclaimed all 473,543 managed bytes; foreign compressor and
buffer counters also returned to zero.

The [dependency register](native-dependencies.json) records this implementation
separately from supported-release qualification.
