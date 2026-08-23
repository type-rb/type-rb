# TypeRB Documentation

TypeRB is in alpha. Public behavior may change as the language is exercised in
larger applications.

## Learn

- [Install and try TypeRB](../README.md#install)
- [Learning path](learning.md)
- [A Tour of TypeRB](https://type-rb.github.io/tour/)
- [Language guide](language.md)
- [AI-assisted TypeRB development](ai-assisted-development.md)

## Build applications

- [TypeRB in containers](containers.md)
- [Package system](guides/packages.md)
- [Shared HTTP values](guides/http.md)
- [Portable web applications](guides/web.md)
- [OIDC bearer authentication](guides/authentication.md)
- [Browser HTTP client](guides/browser-http.md)
- [Editor support](editor-support.md)
- [Database schema workflow](guides/database.md)
- [`trb/orm` guide](guides/orm.md)
- [Portable background Jobs](guides/jobs.md)
- [Experimental React and JSX](guides/react.md)
- [Testing TypeRB applications](guides/testing.md)

## Reference

- [Command-line reference](cli.md)
- [Compiler tooling protocol](guides/compiler-tooling.md)
- [Project configuration](configuration.md)
- [Language specification](specification.md)
- [Standard library](standard-library.md)
- [Migrate to Result control flow in TypeRB 0.3](migrations/0.3-result-control.md)
- [Migrate aliases and adopt newtypes in TypeRB 0.3](migrations/0.3-alias-newtype.md)

## Project

- [Current status](status.md)
- [Roadmap](roadmap.md)
- [Development and compiler architecture](development.md)
- [Release process](releasing.md)
- [Architecture decisions](decisions/)

## Document ownership

- `language.md` teaches implemented syntax; `specification.md` defines its
  normative semantics.
- `learning.md` orders existing material; it does not redefine syntax or APIs.
- `standard-library.md`, `cli.md`, and `configuration.md` are the references for
  those public surfaces.
- `migrations/` records release-specific source and package upgrade steps; it
  does not redefine the current language contract.
- `status.md` records current capability and limitations; `roadmap.md` contains
  future outcomes only.
- Decision records preserve durable rationale. Scoped implementation work
  belongs in GitHub issues rather than progress logs in this directory.
