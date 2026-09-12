---
name: publish-typerb
description: Prepare or update TypeRB GitHub pull requests and their required release metadata.
---

# Publish TypeRB

Publish through a pull request; never push changes directly to the default
branch. Complete the requested edits and checks before requesting review.
A PR-only request ends with an open PR and retained branch/worktree. Merge only
with user authorization; a review boundary overrides older standing approval.

Use local Git for branches, commits, and pushes. Prefer an available authenticated
GitHub connector for PR metadata; use `gh` when it provides the needed capability.
Do not let a missing optional client block an equivalent authenticated workflow.

## Branch, commit, and metadata

Choose a primary change type: `feat`, `fix`, `refactor`, `perf`, `test`, `doc`,
`build`, `ci`, or `chore`. Apply exactly one corresponding PR label; other status
or area labels may coexist. Create a missing change-type label when needed.

Follow an explicit user or repository branch convention; otherwise use
`<type>/<short-kebab-description>`. An explicitly different prefix does not change
the semantic PR label. Avoid agent/username prefixes. Commit coherent units with
a concise imperative subject and only the rationale a reviewer needs. Keep
unrelated user edits out of commits.

## Prepare review

- Complete relevant checks, push the branch, and target the repository's default
  branch. Keep a PR draft while required implementation or verification remains.
- Describe the final problem, resulting behavior, validation, and material limits.
  Include the exact [release-note metadata](references/release-note.md).
- Assign the PR author, using the login returned by current PR metadata, and
  apply its change-type label. Do not guess the author.
- Mark it ready once reviewable and required checks pass. Check results for the
  actual candidate; do not repeat unchanged passing checks without a reason.
- Report the open PR when that is the requested endpoint. This workflow does
  not itself authorize merging, publishing a release, or starting another task.

## After an authorized merge

Confirm the PR merged, fetch with pruning, and synchronize a clean default
branch. Delete only confirmed merged task branches with `git branch -d`; preserve
unrelated changes and follow the active workspace workflow for worktree cleanup.
Never force-delete or bulk-delete branches. Keep open review branches available.
