# Pull request release metadata

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
- During alpha, `Breaking` is descriptive release metadata only. Never alter
  the chosen design, add compatibility behavior, or require migration guidance
  because a change is breaking. Describe the resulting observable behavior;
  provide transition instructions only when the maintainer explicitly asks for
  them.
