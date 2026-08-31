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
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read_config(path: String): Result<String, FileSystemError>
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
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def create_output(path: String, value: String): Result<Unit, FileSystemError>
	return File.open(path, mode: FileMode::CreateNew) do |file|
		try file.write_text(value)
	end
end
```

Source cannot construct `File`. Inside its declaring block the value may only
be the direct receiver of the file methods; it cannot be aliased, passed,
stored, returned, or captured by a nested callback. The compiler identifies
this exact declaration rather than every unrelated type named `File`. This
prevents a target handle from escaping the cleanup scope without introducing a
general public lifetime syntax. The same identity may not appear recursively in
authored value-type positions such as parameters, returns, fields, collections,
function types, or transparent aliases. Compiler-generated and external
declaration contracts cannot mint it either. `File` remains available as the
class owner in `File.open`; the trusted block parameter is its only value
origin.

Opaque Ruby-native fallback syntax is rejected while the resource is in scope.
The compiler cannot prove whether raw native text stores or retains the handle;
native inputs must therefore be computed before `File.open`, and native work on
ordinary returned values must happen after its block finishes.

The compiler represents `open` as a structured
`Result<T, FileSystemError>` block. A prefix `try` inside it returns an error
from the structured operation, the backend closes the file, and only then may
the outer `Result` boundary propagate the error. A body error wins over a
simultaneous close error. A close error replaces a successful body result.

`file.read(max_bytes:)` returns `Bytes`; `file.read_text(max_bytes:)` applies
the same UTF-8 replacement policy as `Bytes#to_s`: one U+FFFD for each maximal
subpart of an ill-formed sequence. Thus adjacent stray continuation bytes are
replaced separately, while a truncated multibyte prefix is replaced once.
Both return `InvalidLimit` for a negative bound and `TooLarge` after observing
one byte beyond the bound. `file.write` accepts `Bytes`, while
`file.write_text` accepts `String`. Writes do not imply locking, flushing, or
durable storage.

`trb/std/dir` is separate because directory enumeration is neither required by
every file operation nor part of a file handle. `Dir.children` returns sorted
immediate `DirEntry` records with `File`, `Directory`, or `Other`. Symbolic
links and non-regular, non-directory entries are `Other`; callers must opt into
any traversal policy. Entry names cross the TypeRB `String` boundary only when
their host representation is lossless valid UTF-8. If any name cannot be
represented, the whole operation returns `FileSystemErrorKind::Other` with
operation `children`, the supplied directory as `path`, and the message
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

The initial filesystem boundary accepts host-native path strings. It does not
normalize separators, resolve `..` before the host does, promise containment
across symbolic links, or provide a general `Path` value. The `Path` name
remains available for a future nominal host-path value after TypeRB can attach
methods to non-class value declarations.

`FileSystemError`, `FileSystemErrorKind`, `DirEntry`, `DirEntryKind`, and
`FileMode` are peer declarations. Class-owned nested declarations are deferred
rather than approximated with a module or declaration merging.

## Deferred scope

This decision does not add watchers, permissions, arbitrary open-flag
combinations, locking, seeking, general streaming or incremental text decoding,
temporary-file management, `fsync`, atomic replacement, recursive walking, or
general path parsing or joining.

## Consequences

- Applications can implement a complete bounded file flow directly in TypeRB.
- Go, Ruby, TypeScript, and the typed-IR REPL share the same filesystem result,
  entry classification, exclusive-create, UTF-8, and cleanup semantics.
- The aggregate `trb/std/filesystem` API, its unbounded one-shot reads, and the
  slash-only static `Path` package are removed rather than retained as
  compatibility aliases.
