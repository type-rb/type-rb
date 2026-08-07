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

Import `trb/std/io` when namespaced access is useful. Both forms resolve to the
same operation:

```trb
import trb/std/io

io.puts("Hello, TypeRB!")
```

## Scalars and strings

Portable built-in types expose receiver methods backed by the same contracts
as `trb/std/numbers`, `trb/std/booleans`, and `trb/std/strings`:

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
root := math.sqrt(9)
even := distance.even?()
ordinary := ratio.finite?()
enabled := true.to_s()
number := "123".to_i()
safe_number := "123".try_to_i()
length := "Hello".size()
upper := strings.uppercase("Hello")
```

Integer parsing accepts a complete ASCII decimal integer with an optional sign.
`to_i()` raises on invalid or non-portable input; `try_to_i()` returns
`Result<Integer, NumberParseError>`. The error preserves its
`NumberParseErrorKind`, input, and message. Parsed integers use the portable
exact range `-9007199254740991..9007199254740991`.

`Integer#to_f()` is an exact widening conversion. `Float#to_i()` truncates
toward zero and raises for non-finite or out-of-range values. `Float#to_s()`
uses a portable fixed decimal spelling without exponent notation, including
`.0` for integral Float values. Their package forms are `numbers.to_float`,
`numbers.truncate`, and `numbers.float_to_string`.

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

`Boolean#to_s()` returns lowercase `"true"` or `"false"`. Package forms use
`numbers.absolute`, `numbers.zero`, `numbers.positive`, `numbers.negative`,
`numbers.even`, `numbers.odd`, `numbers.float_absolute`, `numbers.finite`,
`numbers.infinite`, and `numbers.nan`, plus `numbers.min`, `numbers.max`,
`numbers.clamp`, `numbers.floor`, `numbers.ceil`, `numbers.round`,
`numbers.truncate`, and `booleans.to_string`.

`String#size` counts Unicode code points. Additional receiver operations
include `codepoints`, `empty?`, `strip`, `lstrip`, `rstrip`, `include?`,
`start_with?`, `end_with?`, `split`, `upcase`, and `downcase`. String trimming
uses the pinned Unicode 15.0 `White_Space` set, preserves internal whitespace,
and does not remove U+FEFF. Package forms use the same names from
`trb/std/strings`.

## Bytes

`Bytes` is an immutable binary sequence distinct from `String` and
`Array<Integer>`:

```trb
import trb/std/bytes

payload := "A😀".to_bytes()
byte_length := payload.size()
first := payload.at(0)
text := bytes.to_string(payload)
```

String conversion uses UTF-8. Decoding invalid input replaces it with U+FFFD;
call `valid_utf8()` first when invalid bytes must be rejected. Byte indexing is
strict and concatenation is non-mutating.

## Binary encoding

`trb/std/encoding/hex` converts between `Bytes` and hexadecimal text:

```trb
import trb/std/encoding/hex

text := hex.encode("A😀".to_bytes())
decoded := hex.decode("41F09F9880")
```

Encoding uses lowercase ASCII. Decoding accepts uppercase or lowercase input
and returns `Result<Bytes, HexDecodeError>`. Invalid characters and odd-length
input are distinguished by `HexDecodeErrorKind`; the error also preserves the
input, a zero-based character position, and a message. For odd-length input,
the position is the missing character at the end of the string.

## StringBuilder and Unicode

Use `StringBuilder` for incremental text construction:

```trb
import trb/std/string_builder

mut builder := string_builder.new()
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
is_letter := unicode.letter(hiragana_a)
character := unicode.from_codepoint(hiragana_a)
points := "A😀".codepoints()
```

The package also exposes digit, case, whitespace, valid-scalar, and identifier
classification. `unicode.version()` reports the pinned data version.

## Arrays and hashes

Collection contracts infer their type parameters from the receiver or
arguments:

```trb
import trb/std/arrays
import trb/std/hashes

mut values := [1, 2]
values.push(3)
first := values.shift()
values.unshift(0)
reversed := arrays.reverse(values)
known := values.include?(2)
occurrences := arrays.count(values, 2)

mut labels: Hash<Integer, String> := {1 => "one"}
known_label := labels.key?(1)
label := labels.try_fetch(1)
keys := hashes.keys(labels)
combined := labels.merge({2 => "two"})
labels.update({2 => "two"})
labels.each do |key, value|
	puts(key.to_s() + ": " + value)
end
removed := hashes.delete(labels, 1)
```

Arrays provide size, emptiness, strict `fetch`, safe `try_fetch`, `first`,
`last`, shallow `dup`, mutable `push`/`unshift`, mutable strict `pop`/`shift`,
non-destructive shallow `reverse`, value membership, and occurrence counting.
`Array<String>` also provides `join`.

