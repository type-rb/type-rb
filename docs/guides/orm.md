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

## Integer columns

PostgreSQL `bigint`, MySQL signed `BIGINT`, and SQLite `INTEGER` columns can map
to TypeRB `Integer`, but TypeRB accepts only the portable exact subset
`-9007199254740991..9007199254740991`. In particular, a MySQL `BIGINT` column
has a wider database range; generated Go, Ruby, and TypeScript adapters reject
an out-of-range row, projection, aggregate, count, or generated identifier as
invalid database data instead of exposing a backend-dependent Integer.

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
UTC, and generated runtimes use UTC database sessions so server-evaluated
defaults behave consistently across targets. `DateTime` never gains an implicit
timezone. MySQL `TIME` is accepted only within the portable `TimeOfDay` range
from `00:00:00` through `23:59:59.999999`. Database precision remains controlled
by the column declaration, such as `timestamp(6)`.

## Models and associations

Model and table names follow conventions. Fields, nullability, primary keys,
unique constraints, and foreign keys come from the live schema.

Each source directory is one ORM model group. Models in separate files within
that directory reference one another in association declarations without
source imports:

```trb
# src/models/user.trb
import { Model, has_many } from trb/orm

class User < Model
	has_many(Post, dependent: :destroy)
end
```

```trb
# src/models/post.trb
import { Model, belongs_to } from trb/orm

class Post < Model
	belongs_to(User, name: :author)
end
```

The first argument of `belongs_to`, `has_many`, and `has_one` is a
compiler-resolved declaration reference. This exception does not make the
target model available to ordinary expressions or type annotations. Import it
normally when using it as a query root, constructor, parameter, or return type:

```trb
import { Post } from models/post
import { DbResult } from trb/orm

def recent_posts(): DbResult<Array<Post>>
	return Post.order(created_at: :desc).limit(20).all()
end
```

A subdirectory starts another model group. Every model traversed by a direct or
through association must remain in one group; direct association inverses and
dependent lifecycle targets follow the same boundary. The compiler reports
both declarations when that boundary is crossed. Database foreign-key columns
may still cross groups. Keep the identifier and load the other record through
an application query or repository instead of ORM object navigation. Model
class names are currently unique across the project.

Completion, hover, definition, references, and rename understand declaration
references in association arguments and do not insert target-model imports.
Generated runnable entrypoints bootstrap Ruby and TypeScript model registration
without introducing model-to-model initialization imports; Go emits the group
as one generated package. A model module must not import the runnable
entrypoint directly or transitively, because the registration bootstrap would
close an initialization cycle. Move declarations shared with the entrypoint
into a separate module; the compiler reports the complete cycle.

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
The enum may be declared in the application or imported from a TypeRB package;
package aliases use the same resolution rules as ordinary imports.

## Queries and Results

Model classes are query roots; an empty `where()` is unnecessary. Query values
are immutable, and database terminals return `DbResult<T>`, the package alias
for `Result<T, DbError>`.

```trb
import { DbResult } from trb/orm

def recent_posts(): DbResult<Array<Post>>
	query := Post.where(published: true).order(created_at: :desc).limit(20)
	return query.all()
end
```

Use prefix `try` when a function performs more work after a successful
database operation. Use `catch` when the caller resolves the database error
locally:

```trb
def main()
	posts := recent_posts() catch |error|
		puts(error.message)
		return
	end
	puts(posts.size())
end
```

Associations use the same Result rules. `user.posts` and `post.author` return a
`DbResult` that loads on first access and caches its success value. Preload
fills the same cache. `load()` and `reload()` also return `DbResult`; `loaded?()`
remains an ordinary Boolean cache query.

```trb
import { DbResult } from trb/orm

def print_posts(): DbResult<Integer>
	users := try User.preload(:posts).all()
	users.each do |user|
		posts := try user.posts
		posts.each do |post|
			puts(post.title)
		end
	end
	return DbResult<Integer>::Ok(users.size())
end
```

### Streaming in batches

`find_each()` and `find_in_batches()` are structured Result boundaries. They
return `DbResult<Integer>`, where the success value is the number of records
visited. Assign the raw Result when both outcomes matter, use prefix `try`
inside a function returning a compatible Result, or use `catch` to recover at
the call site:

```trb
import { DbResult } from trb/orm

def print_recent_posts(): DbResult<Integer>
	count := try Post.where(published: true).find_each(batch_size: 100) do |post|
		puts(post.title)
	end
	return DbResult<Integer>::Ok(count)
end

def main()
	count := print_recent_posts() catch |error|
		puts(error.message)
		0
	end
	puts(count)
end
```

Prefix `try` inside the streaming block stops iteration and returns `Err` from
the streaming operation. `break` and `next` keep their ordinary local
iteration behavior. An authored `return` cannot cross this Result boundary;
return after the operation instead. When streaming runs inside
`Database.transaction()`, an Err propagated with `try` rolls the transaction
back before an outer `catch` handler runs.

## Writes and transactions

Drafts and pending changes keep inserts and updates distinct:

```trb
import { DbResult } from trb/orm

def create_post(user: User): DbResult<Post>
	draft := Post.build(user_id: user.id, title: "First post")
	post := try draft.save()
	return post.with(title: "Updated post").save()
end
```

Bulk insert, conflict-aware insert, upsert, relation update/delete, batching,
aggregates, joins, subqueries, row locks, and destroy lifecycles are available.
Transactions use an explicit scope:

```trb
import { DbResult } from trb/orm

def publish_all(): DbResult<Integer>
	return Database.transaction() do |transaction|
		posts := Post.using(transaction)
		try posts.where(published: false).update_all(published: true)
	end
end
```

`Database.transaction()` is a structured Result boundary. A Result propagated
with `try` from its body rolls the transaction back; an outer `catch` runs only
after rollback completes. The block's final successful value becomes the
transaction's `Ok` payload. An authored `return` cannot cross the transaction
boundary.

Model classes and filtered relations both support `update_all()`, `delete_all()`,
and `destroy_all()`. Direct deletes skip lifecycle behavior; destroy operations
run the configured `dependent: destroy`, `delete`, `nullify`, or `restrict` rule.

## REPL

Start `trb` or `trb repl` in the project. The REPL loads the same schema and can
execute reads, associations, transactions, batching, writes, conflicts, and
destroy lifecycles. A top-level database Result displays `Ok` or its structured
`DbError` without ending the session. Top-level `try` is rejected; inspect the
Result directly or recover with `catch` so an error cannot terminate the
interactive session implicitly.
