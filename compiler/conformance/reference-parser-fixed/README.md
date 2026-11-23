# Corrected reference parser regressions

These sources use ordinary TypeRB control expressions inside enclosing operands.
The reference parser accepts them after [TypeRB PR 691](https://github.com/type-rb/type-rb/pull/691),
which is newer than `TYPE_RB_REVISION`. Keep them separate from the exact pinned
shared parity cases until the reference pin includes that correction.

The MIR unit suite and Native CLI suite execute these cases. They check that
lexical returns stop container operands, preserve earlier effects and leave
existing collections intact. No new language semantics are introduced.
