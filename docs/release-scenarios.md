# Exact-candidate release scenarios

The [shared release contract](https://github.com/type-rb/type-rb/tree/main/contracts/v0.5)
owns the release denominator, output/host obligations, qualification decisions
and approved deferrals. The Native [scenario plan](../tools/release-scenarios.json)
binds its declarations to executable examples. It supplements the existing
language/project corpora; it does not modify their frozen expectations or replace
the canonical result validator.

Inspect the complete inventory against an identified reference checkout:

```sh
python3 tools/release-scenarios.py --contract-root /path/to/type-rb --inventory
```

Every canonical row stays visible. New members and surfaces remain unmapped;
removed or foreign declarations invalidate existing bindings. Mapping every
declared member still leaves semantic boundaries, failure cleanup and the other
output/host observations to qualify. Neither mapping counts nor matching examples
are a support percentage. The plan records remaining boundaries independently.

Run a candidate identified by its source revision and expected binary digest:

```sh
python3 tools/release-scenarios.py --contract-root /path/to/type-rb \
  --compiler /path/to/compiler --compiler-sha256 EXACT_SHA256 \
  --source-revision EXACT_COMMIT --implementation go --mode ruby \
  --report /path/to/new-observations.json
```

`--implementation` is `go` or `native`. `--mode` selects `go`, `ruby`,
`typescript` or Native's additional `native` executable output. TypeScript
uses `--typescript-runtime node|bun`. The supplied revision must come from the
candidate's build receipt: a binary hash binds the executable, but cannot prove
its source provenance by itself. The runner records the actual host, compiler,
controller, plan, catalog and manifest hashes and required tool identities.
It verifies the binary identity again after execution. Native observations use
the configured QBE and dependency archives; they do not build a compiler from a
neighboring checkout or fall back to the Go implementation.

Each example exercises project checking, source/native build, actual application
execution and a fresh retained REPL session. Ruby and TypeScript execute the
compiler's generated application entry, which performs initialization and calls
`main`; loading the declarations module alone does not run the application.
Negative inputs require the reviewed diagnostic and cannot pass because a
package or operation is unsupported. A zero REPL exit code with diagnostics or
missing expected output fails. Timeouts, invalid output encoding, missing tools,
missing artifacts and empty plans cannot become skipped passes.

Exit zero means only that the selected candidate/output matched this plan on
this host. Exit one retains semantic mismatches; exit two reports preparation
failure. Reports retain each observed phase, incomplete cases and interruption
reasons, and existing files are never overwritten. Temporary projects and
generated executables are removed after success or failure; keep the small
reports and exact commands rather than copying build caches.

Reports always retain `releaseQualified: false` and unqualified inventory rows.
They are evidence inputs, not canonical v2 result bundles or result-side
waivers. Review the complete mapping and remaining boundaries in #1221 before
changing any canonical `qualificationReady` value. Independent recovery,
performance, memory, distribution and repository-transition gates remain.
