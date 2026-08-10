# `trb/orm`

`trb/orm` is an experimental official package for typed database access. Its
generated runtime currently supports Go with SQLite, PostgreSQL, and MySQL.
Ruby and TypeScript runtime adapters are not implemented yet.

The compiler reads a live database schema. Migrations remain outside TypeRB;
use the schema tool that fits the project. The database must be reachable when
TypeRB checks or builds code, and again when the generated application runs.

## Configuration

Configure the adapter and database source in `trbconfig.jsonc`. An environment
variable avoids embedding a database location in source or generated code:

```jsonc
{
	"mode": "go",
	"sourceDir": "src",
	"packageOptions": {
		"trb/orm": {
			"adapter": "sqlite",
			"database": {
				"environment": "DATABASE_URL"
			}
		}
	}
}
```

SQLite environment values must be absolute paths. Adapter names are `sqlite`,
`postgresql`, and `mysql`.

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
first access and cache their values. Preload fills the same cache in batches.
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

`delete()` and `delete_all()` issue direct deletes. `destroy()` and
`destroy_all()` run the configured `dependent: destroy`, `delete`, `nullify`,
or `restrict` lifecycle.

## REPL

Start `trb` or `trb repl` in the project. The REPL loads the same schema and can
execute reads, associations, transactions, batching, writes, conflicts, and
destroy lifecycles. A fallible top-level expression displays either its success
value or its structured `DbError` without ending the session.
