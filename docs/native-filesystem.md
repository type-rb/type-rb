# Native file and directory operations

Import `File` and `FileMode` from `trb/std/file`, `Dir` from `trb/std/dir`,
`Path` from `trb/std/path`, and the ordinary `Result`, `Unit` and filesystem
error declarations from their standard packages.

`File.open(path, mode:) do |file| ... end` returns a `Result` containing the
block's final value or a `FileSystemError`. The default mode is `FileMode::Read`.
Use the acquisition directly as a declaration, assignment or return value.
The block must end with a value expression. An explicitly yielded `Result`
stays nested; `try` inside the block propagates to that acquisition boundary.

```trb
import { File, FileMode } from trb/std/file
import trb/std/path
import trb/std/result
import trb/std/unit
import { FileSystemError } from trb/std/errors

def copy(source: Path, destination: Path): Result<Unit, FileSystemError>
  bytes := try File.open(source) do |file|
    try file.read(max_bytes: 1048576)
  end
  return File.open(destination, mode: FileMode::CreateNew) do |file|
    try file.write(bytes)
  end
end
```

The block parameter and local aliases are immutable borrows. Synchronous source
helpers may accept a `File`; resources cannot escape through returns, fields,
containers, nullable or `Any` values, retained callbacks or unverified calls. The
compiler checks transitively reached helpers. A `File` cannot be constructed
with `File.new()`. The returned ordinary value can remain in REPL history after
the file closes.

| Operation | Behavior |
| --- | --- |
| `FileMode::Read` | Opens an existing regular file for reading. |
| `FileMode::Write` | Opens or creates a regular file, then truncates the validated handle. |
| `FileMode::CreateNew` | Exclusively creates a new regular file; an existing name is an error. |
| `file.read(max_bytes:)` | Reads bounded raw `Bytes` from the current cursor. |
| `file.read_text(max_bytes:)` | Reads bounded UTF-8 text, preserving a BOM and rejecting malformed input. |
| `file.write(bytes)` / `file.write_text(text)` | Writes the complete value and returns `Result<Unit, FileSystemError>`. |

Negative limits and empty/NUL-containing paths fail. A read observes at most
one byte beyond its allowance before reporting `TooLarge`; that observation
advances the file cursor. Read and Write follow ambient symlinks, while
CreateNew rejects an existing symlink. Acquisition rejects nonregular handles
without waiting for a FIFO peer and validates the opened handle before Write
can truncate it.

The acquired handle closes on success, propagated errors and supported runtime
unwinding. A body error takes precedence over a close error. A close error
replaces successful completion. Writes and recursive directory creation do not
promise atomic replacement, rollback or durability.

`Dir.children(path, max_entries:)` returns sorted immediate `DirEntry<Path>`
values. Entries have `name`, `path` and `kind` fields. Kinds are `File`,
`Directory` and `Other`; symlinks are `Other`. Child paths preserve the parent's
spelling, including `.` components. Names must be valid UTF-8; invalid names produce `UnsupportedName` without
returning a partial list. Enumeration is bounded and an excess yields `TooLarge`.

When several directory errors coexist, the current pinned paths have different
ordering: compiled programs validate names and metadata per bounded batch, while
the REPL first checks the complete bounded name set. Native preserves those paths;
[a common error precedence remains a specification proposal](https://github.com/type-rb/type-rb-native/issues/1287).

`Dir.create_all(path)` creates missing ancestors and succeeds when the directory
already exists. Errors can leave already-created ancestors in place. These
static operations use ambient paths. Opened/anchored directory capabilities and
cooperative locking remain separate coverage work.

The backend uses the platform C runtime for descriptor and directory operations;
it adds no separate filesystem library. See the [dependency register](https://type-rb.github.io/type-rb-native/dependencies/).

## Validation boundary

`tools/native-filesystem-abi-test.py` checks constants, layouts and the actual
header-selected `fstat`, `stat`, `lstat` and `readdir` symbols on the four
Darwin/Linux release host combinations. Header qualification is separate from
executing `tools/native-filesystem-test.py`, which exercises compiled programs
and retained REPL sessions. The ordinary CLI CI matrix runs those semantic checks
on Darwin arm64 and Linux arm64; this does not establish an ordinary Darwin amd64
bootstrap or complete four-host release qualification.

`tools/native-filesystem-check-test.py` checks escaping-resource and unverified
call rejection. It requires scoped-resource diagnostics for direct and transitive
calls, including `concurrent_map` reached through ordinary helpers, parameter
defaults and record initialization. Rejection of unsupported syntax cannot
substitute for the resource-effect check.
