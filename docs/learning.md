# Learning TypeRB

TypeRB is in alpha, so the fastest way to learn it is to combine executable
feedback with focused references. This page is a path through the existing
material, not a second copy of the language specification.

## 1. Try the language

Start with [A Tour of TypeRB](https://type-rb.github.io/tour/). It runs the
compiler in the browser and introduces the core syntax in short lessons.

Use the [Playground](https://type-rb.github.io/play/) when you want to change a
lesson, switch targets, format source, or inspect generated Go, Ruby, and
TypeScript.

For a terminal session:

```console
$ trb
trb:go> puts("Hello, TypeRB!")
Hello, TypeRB!
trb:go> 1 + 2
3 : Integer
```

The scratch REPL uses Go mode. Select another target with
`trb repl --mode ruby` or `trb repl --mode typescript`.

## 2. Build one local program

Follow [Create a project](getting-started.md#create-a-project), then keep this loop
short:

```sh
trb fmt
trb check
trb lint
trb run
```

`trb check` validates the complete configured project without generating
target source or starting a target toolchain. That makes it the normal command
to run while editing. `trb lint` repeats that correctness check and then adds
the project's optional maintainability rules.

Read the [language guide](language.md) alongside the first program. Focus on:

1. functions, canonical type names, and immutable or `mut` bindings;
2. records, classes, enums, Arrays, and Hashes;
3. `if`, exhaustive `case`, nullable narrowing, and iteration; and
4. `Result`, prefix `try`, and postfix `catch` at recoverable-error boundaries.

Use the [language specification](specification.md) only when you need the exact
rule or are evaluating an edge case.

## 3. Choose an application path

Continue with one small vertical slice:

- Complete backend: [Web, ORM, and Jobs tutorial](tutorials/web-orm-jobs.md)
- JSON API: [portable Web guide](guides/web.md)
- Database application: [`trb/orm` guide](guides/orm.md)
- Background worker: [Jobs guide](guides/jobs.md)
- Browser UI: [React and JSX guide](guides/react.md) and
  [browser HTTP guide](guides/browser-http.md)
- Reusable dependency: [package guide](guides/packages.md)

The complete backend tutorial connects request binding, an application
transaction, durable enqueueing, and worker execution in one runnable project.
The focused guides describe each public API and its intentional limitations;
use them when extending that slice or building one of the other paths.

## 4. Look up details

Use the reference that owns the surface you are changing:

- [Command-line reference](cli.md)
- [Project configuration](configuration.md)
- [Standard library](standard-library.md)
- [Current capability and limitations](status.md)
- [Language roadmap](roadmap.md)

The [documentation index](README.md) groups all application and contributor
material.

## 5. Learn with an AI agent

The repository includes a `use-typerb` agent skill. In an agent that discovers
repository skills, ask:

```text
Use $use-typerb to teach me TypeRB by building a small JSON API. Give me one
exercise at a time and validate my code with the compiler before continuing.
```

The skill routes the agent to the current guide and compiler diagnostics rather
than asking it to infer TypeRB from a target language. See
[AI-assisted TypeRB development](ai-assisted-development.md) for the same
workflow when building an application rather than taking a tutorial.

## 6. Understand the compiler

Contributors should first read
[Development and compiler architecture](development.md). Trace one small
feature through tokens, syntax AST, checking, typed IR, and a backend, then run
its focused tests. The language specification defines behavior; generated
target source is an implementation result rather than the source of semantics.
