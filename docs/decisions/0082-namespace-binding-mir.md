# Lexical namespace bindings through global MIR

Status: implemented direction, extending decisions 0080 and 0081 under the
basic-language completion milestone.

Lowercase declarations in a namespace body retain namespace and source-module
identity. They use the same verified initialization, typed reads/writes, mutable
capabilities and persistent managed roots as file globals. This adds no backend
binding analysis or alternate storage path.

A declaration becomes visible after its initializer. Before a later inner
declaration, a method keeps the earlier outer lexical binding. Retained REPL
projections compare authored submission order first and token order within the
same submission. Nested or reopened namespaces keep their identities across
calls, closures and explicit load/replay.

The accepted reference includes TypeRB PR #786's shared namespace storage and
nullable-invalidation correction. Differential cases exercise ordinary files,
imported aliases, independent source modules and retained sessions. Source-erased
MIR and forced-collection controls retain managed values, while negative cases
reject forward reads, readonly writes, duplicate/blank declarations and stale
nullable proofs. Existing global MIR mutation controls remain applicable.

Qualified lowercase binding members remain unsupported while TypeRB issue #787
resolves their currently checked but non-executable behavior. Ruby's independent
same-named namespace collision remains tracked in TypeRB issue #785. Forward
initialization dependencies and untyped empty collection inference are separate
remaining contracts. No full-language or performance qualification is implied.
