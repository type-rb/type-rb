---
name: publish-typerb
description: Publish TypeRB changes through GitHub. Use when creating a branch, choosing commit messages, pushing work, opening or updating a pull request, assigning change-type labels, or preparing a TypeRB change for review and merge.
---

# Publish TypeRB

Never push changes directly to `main`. Publish every change through a pull request.

Use local `git` for branches, commits, and pushes. Prefer the authenticated GitHub Connector for pull requests, metadata, and labels. Use `gh` only when the Connector lacks the required capability, such as GitHub Actions log inspection; do not block on `gh` authentication when the Connector can complete the task.

## Classify the change

Choose one primary type. Use it as both the branch prefix and PR label:

- `feat`: user-facing capability
- `fix`: defect correction
- `refactor`: behavior-preserving restructuring
- `perf`: performance improvement
- `test`: test-only change
- `doc`: documentation-only change
- `build`: build or packaging change
- `ci`: continuous-integration change
- `chore`: maintenance not covered above

Use `<type>/<short-kebab-description>`, for example `feat/homebrew-install` or `fix/repl-history`. Do not use `codex/`, `agent/`, or a username prefix.

## Commit the work

- Commit coherent, independently understandable work units.
- Use a concise imperative subject.
- Add a body only when rationale, constraints, rejected alternatives, or intentional limitations are not evident from the diff.
- Explain why; do not mechanically list changed files.
- Keep unrelated user changes out of the commit.

## Open the pull request

1. Verify the relevant checks before pushing.
2. Push the working branch to `origin`; never push the change to `main`.
3. Open a pull request targeting `main`. Keep it draft while required work remains.
4. Summarize what changed, why, user or developer impact, checks, and intentional follow-ups.
5. Apply the label that exactly matches the branch prefix. Apply exactly one change-type label; unrelated status or area labels may coexist.
6. Mark the pull request ready only when it is reviewable and required checks pass.

If a needed change-type label does not exist, create it before assigning it. Keep label names identical to the prefixes above.
