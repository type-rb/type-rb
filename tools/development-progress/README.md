# Weekly development progress

The [weekly workflow](../../.github/workflows/development-progress.yml) publishes
a short Actions summary and a JSON/Markdown artifact on main. It reads four
diagnostic indicators without compiling programs or accepting pull requests:

- reviewed Native/reference probe differences and pending basic contracts from
  the checked-in ordinary-language registry;
- median and p90 execution and queued feedback time for completed PR workflows
  in the previous seven days, including failed runs;
- the latest validated daily self-compilation build time and QBE text size; and
- per-workload Native/Pure Go runtime ratios from that same daily cohort.

The daily state loader verifies the producing workflow, repository, branch,
artifact size and state schema before this report uses it. Missing or failed
daily observations remain unavailable, and every shown daily value carries its
measurement date and Native revision; a newer main revision is called out. The
PR CI sample is capped at the latest 300 runs; it is a feedback diagnostic, not
a statistical performance test.
Scheduled reports run early Monday Japan time after the weekly language cohort.
Manual dispatch on main allows an immediate report without waiting for the
schedule. Neither path writes benchmark state or source files to the repository.

To exercise the report without GitHub access, pass a reviewed registry, a
validated daily state JSON, and a `gh run list --json` result to `report.py`.
Run the focused tests with `python3 -m unittest discover -s
tools/development-progress -p 'test_*.py'`.