Value membership is `include?` on a receiver and `arrays.contains` in package
form. `count(value)` has the same name in both forms. Both use portable `==`
semantics and are available when the element type is numeric, Boolean, String,
or a payloadless enum. They do not inherit target-native structural equality
for Arrays, Hashes, records, or payload-bearing enums.

Hashes provide size, emptiness, strict `fetch`/`delete`, safe `try_fetch`, key
checks, keys, values, shallow `dup`/`merge`, and mutable `update`. `merge`
returns a new shallow Hash, while `update` mutates its receiver; duplicate keys
use the right-hand value. `delete` requires `mut`, returns the removed value,
and fails when the key is absent. Existing Hash arguments keep their exact key
and value types. `each` binds the key and value with their respective generic
types and traverses a shallow entry snapshot. Hash enumeration order is
unspecified.

Strict operations fail at runtime for missing keys, invalid indexes, or empty
edge removals. Array safe fetch returns `Result<T, IndexLookupError>` with the
requested index and collection size. Hash safe fetch returns
`Result<V, KeyLookupError>` with the missing `String | Integer` key. Both errors
also carry a stable message.

Array `map`, `select`, and `reduce` are structured language expressions rather
than target callbacks. See the
[language guide](language.md#arrays-hashes-and-iteration) for examples.

## Logical paths

`trb/std/path` manipulates portable `/`-separated logical paths without
accessing the host filesystem:

```trb
import trb/std/path

config_path := path.join("config", "../trbconfig.jsonc")
directory := path.directory("src/compiler/main.trb")
parts := path.components("/srv/type-rb")
```

The package provides `clean`, two-part `join`, `absolute`, `components`,
`base`, `directory`, and `separator`. Its behavior does not depend on the
target OS or current directory.

## Filesystem

`trb/std/filesystem` is the explicit host-filesystem bridge. Every operation
returns a `Result`:

```trb
import { FileError, read_text } from trb/std/filesystem
import { Result } from trb/std/result

def load_config(path: String): Result<String, FileError>
	return read_text(path)
end
```

The package provides existence checks, UTF-8 and raw-byte reads and writes,
recursive directory creation, and sorted immediate-child listing. Failures
carry `operation`, `path`, and `message` instead of exposing target exceptions.

Writes and directory creation return `Result<Unit, FileError>`. `Unit` is a
storable value representing successful completion; it is distinct from the
internal `Void` return category.

Filesystem paths are host paths. Use `trb/std/path` separately for portable
lexical path operations.

## Process

`trb/std/process` provides argument, environment, working-directory, and
shell-free process operations:

```trb
import { ProcessError, ProcessResult, run } from trb/std/process
import { Result } from trb/std/result

def run_formatter(files: Array<String>): Result<ProcessResult, ProcessError>
	return run("formatter", files)
end
```

`run` takes a command and separate `Array<String>` arguments. It captures
stdout and stderr. A started process with any exit status is
`Ok(ProcessResult)`; launch or host failures are `Err(ProcessError)`.

## JSON and JSONC

`trb/std/json` provides an explicit JSON value enum plus typed record codecs:

```trb
import { JsonError, decode, encode } from trb/std/json
import { Result } from trb/std/result

record User
	id: Integer @json("user_id")
	name: String
	nickname: String?
end

def decode_user(source: String): Result<User, JsonError>
	return decode<User>(source)
end

def encode_user(user: User): Result<String, JsonError>
	return encode(user)
end
```

Typed codecs support booleans, numbers, strings, nullable values, arrays,
`Hash<String, V>`, nested records, and `@json` wire names. Unknown object fields
are ignored. Schema information remains in typed IR; generated code does not
reflect over target objects.

`json.parse` accepts strict JSON. `jsonc.parse` additionally accepts line and
block comments. Both reject trailing commas. Parse, stringify, encode, decode,
and accessors return typed errors with JSON Pointer paths where applicable.

Go output currently uses the stable `encoding/json` API. The experimental
`encoding/json/v2` package is not required by generated projects.

## Result and Unit

Import `Result<T, E>` explicitly:

```trb
import { Result } from trb/std/result
```

Construct `Result::Ok(value)` or `Result::Err(error)` and handle it with an
exhaustive enum `case`. The current alpha has no propagation operator.

`trb/std/unit` provides `Unit` for successful generic operations with no
application payload.

## Package index

The current portable standard library includes:

- `trb/std/io`
- `trb/std/strings`
- `trb/std/numbers`
- `trb/std/math`
- `trb/std/booleans`
- `trb/std/bytes`
- `trb/std/encoding/hex`
- `trb/std/string_builder`
- `trb/std/unicode`
- `trb/std/arrays`
- `trb/std/hashes`
- `trb/std/path`
- `trb/std/filesystem`
- `trb/std/process`
- `trb/std/json`
- `trb/std/jsonc`
- `trb/std/result`
- `trb/std/errors`
- `trb/std/unit`

Platform packages are mode checked and remain separate from the portable
standard library.
