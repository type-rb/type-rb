# 0012: Scoped files and directories

Status: accepted

## Context

A file-oriented application needs to enumerate immediate inputs, read with an
explicit memory bound, and create output without clobbering a parallel writer.
Unbounded one-shot reads are unsuitable when input size is controlled
externally, and a one-shot write does not provide a scoped place to express
exclusive create-new.

The target runtimes suggest compatible primitives but different ownership
styles:

- [Ruby `File.open`](https://ruby-doc.org/3.4/File.html) yields a file to a
  block and closes it when the block exits.
- [Go `os.OpenFile`](https://pkg.go.dev/os#OpenFile) combines explicit flags
  with `defer Close`; `O_CREATE | O_EXCL` requires a previously absent path.
  [`os.ReadDir`](https://pkg.go.dev/os#ReadDir) returns typed immediate entries.
- [Node.js `fs`](https://nodejs.org/api/fs.html) exposes `openSync`,
  `closeSync`, bounded descriptor reads, exclusive `wx`, and typed `Dirent`
  values; JavaScript code uses `try`/`finally` for ownership.

The common portable shape is therefore a checked lexical resource boundary,
not a public cross-runtime handle lifecycle.

## Options considered

Keeping only one-shot calls would preserve the smallest API, but it cannot
compose several bounded operations under one deterministic close and does not
give exclusive creation a natural typed mode.

Returning a public `File` and requiring callers to close it would resemble Go
most directly. It would also permit forgotten closes, use-after-close, handle
capture, and backend-specific lifetime differences. A convention or warning is
not strong enough for a standard package contract.

A compiler-owned structured block can retain Ruby's concise spelling while
making cleanup and `Result` propagation explicit in typed IR. The cost is a
narrow compiler contract for opaque scoped values. That contract is reusable
for future resources without adding general lifetime types to the language.

## Decision

File and directory operations use separate type roots. `File` is the actual
opaque resource type passed to the block; it is not a module wrapped around a
second file type:

<!-- trb-doc-test: adr-scoped-file-read -->
```trb
import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read_config(path: Path): Result<String, FileSystemError>
	return File.open(path) do |file|
		try file.read_text(max_bytes: 1048576)
	end
end
```

The peer `FileMode` enum has exactly `Read`, `Write`, and `CreateNew` in this
slice. Omitting `mode` selects `Read`. `Write` creates or truncates.
`CreateNew` is one exclusive no-clobber operation and returns
`FileSystemErrorKind::AlreadyExists` for an existing path. It is not atomic
replacement and makes no durability promise:

<!-- trb-doc-test: adr-exclusive-create-new -->
```trb
import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def create_output(path: Path, value: String): Result<Unit, FileSystemError>
	return File.open(path, mode: FileMode::CreateNew) do |file|
		try file.write_text(value)
	end
end
```

Source cannot construct `File` or `Dir`. The acquisition block owns cleanup;
required immutable `File` and `Dir` parameters borrow for the duration of a checked source call.
Immutable local aliases and nested synchronous blocks preserve that lifetime.
There is no separate borrow syntax and no implicit close in a helper. Resources
cannot escape through return, storage, `Any`, nullable/container/function types,
transparent aliases, first-class callbacks, or concurrent blocks. The rule uses
the exact declaration identity and leaves unrelated types named `File` or `Dir` alone.
External declarations cannot mint values or certify non-retention.

A shared typed-IR call-graph pass verifies operations reached while holding a
resource, including source helpers, generated/imported source, defaults and
constructors. Suspension, unknown/native edges and unclassified operations are
rejected independently of mode. Compiler-owned synchronous intrinsics are
explicitly admitted; provider effect flags are not proof. This is not a
deterministic validation profile, purity rule, or termination guarantee.

Opaque Ruby-native fallback syntax is rejected while the resource is in scope.
The compiler cannot prove whether raw native text stores or retains the handle;
native inputs must therefore be computed before `File.open`, and native work on
ordinary returned values must happen after its block finishes.

The compiler represents `open` as a structured
`Result<T, FileSystemError>` block. A prefix `try` inside it returns an error
from the structured operation, the backend closes the file, and only then may
the outer `Result` boundary propagate the error. A body error wins over a
simultaneous close error. A close error replaces a successful body result.

Acquisition validates the opened handle as a regular file before exposing it
or truncating it for `Write`. A prior `stat(path)` cannot establish this
property across a pathname replacement. The supported Linux/macOS adapters
use nonblocking acquisition and suppress controlling-terminal acquisition;
validation failure closes the handle and returns `open` / `Other` without
running the body. Unsupported hosts reject before opening. Ambient symlinks
are still followed for Read/Write; exclusive CreateNew rejects existing links.
This does not provide a device sandbox, filesystem latency bound, or directory
containment. In particular, a special filesystem can expose regular handles.
The [POSIX open contract](https://pubs.opengroup.org/onlinepubs/9799919799/functions/open.html)
distinguishes FIFO peer waiting from other device-specific behavior, and makes
`O_TRUNC` effects on some non-regular objects implementation-defined. Therefore
truncation is a separate operation on the successfully validated handle.

`file.read(max_bytes:)` returns `Bytes`; `file.read_text(max_bytes:)` applies
strict UTF-8 decoding, returning `InvalidEncoding` for an invalid sequence.
A leading BOM is preserved. Explicit `Bytes#to_s` remains the replacement
conversion when the application chooses that display policy.
Both return `InvalidLimit` for a negative bound and `TooLarge` after observing
one byte beyond the bound. `file.write` accepts `Bytes`, while
`file.write_text` accepts `String`. Writes do not imply locking, flushing, or
durable storage.

`trb/std/dir` is separate because directory enumeration is neither required by
every file operation nor part of a file handle. `Dir.children(path, max_entries:)` returns sorted
immediate `DirEntry<Path>` records with `File`, `Directory`, or `Other`. Symbolic
links and non-regular, non-directory entries are `Other`; callers must opt into
any traversal policy. Entry names cross the TypeRB `String` boundary only when
their host representation is lossless valid UTF-8. If any name cannot be
represented, the whole operation returns `FileSystemErrorKind::UnsupportedName` with
operation `children`, `Host(directory)` as the error target, and the message
`directory entry name is not valid UTF-8`; it never replacement-decodes a name
into a path for a different entry. Valid entries are sorted by their UTF-8
bytes.

Each entry has both its `name` and a host-native `path`. The backend preserves
the supplied parent path text and appends exactly that child name, inserting a
native separator only when the parent does not already end in an accepted host
separator. A Windows drive-relative parent such as `C:` remains drive-relative
as `C:child`, while a UNC share root receives a separator. This construction
deliberately does not use a lexically cleaning join: a parent containing
`symlink/../directory` keeps that resolution when the returned path is passed
to `File.open`. It also avoids the slash-only logical path rules of the former
`trb/std/path` package.

The filesystem boundary requires nominal `Path` values. It does not
normalize separators, resolve `..` before the host does, or promise containment
across symbolic links. The separate nominal
[path values](../standard-library.md#path-values) now provide exact host text
and validated logical descendants; callers pass `Path` directly to these I/O
operations. They do not add resource acquisition or containment guarantees.

`FileSystemError`, `FileSystemErrorKind`, `FileSystemTarget`, `DirEntry`, `DirEntryKind`, and
`FileMode` are peer declarations. Class-owned nested declarations are deferred
rather than approximated with a module or declaration merging.

Enumeration requires a nonnegative entry limit, rejects overflow with
`TooLarge`, and applies the bound during iteration. `Dir.create_all(Path)`
creates missing ancestors without rollback, preserving host resolution and
existing permissions. The shared error target is `Host(Path)`,
`Relative(RelativePath)`, or `Root`; ambient operations use `Host`.
`NotFound`, `PermissionDenied`, and `AlreadyExists` map corresponding native
errors. Empty/NUL paths are `InvalidPath`, not a blanket mapping of native
invalid-argument errors. See the [filesystem reference](../standard-library.md#filesystem)
for the complete bounds and error contract.

### Opened directory capabilities

`Dir` is also the opened resource returned to `Dir.open(Path)`'s block. There
is no parallel `RootDir` declaration or second acquisition factory. Pure Path
values name host paths; RelativePath values validate logical descendants; a
held Dir provides anchored authority. These are independent responsibilities.

Instance `children`, `create_all`, and `open_file` accept RelativePath values.
Root listing uses omitted/nil path, not an empty relative value. Entries retain
paths relative to the original anchor. Listing acquires its own child-directory
anchor so enumeration and metadata lookup cannot cross into a replacement
directory. Nonportable names fail the entire listing; the existing RelativePath
factory defines the grammar rather than a second native validator.

The guarantee is that pathname resolution performed by these APIs cannot leave
the opened anchor, including under concurrent name replacement and anchor
rename. Internal relative symlinks are allowed; absolute, escaping, and
unresolvable symlinks fail. Enumeration does not follow symlinks. Anchored Write
uses existing-file acquisition followed, on NotFound, by exclusive creation;
it cannot create a dangling symlink's target. Competing creation may fail with
AlreadyExists instead of retrying implicitly.

The native Go adapter uses [os.Root](https://pkg.go.dev/os#Root) on Linux/macOS.
Ruby and TypeScript currently reject the anchored operations at build time;
other runtime hosts reject acquisition. Go's JavaScript implementation is not
treated as a native secure adapter. The contract does not change by mode: a
backend either implements it or reports unsupported. Realpath checks followed
by pathname reopen and string-prefix checks are not acceptable fallbacks.

Mounts, bind mounts, hardlink provenance, devices, special filesystems, and
other ambient APIs are outside this pathname-resolution guarantee. Acquiring a
Dir is itself ambient, not a restricted execution policy. See the
[Go containment discussion](https://go.dev/blog/osroot) for the distinction
between traversal resistance and a general filesystem sandbox.

Errors from anchored operations retain Relative/Root targets. A File acquired
through Dir carries that origin across helper calls, rather than reconstructing
a Host target from its native handle name. Native error messages are sanitized
to prevent absolute-anchor disclosure. Dir.open's own acquisition and cleanup
errors retain its ambient Host target. Existing portable error classifications
are preserved; containment errors without a stable native category use Other.

Resource acquisition, handle lifetime, and anchored metadata access require
compiler integration because the extension protocol cannot express them.
Providers cannot register a weaker origin or certify non-retention. The narrow
bridge can move to an ordinary package only when that protocol can enforce
equivalent lifetime and authority guarantees.

## Deferred scope

This decision does not add watchers, permissions, arbitrary open-flag
combinations, locking, seeking, general streaming or incremental text decoding,
temporary-file management, `fsync`, atomic replacement, recursive walking, or
general path parsing or joining.

## Consequences

- Applications can implement a complete bounded file flow directly in TypeRB.
- Go, Ruby, TypeScript, and the typed-IR REPL share the ambient filesystem
  result, entry classification, exclusive-create, UTF-8, and cleanup semantics.
  Anchored operations currently have a secure Go native/Go-mode REPL adapter;
  unsupported backends reject rather than weakening their semantics.
- The aggregate `trb/std/filesystem` API, its unbounded one-shot reads, and the
  slash-only static `Path` package are removed rather than retained as
  compatibility aliases.
