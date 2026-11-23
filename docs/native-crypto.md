# Native digest, HMAC and secure comparison

Native implements the pinned portable `Digest.md5`, `sha1`, `sha256`, `sha512`,
`HMAC.sha256`, `sha512`, `equal`, and `SecureCompare.equal` APIs in compiled
applications and the retained REPL. Results retain raw bytes, including NUL and
invalid UTF-8. Public declarations and names follow `TYPE_RB_REVISION`.

The provider is OpenSSL 3.5.9 `libcrypto`, built from the exact commit and archive
hash in [crypto-dependencies.json](../tools/crypto-dependencies.json). The
[dependency register](native-dependencies.json) records its scope and remaining
release qualification. Digest consumers share this crypto archive with the
[OIDC TLS/RS256 provider](native-oidc.md). Database client profiles remain separate
work.

## Dependency scope

| Executable | Crypto dependency |
| --- | --- |
| Core compiler and lockfile checksum implementation | None |
| Native CLI, including the retained REPL | Statically linked libcrypto |
| Application using Digest, HMAC or SecureCompare | Statically linked libcrypto |
| Application without these APIs or OIDC | None |

REPL evaluation calls the ordinary standard APIs, so it has the same results and
provider as compiled programs. It does not run the Go implementation. This means
the full CLI is a crypto consumer even when its current command is not `repl`.
Compiler lockfile SHA-256 stays in the existing independent implementation.

The compiler selects libraries from verified MIR declarations. Authored names or
paths cannot grant access to internal hashing or HMAC intrinsics. Missing or
corrupt archives fail the build while preserving an existing output executable.

## Build and installation

Contributor source builds require a C toolchain, Perl and make. Build the fixed
profile with a shared source archive directory:

```sh
python3 tools/build-crypto-dependencies.py \
  --archives /path/to/shared-source-cache --output bin/crypto
./trbn --version
```

An existing installation is never overwritten. A qualified archive can instead
be verified and installed without rebuilding OpenSSL:

```sh
python3 tools/crypto_artifacts.py verify bin/crypto --target darwin-arm64
python3 tools/crypto_artifacts.py pack bin/crypto crypto-darwin-arm64.tar.gz
```

Application compilation finds `crypto/libcrypto.a` and `crypto/crypto-identity.o`
beside QBE, or in the explicit **build-time** `TRBN_CRYPTO_LIB_DIR` directory.
The same location supplies a checkout CLI build. Its content hashes participate
in CLI cache invalidation. Executables require neither that directory nor a
runtime library environment variable after compilation.

The archive includes the Apache-2.0 license, exact source pins and build receipt.
Each consuming executable embeds one `TRBN-CRYPTO-V1` inventory; inspect it with
`tools/crypto_artifacts.py inspect`. CI builds and tests development artifacts on
Darwin/Linux arm64/amd64. Archive qualification is separate from the current
ordinary compiler host matrix, which still lacks darwin-amd64. Immutable release
publication and advisory/update ownership remain in
[#1257](https://github.com/type-rb/type-rb-native/issues/1257).

## Initialization and lifetime

The fixed profile keeps upstream assembly and disables shared libraries,
loadable modules, DSO loading, engines, legacy provider, tests, apps, documentation
and automatic configuration loading. PIC, sectioned objects, build-path remapping
and the pinned commit timestamp are recorded in the receipt. Before digest or
HMAC use, the runtime also calls `OPENSSL_init_crypto` with
`OPENSSL_INIT_NO_LOAD_CONFIG`.

The default provider is built in. `OPENSSL_CONF` cannot load a host configuration,
and `OPENSSL_MODULES` cannot inject a provider. Tests run with a blocking FIFO as
the configuration path and absent provider directories. This profile does not
claim inherited distribution crypto policy, FIPS behavior, or TLS trust parity.
Future TLS/DB composition must preserve one OpenSSL stack per process as specified
by the [dependency policy](native-dependency-policy.md).

One-shot EVP calls own and release their native contexts. A 64-byte native stack
buffer holds the result across managed allocation, then is cleansed after the
immutable Bytes copy. No TypeRB-owned native crypto context survives a call.
OpenSSL manages its bounded process caches and exit cleanup. Provider failures
remain terminal and do not fall back to another implementation.

Comparison returns false immediately for unequal public lengths, matching the
reference. Equal lengths go through OpenSSL `CRYPTO_memcmp`, including first-byte
and last-byte mismatches. The pinned arm64/amd64 implementations branch on length,
accumulate differences without early mismatch exits, and select the final result
without a content-dependent branch. This is a claim about comparison for a fixed
length, not about hiding lengths or making an entire application constant-time.

## Validation

The frozen [82-observation corpus](../tools/crypto-cases.json) includes standard
digest answers, RFC 4231 HMAC vectors, padding boundaries, long keys, binary data,
empty values and unequal comparisons. It runs through check, build, execution,
run and retained REPL against the exact reference revision. Additional probes
cover declaration/MIR forgery, erased source, forced GC, cleansed output buffers,
initialization/provider failures, archive corruption and relocated executables.

`tools/native-crypto-test.py` runs correctness checks. Its separate `--measure`
mode records the prospectively bounded three-pair application size/build/startup
diagnostic; ordinary correctness reruns do not repeat that cohort. The initial
digest application guardrail is a 4 MiB executable increment. A miss requires
comparison with HACL* or AWS-LC before adoption. These dependencies are not
introduced by this implementation.

### Initial adoption diagnostic

The one three-pair Darwin arm64 run used Apple clang 21.0.0, the pinned Native
QBE variant, and the exact profile above. A SHA-256 application was 3,268,776
bytes versus 51,160 bytes for the plain control: **3,217,616 bytes (3.07 MiB)**
additional, within the predeclared 4 MiB guardrail. The core compiler was
2,796,392 bytes with no crypto inventory; the full CLI was 6,679,192 bytes
(including the REPL provider), compared with 3,426,120 before this batch. The complete 82-observation
application was 3,301,784 bytes and imported only platform libSystem.

| Observation | Plain control | SHA-256 |
| --- | --- | --- |
| Build seconds, all three pairs | 0.1725, 0.1697, 0.1783 | 0.1886, 0.1695, 0.1997 |
| First process execution seconds, all three pairs | 0.0718, 0.0269, 0.0250 | 0.2600, 0.0620, 0.0773 |

These are a bounded local cost diagnostic, including tool launch, linker and OS
first-execution effects. The variable startup samples do not establish a stable
provider overhead or a cross-platform performance claim. No repeat cohort or
profile tuning was used to select the result. Four-host development CI qualifies
ABI and correctness; release artifact and TLS composition qualification remain
separate obligations.
