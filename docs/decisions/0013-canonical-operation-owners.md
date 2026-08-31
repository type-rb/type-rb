# 0013: Canonical operation owners

Status: accepted

## Context

Declaration-root imports give an imported API one visible owner. For example,
`import trb/std/math` binds `Math`; it does not leave the caller choosing
between importing the package and importing `sqrt` directly.

That rule does not by itself choose the right kind of owner. Static utility
roots previously allowed both of these spellings for the same conversion:

```trb
Numbers.to_string(123)
123.to_s()
```

The duplication makes documentation, completion, and application style less
predictable. A static-only root can also consume a name that should denote an
actual value. `File`, `Dir`, and a future `Path` must be usable as type names;
they must not be modules that contain or stand in for second types with those
names.

Relevant language precedents separate these concerns in different ways:

- Ruby normally puts an operation on its value receiver, uses classes such as
  `File` and `Dir` for associated operations, and uses modules such as `Math`
  for domain algorithms.
- Go qualifies free functions with a lowercase package, while values use
  methods. Its package and type names occupy different syntactic roles, so a
  package such as `os` does not consume the name of its `File` type.
- JavaScript and TypeScript put existing-value operations on instances, put
  construction on classes such as `URL`, and retain namespace objects such as
  `Math` where there is no useful instance receiver.

TypeRB should retain Ruby-shaped reading and Go-like explicitness without
copying every compatibility alias from Ruby, making every operation a Go-style
package function, or introducing TypeScript-style declaration merging.

## Options considered

### Keep static utility roots

Every standard package could expose an uppercase module or static-only class:

```trb
Numbers.to_string(123)
Strings.uppercase(text)
FileSystem.open(path)
```

This gives every call an explicit owner, but it leaves duplicate receiver
spellings and reserves value-type names for utility holders. A static-only
class also suggests construction and identity semantics that it does not have.

### Make every operation a receiver method

Receiver calls are concise for existing values, but they are not a coherent
home for factories or algorithms with no natural receiver. Forcing a path
argument to receive file construction, or a number to receive every
multi-value mathematical algorithm, would hide rather than clarify ownership.

### Choose the owner from the operation's semantics

Existing-value operations can use receivers, factories can use the type they
produce, and domain algorithms can use modules. This keeps one normal spelling
without forcing unlike operations into one syntactic category.

### Merge a module and type with the same name

TypeScript can merge some declarations, and generated backends could attempt
to emulate a `File` namespace beside a `File` type. This would add
source-language identity, lookup, import, completion, and lowering rules solely
to preserve a utility namespace. It would also make it unclear whether a
member belongs to the type or to the merged namespace.

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
   constructors and factories such as `StringBuilder.new()` and
   `File.open(path)`, as well as `Dir.children(path)`, which enumerates the
   contents identified by a directory path.
3. A domain algorithm with no natural value receiver is a module member.
   Examples include `Math.sqrt(9.0)` and `JSON.decode<Value>(text)`.
4. An intentionally direct language or DSL operation may remain a top-level
   function imported by exact name, as with `test`.

Two public spellings are allowed only when they communicate a documented
semantic, lifecycle, or performance distinction. A one-shot operation and an
operation on an already-open resource may coexist when their resource lifetime
is genuinely different; a static wrapper that merely forwards its first
argument to an equivalent receiver method may not.

The small portable prelude is an explicit language-level convenience boundary,
not a family of static value wrappers. An operation deliberately present there,
such as `puts`, may also remain available through its declaration root as
`IO.puts` for code that prefers an explicit owner. Such exceptions must be
listed as prelude bindings; packages do not gain duplicate spellings merely for
convenience.

A type-associated operation requires the actual nominal type as its public
concept. It does not justify creating a utility class solely to hold otherwise
free functions. `Dir` is the nonconstructible directory type root even though
this initial slice does not expose an open `Dir` instance.

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

### File, directory, and path names

`trb/std/file` exposes the actual opaque `File` class as its declaration root.
`File.open` is the structured factory, and the yielded value has type `File`.
There is no separate `File` module and no aggregate `FileSystem` owner.

`trb/std/dir` is a separate package with the `Dir` root. Directory enumeration
is not coupled to opening every file, and the separate root matches the user's
file-versus-directory mental model.

The static-only `Path` holder is removed. The name `Path` is reserved for a
future nominal host-path value after its cross-platform representation,
construction, equality, and methods are defined. The initial file and
directory APIs accept host-native path strings rather than presenting a
static-only class as that future value.

An existing domain root may remain a module while it owns algorithms and no
corresponding value type exists. `URL`, for example, currently owns component
and query encoding algorithms. If TypeRB later adds a nominal URL value, that
same root must become the actual type or the algorithms must move to another
canonical owner; a module and type will not silently share the name.

`File` is source-nonconstructible and may be obtained only through its declared
scoped operation. This is a resource-safety contract independent of the import
model: choosing a type root must not expose `File.new()` or allow a handle to
escape its cleanup block. The compiler-owned standard contract supplies this
initial capability; general user-defined opaque or resource declarations are
not added by this decision.

### Supporting declarations

Supporting declarations are peer top-level exports in the initial APIs.
Examples include `FileMode`, `DirEntry`, and `DirEntryKind`; filesystem error
types are peers in the standard errors package. A caller imports each required
peer by exact name.

This decision does not merge modules and classes or add class-owned nested
declarations such as `File::Mode`. Portable class-owned nesting may be designed
later if it provides enough value beyond peer imports and has one identity and
lowering model across Go, Ruby, and TypeScript.

## Consequences

- Declaration-root imports keep their unambiguous binding and completion
  behavior.
- Existing-value operations have one receiver spelling; users do not choose
  between a utility package and a method for the same operation.
- Type names remain available for real values and resources rather than
  static-only namespaces.
- Factories such as `File.open` remain concise and Ruby-shaped without making
  the resource constructible or weakening deterministic cleanup.
- Public source that uses a removed static utility root must use the receiver
  spelling. The standard library does not publish duplicate transition aliases.
- Supporting types require named peer imports until a separate nested
  declaration design is accepted.

## Deferred work

- A general source declaration for opaque or scoped resource types.
- Methods and factories on nominal non-class value declarations, needed before
  committing to a full `Path` or similar value API.
- Portable class-owned nested declarations and any declaration-merging model.
- The concrete future `Path` representation and path normalization policy.
- Re-evaluating domain module roots such as `URL` when a same-named nominal
  value is proposed.
- Additional convenience spellings that do not express a semantic,
  lifecycle, or performance distinction.
