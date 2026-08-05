# Rails guide

TypeRB's Ruby target can be introduced one file at a time in an existing Rails
application. Keep files that do not need TypeRB as `.rb`, rename files being
typed to `.trb`, and set `"mode": "ruby"` in `trbconfig.jsonc`.

## Rails source

Import the Rails platform package before using Rails APIs or DSL constructs:

```trb
import trb/platform/ruby/rails

class Post < ApplicationRecord
	belongs_to :author
	validates :title, presence: true
	scope :published, -> { where.not(published_at: nil) }

	def summary(limit: Integer = 80): String
		return body.to_s().truncate(limit)
	end
end
```

Rails DSL constructs pass through dedicated syntax AST and typed IR nodes. They
are accepted only through the explicit Ruby platform surface and are rejected
in Go and TypeScript projects.

## Build and test

Build a generated Rails tree and run its ordinary toolchain:

```sh
trb fmt --check
trb build --out-dir build .
cd build
bundle exec rails test
```

Set `"packageManagement": "external"` when the existing application should
continue to own its Gemfile. Set `ruby.loader` to `zeitwerk` so project imports
remain compile-time dependencies without generated `require` calls.

## Type providers

Importing `trb/platform/ruby/rails` activates the compiler-owned Rails type
provider. It reads `db/schema.rb` and derives ActiveRecord model columns,
finders, relations, controller APIs, and supported helpers. Application authors
do not maintain a parallel signature file.

Provider coverage is incremental. Known provider types reject unknown members;
uncovered Ruby APIs remain behind the explicit Ruby interoperability boundary.

See [`examples/rails`](../../examples/rails) for models, controllers, concerns,
jobs, mailers, routes, and migrations.
