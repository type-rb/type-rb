---
name: publish-typerb
description: Publish TypeRB changes through GitHub. Use when creating a branch, choosing commit messages, pushing work, opening or updating a pull request, assigning its author and change-type label, or preparing a TypeRB change for review and merge.
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
5. Add the release-note block described below.
6. Apply the label that exactly matches the branch prefix. Apply exactly one change-type label; unrelated status or area labels may coexist.
7. Assign the pull request author to the pull request. Read the author login from the created pull request response or current pull request metadata; do not guess it.
8. Mark the pull request ready only when it is reviewable and required checks pass.

### Record release impact

Include this exact section in every pull request body:

```text
## Release note

Area: <short user-facing subsystem>
Kind: <Added|Changed|Fixed|Performance|Security|Deprecated|Removed|None>
Breaking: <Yes|No>

<one concise user-facing paragraph, or a short reason when Kind is None>
```

- Describe observable behavior, not files or implementation mechanics.
- Use `Kind: None` for internal maintenance, documentation-only changes, release
  preparation, and other changes users do not need in a changelog.
- Keep the area stable and recognizable, such as `Language`, `Compiler`,
  `CLI`, `REPL`, `Standard library`, `Web`, `ORM`, `Jobs`, `Packages`, or
  `Tooling`. Add a new area when none of these fits naturally.
- Set `Breaking: Yes` only when users must change existing code or operation.
- Keep required migration guidance in the paragraph when a change is breaking.

If a needed change-type label does not exist, create it before assigning it. Keep label names identical to the prefixes above.

## After merge

1. Confirm the pull request merged, then switch to `main`.
2. Fetch `origin` with pruning and fast-forward local `main` to `origin/main`.
3. Delete the merged local head branch with `git branch -d <branch>`.
4. Verify that local `main` is clean and synchronized.

Delete only the pull request's confirmed merged branch. Never use `-D` or bulk-delete local branches as a routine post-merge step.
