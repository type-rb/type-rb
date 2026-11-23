# Cyclic module reference compatibility

The selected reference is the published TypeRB `0.4.10` release at
`af8e5f52e18e265d20bab7efbeaa2c36ea9342ae`.
[Release validation](https://github.com/type-rb/type-rb/actions/runs/36565340204)
succeeded before the Native pin was advanced. The release includes
[PR #835](https://github.com/type-rb/type-rb/pull/835), which permits source-module
cycles within one compilation unit and specifies checked, deterministic cyclic
initialization. The reference AST is unchanged.

The Native baseline is `5a6866c58de4a41fdb6089b956071341a9c15f9b` with successful
[Main validation](https://github.com/type-rb/type-rb-native/actions/runs/36542532758).
That run validates the preceding pin, not this candidate. Current exact-pin
observations and acceptance belong to the adopting PR's validation results.

The shared ordinary-path cases cover cross-file recursion, inferred and declared
values, defaults, class preparation and constants, namespaces, self imports,
unused program sources, and initialization or inference failures. Verified MIR
carries initialization membership and order for both execution and the REPL;
independent validation rejects forged order and unverified calls. Recovery
coverage exercises safe cyclic initialization and rejects value cycles before
external tools are invoked. Compilation-unit dependencies remain acyclic.

Compiler implementation sources still retain the import contract accepted by
the current immutable seed. Supporting cyclic input does not by itself authorize
adopting cyclic imports in the compiler's own source closure.

The independent Native `0.1.0-dev` identity, immutable bootstrap assets and
[earlier reference evidence](../README.md) remain unchanged. This update does
not claim complete language or performance qualification.
