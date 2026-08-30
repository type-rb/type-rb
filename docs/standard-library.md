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

Portable built-in types expose receiver methods backed by the same contracts
as `trb/std/numbers`, `trb/std/booleans`, and `trb/std/strings`:

<!-- trb-doc-test: stdlib-scalars -->
```trb
import trb/std/math
import trb/std/strings

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
upper := Strings.uppercase("Hello")
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
range round to signed zero consistently across backends. Declaration-root
forms are `Numbers.parse_float` and `Numbers.try_parse_float`.

`Integer#to_f()` is an exact widening conversion. `Float#to_i()` truncates
toward zero and raises for non-finite or out-of-range values. `Float#to_s()`
uses a portable fixed decimal spelling without exponent notation, including
`.0` for integral Float values. Their declaration-root forms are
`Numbers.to_float`, `Numbers.truncate`, and `Numbers.float_to_string`.

Integers provide `abs()`, `zero?()`, `positive?()`, `negative?()`, `even?()`,
`odd?()`, `min()`, `max()`, and `clamp()`. An invalid clamp range raises.
Floats provide `abs()`, `floor()`, `ceil()`, `round()`, `truncate()`,
`finite?()`, `infinite?()`, and `nan?()`; the integer conversions reject
non-finite and out-of-range results. `round()` resolves halfway values away
from zero. `abs()` converts negative zero to positive zero and leaves NaN as
NaN.

`trb/std/math` provides `sqrt`, `exp`, `log`, `log2`, and `log10`, returning
Float. Integer arguments use the ordinary safe widening to Float. Negative
square roots and logarithms are NaN, and logarithms of zero are negative
infinity. Exponentiation remains the `**` operator.

`Boolean#to_s()` returns lowercase `"true"` or `"false"`. Declaration-root
forms use `Numbers.absolute`, `Numbers.zero`, `Numbers.positive`,
`Numbers.negative`, `Numbers.even`, `Numbers.odd`,
`Numbers.float_absolute`, `Numbers.finite`, `Numbers.infinite`, and
`Numbers.nan`, plus `Numbers.min`, `Numbers.max`, `Numbers.clamp`,
`Numbers.floor`, `Numbers.ceil`, `Numbers.round`, `Numbers.truncate`, and
`Booleans.to_string`.

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
pattern raises. The declaration-root forms are `Strings.characters`,
`Strings.reverse`, and `Strings.replace_all`.

## Bytes

`Bytes` is an immutable binary sequence distinct from `String` and
`Array<Integer>`:

```trb
import trb/std/bytes

payload := "A😀".to_bytes()
byte_length := payload.size()
first := payload.at(0)
text := Bytes.to_string(payload)
```

String conversion uses UTF-8. Decoding invalid input replaces it with U+FFFD;
call `valid_utf8()` first when invalid bytes must be rejected. Byte indexing is
strict and concatenation is non-mutating.

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

Destructive operations require a `mut` binding. `to_s()` returns an immutable
snapshot, and size counts Unicode code points.

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
and reversed Ranges are empty. The package form is `ranges.to_array(range)`.

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

## Logical paths

`trb/std/path` manipulates portable `/`-separated logical paths without
accessing the host filesystem:

```trb
import trb/std/path

config_path := Path.join("config", "../trbconfig.jsonc")
directory := Path.directory("src/compiler/main.trb")
parts := Path.components("/srv/type-rb")
```

The package provides `clean`, two-part `join`, `absolute`, `components`,
`base`, `directory`, and `separator`. Its behavior does not depend on the
target OS or current directory.

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

## Filesystem

`trb/std/filesystem` is the explicit host-filesystem bridge. Every operation
returns a `Result`:

```trb
import trb/std/filesystem
import trb/std/result

def load_config(path: String): Result<String, FileSystem::Error>
	return FileSystem.read_text(path)
end
```

The package provides existence checks, UTF-8 and raw-byte reads and writes,
recursive directory creation, and sorted immediate-child listing. Failures
carry `operation`, `path`, and `message` instead of exposing target exceptions.

Writes and directory creation return `Result<Unit, FileSystem::Error>`. `Unit` is a
storable value representing successful completion; it is distinct from the
internal `Void` return category.

Filesystem paths are host paths. Use `trb/std/path` separately for portable
lexical path operations.

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
- `trb/std/strings`
- `trb/std/numbers`
- `trb/std/math`
- `trb/std/booleans`
- `trb/std/bytes`
- `trb/std/encoding/hex`
- `trb/std/encoding/base64`
- `trb/std/digest`
- `trb/std/hmac`
- `trb/std/secure_compare`
- `trb/std/string_builder`
- `trb/std/unicode`
- `trb/std/ranges`
- `trb/std/path`
- `trb/std/url`
- `trb/std/filesystem`
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
