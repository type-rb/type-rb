# Native HTTP transport

The Native Web project provider connects the pinned `Web.serve(ServerConfig)`
API to libuv for TCP and event delivery, and llhttp for HTTP/1.1 framing. Routing,
request/response values, typed input, middleware and recovery remain TypeRB
implementations. The public package source remains pinned to `TYPE_RB_REVISION`.
The public [dependency register](https://type-rb.github.io/type-rb-native/dependencies/)
tracks runtime, tooling and candidate dependencies separately.
Whole-buffer [response compression](native-compression.md) uses a separately
pinned static gzip archive when the compression middleware is imported.

Compile a file-route project that calls `Web.serve()` or passes a
`Web::ServerConfig`. The server supports IPv4, IPv6 and host-name binding. REPL
evaluation reports the reference's explicit unavailable-runtime diagnostic;
it does not start a listener. See [Native CLI](native-cli.md) for building the
compiler and applications.

## Build and distribution

`tools/network-dependencies.json` pins libuv 1.53.0 and llhttp 9.4.3 by version,
upstream commit and source archive SHA-256. Both use the MIT license. The generated
llhttp release archive contains C sources; Node.js is not a build or runtime
requirement. Repository-owned runtime behavior remains TypeRB. A C toolchain
builds the external libraries and compile-only ABI assertions; the artifact
identity is assembly data, with no additional C runtime implementation.

The compiler selects static archives from verified network MIR. It links them
only for an application containing the network provider. Importing `trb/web`
currently includes that provider, including programs using only its response
values. The CLI, ordinary compiler and programs using only `trb/http` values
need no HTTP archives. An HTTP
application requires only its executable, the platform runtime and the dependency
notices at deployment; it does not load libuv/llhttp from the environment.

Development packages keep `network/` beside QBE. Alternatively,
`TRBN_NETWORK_LIB_DIR` selects an explicit **build-time** installation:

```sh
python3 tools/build-network-dependencies.py \
  --archives /path/to/shared/source-archives \
  --output /path/to/network
TRBN_NETWORK_LIB_DIR=/path/to/network ./trbn build --compile
./bin/application
```

Building the dependency archives requires Python, CMake and a platform C toolchain.
Installing prebuilt archives and compiling applications does not require CMake.
The platform assembler/linker and QBE remain application build tools. If QBE is
overridden, its adjacent `network/` directory becomes the default; set the
build-time override when using a separate installation. Paths containing spaces
are passed as individual arguments, without shell evaluation. Missing dependencies
or failed links preserve an existing output and remove the build's intermediates.

```sh
python3 tools/network_artifacts.py pack /path/to/network network.tar.gz
python3 tools/network_artifacts.py install network.tar.gz /path/to/new/network \
  --sha256 EXPECTED_ARTIFACT_SHA256
python3 tools/network_artifacts.py inspect ./bin/application
```

Installation checks the artifact hash, target, exact source pins, file inventory,
archive hashes and embedded identity before publishing a new directory. Obtain the
expected hash from a trusted qualification or release receipt; a hash file supplied
by an untrusted archive is not authentication. Existing installations are not
overwritten. The executable inventory includes versions, commits, source and static
archive hashes, configuration and build tools. It identifies the selected build;
it is not a signature or a runtime integrity attestation.

Temporary source/object trees are removed after success and failure. Reuse source
archives and one installation across applications. A 128 MiB budget covers the
installation and source archives. Keep both license notices with deployed apps.
The optional `--linkage dynamic` builder profile is retained for comparison and
older development binaries; the current compiler emits static calls.

| Component | Role | Application dependency |
| --- | --- | --- |
| Platform C runtime / system library | Allocation, process and OS primitives | Existing macOS libSystem / Linux libc and recorded platform imports |
| libuv 1.53.0 | TCP, address resolution, events, signals and timers | Selectively static for the network provider |
| llhttp 9.4.3 | HTTP/1 request framing and validation | Selectively static for the network provider |
| QBE and platform compiler/linker | Build the Native executable | Build tools, not invoked by a running server |
| Python and CMake | Build and qualify external archives | Dependency build tools; prebuilt installation needs only Python |

The CI qualification matrix builds and links archives on Darwin/Linux arm64/amd64,
including a fresh installation and isolated execution. Application socket/lifetime
and compiler verification remain separate authorities on the registered Native
targets; archive ABI success alone does not establish a darwin-amd64 compiler or
seed. CI bundles retain source archives, notices and build receipts. They are
qualification artifacts with finite retention, not immutable release assets.
Before supported release distribution, #1257 still requires immutable publication,
source mirrors, advisory/update ownership and deadlines, plus the remaining TLS/DB
profiles. See the [dependency policy](native-dependency-policy.md).

## Ownership and execution

The source loader assigns the internal Web server and network declarations their
identities. An authored path, alias, function name or matching declaration cannot
grant foreign-call privileges. Full signatures are checked again by MIR; network
calls conservatively retain allocation, failure, I/O and mutation effects.
The application-specific server/router is a project provider. The exact owned
`Web` to server-provider edge binds the application's routes and is excluded from
ordinary package-dependency traversal; authored package cycles remain errors.

One thread runs the libuv loop and TypeRB handlers. Foreign callbacks only parse
or copy into explicitly owned raw buffers; they cannot invoke managed handlers or
allocate GC-managed values. A complete request pauses reading on its connection.
The TypeRB loop copies immutable request bytes into managed objects, dispatches
with the existing recovery boundaries, and copies the response out before an
asynchronous write. No managed address remains in libuv or llhttp after returning
to TypeRB. Write and close callbacks release raw buffers after their last use.

Each connection has an opaque checked identity. The transport bounds the read
chunk, pipelined remainder, header storage and number of accepted connections.
`body_limit_bytes` bounds each decoded body, including chunked input. A partial
request does not prevent another connection from receiving a complete request.
Only one managed handler executes at a time; this is not a general task scheduler
or a claim of concurrent TypeRB handler execution.

SIGINT and SIGTERM stop accepting connections, close idle connections, and allow
pending responses to drain until `shutdown_timeout_milliseconds`. Close callbacks
run before the loop is released. Shutdown signals and drain deadlines are
serviced by the event loop: interrupting an indefinitely running synchronous TypeRB handler requires
execution cancellation and is not established by transport shutdown tests.


Per-request [Web deadlines](native-deadlines.md) use managed checkpoints,
independently of the transport shutdown drain deadline. Their application
acceptance includes repeated requests after expiration and ordinary failure.

## HTTP behavior

HTTP/1.0 and HTTP/1.1 requests share the existing Web route and middleware
dispatcher. The transport handles fragmented headers and bodies, chunked input,
binary bodies, keep-alive, pipelining and `Expect: 100-continue`. Request method
tokens retain the public `HttpMethod` semantics rather than llhttp's narrower
method enumeration. Host and transfer-framing headers do not become application
headers; trailers are not promoted into them. Framing/header failures are rejected
before application middleware. Decoded-body parsing failures and limits enter the
existing Web error dispatcher; an oversized body retains its accepted prefix up
to the configured limit for middleware inspection. Peer EOF terminates an
incomplete request without dispatch, matching the pinned server.

The transport retains llhttp's strict parser with one explicit reference
compatibility setting: chunked Transfer-Encoding takes precedence over
Content-Length, and the latter is removed before application dispatch. The shared
suite checks the decoded body and the following request boundary. Other leniency
flags stay disabled. A common policy that rejects conflicting framing before
middleware is tracked in [proposal #1239](https://github.com/type-rb/type-rb-native/issues/1239).
Adopting stricter behavior requires agreement across implementations rather than a
Native-only change.
This slice does not establish byte-for-byte equivalence to every `net/http`
behavior. Protocol upgrades, streaming and HTTP/2 need separate transport contracts.

## Validation and remaining work

`tools/native-network-distribution-test.py` checks selective linkage, missing/corrupt
archives, preserved outputs, relocated packages and execution without runtime
libraries. Its build/startup/size observations are descriptive qualification data,
not a throughput benchmark. `tools/native-web-network-test.py` retains real socket tests for fragmented and chunked input,
keep-alive/pipelining, body limits, malformed/conflicting framing, binary bodies,
routing and middleware, handler failure, disconnects, bind failure, concurrent
partial requests, signals and restart. Force GC at managed allocation boundaries
while requests remain alive, and verify raw/managed cleanup independently.
The CLI CI matrix exercises Darwin arm64 and Linux arm64; the Linux amd64 target
lane exercises its built core compiler. Cross-target ABI, compiler recovery,
ordinary fixed points and hosted integration remain required. Process supervision,
streaming, generic scheduling, TLS and
performance qualification have separate acceptance; do not infer them from this
transport.
