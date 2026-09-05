# Standard library

Portable packages use the `trb/std/*` namespace. They resolve to
compiler-owned contracts and typed IR, so their argument types, results, and
runtime behavior remain consistent across Go, Ruby, and TypeScript.

Target-only functionality belongs to explicit `trb/platform/<mode>/*`
packages.

## Output

`puts` is available in the small portable prelude and is the simplest way to
write a line:

```trb
puts("Hello, TypeRB!")
puts(1 + 2)
```

Import `trb/std/io` when declaration-root access is useful. Both forms resolve to the
same operation:

```trb
import trb/std/io

IO.puts("Hello, TypeRB!")
```

## Scalars and strings

Portable built-in types expose import-free receiver methods. They do not also
provide public static mirrors, so each operation has one ordinary spelling:

<!-- trb-doc-test: stdlib-scalars -->
```trb
import trb/std/math

text := 123.to_s()
ratio_text := 0.25.to_s()
ratio := 25.to_f()
whole := 2.75.to_i()
distance := (-8).abs()
bounded := 12.clamp(0, 10)
rounded := (-2.5).round()
root := Math.sqrt(9)
even := distance.even?()
ordinary := ratio.finite?()
enabled := true.to_s()
number := "123".to_i()
safe_number := "123".try_to_i()
decimal := "12.5".to_f()
safe_decimal := "+.5e1".try_to_f()
length := "Hello".size()
character := "A😀"[1]
last_character := "A😀"[-1]
safe_character := "A😀".try_fetch(2)
middle := "A😀BC".slice(1...3)
safe_middle := "A😀BC".try_slice(1..2)
first_emoji := "A😀B😀".index("😀")
last_emoji := "A😀B😀".rindex("😀")
characters := "A😀".chars()
reversed := "A😀".reverse()
replaced := "a😀a".replace_all("a", "$&")
upper := "Hello".upcase()
```

Integer parsing accepts a complete ASCII decimal integer with an optional sign.
`to_i()` raises on invalid or non-portable input; `try_to_i()` returns
`Result<Integer, NumberParseError>`. The error preserves its
`NumberParseErrorKind`, input, and message. Parsed integers use the portable
exact range `-9007199254740991..9007199254740991`.

Float parsing accepts a complete ASCII decimal with an optional sign, decimal
point, and exponent. At least one digit is required. `to_f()` raises on an
invalid spelling or a value that overflows the portable binary64 range;
`try_to_f()` returns `Result<Float, NumberParseError>`. Non-finite spellings
such as `NaN` and `Infinity` are not accepted. Values smaller than the binary64
range round to signed zero consistently across backends.

`Integer#to_f()` is an exact widening conversion. `Float#to_i()` truncates
toward zero and raises for non-finite or out-of-range values. `Float#to_s()`
uses a portable fixed decimal spelling without exponent notation, including
`.0` for integral Float values.

Integers provide `abs()`, `zero?()`, `positive?()`, `negative?()`, `even?()`,
`odd?()`, `min()`, `max()`, and `clamp()`. An invalid clamp range raises.
Floats provide `abs()`, `floor()`, `ceil()`, `round()`,
`finite?()`, `infinite?()`, and `nan?()`; the integer conversions reject
non-finite and out-of-range results. `round()` resolves halfway values away
from zero. `abs()` converts negative zero to positive zero and leaves NaN as
NaN.

`trb/std/math` provides `sqrt`, `exp`, `log`, `log2`, and `log10`, returning
Float. Integer arguments use the ordinary safe widening to Float. Negative
square roots and logarithms are NaN, and logarithms of zero are negative
infinity. Exponentiation remains the `**` operator.

`Boolean#to_s()` returns lowercase `"true"` or `"false"`.

`String#size`, `[]`, `try_fetch`, `slice`, `try_slice`, `index`, and `rindex`
operate on Unicode code points rather than encoded bytes. Indexes are
zero-based from the start; a negative index counts from the end, with `-1`
naming the final code point. `value[index]` is the sole strict single-element
form; it raises when the normalized index is outside the string, while
`try_fetch()` returns `Result<String, IndexLookupError>` and preserves the
requested index in an error. `slice(range)` accepts an inclusive `..` or
exclusive `...` `Range<Integer>` and raises for negative, reversed, or
out-of-bounds limits. `try_slice()` returns
`Result<String, SliceRangeError>` instead. An exclusive `size...size` range is
a valid empty slice. `index()` and `rindex()` search for literal substrings and
return a code-point offset as `Integer?`; an empty substring is found at zero
or at the string size, respectively. Additional receiver operations include
`chars`, `codepoints`, `reverse`, `empty?`, `strip`, `lstrip`, `rstrip`,
`include?`, `start_with?`, `end_with?`, `split`, `replace_all`, `upcase`, and
`downcase`.
`chars()` returns one String per code point, and `reverse()` reverses that same
sequence. String trimming uses the pinned Unicode 17.0 `White_Space` set,
preserves internal whitespace, and does not remove U+FEFF. `replace_all()`
replaces every non-overlapping literal occurrence. The replacement is also
literal, so strings such as `$&` and `$1` have no special meaning; an empty
pattern raises.

