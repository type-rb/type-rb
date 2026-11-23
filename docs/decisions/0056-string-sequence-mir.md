# String sequences and reversal through MIR

Status: implemented subset under the basic-language and MIR consolidation milestones.

Ordinary programs and the REPL support String `codepoints`, `chars` and
`reverse`. Code points are Unicode scalar values; combining marks and each
component of a grapheme cluster remain separate. Invalid external UTF-8 bytes
become replacement characters consistently with the pinned reference. Embedded
NUL is retained. The source String is unchanged, and each returned Array has
independent storage whose mutation follows its binding's ordinary capability.
Optional receivers skip work when absent and retain the typed optional result.

`string_methods.trb` now owns classification, arity and typed construction for
queries and sequence methods. It replaces the narrower `string_queries.trb`
name; the REPL owner is renamed consistently. Query instruction 44 is unchanged.
New internal instruction 45 takes one available String and a closed mode:
code points, characters or reverse. Its result must respectively be
`Array<Integer>`, `Array<String>` or `String`; extra operands, modes, result
shapes and failure edges are rejected independently. Composite results use the
same semantic type catalog as other managed values. Allocation and terminal
failure effects are explicit, so liveness includes the source at each call.

`qbe_string_sequences.trb` contains the bounded runtime. Code-point and character
Arrays use a forward byte cursor instead of repeatedly locating a code-point
index. After each character is stored, the helper drops only its temporary
character roots while retaining the result Array and the caller's roots.
Reverse first measures the normalized byte length, then copies decoded spans
into reverse order with one result String allocation. Both passes are linear
in input length. The retained REPL evaluator uses existing Unicode adapters;
its interpreted indexing and concatenation are separate from the ordinary
runtime's linear traversal.

Shared check/build/run/REPL cases cover Unicode units, empty and single-character
values, fresh result storage, readonly rejection, aliases, closures, optional
receivers, NUL and rejected signatures. MIR controls erase source, reorder blocks
and force collection before transformations and surrounding allocations.
Malformed-operation and forged-root controls ensure the verifier owns the
contract. Native UTF-8 tests collect on every String allocation, including
multiple Array buffer growths while earlier results remain live, and exercise
malformed external input.

The ordinary compiler closure now contains 105 modules. Recovery import prefixes
and module mutations include the renamed owner and the new runtime module.
The implementation remains compatible with the current immutable seed. Public
slicing, other String methods, diagnostic parity and performance qualification
remain separate unfinished work; the shared combined String case still records
its unsupported `slice` operation.
