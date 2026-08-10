# Database schema workflow

`trb db` provides a stable TypeRB workflow around an external schema engine.
The first engine is [sqldef](https://github.com/sqldef/sqldef), while
`trb/orm` remains usable with any migration system.

## Configuration

Add a mode-independent `db` section to `trbconfig.jsonc`:

```jsonc
{
  "db": {
    "adapter": "postgresql",
    "database": {
      "environment": "DATABASE_URL"
    },
    "schema": "db/schema.sql",
    "lock": "db/schema.lock.json",
    "sqldef": {
      "command": "psqldef",
      "version": "3.11.19"
    }
  }
}
```

Adapters are `sqlite`, `postgresql`, and `mysql`. Their default commands are
`sqlite3def`, `psqldef`, and `mysqldef`. Schema and lock paths default to the
values above.

Install the configured sqldef executable for the current operating system and
architecture. TypeRB checks its exact version before invoking it and does not
download or update external binaries. A project can test and pin a newer
version explicitly through `db.sqldef.version`.

## Workflow

```sh
# Preview the SQL needed to reach db/schema.sql.
trb db plan

# Apply non-destructive changes, then refresh the lock from the database.
trb db apply

# Explicitly permit DROP operations.
trb db apply --allow-destructive

# Build or verify the deterministic portable type contract without a database.
trb db lock
trb db check

# Build or verify the same contract from the live database.
trb db lock --from-db
trb db check --from-db

# Print the live database schema, or write it only when requested.
trb db export
trb db export --output db/exported.sql
```

`db/schema.lock.json` uses name-keyed objects, sorted JSON keys, and no
timestamps, connection strings, tool versions, or generated constraint names.
It records the portable column and relationship contract used by TypeRB. Full
database-specific drift, such as a varchar length change, remains the concern
of `trb db plan`.

`trb db lock` and `trb db check` do not invoke sqldef. The `--from-db` variants
use TypeRB's database introspection. `plan`, `apply`, and `export` require the
configured sqldef executable. Passwords parsed from PostgreSQL URLs and MySQL
DSNs are passed through process environment variables rather than command-line
arguments.

## Using another migration system

The `db` section and every `trb db` command are optional. A project can manage
its schema with another tool and use `trb/orm` through live introspection as
before. To make compiler checks independent of a live database, generate the
default lock after the other tool applies its schema:

```sh
trb db lock --from-db
```

The migration engine never becomes a runtime dependency of `trb/orm`.
