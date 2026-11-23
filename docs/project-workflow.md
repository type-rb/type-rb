# Project workflow

Planned work is tracked in issues and the associated project board.
Agents do the implementation work. The maintainer approves initiatives and
answers decisions. There are no sprints: a weekly review and work-in-progress
limits keep the flow moving.

## Items

- **Initiative** (issue type): a measurable outcome delivered through several
  tasks, such as a reduced benchmark ratio or a completed language family.
  Long-running milestone issues, such as MIR consolidation (#439) and
  basic-language completion (#454), stay open as parents of initiatives.
- **Task** or **Bug** (issue type): one reviewable change, normally one PR. Each
  task is a sub-issue of exactly one initiative. A bug waiting for triage and an
  emergency repair of `main` may exist without a parent.

Open initiatives with the Initiative form, tasks with the Task form, and
specification improvements with the Specification proposal form. They set
the issue type and add it to the project when the author has project write
permission. The auto-add workflow also collects new or updated open issues from
this repository, including those created without the forms. A project
collaborator handles any missed additions during triage; the form's project
setting does not grant access. Use All for triage and historical lookup.

## Views and fields

| View | Shows |
| --- | --- |
| Initiatives | Open and recently closed initiatives, grouped by Initiative state |
| Tasks | Open work and issues closed in the last seven days, grouped by Status |
| Decisions | Items whose Attention is Needs decision |
| All | Every item as a table |

The Tasks filter is `-type:Initiative -closed:<@today-7d`; Initiatives uses
`type:Initiative -closed:<@today-7d`. Both hide work closed more than seven days
ago without removing it from All or requiring a sprint assignment. The rolling
window advances automatically. An issue without a closed date stays visible.

| Field | Values and meaning |
| --- | --- |
| Status (tasks) | Todo, In progress (including PR review and CI), Done |
| Initiative state | Proposed, Approved, Active (including measurement), Done |
| Attention | Empty when no help is needed; Blocked (link the blocker), Needs decision (state the question) |
| Priority | P0 urgent, P1 next, P2 later |
| Area, Family | Surface of the work; Family is a registry id from `tools/native-language-cases.json` |
| Claim, Claim updated | Current owner as `<agent>:<run-id>@<claim-sha>` and the date it last confirmed the claim |
| Touches | Main files or modules the task changes, used to avoid conflicting work |
| Metric, Target, Baseline, Current, Measured, Evidence | Initiative measurement; see below |

Todo records pending work; it does not authorize implementation. A task is
eligible to start when its initiative is Active, its acceptance criteria and
Touches are clear, its dependencies are resolved, and no decision is pending.
Imported issues without a parent remain available for triage, not automatic
execution. Closed issues remain in the project as Done; the close reason
distinguishes completed work from work closed as not planned.

## Starting work

- Anyone may record a Todo task, a bug or a Proposed initiative.
- Tasks may be planned under an Approved or Active initiative.
- Implement only eligible tasks under an Active initiative. Take P0 before P1
  before P2, and older items first within a priority.
- Keep at most five initiatives Active so that started work finishes. When a
  slot is free, an agent activates the highest-priority Approved initiative
  after recording its Metric, Target and Baseline and preparing the first task,
  and notes the activation on the initiative issue.
- An agent run holds at most two tasks In progress: one being implemented and
  at most one waiting for PR review or CI. Waiting work still counts toward WIP.
- When `Main validation` fails, repairing or reverting `main` comes first and may
  skip the normal eligibility and initiative rules. Record the repair as a task
  afterwards when the fix needs follow-up work.

## Claiming a task

Claims are serialized through a create-only branch with a unique commit for
each acquisition attempt. Run this from the repository, replacing the task
number and owner with public identifiers:

```bash
set -eu
task=123
claim_owner='codex:REPLACE_WITH_RUN_ID'
claim_ref="refs/heads/claim/task-$task"
git fetch origin main
claim_base=$(git rev-parse origin/main)
claim_tree=$(git rev-parse "$claim_base^{tree}")
claim_nonce=$(python3 -c 'import uuid; print(uuid.uuid4())')
claim_sha=$(git commit-tree "$claim_tree" -p "$claim_base" \
  -m "Claim task $task by $claim_owner; nonce $claim_nonce")
git push --force-with-lease="$claim_ref:" origin "$claim_sha:$claim_ref"
```

The nonce must be fresh for every acquisition attempt, even in the same run.
Pushing `origin/main` directly is unsafe: when the existing branch already
points to that commit, Git returns success with `Everything up-to-date` even
with the empty lease. The distinct claim commit makes an existing branch reject
the create-only push. A plain push is not a claim either. If creation fails,
do not start work or update the board. If the network result is uncertain,
compare the remote ref with the saved `claim_sha` before retrying or proceeding.

The claim commit has the baseline tree and is only an ownership token. Create
the implementation branch from main, not from the claim branch. Keep the token
SHA for the lifetime of the claim and never add implementation commits to it.

After a successful claim, set Claim to `<agent>:<run-id>@<claim-sha>`, set Claim
updated to today and move Status to In progress. If a board update fails, retain
the branch and reconcile the fields before working. Refresh Claim updated each
day the work continues, and verify the remote ref still matches the saved token
before resuming work or changing ownership fields.

When the task reaches Done, or the owner stops working, reconcile Status and
clear Claim and Claim updated while still holding the branch. Unfinished work
returns to Todo; record any blocker in Attention. Then release only the saved
token:

```bash
git push --force-with-lease="$claim_ref:$claim_sha" origin ":$claim_ref"
```

Never replace the saved SHA with a newly observed owner's SHA to force release.
A claim that has not been updated for two days and has no open PR needs
investigation: comment on the issue and set Attention to Needs decision. A date
alone does not prove the owner stopped. Reclaim only after the maintainer
confirms it is inactive, using the old token's conditional deletion before a
fresh acquisition. A resumed former owner must stop if its token no longer
matches. Claim branches are coordination records and are never merged.

## Delivering a task

1. Work on a separate branch and open a PR. Use `Closes #<n>` for the Task or Bug
   whose acceptance criteria the PR completes. Keep Status In progress through
   review and CI. For partial work, use a plain issue reference and leave it open.
2. Merge once PR acceptance passes, following [CI validation](ci-validation.md),
   within the task's merge authorization. Merging to the default branch closes
   the linked task, and the project automation sets Done. For investigation or
   documentation work without a PR, record the result and evidence before
   closing the completed task.
3. Release the claim and clean up the implementation branch and worktree.
   Task closure does not require a separate wait for `Main validation`.
4. `Main validation` remains the repository integration authority. Monitor its
   result, and fix or revert a failure before further feature merges. If a
   revert removes a task's delivered result, reopen it and return it to Todo,
   or In progress if its owner still holds a valid claim and is fixing it.

Never use a closing keyword or a closing Development link for the parent
initiative in an implementation PR. A completed child does not prove the
initiative's outcome. Inspect the PR's linked issues before merging; even a
negated closing keyword can close an issue.

A dropped task records the reason in a comment and is closed as not planned.

## Flow checkpoints

At work start, before waiting or handing off, after a PR merges, and before
reporting completion, reconcile the affected issues with current PRs, checks
and dependencies. Review the changed work and its parents, not the entire
history each time. Reuse the latest checkpoint when its evidence remains
current; do not post an unchanged status report.

A waiting item needs a concise issue checkpoint containing:

- its phase, such as implementation, local validation, PR/CI, acceptance,
  decision or dependency wait;
- the last substantive result and its evidence, not merely a claim heartbeat;
- the next concrete action and its responsible owner;
- the blocker and a review trigger, such as a dependency merge, CI completion,
  initiative activation or a review date.

Use the existing Status, Attention, Current and Evidence fields where applicable;
the checkpoint does not require new Project fields. Waiting still counts toward
the applicable WIP limit. A review trigger prompts investigation, not automatic
claim reassignment, closure or experiment restart. Follow the stale-claim and
authority rules above.

For a batch PR, map each affected task to the acceptance criteria it satisfies,
the evidence and any remaining work. Reconcile each task after merge. A partial
reference stays open with its actual next action; completed implementation must
not remain described as untouched work. Parent outcome and Main integration
checks retain their independent authority.

Each Active initiative must have a next eligible task or an owned action to
resolve acceptance, measurement, a dependency or a decision. Flag an initiative
with neither for review; do not invent another implementation task merely to
keep it Active. Before completing an initiative, inspect every open child and
record its disposition: completed, not adopted, continuing under another
approved outcome, or deferred with an explicit reconsideration condition.
Respect the authority required by each disposition. Do not bulk-close children
or activate a different initiative to hide unfinished acceptance.

Keep the Decisions view for actual unresolved choices. State the question,
options, recommendation, affected work and decision owner, with a review
trigger. When a recorded decision settles non-adoption, preserve its reason,
failed observations and source recovery, then close the dropped task as not
planned. An inconclusive experiment is not proof of no benefit: record what
method or evidence is missing. Repeated preparation failures need a shared
tooling repair before another candidate repeats the same broken procedure.
None of these transitions renews a closed measurement budget.

The lightweight review looks for stale claims without PRs, completed checks
still described as pending, blockers whose dependencies have completed, open
children of completed initiatives without a disposition, and decision requests
without a concrete question. Report actionable changes only. This is an
on-demand procedure; it does not enable a scheduled workflow or change Project
settings.

## Project automation

Use issue cards as the work records; linked PRs supply review and CI context.
Enable these built-in workflows in Project settings:

- Auto-add to project: new or updated open issues in this repository, using
  `is:issue is:open` with the repository selected.
- Item added to project: issues get Status Todo. When importing already closed
  issues, explicitly set Done after adding them; this workflow has no state filter.
- Item closed: issues get Status Done.
- Item reopened: issues return to Todo.
- Auto-add sub-issues to project: keep child tasks with their parent.

Keep Auto-close issue disabled: dragging a card must not close an unfinished
issue. Pull request merged is unnecessary for this issue-only board; task
closure through its PR triggers Item closed. After reopening an issue, restore
In progress only if its owner still holds a valid claim and is resuming work.
Initiative state is managed separately from Status and is never inferred from
a merged child PR. Set a newly added initiative to Proposed during triage.

## Authority

Work that preserves specified behavior and the approved scope
proceeds without further approval, from task creation through merge and cleanup.
This includes reference conformance, MIR and performance improvements, and
maintenance. The maintainer decides what TypeRB is, including specification
changes and any Native behavior that differs from the pinned reference. The
maintainer also decides direction, publication, irreversible actions and
security-sensitive settings.

Agents may adopt performance or maintenance candidates with unmet numerical
targets when the evidence supports an overall improvement under
[the trade-off policy](optimization-tradeoffs.md#acceptance-after-evaluation).
Record the benefit, costs and remaining misses before merging; another
confirmation is not required solely because a target is unmet. Candidate
adoption does not change an initiative's target or establish its completion.

| Agents act without asking | The maintainer decides; agents set Needs decision |
| --- | --- |
| Create, refine and order tasks, and activate the next Approved initiative when a slot is free | Approve a Proposed initiative or a decision record that changes direction; rescope, drop or retarget an initiative |
| Implement, open PRs, merge after PR acceptance passes and clean up branches | Adopt a specification change or a deviation from the pinned reference, or send one upstream |
| Fix or revert a failing `main` | Publish releases or new bootstrap seed assets |
| Update the pinned reference revision through the release integration procedure | Remove or weaken required CI checks |
| Update documentation, fixtures and generated views within the task's scope | Change secrets, permissions, organization or project settings |
| File, reproduce and triage bugs, and file specification proposals | Post to other repositories or external services |
| Record current evidence and complete an initiative whose recorded criteria are met | Change an initiative's target or accept a different completion outcome; stop or reopen an initiative |

Waiting for a decision blocks only the affected item. Agents continue with other
eligible work in the meantime.

A maintainer's direct request defines its own scope and completion point; these
rules apply to work an agent takes from the board.

## Initiative metrics

An initiative states one Metric with the command or workflow that measures it,
and a Target. Baseline records the value, revision and conditions. Current,
Measured and Evidence record the latest value, its date and a link to the run or
comment that produced it. A value without Measured and Evidence is not a
measurement, and a value measured before the latest relevant merge is stale.
Compare only measurements taken under the Baseline conditions.

Keep an initiative Active while measuring its outcome. When current evidence
under the recorded baseline conditions meets its existing Target and every
recorded completion criterion, an agent updates Current, Measured and Evidence,
records the comparison on the issue, sets Initiative state to Done and closes
the issue as completed without another approval. A merged child alone is not
completion evidence. Do not change a target or its measurement conditions to
claim success. Incomplete or ambiguous evidence leaves the initiative Active;
continue measuring or request a decision on the unresolved criterion while
other eligible work proceeds.

Closing an initiative with an unmet target or a different outcome requires the
maintainer's explicit decision with the reason recorded. Adopting an improving
candidate under the trade-off policy leaves an unmet initiative Active.
To stop an initiative, record that decision, set Done and close as not planned.
Reopening requires an explicit
lifecycle decision and must respect the five-Active limit; it does not resume
work by itself.

## Weekly review

Each week the maintainer, with an agent preparing the summary, reviews:

- the Decisions view and Blocked items;
- Current against Target for each Active initiative;
- Proposed initiatives to approve or drop, and the priority order that agents
  follow when they activate Approved initiatives;
- stale claims, long-running tasks and PRs waiting for review or CI.

Use the flow checkpoints to prepare this review. Focus on unresolved direction
and ownership choices rather than rereading unchanged history. Track whether
next actions happened, time spent waiting for decisions or acceptance, and
recurring preparation failures; issue or comment counts alone are not progress.

The board and issues are the source of truth for work state. Local session notes
may hold environment details and handoffs, but not task ownership or status.