## Bytes

`Bytes` is an immutable binary sequence distinct from `String` and
`Array<Integer>`:

```trb
payload := "A😀".to_bytes()
byte_length := payload.size()
first := payload.at(0)
text := payload.to_s()
```

String conversion uses UTF-8. Decoding invalid input emits one U+FFFD for each
maximal subpart of an ill-formed sequence: adjacent stray continuation bytes
are replaced separately, while a truncated multibyte prefix is replaced once.
Call `valid_utf8?()` first when invalid bytes must be rejected. Byte indexing
is strict and concatenation is non-mutating.

## Binary encoding

`trb/std/encoding/hex` converts between `Bytes` and hexadecimal text:

```trb
import trb/std/encoding/hex

text := Hex.encode("A😀".to_bytes())
decoded := Hex.decode("41F09F9880")
```

Encoding uses lowercase ASCII. Decoding accepts uppercase or lowercase input
and returns `Result<Bytes, Hex::DecodeError>`. Invalid characters and odd-length
input are distinguished by `Hex::DecodeErrorKind`; the error also preserves the
input, a zero-based character position, and a message. For odd-length input,
the position is the missing character at the end of the string.

`trb/std/encoding/base64` provides strict standard and URL-safe Base64:

```trb
import trb/std/encoding/base64

text := Base64.encode("A😀".to_bytes())
url_text := Base64.url_encode("???".to_bytes())
decoded := Base64.decode("QfCfmIA=")
url_decoded := Base64.url_decode("Pz8_")
```

`encode()` emits padded RFC 4648 Base64. `url_encode()` uses the URL-safe
alphabet without padding. Their matching decode functions accept only the
canonical form they emit and return `Result<Bytes, Base64::DecodeError>`.
`Base64::DecodeErrorKind` distinguishes invalid length, characters, padding, and
non-canonical trailing bits; the error includes the input, zero-based position,
and a message.

## Hashing

`trb/std/digest` computes one-shot SHA-256 and SHA-512 digests. It also provides
SHA-1 and MD5 for legacy compatibility and non-security checksums:

```trb
import trb/std/digest
import trb/std/encoding/hex

digest := Digest.sha256("message".to_bytes())
text := Hex.encode(digest)
larger_digest := Digest.sha512("message".to_bytes())
legacy_digest := Digest.sha1("legacy identifier".to_bytes())
checksum := Digest.md5("legacy payload".to_bytes())
```

All four functions accept and return `Bytes`. Digest formatting stays explicit by
passing the result to hexadecimal or Base64 encoding. These synchronous APIs
have the same behavior in browsers, Bun, Node, Go, and Ruby.

SHA-1 and MD5 are not suitable for passwords, signatures, certificates, or
other collision-resistant security decisions. Prefer SHA-256 or SHA-512 unless
an external legacy format specifically requires the older digest.

`trb/std/hmac` provides HMAC-SHA-256, HMAC-SHA-512, and tag comparison:

```trb
import trb/std/hmac

def valid_tag?(key: Bytes, message: Bytes, expected: Bytes): Boolean
	return HMAC.equal(HMAC.sha256(key, message), expected)
end
```

Keys, messages, and tags are `Bytes`. `equal()` compares equal-length tags
without content-dependent branching and returns `false` when their lengths
differ. Use it instead of ordinary collection comparison when verifying a tag.

Use `trb/std/secure_compare` for the same comparison contract outside HMAC:

```trb
import trb/std/secure_compare

matches := SecureCompare.equal(actual_digest, expected_digest)
```

`equal()` accepts two `Bytes` values. For equal-length values it examines every
byte without content-dependent branching. A length mismatch returns `false`;
the input lengths are not treated as secret.

## Randomness

`trb/std/random` provides non-cryptographic random values for sampling,
simulation, and user-interface behavior:

```trb
import trb/std/random

fraction := Random.float()
index := Random.integer(10)
```

`float()` returns a `Float` in the half-open interval `[0.0, 1.0)`.
`integer(upper)` returns an `Integer` in `[0, upper)` and fails at runtime when
`upper` is not positive. The generator and its sequence are backend-defined;
these functions must not be used for secrets, tokens, or other security
decisions.

