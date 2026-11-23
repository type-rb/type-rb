# Module initialization reference compatibility

The selected reference is the published TypeRB `0.4.9` release at
`77cdff4a1e7588d21697b777b352284e812c7567`.
[Release validation](https://github.com/type-rb/type-rb/actions/runs/36536006188)
succeeded before the Native pin was advanced. The release includes
[PR #831](https://github.com/type-rb/type-rb/pull/831) for unary-expression
formatting and [PR #832](https://github.com/type-rb/type-rb/pull/832) for
source-directory-independent Go emission and explicit module initialization.
The reference AST is unchanged. The structural-test failure observation changes
only its generated Go type prefix from `main.Point` to `application.Point`;
the assertion location, values and failure status are unchanged. The same-named
record case `type-alias-scope` now builds and executes successfully, matching
Native. Two existing rejected-output cases retain their failures with generated
Go paths under `trb/application`; their known semantic gaps remain recorded.

The Native baseline is `095adfe5758b8c94d1f14174d7923915df86bb9e` with successful
[Main validation](https://github.com/type-rb/type-rb-native/actions/runs/36522013285).
That run validates the preceding pin, not this candidate. Current exact-pin
observations and acceptance belong to the adopting PR's validation results.

Native keeps checking every configured production source while initializing
only the runtime import closure. Verified MIR carries initialization membership;
the QBE backend preserves global roots and initialization guards. Shared
ordinary-path cases and configured-project scenarios cover directory crossings,
dependency order, shared dependencies and unused-file effects. Existing
source-cycle rejection and known Native test-runner gaps remain explicit.

The independent Native `0.1.0-dev` identity, immutable bootstrap assets and
[earlier reference evidence](../README.md)
remain unchanged. This update does not claim complete language or performance
qualification.
