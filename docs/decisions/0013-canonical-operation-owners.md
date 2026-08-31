# 0013: Canonical operation owners

Status: accepted; implemented for built-in value operations and
`StringBuilder`

## Context

[Declaration-root imports](0011-declaration-root-imports.md) give an imported
API one visible owner. For example, `import trb/std/math` binds `Math`; it does
not leave the caller choosing between importing the package and importing
`sqrt` directly.

That rule does not by itself choose the right kind of owner. Static utility
roots previously allowed both of these spellings for the same conversion:

```trb
Numbers.to_string(123)
123.to_s()
```

The duplication makes documentation, completion, and application style less
predictable. A static-only root can also consume a name that should denote the
actual value type.

Relevant language precedents separate these concerns in different ways:

- Ruby normally puts an operation on its value receiver, uses classes for
  construction and associated operations, and uses modules such as `Math` for
  domain algorithms.
- Go qualifies free functions with a lowercase package, while values use
  methods. Package and type names occupy different syntactic roles.
- JavaScript and TypeScript put existing-value operations on instances, put
  construction on classes, and retain namespace objects such as `Math` where
  there is no useful instance receiver.

TypeRB should retain Ruby-shaped reading and Go-like explicitness without
copying compatibility aliases from Ruby, making every operation a Go-style
package function, or introducing TypeScript-style declaration merging.

## Options considered

### Keep static utility roots

Every standard operation could remain on an uppercase module or static-only
class:

```trb
Numbers.to_string(123)
Strings.uppercase(text)
```

This gives every call an explicit owner, but it leaves duplicate receiver
spellings. A static-only class also suggests construction and identity
semantics that it does not have.

### Make every operation a receiver method

Receiver calls are concise for existing values, but they are not a coherent
home for factories or algorithms with no natural receiver. Forcing an
arbitrary argument to receive construction or a domain algorithm would hide
rather than clarify ownership.

### Choose the owner from the operation's semantics

Existing-value operations can use receivers, factories can use the type they
produce, and domain algorithms can use modules. This keeps one normal spelling
without forcing unlike operations into one syntactic category.

### Merge a module and type with the same name

Some target languages can merge declarations, and generated backends could
attempt to emulate a namespace beside a same-named type. This would add
source-language identity, lookup, import, completion, and lowering rules solely
to preserve a utility namespace. It would also make member ownership unclear.

## Decision

Declaration-root imports remain the canonical import model. The root's kind is
chosen from the public concept rather than standardized on modules.

### Owner rules

Public operations use these owners:

1. An operation on an existing value is an instance method on that value.
   Examples include `123.to_s()`, `2.5.to_i()`, `text.upcase()`, and
   `bytes.valid_utf8?()`.
2. An operation naturally associated with an actual nominal type, but not with
   an existing instance, is a class member on that type. This includes genuine
   constructors and factories such as `StringBuilder.new()`.
3. A domain algorithm with no natural value receiver is a module member.
   Examples include `Math.sqrt(9.0)` and `JSON.decode<Value>(text)`.
4. An intentionally direct language or DSL operation may remain a top-level
   function imported by exact name, as with `test`.

Two public spellings are allowed only when they communicate a documented
semantic, lifecycle, or performance distinction. A static wrapper that merely
forwards its first argument to an equivalent receiver method is not a distinct
operation.

The small portable prelude is an explicit language-level convenience boundary,
not a family of static value wrappers. An operation deliberately present there,
such as `puts`, may also remain available through its declaration root as
`IO.puts`. Such exceptions must be listed as prelude bindings; packages do not
gain duplicate spellings merely for convenience.

A type-associated operation requires the actual nominal type as its public
concept. It does not justify creating a utility class solely to hold otherwise
free functions.

Compiler-owned contracts and intrinsics may retain internal operation names.
Those names are implementation details and do not create another importable
source API.

### Standard value operations

The utility packages `trb/std/numbers`, `trb/std/strings`,
`trb/std/booleans`, `trb/std/bytes`, and `trb/std/ranges` are removed from the
public import surface because they only mirror or import-gate built-in receiver
operations. The receiver methods remain available without an import. No
compatibility alias preserves forms such as `Numbers.to_string(value)`.

The same canonical-spelling rule applies within receiver APIs. Float-to-Integer
conversion uses `value.to_i()` without a synonymous `truncate()` method, and
the Bytes UTF-8 predicate is `bytes.valid_utf8?()`.

`StringBuilder` remains a type root because it is the value being constructed.
Its `new` and `from_string` factories are class members, while operations on an
existing builder are instance methods.

This is the first implementation boundary for the owner rule. Other public API
families are evaluated and migrated in their own coherent changes; this record
does not silently rename an existing package or introduce a compatibility
alias.

## Consequences

- Declaration-root imports keep their unambiguous binding and completion
  behavior.
- Existing-value operations have one receiver spelling; users do not choose
  between a utility package and a method for the same operation.
- Type names remain available for actual values rather than static-only
  namespaces.
- Public source that uses a removed static utility root must use the receiver
  spelling. The standard library does not publish duplicate transition aliases.
- Backend intrinsics can retain stable internal identities while the public
  source surface has one owner.

## Deferred work

- Methods and factories on nominal non-class value declarations.
- Portable class-owned nested declarations and any declaration-merging model.
- Re-evaluating domain module roots when a same-named nominal value is
  proposed.
- Additional convenience spellings that do not express a semantic,
  lifecycle, or performance distinction.
