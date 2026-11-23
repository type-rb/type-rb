# Checked facts across REPL submissions

## Ownership

The REPL previously reconstructed only declared binding types before checking a
new input. That lost successful assignment narrowing, so a retained `String?`
could not use String members even after `value = "hello"`. Rechecking successful
history alone is also insufficient: a failed input may have changed the retained
value before reporting its error.

`checked_submission.trb` projects the ordinary checker's entry and exit facts for
one interactive input. Facts identify a binding and optional stable record field
by name, plus nil or present state; they never inspect runtime payload presence.
The next check resolves those names against current declared types and canonical
record identities. Unknown bindings, fields and invalid presence states fail
checking. Ordinary file compilation leaves this optional projection empty.

Expression and assignment displays, including `:type`, read results recorded by
that same checker. Direct assignment preserves the declared target type and the
mutable marker; a nil assignment displays Nil. `:type` executes no expression and
does not commit its temporary flow changes. Authored REPL `return` is rejected
outside a function or method despite the private main wrapper.

`compiler/cli/repl_check.trb` owns source loading and submission boundary mapping.
The session owns retained values, accepted inputs and completion. Only a checked,
successfully evaluated input commits its exit facts. Static rejection preserves
the preceding facts; runtime failure or interruption clears them conservatively.
Completed writes and external effects remain observable. Earlier effects are
never replayed by an ordinary submission or declaration check.

## Replay and invalidation

Failure boundaries are recorded against accepted submission indices. Declaration
validation and explicit `:reload` or `:load` preserve those boundaries when
rechecking accepted history. Replay executes accepted inputs only. It rebuilds
runtime state without the failed input's partial effects, while retaining the
conservative checking boundary needed by later accepted guards or nil comparisons.
Subsequent assignment or a new guard can establish narrowing again.

The session stores no stale nominal type IDs in its facts. Imports and new record
declarations resolve the same binding/field names in the current checked program.
This does not extend the underlying checker's supported narrowing rules, infer
facts from an executed branch, or add general pattern/union support.

## MIR, recovery and evidence

QBE consumes verified nullable MIR operations and has no dependency on this
projection. A regression erases submission and source metadata after checking,
then checks identical QBE and execution. Existing assignment and backedge
invalidation remain the semantic owners. The new helper is part of the ordinary
66-module compiler closure and its exact recovery import/mutation inventory.
No seed, snapshot version or reference pin changes.

The shared registry adds retained assignments, rejected assignments, conditional
replacement, partial failure, lazy safe navigation and top-level return probes.
CLI controls additionally cover `:type`, numeric widening, imports, declaration
remapping, explicit replay and SIGINT. The generated Capabilities views retain
all known differences, including statement-control result display and Hash
separators. This completes the retained assignment portion of issue #469; it does
not claim full REPL display or diagnostic parity.

The pinned Go reference also has stale narrowing after a successful conditional
replacement ([TypeRB #699](https://github.com/type-rb/type-rb/issues/699)), and after
partial runtime failure. The latter has a separate fix in
[TypeRB PR #698](https://github.com/type-rb/type-rb/pull/698). Shared expectations
remain tied to the exact pin; corrected behavior is not counted as matching that
older compiler. This integration makes no performance qualification claim.