Use `trb/std/secure_random` when bytes must be unpredictable:

```trb
import trb/std/secure_random

token := SecureRandom.bytes(32)
```

`bytes(length)` returns cryptographically secure `Bytes`. The portable
synchronous contract accepts lengths from 0 through 65,536 bytes so it can use
the browser Web Crypto source as well as native Go and Ruby sources. An invalid
length or unavailable secure source is a runtime failure; it never falls back
to the non-cryptographic generator.

## StringBuilder and Unicode

Use `StringBuilder` for incremental text construction:

```trb
import trb/std/string_builder

mut builder := StringBuilder.new()
builder.append("Hello")
builder.append_codepoint(33)
blank := builder.empty?()
message := builder.to_s()
```

Only `new()` and `from_string()` are class members. Operations on an existing
builder are receiver methods. Destructive operations require a `mut` binding;
`to_s()` returns an immutable snapshot, and size counts Unicode code points.

`trb/std/unicode` provides scalar and identifier classification from one pinned
data set shared by all targets:

```trb
import trb/std/unicode

hiragana_a := 12354
is_letter := Unicode.letter(hiragana_a)
character := Unicode.from_codepoint(hiragana_a)
points := "A😀".codepoints()
```

The package also exposes digit, case, whitespace, valid-scalar, and identifier
classification. `Unicode.version()` reports the pinned data version.

## Arrays and hashes

Collection contracts infer their type parameters from the receiver or
arguments. Array and Hash operations use receiver methods; there are no public
`trb/std/arrays` or `trb/std/hashes` packages:

```trb
mut values := [1, 2]
values.push(3)
first := values.shift()
values.unshift(0)
reversed := values.reverse()
ascending := values.sort()
descending := values.sort_descending()
ranked := ["second", "first"].sort_by do |label|
	label.size()
end
deduplicated := [3, 1, 3, 2].uniq()
combined_values := values.concat([4, 5])
known := values.include?(2)
position := values.index(2)
occurrences := values.count(2)
has_even := values.any? do |value|
	value % 2 == 0
end
all_positive := values.all? do |value|
	value > 0
end
none_negative := values.none? do |value|
	value < 0
end
first_even := values.find do |value|
	value % 2 == 0
end
first_even_index := values.find_index do |value|
	value % 2 == 0
end
inclusive_values := (1..3).to_a()
exclusive_values := (1...3).to_a()

mut labels: Hash<Integer, String> := {1 => "one"}
known_label := labels.key?(1)
label := labels.try_fetch(1)
keys := labels.keys()
combined := labels.merge({2 => "two"})
labels.update({2 => "two"})
labels.each do |key, value|
	puts(key.to_s() + ": " + value)
end
removed := labels.delete(1)
```

Arrays provide size, emptiness, strict `[]`, safe `try_fetch`, `slice`,
`try_slice`, `first`,
`last`, shallow `dup`, mutable `push`/`unshift`, mutable strict `pop`/`shift`,
non-destructive shallow `reverse`, stable non-destructive sorting, value
membership, first-position lookup with `index(value)`, and occurrence counting.
`Array<String>` also provides `join`.

An Array is a shared reference value. Destructive operations and element
assignment update the same Array observed by aliases and by callers that pass
it to a `mut` parameter, including across capacity growth. Reassigning the
parameter itself remains local. `dup` and every operation documented as
returning a new Array create a distinct outer Array and copy elements
shallowly.

`Range<Integer>#to_a()` returns a new Array containing the same sequence that
Range iteration would visit. Inclusive and exclusive ends are honored; an
inclusive equal-bound Range contains one value, while exclusive equal-bound
and reversed Ranges are empty.

`sort()` and `sort_descending()` order an Array of `Integer`, `Float`, or
`String`. `sort_by` and `sort_by_descending` evaluate one non-fallible key
expression per element and accept the same key types. All four operations are
stable, including descending order. Strings use Unicode code point order,
independent of locale. Float `NaN` values sort after ordinary values in either
direction, and negative and positive zero compare as equal. Custom comparator
and locale-sensitive ordering APIs remain future design work.

`uniq()` returns the first occurrence of each element in input order and uses
the same portable equality contract as `include?` and `count`. `concat(other)`
returns a new shallow Array containing both inputs in order. Neither method
mutates its receiver; specifically, TypeRB `concat()` is not Ruby's destructive
`Array#concat`.

`any?`, `all?`, and `none?` evaluate a typed Boolean predicate from left to
right and stop as soon as the result is known. Empty Arrays return `false` for
`any?` and `true` for `all?` and `none?`.

