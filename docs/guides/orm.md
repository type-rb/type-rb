# `trb/orm`

`trb/orm` is an experimental official package for typed database access. Its
generated runtime supports Go, Ruby, and TypeScript with SQLite, PostgreSQL,
and MySQL. TypeScript server applications currently use the Bun runtime.

When `db/schema.lock.json` exists, the compiler reads its deterministic type
contract and does not require a database connection during checks or builds.
Without a lock, it falls back to live schema introspection. The generated
application still connects to the database at runtime.

Migrations are not a runtime dependency of `trb/orm`. Use any schema tool, or
use the optional [`trb db` workflow](database.md) backed by sqldef.

## Configuration

Configure the adapter and database source in `trbconfig.jsonc`. An environment
variable avoids embedding a database location in source or generated code:

```jsonc
{
	"name": "my-app",
	"mode": "go",
	"sourceDir": "src",
	"packageOptions": {
		"trb/orm": {
			"adapter": "sqlite",
			"database": {
				"environment": "DATABASE_URL"
			}
		}
	},
	"go": {
		"module": "example.com/my-app"
	}
}
```

SQLite environment values must be absolute paths. Adapter names are `sqlite`,
`postgresql`, and `mysql`. Set `mode` to `ruby` to generate a Ruby application;
the ORM source and package options stay the same. TypeScript projects must set
`typescript.runtime` to `"bun"`.

The compiler looks for `db/schema.lock.json` by default. Set the optional
`schemaLock` package option to another project-relative path. If an explicitly
configured lock is missing or invalid, compilation fails instead of silently
using the live database.

## Date and time columns

Schema introspection and schema locks expose portable time types without model
annotations:

| TypeRB | PostgreSQL | MySQL | SQLite declared type |
| --- | --- | --- | --- |
| `Date` | `date` | `date` | `DATE` |
| `TimeOfDay` | `time without time zone` | `time` | `TIME` |
| `DateTime` | `timestamp without time zone` | `datetime` | `DATETIME` or `TIMESTAMP` |
| `Instant` | `timestamp with time zone` | `timestamp` | `TIMESTAMPTZ` or `INSTANT` |

These values work in model fields, typed predicates, writes, projections, and
`minimum()` / `maximum()`. `Instant` database traffic is normalized through
UTC; `DateTime` never gains an implicit timezone. MySQL `TIME` is accepted only
within the portable `TimeOfDay` range from `00:00:00` through `23:59:59.999999`.
Database precision remains controlled by the column declaration, such as
`timestamp(6)`.

## Models and associations

Model and table names follow conventions. Fields, nullability, primary keys,
unique constraints, and foreign keys come from the live schema.

```trb
import { Database, DbError, Model, belongs_to, has_many } from trb/orm

class User < Model
	has_many(Post, dependent: :destroy)
end

class Post < Model
	belongs_to(User, name: :author)
end
```

Use `name`, `foreign_key`, `references`, `inverse`, `through`, and `source`
only when conventions are insufficient. Association scopes are typed blocks:

```trb
class User < Model
	has_many(Post, name: :published_posts) do |posts|
		posts.where(published: true).order(created_at: :desc)
	end
end
```

## Enum columns

Use `enum_column` when a schema column stores a domain enum. Model fields,
query values, writes, and REPL results then use the nominal enum type rather
than its database scalar type:

```trb
import { Model, enum_column } from trb/orm

enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"
end

enum FulfillmentPhase
	PendingReview
	ReadyToShip
end

class Order < Model
	enum_column(:status, OrderStatus)
	enum_column(:phase, FulfillmentPhase)
end
```

Raw-value enums store their exact String or Integer raw values. Ordinary enums
use lower snake case, so `PendingReview` maps to `pending_review`. Every mapping
is checked against the schema or schema lock during compilation. Nullability
still comes from the database column, and an unknown stored value is reported
as `DbErrorKind::InvalidData` instead of being accepted as an enum member.

## Queries and effects

Model classes are query roots; an empty `where()` is unnecessary. Query values
are immutable, and database terminals declare `fails DbError`.

```trb
def recent_posts(): Array<Post> fails DbError
	query := Post.where(published: true).order(created_at: :desc).limit(20)
	return query.all()
end
```

Inside a compatible `fails DbError` function, errors propagate automatically.
Use `attempt` only when the error must become a `Result` value:

```trb
def main()
	result := attempt recent_posts()
	puts(result)
end
```

Associations use the same effect rules. `user.posts` and `post.author` load on
first access and cache their values. Preload fills the same cache.
`load()`, `reload()`, and `loaded?()` provide explicit cache control.

```trb
def print_posts(): Integer fails DbError
	users := User.preload(:posts).all()
	users.each do |user|
		user.posts.each do |post|
			puts(post.title)
		end
	end
	return users.size()
end
```

## Writes and transactions

Drafts and pending changes keep inserts and updates distinct:

```trb
def create_post(user: User): Post fails DbError
	draft := Post.build(user_id: user.id, title: "First post")
	post := draft.save()
	return post.with(title: "Updated post").save()
end
```

Bulk insert, conflict-aware insert, upsert, relation update/delete, batching,
aggregates, joins, subqueries, row locks, and destroy lifecycles are available.
Transactions use an explicit scope:

```trb
def publish_all(): Integer fails DbError
	return Database.transaction() do |transaction|
		posts := Post.using(transaction)
		posts.where(published: false).update_all(published: true)
	end
end
```

Model classes and filtered relations both support `update_all()`, `delete_all()`,
and `destroy_all()`. Direct deletes skip lifecycle behavior; destroy operations
run the configured `dependent: destroy`, `delete`, `nullify`, or `restrict` rule.

## REPL

Start `trb` or `trb repl` in the project. The REPL loads the same schema and can
execute reads, associations, transactions, batching, writes, conflicts, and
destroy lifecycles. A fallible top-level expression displays either its success
value or its structured `DbError` without ending the session.
