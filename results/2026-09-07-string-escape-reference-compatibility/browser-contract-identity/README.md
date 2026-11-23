# Browser and JSON contract identity compatibility

This update uses the registered compatibility evidence slot. Earlier observations
retain their original source and reference identities.

The selected development reference is TypeRB `0.4.12-dev` at
`c49b67ec0f9e5941c52b787885f11a33c19f795f`, merged in
[type-rb/type-rb#850](https://github.com/type-rb/type-rb/pull/850).
It preserves defining-module provider contracts, distinguishes caller type
parameters, and retains declaration identity in browser and JSON code generation.
Its complete local Go suite and required PR checks passed. This is an exact
source pin, not a release or supported version range.

The preceding qualified Native main is
`56e5375ffc799967b9b6628f2c13147549863ee9`, validated by
https://github.com/type-rb/type-rb-native/actions/runs/41421430635.
That workflow predates this reference update; it does not qualify the new pin.
Historical manifests and immutable seeds retain their source-era identities.

## Local qualification

The candidate source is
`37c321e478253184bb29b0e739d8180789c6ac61`. Local checks on Darwin arm64 completed
with the exact reference above. The 742 compiler/recovery source hashes remained
unchanged throughout qualification.

| Maintained command | Observation |
| --- | --- |
| `python3 tools/build-reference.py <reference-checkout> <reference-trb>` | Exact reference executable built successfully |
| `sh tools/preflight.sh <reference-trb>` | All maintained quick checks passed |
| `python3 tools/native-web-client-test.py <reference-trb> --reference --browser-check --browser-runtime <node> --report <report>` | 47 generation/output checks; 19 accepted browser projections; one explicit renamed-import gap; 12 synthetic Fetch checks each for `Body` and `Payload` |
| `sh tools/build-native.sh` | Ordinary Native core and CLI fixed points reproduced |
| `python3 tools/native-web-client-test.py bin/trbn --report <report>` | 47 generation/output checks passed with the freshly built Native CLI |
| `python3 tools/check-conformance-sources.py .trb/bootstrap/core/compiler` | 424 sources checked, no mismatches |
| `python3 tools/native-language-coverage.py --reference <reference-trb> --reference-ast <reference-checkout>/internal/ast/ast.go --jobs 4` | 2,739 reference observations, no failures, exact AST verified |

The final row verifies the reference oracle and registry expectations; it is not
2,739 passing Native parity cases. The registry's 67 declared parity gaps remain
visible. Both browser reports have `complete: true` and no generation failures;
the reference report retains the one known projection gap separately.

The ordinary build used the maintained QBE backend, immutable
`bootstrap-seed-2026-09-29-module-cycles` Darwin arm64 seed and the configured
network/crypto/OIDC development libraries. `TYPE_RB_CHECKOUT` identified the exact
reference checkout for preflight and source provenance. These are local
development inputs, not newly qualified distribution assets.

| Ordinary artifact | Bytes | SHA-256 |
| --- | ---: | --- |
| `bin/trbn` | 7,009,976 | `c91158d7494807c666733eda72abbe2db6c516bfa75dcb571f35ea9a8fbcff9a` |
| `.trb/bootstrap/core/compiler` | 3,094,136 | `89a810498859d6355264c2e6d8b0b90f2e809fd449309b9647f65e8751a3cdb1` |

Hosted PR acceptance and subsequent Main validation remain separate integration
authorities; the preceding main workflow above cannot stand in for them.

## Scope

The original generated client with application `Body` and the `Payload` control
must both compile through the reference browser backend and pass twelve synthetic
Fetch checks each. Native client generation retains its exact output and rejects
invalid output paths without invoking target tools. Browser backend execution is
reference qualification; it does not establish Native TypeScript output support.
The renamed shared-import gap #1183 and package ownership review #1211 remain
separate. Full shared v0.5 and release qualification remain open.