`find` returns the first matching element as `T?`, while `find_index` returns
its position as `Integer?`. Both short-circuit and return `nil` when no element
matches. A nullable element type remains nullable, so finding a stored `nil`
and finding no element intentionally have the same result.

Value membership uses `include?`. `index(value)` returns the first matching
position as `Integer?`, and `count(value)` counts all matches. These operations
use portable `==` semantics and are available when the element type is numeric,
Boolean, String, or a payloadless enum. They do not inherit target-native
structural equality for Arrays, Hashes, records, or payload-bearing enums.

Hashes provide size, emptiness, strict `fetch`/`delete`, safe `try_fetch`, key
checks, keys, values, shallow `dup`/`merge`, and mutable `update`. `merge`
returns a new shallow Hash, while `update` mutates its receiver; duplicate keys
use the right-hand value. `delete` requires `mut`, returns the removed value,
and fails when the key is absent. Existing Hash arguments keep their exact key
and value types. `each` binds the key and value with their respective generic
types and traverses a shallow entry snapshot. Hash enumeration order is
unspecified.

Strict operations fail at runtime for missing keys, invalid indexes, ranges,
or empty edge removals. Array element access uses `value[index]`; safe fetch
returns `Result<T, IndexLookupError>` with the requested index and collection
size. Nonnegative indexes count from the start and negative indexes count from
the end, with `-1` naming the last element. Array subsequences use
`slice(range)` and `try_slice(range)` with the same
range rules as String. The safe form returns
`Result<Array<T>, SliceRangeError>` and both forms return a new shallow Array.
Hash safe fetch returns
`Result<V, KeyLookupError>` with the missing `String | Integer` key. Both errors
also carry a stable message.

