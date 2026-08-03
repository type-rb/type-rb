# Self-hosting

TypeRB uses a staged bootstrap. The checked-in Go compiler is stage 0. It
compiles the portable sources in `src/` into `generated/`; the generated source
must match the checked-in snapshot exactly.

Stage 1 currently owns the target-independent position, span, token,
diagnostic, and type-reference values. This is intentionally a small boundary:
the current language cannot yet express all of the bootstrap compiler without
falling back to Go-native code.

Run the bootstrap check from the repository root:

```sh
./scripts/check-self-host.sh
```

A complete self-host is reached when stage 1 can build the next `trb` binary,
that binary can rebuild itself, and both generated trees are byte-identical.
Before that closure is possible, TypeRB needs portable enums/sum types,
collection iteration, exhaustive matching/type dispatch, fallible results, and
filesystem/process APIs.
