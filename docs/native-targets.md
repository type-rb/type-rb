# Native target profiles

The compiler shares TypeRB semantics, MIR, runtime generation and target-neutral
QBE across the registered profiles. These internal profiles are under development.
Supported release targets are a [project goal](mir-consolidation.md); current
profile verification does not yet establish stable installation or release support.

| Profile | QBE target | Executable and toolchain |
| --- | --- | --- |
| `darwin-arm64-v0` | `arm64_apple` | Mach-O; system assembler/linker through the registered C toolchain driver. |
| `linux-arm64-v0` | `arm64` | ELF; system assembler and LLD. |
| `linux-amd64-v0` | `amd64_sysv` | ELF; system assembler and LLD. |

Current [target validation](../.github/workflows/linux-amd64-targets.yml) checks
ordinary compiler generations, application results and failure behavior, executable
identity/dependencies, process boundaries and target-neutral output equality.
Unknown profiles fail before source or tool access. Compiler-owned builds invoke
explicit tool paths directly and publish output only after all phases succeed.

Darwin and Linux arm64 have [verified immutable checkout seeds](bootstrap-seed-updates.md).
Linux amd64 currently derives its setup compiler from the immutable initial root
and exact accepted source transitions. That recovery/setup evidence remains
separate from ordinary candidate self-hosting; it is not a published amd64 seed.
The checkout launcher currently bootstraps on the two arm64 platforms.

See [CI validation](ci-validation.md) for integration requirements and the
[historical target contracts](history.md) for source-era observations. Migration
correctness checks do not establish final Pure Go performance qualification.

## Time services

The registered POSIX LP64 profiles use `clock_gettime(CLOCK_REALTIME)` for
`Instant.now`. This is wall-clock time. It does not establish a monotonic-clock
or scheduling API. The small verified MIR adapter transfers two integers into
an existing array; ordinary TypeRB validates and constructs the Instant.

Named TimeZone values read installed TZif v1/v2/v3 data from
`/usr/share/zoneinfo`; `Local` reads `/etc/localtime`. UTC needs no file. Each
read is bounded to 1 MiB and closes its stdio handle on success or failure.
The runtime does not bundle another timezone database. Calendar calculations,
transition lookup, POSIX footer rules, ambiguous/nonexistent local times and
public errors remain TypeRB code above MIR. The backend only transfers bytes.

The current implementation requires system zone data. Alternate `TZ`/`ZONEINFO`
search paths, zip or embedded databases and TZif v4 are not qualified. These
remain coverage gaps in the full Go parity release gate. Missing zone files and
rejected TZif structures fail explicitly; it never silently substitutes UTC for a named zone.
