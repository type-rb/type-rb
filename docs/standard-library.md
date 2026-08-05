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

## Strings and numbers

Portable built-in types expose receiver methods backed by the same contracts
as `trb/std/strings` and `trb/std/numbers`:

```trb
import trb/std/strings

text := 123.to_s()
number := "123".to_i()
safe_number := "123".try_to_i()
length := "Hello".size()
upper := strings.uppercase("Hello")
```

Integer parsing accepts a complete ASCII decimal integer with an optional sign.
`to_i()` raises on invalid or non-portable input; `try_to_i()` returns
`Result<Integer, String>`. Parsed integers use the portable exact range
`-9007199254740991..9007199254740991`.

`String#size` counts Unicode code points. Additional receiver operations
include `codepoints`, `empty?`, `include?`, `start_with?`, `end_with?`, `split`,
`upcase`, and `downcase`.

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
first := arrays.first(values)

labels: Hash<Integer, String> := {1 => "one"}
known := labels.key?(1)
label := labels.try_fetch(1)
keys := hashes.keys(labels)
```

Arrays provide size, emptiness, strict `fetch`, safe `try_fetch`, `first`,
`last`, shallow `dup`, mutable `push`, and mutable strict `pop`.
`Array<String>` also provides `join`.

Hashes provide size, emptiness, strict `fetch`, safe `try_fetch`, key checks,
keys, values, and shallow `dup`. Hash enumeration order is unspecified.

Strict operations fail at runtime for missing keys, invalid indexes, or empty
edge/pop operations. Safe fetch returns a `Result` with a stable error value.

`map`, `select`, and `reduce` are structured language expressions rather than
target callbacks. See the [language guide](language.md#arrays-hashes-and-iteration)
for examples.

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
- `trb/std/bytes`
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
- `trb/std/unit`

Platform packages are mode checked and remain separate from the portable
standard library.