Array `map`, `select`, and `reduce` are structured language expressions rather
than target callbacks. See the
[language guide](language.md#arrays-hashes-and-iteration) for examples.

`Array#concurrent_map` is the import-free bounded I/O-concurrency form. It
returns block results in input order, uses a portable default limit of 8, and
accepts an explicit positive named `limit`. Nested calls share the current
structured task group's capacity. The operation does not imply CPU parallelism
or special-case `Result`; a Result-returning block produces an Array of Result
values. See the [language guide](language.md#arrays-hashes-and-iteration) and
[decision record](decisions/0004-bounded-structured-concurrent-map.md).

## URL encoding

`trb/std/url` encodes individual URL components and ordered query parameters:

```trb
import trb/std/url
import trb/std/result

encoded := URL.encode_component("todos/日本語")
query := URL.build_query([
	URL::QueryParameter.new(name: "tag", value: "type rb"),
	URL::QueryParameter.new(name: "tag", value: "go"),
])

def decode_segment(value: String): Result<String, URL::DecodeError>
	return URL.decode_component(value)
end

def decode_query(value: String): Result<Array<URL::QueryParameter>, URL::DecodeError>
	return URL.parse_query(value)
end
```

`encode_component` preserves only RFC 3986 unreserved ASCII characters and
encodes all other UTF-8 bytes with uppercase hexadecimal escapes. It does not
encode spaces as `+`. `decode_component` likewise preserves a literal `+` and
returns `Result<String, URL::DecodeError>` for malformed escapes or decoded
bytes that are not valid UTF-8.

`parse_query` and `build_query` use an ordered `Array<URL::QueryParameter>` so
duplicate names and their global order are preserved. They accept and produce
query strings without a leading `?`. Parsing treats `+` as a space, skips empty
`&` segments, and normalizes a name without `=` to an empty value. Building
uses `+` for spaces and percent-encodes reserved bytes. Invalid escapes and
decoded bytes that are not valid UTF-8 return `URL::DecodeError`.

Complete URL parsing remains a future addition to the package.

## Path values

`trb/std/path` provides two distinct String-backed nominal values. A `Path`
contains host-native path text. A `RelativePath` contains a validated logical
descendant, independent of the host's separator rules. Neither represents an
opened resource or grants filesystem access.

<!-- trb-doc-test: stdlib-path-values -->
```trb
import { Path, RelativePath, RelativePathError } from trb/std/path
import trb/std/result

def output_path(root: Path, name: String): Result<Path, RelativePathError>
	base := try RelativePath.parse("generated/reports")
	leaf := try base.child(name)
	return Result<Path, RelativePathError>::Ok(root.join(leaf))
end
```

The declaration-root import `import trb/std/path` binds `Path`. Import the peer
`RelativePath` and `RelativePathError` declarations by exact name when needed.

| Operation | Result |
| --- | --- |
| `Path.new(source: String)` | `Path` |
| `Path#to_s()` | `String` |
| `Path#join(path: RelativePath)` | `Path` |
| `RelativePath.parse(source: String)` | `Result<RelativePath, RelativePathError>` |
| `RelativePath#to_s()` | `String` |
| `RelativePath#join(path: RelativePath)` | `RelativePath` |
| `RelativePath#child(name: String)` | `Result<RelativePath, RelativePathError>` |
| `RelativePath#parent()` | `RelativePath?` |

Both values retain the ordinary public newtype `value()` projection and String
equality. Equality compares exact text, not filesystem identity or normalized
filenames. `Path.new` accepts any String, including an empty one; it performs
no validation, normalization, existence check, environment or home expansion,
or conversion to an absolute path. `RelativePath.new` is private to its type
body, so portable source must use its public factory.

### Logical descendant grammar

`RelativePath.parse` accepts a nonempty sequence of nonempty `/`-separated
components and preserves the input exactly. The fixed rules reject:

- Leading or trailing `/`, repeated `/`, and components equal to `.` or `..`.
- Backslash, U+0000 through U+001F, and the ASCII characters `< > : " | ? *`.
  This also excludes absolute, drive-prefixed, UNC, and alternate-data-stream
  spellings; it does not reinterpret any of them as relative input.
- A component ending in an ASCII dot or space.
- Reserved device-name stems, using ASCII-only case-insensitive comparison:
  `CON`, `PRN`, `AUX`, `NUL`, `CONIN$`, `CONOUT$`, and `COM` or `LPT` followed by
  one of `0`–`9`, `¹`, `²`, or `³`. The stem is the text before the first dot,
  with trailing ASCII spaces removed for this check only. Extensions do not
  escape the rule: `con.txt`, `CON .txt`, and `lpt¹.log` are rejected.

This deliberately conservative logical grammar is informed by
[Windows filename rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file),
but does not delegate validation to the current operating system. It does not
normalize Unicode, change case, trim the returned value, limit path length, or
guarantee that every accepted name can be created on every filesystem. Case
and Unicode-normalization collisions remain application concerns. `.config`,
`..name`, leading spaces, and non-ASCII names such as `日本語/😀.md` are valid.

`join` composes two validated descendants with one `/` and cannot introduce an
invalid component. `child` accepts exactly one new component and validates it;
it is not a String overload of `join`. `parent` removes the last component and
returns `nil` for a one-component path. There is no empty `RelativePath` value
representing the root.

`RelativePathError` is a payload-free enum, separate from I/O errors:

| Case | Failure |
| --- | --- |
| `Empty` | The input is empty. |
| `EmptyComponent` | A leading, trailing, or repeated `/` occurs. |
| `DotComponent` | A component is `.` or `..`. |
| `InvalidCharacter` | A component contains a prohibited character. |
| `TrailingDotOrSpace` | A component ends with an ASCII dot or space. |
| `ReservedName` | A component has a reserved device-name stem. |
| `MultipleComponents` | A `child` argument contains `/`. |

`parse` checks empty input and empty components first, then components in
source order, using the remaining validation cases in the order above. `child`
checks for `/` before calling the same parser. `error.to_s()` provides a human
diagnostic without embedding the input; applications should branch on the enum,
not the message text.

Generic inbound representation decoding cannot construct `RelativePath`.
Decode a String or representation DTO and call `RelativePath.parse` explicitly.
Outgoing codecs may project it to String, as for other closed newtypes.

### Host composition

`Path#join` appends a validated descendant using the **target runtime's** host
separator, not the compiler machine's. The parent is preserved byte for byte,
including `..`, repeated separators, and mixed slash styles. On Windows only
the child's `/` characters become `\`. A separator is inserted unless the
parent is empty, already ends in an accepted host separator, or is exactly a
Windows drive letter followed by `:`. Thus POSIX `/` and `//` stay distinct,
Windows `C:` produces `C:child`, `C:\` produces `C:\child`, and a UNC share
root receives a separator. An empty parent produces just the host-spelled child.
Receiver and argument are each evaluated once, in that order; safe navigation
skips the argument when the receiver is `nil`.

There is no cleaning join, parent resolution, or filesystem access. A parent
containing `symlink/..` retains its host interpretation, and a descendant can
still traverse a symlink when later opened: these value types do **not** provide
root containment or a security boundary.

Logical validation and composition are ordinary TypeRB source. The one
compiler-owned operation is host joining: portable source and the current
adapter protocol do not expose a target-host separator query without introducing
a platform dependency. The intrinsic keeps that host choice separate from the
pure package and can move to an ordinary adapter when that boundary is available.

Go, Ruby, and Node/Bun TypeScript support host joining. An explicit
`typescript.runtime: "browser"` build rejects `Path#join`; it still supports
`Path.new`, `to_s`, and all `RelativePath` operations. The typed-IR REPL uses
the machine running the evaluator for host joining, regardless of output mode.

## Filesystem

`trb/std/file` and `trb/std/dir` are separate host-filesystem bridges. `File`
is an actual opaque resource type rather than a module containing a second file
type. Explicit TypeScript browser builds reject these host operations; merely
using their ordinary value declarations does not acquire host access. Every
operation returns a `Result`:

<!-- trb-doc-test: stdlib-file-read -->
```trb
import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def load_config(path: Path): Result<String, FileSystemError>
	return File.open(path) do |file|
		try file.read_text(max_bytes: 1048576)
	end
end
```

Failures carry `operation`, `target`, `message`, and a
`FileSystemErrorKind` instead of exposing target exceptions. Import the peer
error declarations from `trb/std/errors` only when source names them directly.
`FileSystemTarget` is the immutable sum `Host(Path)`, `Relative(RelativePath)`,
or `Root`. The currently implemented ambient operations return `Host`, with
the original, unnormalized input (or the exact child path for a metadata
failure). Relative/root targets do not themselves acquire a directory or
grant authority. Rooted directory operations are not implemented yet.

`NotFound`, `PermissionDenied`, and `AlreadyExists` classify the corresponding
host errors consistently across open, enumeration, and recursive creation.
Other host errors remain `Other`; an intermediate non-directory is not
promised a distinct portable classification. Empty paths and paths containing
NUL return `InvalidPath`, without converting every host invalid-argument error
into that kind. Human-readable messages are not stable machine-readable data.

`File.open` requires a block and accepts an optional typed `FileMode`. Omitting
the mode selects `Read`, which opens an existing file for reading. `Write`
opens a file for writing, creating it or truncating it to zero bytes.
`CreateNew` opens a newly created file for writing. The `File` value is opaque
and scoped: it may only be used as a direct receiver for its file methods
inside that block. It cannot be constructed, assigned, passed to another
function, placed in a collection, returned, or captured by a nested callback.
The compiler closes the file before the `Result` leaves the structured block.
Prefix `try` may end the block with an error, but cleanup still finishes before
that error propagates to the enclosing `Result` boundary. The exact standard
`File` identity also cannot appear in an authored parameter, return, field,
collection, function type, or transparent alias. Compiler-generated and
external declaration contracts cannot supply it as a value; only the trusted
`File.open` block introduces it. An unrelated declaration with the same name
is unaffected.

Ruby-native fallback syntax is opaque to resource analysis, so it is rejected
while the scoped file is in scope. Compute native values before `File.open`, or
move native work after the block has returned a non-resource value.

`file.read(max_bytes:)` returns `Bytes`, and
`file.read_text(max_bytes:)` returns `String`. A successful result contains at
most the given number of bytes. A negative bound returns `InvalidLimit`; input
exceeding the bound returns `TooLarge` after the operation observes one byte
beyond it. The bound is checked while reading the open handle, not only by a
prior size check. Text decoding is strict UTF-8: invalid bytes return
`InvalidEncoding`. A leading UTF-8 BOM is preserved as U+FEFF. Size overflow
takes precedence over invalid encoding. For replacement display, explicitly
read Bytes and call `Bytes#to_s`; reading text never silently repairs the input.
`file.write` accepts `Bytes`, while `file.write_text` accepts
`String`.

`CreateNew` returns `AlreadyExists` if the path already exists. The existence
check and creation are one exclusive host operation, so parallel writers
cannot both create the same path. This is no-clobber creation, not atomic
replacement of an existing file. It does not promise `fsync`, directory
synchronization, or persistence after power loss.

<!-- trb-doc-test: stdlib-exclusive-create-new -->
```trb
import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def create_output(path: Path, bytes: Bytes): Result<Unit, FileSystemError>
	return File.open(path, mode: FileMode::CreateNew) do |file|
		try file.write(bytes)
	end
end
```

`Dir.children(path, max_entries:)` requires an explicit named bound and returns
sorted immediate `DirEntry<Path>` values. Each entry has a `name: String`,
a host-native `path: Path`, and a typed `DirEntryKind`: `File`, `Directory`,
or `Other`. Symbolic links and other entries that must not be traversed as
directories are `Other`. The operation does not recurse. `Dir` is a
nonconstructible type root in this slice; it owns directory operations but does
not yet represent an open directory resource.

A negative `max_entries` returns `InvalidLimit`. A directory containing more
entries than the bound returns `TooLarge`, not a truncated success. Zero
succeeds only for an empty directory. Enumeration consumes bounded batches and
checks the limit before retaining a full result; it does not read the entire
directory into an unbounded Array first. Metadata errors fail the whole
operation, as do close errors after successful enumeration. These are memory
and result bounds, not deadlines or a consistent snapshot of concurrent writes.

Directory entry names must have a lossless valid UTF-8 representation. If any
name does not, `Dir.children` returns `FileSystemErrorKind::UnsupportedName` for operation
`children`, with `Host(directory)` as the error target and
`directory entry name is not valid UTF-8` as the message. It never substitutes
U+FFFD and returns a path that names a different entry. Valid names are sorted
by their UTF-8 bytes.

<!-- trb-doc-test: stdlib-dir-children -->
```trb
import trb/std/path
import trb/std/dir
import { DirEntryKind } from trb/std/dir
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def regular_file_paths(directory: Path): Result<Array<Path>, FileSystemError>
	entries := try Dir.children(directory, max_entries: 10000)
	mut paths: Array<Path> := []
	entries.each do |entry|
		if entry.kind == DirEntryKind::File
			# entry.path can be passed directly to File.open on this host.
			paths.push(entry.path)
		end
	end
	return Result<Array<Path>, FileSystemError>::Ok(paths)
end
```

Writes return `Result<Unit, FileSystemError>`. `Unit` is a storable value
representing successful completion; it is distinct from the internal `Void`
return category.

Filesystem inputs are nominal `Path` values. Their host-native text is passed
to the target runtime without slash-only normalization. `DirEntry.path`
preserves the directory text
given to `Dir.children` and appends the entry name without a cleaning join. It
therefore preserves host resolution for parents containing symbolic links and
`..`. The appended separator is host-native, existing Windows `/` or `\`
suffixes are accepted, `C:` remains drive-relative as `C:child`, and UNC share
roots receive a separator. These I/O calls require `Path`, not String or
`RelativePath`; String DTOs use explicit `Path.new(input)` construction. The separate
[path value API](#path-values) can construct a not-yet-existing descendant
without enumerating a directory. General host-path parsing, cleaning, volume
inspection, and cross-host conversion are not provided.

### Recursive directory creation

`Dir.create_all(path: Path): Result<Unit, FileSystemError>` creates missing
ancestors and the requested directory. An existing directory, `.`, or an
existing filesystem root succeeds. A competing creator winning the same
directory creation is not by itself a failure. A file, symlink to a file, or
dangling link that cannot resolve as a directory fails. Ambient operations
follow the host's ordinary symbolic-link resolution; they do not promise
containment. Failure does not roll back directories already created.

New directories use the ordinary host creation permissions; existing directory
permissions are not changed. No exact portable permission bits are promised.
Errors retain operation `create_all` and the original input Path.

<!-- trb-doc-test: stdlib-dir-create-all -->
```trb
import trb/std/path
import trb/std/dir
import trb/std/result
import trb/std/unit
import { FileSystemError } from trb/std/errors

def prepare(directory: Path): Result<Unit, FileSystemError>
	return Dir.create_all(directory)
end
```

These host operations need privileged runtime bridges: ordinary packages cannot
yet acquire scoped descriptors or enumerate native directory entries through
the extension protocol. The integration does not grant that authority to
external providers. It adds neither an opened `Dir` capability, resource
borrowing, atomic publication, locks, nor a sandbox.

## Process

`trb/std/process` provides argument, environment, working-directory, and
shell-free process operations:

```trb
import trb/std/process
import trb/std/result

def run_formatter(files: Array<String>): Result<Process::Output, Process::Error>
	return Process.run("formatter", files)
end
```

`run` takes a command and separate `Array<String>` arguments. It captures
stdout and stderr. A started process with any exit status is
`Ok(Process::Output)`; launch or host failures are `Err(Process::Error)`.

## JSON and JSONC

`trb/std/json` provides an explicit JSON value enum plus typed record codecs:

```trb
import trb/std/json
import trb/std/result

record User
	id: Integer @json("user_id")
	name: String
	nickname: String?
end

def decode_user(source: String): Result<User, JSON::Error>
	return JSON.decode<User>(source)
end

def encode_user(user: User): Result<String, JSON::Error>
	return JSON.encode(user)
end
```

Typed codecs support booleans, numbers, strings, nullable values, arrays,
`Hash<String, V>`, nested records, raw-value enums, and `@json` record-field
wire names. Raw-value enums encode as their declared String or Integer raw
value and reject unknown values during decode. Unknown object fields are
ignored. Schema information remains in typed IR; generated code does not
reflect over target objects.

`JSON.parse` accepts strict JSON. `JSONC.parse` additionally accepts line and
block comments. Both reject trailing commas. Parse, stringify, encode, decode,
and accessors return typed errors with JSON Pointer paths where applicable.

Go output currently uses the stable `encoding/json` API. The experimental
`encoding/json/v2` package is not required by generated projects.

## Date and time

`trb/std/time` separates exact instants from timezone-free civil values:

```trb
import { Date, DateTime, Duration, Instant, TimeZone } from trb/std/time

release_date := Date.parse("2026-08-11")
puts(release_date.to_s())
local_start := DateTime.parse("2026-08-11T09:30:00")
tokyo := TimeZone.get("Asia/Tokyo")
start := local_start.to_instant(tokyo)
finish := start.add(Duration.minutes(90))
puts(finish.to_datetime(tokyo).to_s())
```

The initial immutable types are `Date`, `TimeOfDay`, `DateTime`, `Instant`,
`Duration`, and `TimeZone`. `DateTime` is a civil value and has no implicit
timezone. `Instant` is an exact point and formats in UTC. Converting a local
`DateTime` through `try_to_instant()` reports nonexistent and ambiguous DST
times as `DateTimeError`; `to_instant()` is the strict convenience form.

Constructors and strict `parse()` reject invalid values. `try_new()`,
`try_parse()`, and `TimeZone.try_get()` return typed errors. Values support
component access, canonical `to_s()`, `before?()`, `after?()`, and `same?()`.
`Date` adds whole civil days. `Instant` adds or subtracts fixed `Duration`
values and computes `duration_since()`. `Instant.now()` and package-level
`now()` read the current clock.

Canonical JSON codecs use strings: ISO dates and local date-times, RFC 3339
UTC instants, named timezone identifiers, and `PT...S` durations. The portable
range is Gregorian year 0001 through 9999 with nanosecond fields. A target or
database with lower storage precision may round-trip only the precision its
schema supports. Calendar periods, zoned date-time objects, locale formatting,
and injectable clocks are not part of the initial package.

## Result and Unit

Import `Result<T, E>` explicitly:

```trb
import trb/std/result
```

Construct `Result::Ok(value)` or `Result::Err(error)` and handle it with an
exhaustive enum `case`. Prefix `try` unwraps `Ok` and returns `Err` from the
nearest compatible Result-returning function. Postfix `catch` unwraps `Ok` and
evaluates a local fallback or control transfer for `Err`. These constructs are
described in the [language guide](language.md#result-control-flow).

`trb/std/unit` provides `Unit` for successful generic operations with no
application payload, including `Result<Unit, E>` operations.

`trb/std/errors` provides `EnumValueError` for an unsuccessful raw-value enum
conversion. `from_raw()` infers this error type and its runtime dependency; an
explicit import is needed only when source code names `EnumValueError` itself.

## Tests

`trb/std/test` exports `describe`, `test`, `expect`, `expect_ok`, and
`expect_err`. Suites and cases take literal names and parameterless blocks so
the compiler, CLI, language server, and editor use one deterministic discovery
model. `expect()` returns a typed `Expectation<T>` with equality, inequality,
Boolean, and nil assertions. `expect_ok(Result<T, E>)` returns `T`, while
`expect_err(Result<T, E>)` returns `E`; either helper fails the current case if
the Result has the opposite variant.

The package is portable compiler-owned source plus a small target runtime.
Assertion failures retain their original `.trb` path, line, and column. The
runtime protocol is consumed by `trb test` and editor integrations; application
code should use the public assertion API rather than its internal functions.
See the [testing guide](guides/testing.md).

## Package index

The current portable standard library includes:

- `trb/std/io`
- `trb/std/math`
- `trb/std/encoding/hex`
- `trb/std/encoding/base64`
- `trb/std/digest`
- `trb/std/hmac`
- `trb/std/secure_compare`
- `trb/std/string_builder`
- `trb/std/unicode`
- `trb/std/url`
- `trb/std/file`
- `trb/std/dir`
- `trb/std/process`
- `trb/std/json`
- `trb/std/jsonc`
- `trb/std/time`
- `trb/std/result`
- `trb/std/errors`
- `trb/std/unit`
- `trb/std/test`

Platform packages are mode checked and remain separate from the portable
standard library. Portable official application packages are documented in the
[`trb/http`](guides/http.md), [`trb/web`](guides/web.md), and
[`trb/orm`](guides/orm.md) guides.
