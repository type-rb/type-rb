# TypeRB

Keep committed documentation, code comments, commit messages, and pull request
text in English.

## Public repository boundary

- Keep every repository artifact, issue, pull request, and release note
  explainable solely from public information in this repository or other public
  sources.
- Never include names, URLs, local paths, quotations, descriptions, or
  provenance from private repositories, applications, data, or discussions.
- Reproduce externally discovered problems with generic synthetic examples and
  a self-contained public rationale that does not reveal private provenance.

## Task scope and guidance

Complete the requested change and relevant verification within the user's scope.
A review-only request stays read-only; preparing a PR includes the requested
edits and checks, then stops before merge when review is requested. Use existing
authorization for routine fixes instead of adding approval steps, and honor any
later budget or scope limit.

Use `.agents/skills/develop-typerb/SKILL.md` for compiler or language changes,
`use-typerb` for application work, and `publish-typerb` when preparing GitHub
changes. Read the relevant specification or guide for the affected behavior;
a documentation correction does not require a full language review.

Choose local checks by the changed surface and retain required repository CI.
After relevant checks pass, repeat or broaden them for new changes, failures,
or unresolved concerns rather than by default.
