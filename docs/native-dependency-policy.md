# Native dependency and distribution policy

Accepted preparation for v0.5 is tracked in
[#1255](https://github.com/type-rb/type-rb-native/issues/1255), under
[#1164](https://github.com/type-rb/type-rb-native/issues/1164). The
[register](native-dependencies.json) separates source implementation from adoption
decisions. Artifact delivery is tracked by
[#1257](https://github.com/type-rb/type-rb-native/issues/1257), and post-v0.5
browser restoration by [#1256](https://github.com/type-rb/type-rb-native/issues/1256).
A selected library is not installed or qualified until an implementing
PR supplies exact pins, configuration and artifact evidence. Existing source pins,
compatibility revisions and failed observations remain unchanged.

## Release priorities

Use mature transport, TLS and storage implementations behind small TypeRB-owned
boundaries for typed values, errors, cancellation and resource lifetime. The
[canonical contract](https://github.com/type-rb/type-rb/blob/main/contracts/v0.5/manifest.json)
owns release scope; [0086](decisions/0086-production-web-application-path.md) orders
the application milestones. Each register entry records both its requirement and
the reference behavior to qualify. In particular:

- OIDC discovery/JWKS HTTPS retrieval and RSA verification, MySQL TLS, gzip, and
  standard digest/HMAC APIs are required application paths.
- A general HTTP client and server TLS are not inferred from HTTP values or a
  browser-only generated client. Do not add these to the v0.5 critical path.
- SQLite is first; MySQL follows M6. The existing Go PostgreSQL support remains
  in the complete parity scope, even though the earlier M5/M6 milestone sequence
  did not implement it. Evaluate libpq before a new protocol implementation.
- REPL stays required. Native local play/tour and hosted Run/Transpile/Format
  have explicit v0.5 deferrals. Canonical documentation stays current; build a
  pinned Go site generator in site CI initially. This creates no application
  runtime dependency. Local Go play/tour remains available separately.

## Linking profiles

Choose distribution by the application's complete TLS stack, not one library at
a time. These are target profiles, not descriptions of today's binaries:

| Application uses | Intended dependency delivery |
| --- | --- |
| No networking, DB or cryptographic APIs | No corresponding optional libraries |
| HTTP server | Selectively static libuv and llhttp |
| SQLite or whole-buffer gzip | Selectively static SQLite amalgamation / compression library |
| Digest only | Static libcrypto first; measure its provider and size cost |
| HTTPS/OIDC, without a dynamic DB client requiring OpenSSL | Selectively static compatible libcurl/OpenSSL and optional libpq |
| MariaDB Connector/C | Bundled dynamic Connector/C and a single shared OpenSSL build used by all in-process TLS/crypto consumers |

Platform libc, SDK frameworks and trust services remain system dependencies.
A shared OpenSSL build does not imply shared credentials, trust contexts or DB
verification defaults. Build all dependent clients against the selected identity;
inspect actual loader imports and test in minimal deployment images. Do not ship
a static libcrypto beside a second OpenSSL loaded by a DB client. MySQL static
linking would need a separate license/relinking qualification; an owned wire
client remains a later measured alternative.

Web transport uses selective static archives with an embedded build inventory.
Its CI bundles qualify development installations; immutable release publication
and advisory/update ownership remain tracked in #1257. Publish prebuilt dependency
archives as immutable, per-target assets so
application users do not need CMake. Record source revision, hash, flags, license,
notices and transitive libraries; embed a machine-readable dependency inventory
in resulting executables. Sidecar size counts toward distribution cost.

## TLS, trust and cryptography

Select OpenSSL 3.5 LTS and the latest qualified patch, with a recorded patch/update
process. Before any consumer initializes crypto, explicitly suppress automatic
configuration loading (`OPENSSL_INIT_NO_LOAD_CONFIG`). Test initialization ordering,
provider loading and absence of build-machine paths. This does not claim inherited
distribution crypto policy or FIPS behavior. See the
[release policy](https://openssl-library.org/policies/releasestrat/) and
[initialization API](https://docs.openssl.org/3.5/man3/OPENSSL_init_crypto/).

Use explicit trust adapters: macOS platform verification normally, disk roots
when configured by `SSL_CERT_FILE` / `SSL_CERT_DIR` under the fixed Go reference
semantics; Linux uses the reference's bundle/directory probing and overrides.
Do not depend on OpenSSL's compiled-in CA path or bundle Mozilla roots by default.
Record the Go toolchain, generated `go.mod` language version, GODEBUG and environment
in comparisons; [Go compatibility defaults](https://go.dev/doc/godebug) matter.
Qualify hostname failures, custom corporate roots and each DB's verification mode.
[libcurl trust options](https://curl.se/docs/sslcerts.html) alone do not establish
trust parity for other libraries.

Use a minimal HTTP/HTTPS libcurl configuration for synchronous OIDC retrieval
first; preserve bounded body size, total deadline, redirects, environment proxies
and cache behavior. Do not infer event-loop or asynchronous support from adopting
the library. RSA/X.509/TLS stay external. Start digest/HMAC with libcrypto and
measure a digest-only application before selecting a size budget and pin. Keep
compiler lockfile hashing independent of libcrypto. An owned wrapping-integer
implementation, HACL* or AWS-LC requires separate comparison; known-answer tests
alone do not prove constant-time behavior. Verify tag comparison and any checked
integer overflow lowering in such an evaluation.

## Other dependencies and future replacement

- SQLite: select an amalgamation, compare its exact version and externally
  observable compile options with the Go driver, and qualify transactions/errors.
  System SQLite is not the default reproducible distribution strategy.
- MariaDB: provisional dynamic Connector/C; compare normalized Go DSN/auth behavior
  and reassess only after M6 socket/TLS evidence. PostgreSQL SCRAM also involves
  normalization/channel binding, not just HMAC and PBKDF2.
- Compression: evaluate [libdeflate](https://github.com/ebiggers/libdeflate) first
  for the buffered gzip contract. Compare CPU, size, peak memory and compression
  ratio with zlib/zlib-ng. Streaming would require a new decision.
- Unicode: move generation to fixed, hashed UCD files and use Go for exhaustive
  comparison. The current Go-based generator remains recorded until replaced.
- `sqldef`: track external commands separately from DB clients and engines.
- Event loops/socket adapters and assemblers/linkers are conditional future
  replacement candidates when scheduler, build-time or distribution measurements
  justify the maintenance cost. JSON, routing and GC remain owned boundaries.

## Artifact qualification and updates

Every adoption PR records executable plus sidecar size, build time and startup
cost against the same application baseline; include peak memory where material.
Set an explicit measurement budget before alternatives are tested. Qualify all
four release hosts, including darwin-amd64; current three-profile evidence does
not qualify the missing host. Keep positive and negative compatibility results.

Pin upstream commits as well as archive hashes. In particular, mirror qualified
llhttp source archives as immutable assets before release; today's hash-verified
codeload download is not yet that mirror. Source archives have
[documented stability limits](https://github.blog/open-source/git/update-on-the-future-stability-of-source-code-archives-and-hashes/).
Never replace an old immutable asset to repair an update.

Before distributing static dependencies, the release maintainer must assign an
update owner, advisory monitoring for upstream and OSV, severity-based response
and rebuild deadlines, affected-artifact inventory and replacement release path.
Record these in the adoption/release task; no automated monitoring or SLA is
claimed by this document. Rebuild and requalify affected static executables for a
security update; updating a host shared library does not repair them. Keep this
register and per-artifact inventory synchronized, and verify the public Pages
view after merge.
