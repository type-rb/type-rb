# Portable assertion reference compatibility

This update shares the registered compatibility evidence slot; preceding
observations in that slot retain their original source and reference identities.

The selected development reference is TypeRB `0.4.12-dev` at
`d34ff9a21a10167c4ddc13820b73186c4898fa5a`.
[Reference PR #849](https://github.com/type-rb/type-rb/pull/849) and its
[main validation](https://github.com/type-rb/type-rb/actions/runs/41439086819)
implement the approved common assertion value display. This exact revision also
includes aliased library factory type preservation and the shared v0.5 contract
and scoped release deferrals. It is not a release or supported version range.

The preceding fully validated Native main baseline is
`56e5375ffc799967b9b6628f2c13147549863ee9`, validated by
https://github.com/type-rb/type-rb-native/actions/runs/41421430635.
That evidence belongs to the previous reference pin. New exact-pin source,
bootstrap, language and test-runner observations are recorded below. Historical
observations and immutable seeds are unchanged. No broader language or release
completion is inferred from this pin.

## Local qualification

The final compiler source is
`c46ab67f0cdc7358aacc1f7536f07c7efef084d5`. On Darwin arm64 it passed:

- The maintained `tools/preflight.sh` against the exact reference, including
  source checks, the recovery snapshot subset, bundled-source verification,
  current project scenarios and the frozen test contracts.
- The ordinary compiler and CLI build with fixed-point verification, followed
  by all 424 conformance sources using the resulting core compiler.
- All 19 required scenarios in `native-test-manifest.json`; the approved
  name-selection deferral is excluded from the passing count.
- All nine groups in `native-test-values.json` (97 cases), with exact failure
  messages, source locations, event order, counts and exit status.
- All 26 CLI/reference controls and eight full-width comparison boundary
  checks, including standalone test output preservation and later-test
  continuation.
- The 72 focused compiler/MIR/runtime checks, including independently verified
  object identities, source erasure, forced collection and scoped recovery.
- The complete 1472-test compiler suite at `55c2dac2`, with the recovery root,
  exact reference and QBE environment enabled. The repaired fatal-status and
  isolated-runtime boundaries also passed 116 focused tests. Thirteen ordinary
  JSON diagnostic cases now report the authored call line matching the reference;
  their statuses, messages and remaining unsupported contracts are unchanged.

The reference-language replay used `tools/native-language-coverage.py` with the
exact reference executable and AST. All 2739 reviewed observations matched;
this is reference-pin qualification, not complete Native language parity. The
registry's existing gaps remain visible. Current reference project scenarios
preserve every historical command, with only the accepted structural-value
display changes; the historical M2 manifest remains unchanged.

Earlier failed observations exposed duplicate generic display identities and
eager instantiation of unused assertion methods. The final source retains
concrete generic type IDs independently of display names and instantiates only
the selected loader-owned assertion methods. The corresponding rejection and
positive regressions are part of the final checks above. Nullable Boolean and
authored `Nil` reference inconsistencies remain tracked in #1310 and #1297.

## Integration boundary

Required hosted PR acceptance, target/recovery authorities and the subsequent
Main validation remain integration gates. Local success does not establish
those results, complete source-backend test execution, four-host release
qualification or v0.5 release readiness. The compatibility manifest retains the
preceding validated main identity until current hosted evidence is available.
