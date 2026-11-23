# Optimization costs and trade-off evaluation

The active [MIR consolidation policy](mir-consolidation.md) supersedes per-slice
cost acceptance during the migration. Temporary cost regressions are allowed in
explicit migration integration; full correctness remains required. Strict/manual
contracts below remain available. Their historical failures are not relabeled.

Status: adopted on 2026-09-07. This policy governs future decisions; it does
not relabel any earlier failed measurement or accept an existing candidate.

## Separate the goals from the regression alarms

The product goals remain competitive end-to-end application build time and
deployed application size against the optimized TypeRB Go backend, and runtime
performance competitive with established static languages. Self-hosted compiler
cost, toolchain distribution, memory, and maintenance remain measured costs.

An application's build time is not the compiler's self-build time. Compiler
executable size, compiler text, compiler QBE, generated application size and
complete toolchain distribution are separate columns. A small-program Go
comparison is not a general budget for future features or a Pure Go build result.
Report the exact revision, coverage and age of every Go comparison; missing
matched measurements remain unknown, not inferred spare capacity.

There are two routes:

| Route | Purpose | Outcome |
| --- | --- | --- |
| Ordinary acceptance | Detect regressions against the registered Native baseline using existing limits and all correctness authorities | Eligible for normal review only when the complete contract passes |
| Bounded trade-off evaluation | Learn a promising candidate's runtime benefit even when an ordinary cost limit fails | Diagnostic evidence for a documented candidate-specific adoption decision |

