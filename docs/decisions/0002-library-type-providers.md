# 0002: Library-owned automatic type providers

## Context

TypeRB must type-check calls into target-language libraries without requiring
application authors to maintain a second declaration of code they do not own.
Ruby and Rails are especially dynamic: ActiveRecord methods depend on the
receiver model and database schema, while controller methods arrive through
inheritance and included modules.

## Decision

Platform packages may name a compiler type provider. Importing such a package
loads its declarations automatically into a target-independent Declaration IR.
Application source does not contain or reference shadow signature files.

An independent Ruby TypeRB package may instead publish a fixed, data-only
Declaration Protocol catalog selected by `declarationProviders.ruby` in its
manifest. Importing the package root activates the catalog, while ordinary
TypeRB source in that root owns `require` and any runtime wrappers. The host
strictly decodes a non-executable subset and rejects project-aware rules,
compiler execution hooks, source-module claims, controlled block behavior, and
nominal representation-boundary privileges. This gives static gem APIs a
package-owned path without opening the compiler-integrated provider API.

The built-in Rails provider attached to `trb/platform/ruby/rails` supplies
controller and ActiveRecord contracts and parses `db/schema.rb` into a Schema
AST. It derives model column properties, finder keyword types, generic
relations, and other schema-owned declarations from that AST. It does not
synthesize application controller classes, application helpers, or third-party
gem APIs. Those declarations belong to their own fixed package catalog or
project provider. Future providers may
compile library-owned RBS, RBI, `.d.ts`, or Go export data into the same IR.

Unknown members on a provider-owned type are errors instead of silently
becoming `Any`. Existing Ruby types not yet covered by a provider remain a
gradual boundary until their library provider is expanded or the source is
migrated to TypeRB.

## Consequences

- Users install or import runtime libraries; they do not write parallel type
  declarations for them.
- Providers are versionable compiler packages analogous to `@types`, and may
  contain both static declarations and framework-specific inference.
- Independent Ruby packages may ship fixed static declarations, but cannot run
  provider logic or inspect the consuming project merely by being imported.
- Rails schema parsing and library declaration parsing remain separate from the
  TypeRB source parser but converge on Declaration IR.
- Project-specific controller concerns and value objects are not part of the
  Rails provider merely because a Rails application uses them.
- Provider coverage is incremental. Known APIs are strict; uncovered Ruby APIs
  retain the explicit Ruby interoperability escape hatch.

## Alternatives considered

- Application-maintained `.d.trb` files were rejected as the normal workflow
  because they duplicate library code and drift from runtime versions.
- Treating every Ruby call as `Any` preserves execution but provides too little
  evidence that TypeRB improves real application development.
