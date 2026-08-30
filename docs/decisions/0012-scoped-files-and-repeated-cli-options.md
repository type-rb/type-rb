# 0012: Scoped files and repeated CLI options

## Context

A file-oriented command-line application needs to enumerate immediate inputs,
read with an explicit memory bound, create output without clobbering a parallel
writer, and report an application failure without an application-owned host
language wrapper. The existing filesystem package provided useful one-shot
operations, but a one-shot write cannot express exclusive create-new and an
unbounded read is unsuitable when input size is controlled externally.

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
- [Node.js `util.parseArgs`](https://nodejs.org/api/util.html#utilparseargsconfig)
  uses an explicit multiple-value option to collect repeated occurrences.

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

`trb/std/filesystem` provides the canonical structured operation:

```trb
return FileSystem.open(path, mode: FileSystem::OpenMode::Read) do |file|
	try file.read_text(max_bytes: 1048576)
end
```

`OpenMode` has exactly `Read`, `Write`, and `CreateNew` in this slice. `Write`
creates or truncates. `CreateNew` is one exclusive no-clobber operation and
returns `ErrorKind::AlreadyExists` for an existing path. It is not atomic
replacement and makes no durability promise.

The block parameter has opaque type `FileSystem::File`. Source cannot construct
it. Inside its declaring block it may only be the direct receiver of the
package's file methods; it cannot be aliased, passed, stored, returned, or
captured by a nested callback. This prevents the target handle from escaping
the cleanup scope without introducing a general public lifetime syntax.

The compiler represents `open` as a structured `Result<T,
FileSystem::Error>` block. A prefix `try` inside it returns an error from the
structured operation, the backend closes the file, and only then may the outer
`Result` boundary propagate the error. A body error wins over a simultaneous
close error. A close error replaces a successful body result.

Reads require `max_bytes`. They return `InvalidLimit` for a negative bound and
`TooLarge` after observing one byte beyond the bound. Text uses the standard
UTF-8 replacement policy. Writes do not imply locking, flushing, or durable
storage.

`FileSystem.entries` returns sorted immediate `DirectoryEntry` records with
`File`, `Directory`, or `Other`. Symbolic links and non-regular,
non-directory entries are `Other`; callers must opt into any traversal policy.

`trb/cli` accepts an option field whose type is `Array<String>`,
`Array<Integer>`, `Array<Float>`, or `Array<Boolean>`. Every occurrence appends
one value in command-line order. Arrays are deliberately not accepted as
positionals in this slice, so their termination rule remains unambiguous. The
existing explicit root subcommand remains required; repeated options do not
introduce a default root command.

`trb/cli.fail(message)` unwinds scoped cleanup, writes one application
diagnostic to standard error, and exits with status 1. Parser failures retain
status 2. It is a terminal single-binary application operation, not a portable
process-termination API.

## Deferred scope

This decision does not add watchers, permissions, arbitrary open-flag
combinations, locking, seeking, general streaming or incremental text decoding,
temporary-file management, `fsync`, atomic replacement, recursive walking,
variadic positionals, shell completion, nested subcommands, default root
commands, or Ruby and TypeScript CLI launcher generation.

## Consequences

- Applications can implement the complete bounded file flow in TypeRB while
  the Go build still produces one executable.
- Go, Ruby, TypeScript, and the typed-IR REPL share the same filesystem result,
  entry classification, exclusive-create, and cleanup semantics.
- Existing one-shot filesystem operations and scalar CLI options remain
  source-compatible. `FileSystem::Error` gains a defaulted `kind` field.
- A release containing this change must publish the compiler and bundled
  `trb/cli` package together because the schema and generated parser changed in
  lockstep.