The ordinary 1.05 ratios and absolute limits of the retired
[transition policy](https://github.com/type-rb/type-rb-native/blob/d1c151d02e051c6557acb479a9e9d7c713226858/tools/native-mir-transition-policy.sh)
governed strict acceptance; during MIR migration compiler size is recorded
without a limit. An explicit complete-compiler budget revision for safe Array
assignment is recorded in
[Decision 0029](decisions/0029-array-assignment-compiler-budget.md); it changes
no relative limit or historical result. The separately validated
[record Array budget](decisions/0030-record-array-compiler-budget.md) retains
the ordinary structured-data implementation cost without changing those ratios.
The separately validated [scalar guard budget](decisions/0031-scalar-guard-compiler-budget.md)
retains its bounded range-analysis cost without changing those ratios.
The separately validated [loop-local header budget](decisions/0032-loop-local-header-compiler-budget.md)
retains verified loop-effect analysis under the same relative limits.
The separately validated [Hash capability budget](decisions/0033-hash-compiler-budget.md)
retains compact Hash support, with a one-time source-pinned compiler size and
self-build allowance. Subsequent compiler changes use ordinary ratios again.
The separately validated [Hash index budget](decisions/0034-hash-index-compiler-budget.md)
retains compiler Hash self-use through a narrow amd64 ceiling revision, with
ordinary ratios unchanged. The separately validated
[Array iteration budget](decisions/0035-array-iteration-compiler-budget.md) retains
checked ordinary iteration through narrow complete-compiler ceilings, also
without changing relative limits. The separate
[Array self-build decision](decisions/0036-array-iteration-self-build-budget.md)
permitted an arm64 self-build ratio of 1.08 for one exact compiler-source pair;
that cohort failed and its old source is no longer eligible. The later Linux
assessment below has a separate source and scope. A cost failure blocks
ordinary acceptance, not necessarily further investigation. There is no general relaxed percentage or automatic
conversion from an evaluation budget to a merge budget.

Correctness, portable semantics, memory safety, failure ordering, MIR ownership,
reproducible self-hosting, target/process/cleanup requirements and evidence
integrity cannot be traded for performance. A diagnostic may stage expensive
authorities, but must say which remain unrun; acceptance still requires them.

## Runtime RSS and execution-performance trade-offs

New runtime optimization assessments use `go-competitive-runtime-rss-v1`,
implemented by `tools/daily-performance/rss_policy.py`. Apply this policy only
through an explicit prospective registration. Historical contracts and their
failed observations remain unchanged; a later assessment records its own policy
and sources instead of rewriting an earlier result.

The required application-memory goal is lower runtime peak RSS than the
same-source optimized TypeRB Go program and every available equivalent Pure Go
counterpart, measured with the same inputs and environment. Missing Pure Go
programs remain explicit coverage gaps, not assumed wins. A declared comparator
that is missing or invalid blocks assessment. A tie is not a memory advantage.

A Native-relative RSS increase is no longer an automatic rejection under this
policy. Record both absolute bytes and ratios against the immediately preceding
accepted implementation and the fixed cumulative baseline. An increase exceeding
both 10% and 256 KiB requires a documented trade-off review: identify the runtime
benefit, memory cost, implementation necessity and expected workload impact.
Do not adopt an unexplained increase without a useful compensating benefit.
These warning thresholds are operational choices, not statistical significance
or a reusable exemption for memory safety. Do not reset the cumulative baseline
after each adoption.

An RSS advantage smaller than 10% against either Go comparator also requires
review of the retained dispersion before calling the advantage reliable. Freeze
any necessary additional validation and its finite budget before execution;
do not repeat samples until the comparison passes. A wider measured advantage
does not require identical Native binaries to reproduce within a separate 5%
RSS band. A failed historical A/A observation remains valid evidence of that
protocol's dispersion.

This policy changes runtime RSS acceptance only. Required useful execution-time
improvements, other-case runtime regression limits, application-build memory,
artifact size, correctness, exact outputs, GC and lifetime safety, sustained
memory-growth checks, reproducibility and platform authorities remain binding.
RSS is a process high-water measurement, not a replacement for those checks.
Application-build time trades require measured competitive comparisons;
compiler self-cost remains a separate observation. A promising optimization can
be adopted while still slower than Go if it demonstrably closes the execution
gap and passes its registered benefit and regression contract. The intermediate
goal remains faster execution than same-source TypeRB Go across the public
runtime suite; the longer-term target remains equivalent Pure Go.

The RSS assessor returns `met`, `review-required` or `unmet`; none independently
authorizes an optimizer merge. Complete source/raw-data validation and every
other required authority in the owning assessment. If a review is required,
retain its explicit case-specific rationale before acceptance. Reconsidering a
held candidate under this policy needs a new finite validation scope; old repair
and measurement allowances are not silently renewed.

## Register a bounded evaluation

Before new measurements, put a public record in the candidate issue or PR with:

1. **Question and identity:** expected general-purpose benefit, exact candidate
   revision/source digests, backend/target, and any already known failures.
2. **Comparisons:** the frozen cumulative Native baseline; a same-feature
   control or narrowly described ablation to isolate the optimization; and the
   applicable Go comparison, or the explicit missing measurement needed before
   a Go-competitive claim. Retain both baselines when compatibility work moves on.
3. **Limits:** list the ordinary cost limits that fail and a numeric,
   candidate-specific investigation ceiling for each affected cost. Keep other
   limits unchanged. Include absolute bytes as well as ratios and resource/time
   limits. Explain the chosen headroom; do not inflate all limits together.
4. **Measurements:** selected and control programs, full input sizes, runtime
   wall/CPU/RSS, application build/size, compiler self-build/size and relevant
   external tools. Register repetitions, ordering, minimum useful runtime gain,
   permitted control regressions, and a bounded number of cohorts/runs.
5. **Prerequisites and stops:** correctness checks required before execution,
   remaining acceptance authorities, and explicit stops for wrong output,
   unsafe behavior, catastrophic regressions, budget exhaustion or expiry.
6. **Decision checkpoint:** expiry at a named coherent result or date, expected
   evidence, and whether to improve, propose a reviewed cost budget, defer with
   a concrete revisit trigger, or reject. Record maintenance cost and superseded
   code removal separately from unavoidable useful optimizer code.

Standing development authorization permits registering and running an in-scope
bounded diagnostic. It does not authorize silently changing ordinary CI,
acceptance thresholds, benchmark conditions, or the project's product goals.
Existing observations may motivate the registration, but retain their original
failure status and distinguish them from the prospectively measured cohort.
An expired budget is not renewed automatically. Reassessment requires a stated
question and justification, not repeated attempts until a favorable sample wins.

## Evaluate the whole trade

Report incremental cost against the same-feature control and cumulative cost
against the frozen baseline. Never reset the cumulative baseline merely to fit
the next 5% increase. Separate source growth from compiler implementation cost
where self-build comparisons compile different-sized compiler sources.

Prefer a candidate that improves representative runtime without worsening
other dimensions. If dimensions conflict, show their absolute and relative
changes without hiding them in a weighted score. Consider rebuild frequency,
runtime workload, memory, generated artifacts, portability and maintainability.
For example, a repeatable 20% runtime reduction with a 10% build-time increase
may warrant review; it is not an automatic exchange rate or acceptance rule.

QBE text size is a diagnostic signal, not a deployed artifact. Preserve its
ordinary CI check and historical values until a reviewed policy change, but do
not reject a bounded evaluation solely for QBE growth. Prioritize actual code,
complete compiler/application sizes, runtime, build costs and distribution.

Distinguish removal of duplicate/superseded emitter logic from the cost of a
useful verifier or optimization pass. Remove superseded ownership as a slice
migrates; do not require every useful new pass to become free before its value
can be investigated. A later fact-family proposal must account for outstanding
duplication and cumulative cost, rather than treating return to the exact
pre-MIR byte count as its sole eligibility condition. This does not authorize
another semantic owner, an unbounded optimizer, or premature LLVM expansion.

## Avoid both missed opportunities and budget drift

An attempt's stopping condition and an optimization idea's value are separate.
Distinguish implementation or validation incomplete, measured benefit absent,
measured benefit with an acceptance blocker, and adopted. A setup failure,
unsupported compiler-source construct or exhausted repair allowance does not
establish absent runtime benefit. Keep unfinished work pending with its concrete
blocker and next check rather than closing it as not planned for that reason.

Reconsider earlier ideas when a corrected implementation, resolved prerequisite,
changed policy or improved measurement method answers a stated open question.
Reuse their source and evidence, including unfavorable results, in the existing
task. Historical budget exhaustion prevents silently continuing that attempt;
it is not a permanent exclusion of the mechanism. Register any new comparative
cohort prospectively and do not resample unchanged candidates until they pass.

At each coherent checkpoint, record the next action and why. After two bounded
size-only attempts without a current runtime-benefit assessment, explicitly
choose a bounded benefit measurement, a cost/benefit review, or deferral with a
concrete reason before another size-only attempt. Do not let polishing a tiny
overrun become the default indefinitely. This is a reassessment trigger, not a
requirement to run expensive benchmarks for every speculative idea.

Keep a compact candidate assessment in the existing issue/result slot:
ordinary status, diagnostic budget status, runtime benefit (or unmeasured),
incremental and cumulative costs, remaining coverage, and next decision.
Retain complete active cohorts, including failures and warmups, under the
[evidence lifecycle](evidence-retention.md); do not create permanent dated
folders for each attempt. Expensive formal benchmarks remain explicitly
dispatched, not added to every PR's CI.

## Acceptance after evaluation

Within authorized performance or maintenance work, agents may adopt and merge a
candidate with unmet numerical targets without another maintainer confirmation
when the evidence supports an overall improvement. Judge application runtime
and build costs, compiler costs, RSS, generated artifacts, code quality and
maintenance burden together. Small or absent speed gains can justify simpler
code; extra complexity needs a useful compensating benefit. If the evidence
does not support that judgment, revise or defer within the authorized scope.

Before merging, record a candidate-specific decision with the benefit,
incremental and cumulative costs, every unmet or unknown metric, maintenance
trade-offs and any review or retirement conditions. All required correctness,
safety, reproducibility and CI authorities must pass. A diagnostic result alone
does not supply missing validation. Retain the registered assessment's actual
result: adoption does not turn an unmet measurement into a pass, change numerical
thresholds, renew an expired experiment budget, or complete an unmet initiative.

Do not use an admin bypass or weaken required CI to adopt a candidate. Changes
to enforced limits or project goals still require a separate maintainer decision
and the applicable reviewed implementation and checks. A later review-only
instruction or narrower task endpoint overrides standing merge authorization.

The detailed cross-language Pages comparison changes only with complete
accepted formal evidence. The separate [daily performance page](daily-performance.md)
publishes bounded diagnostic observations from accepted main, including
regressions and failed cases; it does not grant optimization acceptance or
replace individual rows in the formal comparison. Compiler size cleanup, a
focused diagnostic runtime gain, and a revised policy alone do not establish
general application speedup or Pure Go parity.

The separate [String output budget](decisions/0037-string-output-compiler-budget.md)
retains the measured stack-vector runtime cost through an amd64-only absolute
ceiling. It grants no self-build ratio exception: the output candidate must
pass ordinary 1.05 after retiring Decision 0036's exhausted source selector.

The separate [declaration-index assessment](decisions/0038-declaration-index-memory-tradeoff.md)
records a measured large-source time/space trade after its diagnostic RSS
condition failed. It changes no ordinary CI limit and grants no reusable memory
allowance; implementation acceptance remains separate.

The later [Linux Array assessment](decisions/0039-linux-array-self-build-assessment.md)
retains one exact post-refinement source pair at 1.07 for Linux self-build time
only. Decision 0036's old source is excluded; every other cost and source keeps
its existing contract. The new policy must pass independently before its single
full implementation acceptance cohort.
