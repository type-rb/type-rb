# 0005: Record field defaults and declaration metadata

## Context

Compiler-integrated schemas need declarative metadata without adding a second
data-model declaration. They may also need to omit an input field so the data
type, rather than an integration, owns its default value. The same omission
semantics are useful outside any one integration.

Encoding these concerns as CLI-only field wrappers would change a record's
ordinary field type. Adding `option`, `argument`, or `subcommand` keywords to
the core grammar would couple the language to one package. Annotations alone
cannot represent a fresh or computed constructor default.

## Decision

Record fields may declare a general constructor default:

```trb
record ServerConfig
	host: String
	port: Integer = 8080
	labels: Array<String> = []
end
```

Required fields precede fields with defaults. A default is checked against the
declared field type, is evaluated once for each construction, and may reference
only fields declared before it. `[]` and `{}` therefore produce fresh
collections for each record value.

`Record.new` remains keyword-only. Explicit argument expressions are evaluated
left to right in authored order. The values are then bound by field name, and
omitted defaults are evaluated in declaration order in the record's defining
module. Explicit `nil` remains different from omission.

Defaults belong only to ordinary record construction. JSON decoding, ORM
loading, web request decoding, and other wire boundaries continue to require
their own missing-field policy. They do not silently apply constructor
defaults.

The canonical field order is:

```trb
name: Type = default @attribute(...)
```

Record fields retain postfix attributes. Enum members also accept the same
postfix attribute form after a payload or raw value:

```trb
enum Command
	Serve(args: ServeArgs) @schema(label: "Start the server")
end
```

An attribute has no core-language behavior merely because it is present. A
compiler-integrated package owns the meaning and validation of its attribute
name. The Project Declaration Input protocol is version 6: record fields
expose `hasDefault`, and enum members expose their data-only attributes. It
does not serialize executable default expressions.

## Consequences

- Records remain ordinary portable product types; declaration metadata does
  not alter field types or construction.
- Default expressions have one definition-side owner across Go, Ruby,
  TypeScript, imported modules, and the REPL.
- Package integrations can inspect enum-member metadata without receiving AST,
  typed IR, executable expressions, or filesystem access.
- Adding another metadata consumer does not require another grammar keyword.

## Alternatives considered

- CLI-specific `option<T>` and `argument<T>` wrappers were rejected because
  they leak parsing concerns into application data and pattern matching.
- CLI-specific record keywords were rejected because they reserve core syntax
  for one package and do not generalize to other declarative integrations.
- Treating nullable fields as defaults was rejected because `nil` is a value,
  not omission.
- Applying defaults during every decoder operation was rejected because it
  would make wire compatibility depend on constructor behavior.
