# Byte encodings and request IDs

Native loads the pinned public `Hex` and `Base64` wrappers unchanged. Their
private implementations are TypeRB sources, checked and lowered like ordinary
code. `encode` operates on `Bytes`; `decode` returns the original public
`Result<Bytes, DecodeError>` identity. Hex accepts either letter case and emits
lowercase. Base64 uses padded standard encoding, while its URL variant uses the
URL alphabet without padding. Decoders preserve the pinned error kind, original
input, code-point index, validation order and strict unused-bit checks.

Plain payloadless enum output is lowered to its declaration ordinal for compiled
execution, matching the pinned Go output used by decode error kinds. The REPL
retains its symbolic display. The frontend selects this projection and lowers it
to existing enum tests and scalar joins; source-erased MIR preserves the result.
This does not broaden output for enums with payloads or explicit raw values.

The immutable octet table is private to the decoder implementations. Aliases,
record names or authored import paths cannot acquire standard-source ownership.
Public wrappers and their implementations form one compilation unit each;
canonical public error identities remain distinct between Hex and Base64.

`SecureRandom.bytes` checks the public length range in TypeRB and obtains bytes
from the operating system. The private, loader-owned entropy declaration has a
complete checked signature and an independently verified MIR adapter. The
backend transfers bytes through libc `getentropy` in chunks no larger than 256
bytes on supported Darwin and Linux profiles. A failed transfer returns no
partial result and the checked wrapper fails; there is no weaker fallback.
Zero-length requests do not call the entropy source. This implementation is
bounded by the pinned public maximum rather than a new Native option.

The retained REPL uses the same host bridge and executes the same library bodies.
MIR retains conservative allocation, I/O and failure effects, and ordinary root
liveness protects returned bytes across later allocation and collection.

These services allow the pinned `trb/web/middleware/request_id` source to run
unchanged: it validates incoming identifiers, generates replacements, updates
request and response headers, and retains typed context values through
`with_request`. Header names and length limits follow the package's existing
options. This does not establish routing, server execution or other cryptographic
package support.

Validation combines the shared language-coverage registry, source-erased and
mutated-MIR checks, cross-module and retained-session execution, forced garbage
collection, and an injected OS test fixture for chunk boundaries and failures.
Random outputs are tested by their public properties and round trips; they are
not stored as exact expected values or judged by probabilistic uniqueness tests.
