# CLI key-reader interruption boundaries

The current TypeRB editor uses a finite poll followed by one byte read. A
finite poll does not make the subsequent read nonblocking: readiness can be
invalidated before read starts. A pending-signal check immediately after poll
cannot cover a signal delivered after that check.

The key reader now temporarily sets `O_NONBLOCK` for the read and restores the
exact original file-status flags before returning. It captures errno before
restoration, distinguishes EOF and real read errors from EAGAIN/EINTR, and
checks pending interruption after restoring flags. Poll errors never enter the
read path. An interrupted flag-restoration syscall retries that syscall;
terminal shutdown also restores any active temporary flags, including SIGTERM
at the read boundary. Originally nonblocking input remains nonblocking.

Supported Darwin/Linux hosts resolve their libc errno accessor during terminal
setup. Setup fails with terminal restoration if that accessor is unavailable;
it never calls a missing function pointer. The host bridge remains repository-
owned TypeRB source emitting the existing internal QBE ABI. The standalone
compiler source closure is unchanged.

## Deterministic regression

`tools/native-cli-key-test.py` extracts the actual host QBE and compiles it with
test-only syscall wrappers. It uses a real PTY, waits for terminal setup before
sending bytes, and injects events at syscall boundaries. The production bridge
contains no test hooks. Each observation has a fixed one-second completion
bound; a timed-out child is inspected, killed and reaped, and its transcript
and state are retained. There is no retry-until-success policy.

The 17 controls cover normal bytes and raw Ctrl-C, idle timeout, pending SIGINT,
SIGINT at poll entry, SIGINT before read, input drain before read with/without
SIGINT, poll/read errors, EOF, invalid descriptors, preexisting nonblocking
flags, get/set flag errors, interrupted restoration and SIGTERM before read.
The parent checks flags and terminal modes after exit. PENDIN is retained in
the observations but excluded from mode equality: the kernel can mark queued
input for reprocessing when canonical mode returns. All other mode fields and
control characters are compared.

Against source `8980b5978f0e8519964d200b7f7799b741796478`, both drain-at-read
controls block until the test deadline; poll failure incorrectly continues to
read the available byte. New flag-syscall failure controls also distinguish the
new path from the old implementation, which had no such calls. The repaired
bridge passes all 17 controls locally. Both hosted CLI jobs run the same test
and retain its full JSON alongside the existing complete CLI/editor authorities.

Run it after building the ordinary CLI:

```sh
python3 tools/native-cli-key-test.py --qbe bin/qbe --output key-observations.json
```

## Relationship to the original intermittent failure

[Issue #298](https://github.com/type-rb/type-rb-native/issues/298) originally
observed a Darwin timeout in the retired libedit path. This test proves and
repairs a blocking-read boundary in the replacement host implementation. It
does not establish the exact interleaving of the historical failure. In
particular, the test explicitly drains input; raw-mode Ctrl-C is a byte and
must not be described as automatically flushing input through an OS signal.
The original failure and unchanged-head successful retry remain historical
evidence. Full editing Ctrl-C, explicit SIGINT, evaluation after interruption,
running-program interruption and SIGTERM restoration remain separate real-CLI
regressions. No timeout is extended and no benchmark result is relabelled.
