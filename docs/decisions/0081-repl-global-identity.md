# Shared REPL global cells and lexical identity

Status: implemented, with ordinary-path and retained-session coverage.

Interactive lowercase declarations and later named functions must refer to the
same storage. They must also preserve the lexical distinction between an earlier
named-function reference and a later same-named value binding.

## Ownership

Persistent environment bindings use shared cells. Named-function global accesses
and anonymous captures refer to those cells, so assignment does not leave an old
copy in either call path. Existing pool compaction traces and remaps the cells.

Typed header witnesses describe persistent bindings to the ordinary checker.
They are never evaluated to recreate those bindings. The CLI supplies authored
submission order before resolution, independently of the header's grouping of
function declarations and value witnesses. Ordinary files use their token order.
A lowercase binding enters scope after its initializer; earlier function bodies
retain their checked declaration targets. Generic instantiations keep that order.

The checked global catalog places existing projected cells before new entry
initializers while preserving imported initialization order. This uses the
ordinary initialization and MIR verifiers, with no REPL-specific verifier waiver.

## Replay and failure

Load and replay validate the complete replacement program before effects. Replay
reserves future cells and initializes each authored declaration at its original
turn. Fresh untyped empty collections retain existing body inference rather than
receiving an invented untyped file-global witness. A failed initializer retains
prior effects, clears stale narrowing and refreshes the surviving typed witnesses.

## Verification and limits

Shared ordinary/REPL cases cover earlier function references, defaults, generic
bodies, closures, later values, source ordering and rejected forward/self uses.
Source-erased MIR tests preserve runtime output. CLI controls cover sharing,
initialization counts, constant order, replay, empty-Hash load and failed-load
atomicity; existing retained-session and managed-lifetime controls remain active.

Optional scalar output lowers to existing typed MIR branches, payload extraction
and conversions. It follows current reference output on each path, including the
tracked compiled/REPL Nil formatting difference in TypeRB issue #783. It does not
introduce backend type inference or general union/nominal printing.

Namespace-body lowercase declarations are extended by [decision 0082](0082-namespace-binding-mir.md).
Forward initialization dependencies and untyped collection inference outside the
already supported session body remain explicit pending contracts. These changes
do not close the complete bindings or basic-language inventory.
