# Shared v0.5.0 release contract

This is the versioned inventory required by the accepted
[implementation transition](../../docs/native-transition.md). The reviewed
source baseline is `type-rb/type-rb` at
`c1e4a898fcb546080c4a3f6b6dda10ba4f7698e1`. It inventories existing Go
contracts; the proposed 1.0 capability roadmap is not the denominator.
The generated [feature view](../../docs/v0.5-feature-contract.md) explains the
91 release rows. The machine-readable catalog retains all 676 discovered
owners and their 4,067 declarations, fields, flags and owning test names.
Those counts describe inventory structure, **not supported features or percent
completion**. A test name is not a scenario, and a package signature is not
proof of its semantics.

## Sources and ownership

`surfaces.json` records the public standard/official registry, builtin receiver
signatures, lexical/type kinds, AST ownership, documented sections, CLI
dispatch/flags, structured protocols, LSP messages, editor/distribution inputs
and existing Go test owners. Each surface has exactly one explicit owning row
in `manifest.json`; related implementation/decision tasks remain linked without
transferring their claims. Source wrapper bodies and private helpers are not
advertised as public APIs. Platform-only packages retain their actual output
applicability. Portable packages also require Native executable evidence.

The discovery guard detects added/removed owners and declaration, flag, field,
mode or test-name drift. It does not prove semantic completeness, infer
behavior from bodies, or treat internal schema records as extra user features.
Review behavior changes against the specification and owning tests even when
the census is unchanged. Any shared feature added before release needs a
reviewed row/member update and both implementations' applicable evidence.
Deferred class design and unresolved cross-mode differences remain decisions;
this inventory neither adopts proposals nor changes compatibility pins.

The initial baseline is deliberately unqualified. Existing green CI or a
Native feature batch is useful evidence, but neither qualifies the complete
paired release. Contradictory rows link known decisions, including project test
selection, class dispatch, time/JSON and Web behavior. Other rows can contain
additional unknown gaps; `unqualified` must never be interpreted as supported.

The next qualification owner is
[#1221](https://github.com/type-rb/type-rb-native/issues/1221), which maps every
row/member to exact-candidate scenarios and fills missing evidence. Distribution
and editor migration remain under
[#1168](https://github.com/type-rb/type-rb-native/issues/1168). Existing practical
Web/ORM/jobs and language tasks keep their implementation scope and acceptance.

## Maintaining the inventory

Run from the canonical repository root:

```sh
go run ./tools/release-contract --discover > candidate-surfaces.json
go run ./tools/release-contract --check
go test ./internal/releasecontract ./tools/release-contract
```

Compare discovered additions, removals and signatures against the exact source
revision. Review each affected row's observable success/failure contract,
applicable implementations/outputs, commands, scenario obligations and remaining
owner. Then update `surfaces.json`, its SHA-256 in `manifest.json`, and the
reviewed source revision as appropriate. Do not blindly regenerate a smaller
denominator or use a changed Go bug as the specification. Generate the view:

```sh
go run ./tools/release-contract --write-view
go run ./tools/release-contract --check
```

The manifest's identity is SHA-256 of its exact committed bytes. Its embedded
catalog digest binds the complete declaration inventory. Formatting a manifest
therefore deliberately invalidates old result bundles. CI checks the inventory
and generated view even when only documentation/JSON changed.

## Result schema and candidate qualification

The v2 result schema is defined by the JSON-tagged `Results`, `Candidate`, `Cell`
and `Observation` records in
[`internal/releasecontract/results.go`](../../internal/releasecontract/results.go)
and the strict decoder/validator. Unknown or missing fields, null required
arrays, duplicate cells/candidates, stale manifest identities and omitted cells
are errors. Generate a complete starting document rather than a partial report:

```sh
go run ./tools/release-contract --results-template > results.json
go run ./tools/release-contract --results results.json
go run ./tools/release-contract --results results.json --qualify
```

The first validation accepts a structurally complete unqualified progress
report. The final command intentionally **fails** for that template. Cells with
manifest-approved deferrals use `qualified-with-deferrals`, never
`passed`; all their other obligations must still pass. These cells qualify the
approved release scope, not full parity. `unqualified`, `blocked`,
`contradictory` and `failed` require a reason and remain in the matrix.

- Top level: `schemaVersion: 2`, `release: "0.5.0"`, `manifestSHA256`,
  `candidates`, and `cells`.
- Candidate: `implementation` (`go` or `native`), `host`, exact 40-character
  `sourceRevision`, 64-character `compilerSHA256`, and nonempty `toolchains`
  mapping tool names to exact versions/identities. One identity per
  implementation/host prevents mixed candidate receipts.
- Cell: `row`, `implementation`, `output`, `host`, `status`, optional `reason` and
  `deferrals` (exact manifest IDs),
  and `observations`. The manifest expands every required row/output across
  both implementations and retained Darwin/Linux amd64/arm64 hosts. `service`
  denotes CLI/protocol/distribution behavior independent of program output.
  Native-only qualification has no Go implementation cell. Go never acquires
  a Native output obligation.
- Observation: `obligation`, owning `commandID`, actual `executedArgv`, positive
  `cases`, explicit `skipped`, `failures`, `exitCode`, relative `evidence` file
  and its `sha256`. A passing cell requires every obligation exactly once,
  zero skipped/failed cases, successful runner exit and matching file hashes.
  A negative-language/runtime scenario passes when the runner verifies its
  expected rejection; it must not be omitted as unsupported. Evidence files
  must remain inside the supplied bundle, including through symlinks.

Command arrays are existing owning checks, not automatically executed by this
tool. `${REFERENCE_TRB}`, `${NATIVE_COMPILER}` and `${NATIVE_CLI}` are explicit
candidate inputs whose actual expanded paths and hashes belong in receipts.
Go source-unit tests cannot qualify a Native candidate. Existing Native language
and project manifests must keep their frozen expectations and cannot substitute
for every release row. Backend compilers/runtimes, databases, external adapters,
editor packages and browser tools must be recorded when a scenario uses them;
a missing dependency cannot become a skip-based pass.

The v2 manifest may attach scoped `deferrals` to a row. Each has a unique ID,
exact release, implementations, outputs, existing obligation names, plain-language
scope/reason, public decision/owner issues and review trigger. Hosts cannot be
waived. Unknown or overlapping selectors are rejected. The manifest hash binds
these decisions; a result file cannot add, omit or broaden them. Required test
selection and browser availability diagnostics remain outside the exemptions.
The distribution exemption covers only hosted browser tools, not documentation.

For every cell the result template derives deferral IDs from the manifest.
Passing receipts must omit deferred obligations and provide all other obligations
with no skipped cases. A cell containing deferrals must use
`qualified-with-deferrals`; reporting it as `passed` is an error. The qualification
summary counts these separately. Ordinary progress/failure statuses retain the
same exact IDs, reasons and unsuccessful observations. Changing a deferral or
schema invalidates old result bundles; never rewrite historical evidence to
turn an earlier failure into success.

Every row starts with `qualificationReady: false`. Before changing it, review
the complete member-to-scenario mapping, exact candidate consumption and
per-output command plan in #1221. Passing evidence is rejected without that
reviewed plan. The structural validator checks receipt consistency, not whether
a runner honestly exercised the claimed behavior; scenario review and required
CI remain essential. Keep compact logs/counts/identities and failures, not
duplicated binaries, caches or extracted dependency trees.

Even all functional cells passing does not authorize release or cutover. The
independent performance, memory, target, ordinary self-hosting, recovery,
security and reversible migration criteria still apply. No release version,
history, seed, source pin or repository setting changes in this inventory.
