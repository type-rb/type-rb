# Performance experiments and result retention

Read for optimization acceptance, benchmark claims, diagnostic measurements,
or changes to retained results. Paths below are repository-relative.
Documentation-only edits do not start a new measurement cohort.

- Follow `docs/mir-consolidation.md` during the active migration. Integration
  explicitly observes intermediate cost overruns while correctness, safety and
  reproducibility remain blocking. Use daily/weekly trends and assess performance
  at coherent milestones; do not open a new size-budget review for each slice.
- Outside that phase, use `docs/optimization-tradeoffs.md` and the selected strict
  contract. Explicit standalone experiments retain exact identities, controls,
  run budgets and observations. A diagnostic is not final qualification; do not
  reuse historical source-specific exceptions or relabel a failed observation.
- Distinguish the assessment result from the adoption decision. A candidate with
  unmet numerical targets may be adopted without another confirmation when its
  overall benefit justifies the measured costs and maintenance burden under
  `docs/optimization-tradeoffs.md`. Record the rationale and every remaining
  miss or unknown; all required correctness and CI authorities still apply.
- Preserve all earlier failed results. Compare actual application build/size and
  runtime separately from compiler self-build/text/QBE/distribution costs; old
  Go measurements do not establish current or general-purpose spare capacity.
- At coherent checkpoints track incremental and cumulative cost and the next
  action. After two bounded size-only attempts without a current runtime-benefit
  assessment, choose bounded measurement, trade-off review, or justified deferral
  before another such attempt. Keep active evidence in its existing result slot.

Before importing or reorganizing `results/`, read `docs/evidence-retention.md`.
Retain only results with a current purpose in `results/active.json`. Replace
the occupied slot, update consumers and remove superseded directories in the
same PR; do not accumulate dated snapshots or archive every intermediate run.
Preserve active/public measurement cohorts, frozen comparison baselines and
seed consumers. Keep significant rejected-approach reasons in the development
history and link retired evidence to its exact Git revision. Use a verified
public archive only when detailed evidence has a durable purpose. Run the
lifecycle/global-budget check with the actual PR base before publishing results.

For executable checks, run the same differential corpus through the optimized
Go reference baseline and every active native candidate. Count frontend,
serialization, lowering, code generation, assembly, linking, runtime, sidecar,
and distribution costs according to `docs/experiment-plan.md`.

Interleave adjacent comparative measurements on shared runners, write medians
before applying bounds, and retain evidence from failed as well as successful
verification runs.
