# Literal union Hash keys

The reference admits a Hash key made from a union of Integer literals or a union
of String literals. Mixed scalar domains and nullable keys remain invalid.
Preserve the exact key constraint: equal payloads in different union types do
not make their Hash operations interchangeable. Method arguments retain the
reference's contextual literal conversion; indexed access still requires the
key's exact checked type.

MIR derives the key layout from the verified Hash type. A String-content flag
selects byte equality, and a separate union flag selects the checked union
payload. The QBE adapter consumes those flags without inspecting source syntax.
Buckets retain the original union object, so keys and iteration snapshots keep
their type and lifetime. Hashing and collision comparison inspect its scalar
payload without allocating a replacement key. Existing scalar Integer keys keep
their reserved-slot encoding, and ordinary String keys keep their layout.

Union keys require managed key arrays even when their payload is an Integer.
Deletion clears those references with the managed tombstone. Growth, compaction,
copies and snapshots preserve the descriptor and flags. The REPL similarly
unwraps payloads only for hashing and comparison, retaining typed value IDs in
the table. Key hashing and probing have one runtime source owner, extracted from
the larger Hash runtime with the recovery closure and mutation controls updated.

Validation pairs file check/build/execution and retained REPL cases, including
Unicode/NUL contents, with independent MIR constraint rejection. Source-erased
and reordered MIR exercises growth, deletion, copies and snapshots under forced
collection. A returned table owns its keys after their source Array has expired;
retained closures own snapshots after table deletion and declaration remapping.
Allocation accounting requires exact reclamation and no live bytes at shutdown.

The complete compiler recovery snapshot is 83,927,300 bytes, 41,220 bytes above
the previous compiler-only 80 MiB decode bound. Raise that bound to 96 MiB with
exact-limit and one-byte-over rejection controls. The ordinary 4 MiB snapshot
limit and all other schema bounds remain unchanged. This budget describes
verbose recovery JSON, not compiler or application binary size. Retain the failed
smaller-bound attempt; the larger budget still requires complete recovery checks.

Remaining empty-collection inference, other API gaps and REPL presentation
differences stay explicit. This implements an existing language contract and
does not qualify performance or complete the entire basic-language inventory.
