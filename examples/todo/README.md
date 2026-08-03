# TypeRB Todo vertical slice

This is the first v0.1 product target: one TypeRB workspace compiled to a Go
JSON API and a React/TypeScript client.

```text
packages/contracts/src/index.trb       shared API records
apps/api/src/domain                    GORM persistence records
apps/api/src/repositories              1:N and N:M queries/mapping
apps/api/src/transport                 net/http handlers
apps/api/schema.sql                    sqldef source of truth
apps/web/src/api                       typed JSON client
apps/web/src/components                React component
```

The shared package is declared once in each application's `localPackages`.
Both builds parse it into the same Record/Contract IR. Go receives structs and
JSON tags; TypeScript receives interfaces.

## Run

The commands below assume the `trb` bootstrap launcher or an installed binary
is already on `PATH`:

```sh
trb version
```

Start the API:

```sh
cd examples/todo/apps/api
trb install
trb run
```

`trb run` reads the explicit `go.sqldef` config, applies `schema.sql` to
`todo.db` with `sqlite3def`, compiles all application and local-package `.trb`
files, then starts the generated Go server on port 8080.

In another terminal, start the client:

```sh
cd examples/todo/apps/web
trb install
npm run dev
```

Open `http://localhost:5173`. A Todo belongs to one list, and its tags are
stored through `todo_item_tag_entities`, exercising both 1:N and N:M
relations.

Production checks are:

```sh
cd examples/todo/apps/api
trb build
cd build && go test ./...

cd ../../web
npm run build
```
