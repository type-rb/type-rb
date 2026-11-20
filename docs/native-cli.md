# Native CLI and REPL

`trbn` is TypeRB Native's command-line executable. Its default mode is
`trb`, including a standalone file, a configuration without `mode`, and the
REPL. An explicit configuration mode is authoritative. `go`, `ruby`, and
`typescript` report an unsupported-mode error for execution; they never silently
run as native code. Checking, portable package installation and Web contract
generation accept every declared mode. The reference `trb` command and its
defaults are unchanged.

Compiled Web projects can call `Web.serve` using the pinned
[HTTP network libraries](native-network.md). Install those libraries once and
keep them in `network/` beside QBE, or set `TRBN_NETWORK_LIB_DIR` when building.
The compiled server contains the HTTP libraries and needs no runtime override.
The [gzip middleware](native-compression.md) additionally uses the pinned static
libdeflate installation in `compression/` beside QBE, or the build-time
`TRBN_COMPRESSION_LIB_DIR` override. The compiler itself needs no gzip library.
The [digest/HMAC provider](native-crypto.md) uses static OpenSSL `libcrypto`.
The CLI includes it for REPL parity; the core compiler and lockfile checksum
implementation remain independent. Applications acquire it only when they import
Digest, HMAC or SecureCompare.

[Time services](native-time-services.md) use system or configured timezone data.
`TZ` selects the local zone; `ZONEINFO` can provide a directory or stored ZIP
archive for named zones. Applications do not yet bundle an embedded timezone
database. Record the chosen data source when distributing an application.

## Build and run from a checkout

On Darwin arm64, install the Xcode command-line tools. On Linux arm64, install
a C toolchain, `make`, `curl`, `xz`, and `lld`. The contributor crypto source
build also needs Perl; development CI archives can provide it prebuilt. Then run:

```sh
python3 tools/build-crypto-dependencies.py --archives /path/to/shared-source-cache --output bin/crypto
./trbn --version
./trbn run
./trbn
```

The first invocation downloads a checksum-verified native bootstrap seed and
QBE 1.3, builds the compiler from TypeRB sources, and checks byte-identical
core and CLI rebuilds. It does not invoke Go. Subsequent invocations reuse
`bin/trbn` until the compiler sources or build inputs change. Build messages
go to stderr. Source hashes are computed in batches. Documentation-only
commits and timestamp-only changes reuse the binary. A CLI-only source change
reuses the cached, verified core and performs both CLI builds; core or toolchain
changes still perform the full four-core/two-CLI chain. Test sources are outside
this bootstrap input set. `build-info.txt` records the revision at which the
artifact was built, which can precede the current checkout revision when the
build inputs are unchanged. To build without launching a command:

```sh
tools/build-native.sh
```

The outputs are `bin/trbn`, its `bin/qbe` sidecar, and `bin/build-info.txt`.
Keep the executables and `crypto/` archives together when copying a development
compiler installation elsewhere. The CLI itself runs without the archives, but
compiling a crypto application needs them. The system C
driver/linker and system libraries remain runtime build dependencies.
The terminal editor is implemented in TypeRB and uses POSIX terminal I/O on
both systems; it has no libedit or Wasm dependency.
Checkout bootstrap currently supports Darwin arm64 and Linux arm64, even
though the internal compiler also has a Linux amd64 target profile.

The script pins the `bootstrap-seed-2026-09-29-module-cycles` release assets and their SHA-256
digests, and the QBE 1.3 archive digest. Cache files live in `.trb/bootstrap`;
downloaded seeds live under their release tag there. Updating the pin selects
a new cache entry without overwriting an older seed. The changed build script
also invalidates the verified core/CLI cache by content.
See [seed update policy](bootstrap-seed-updates.md) for compatibility-driven
refreshes and the separate pre-/post-publication verification boundary.
`TRBN_BOOTSTRAP_SEED` can select a local copy of that exact pinned seed.
`TRBN_QBE` and `TRBN_CC` select explicit executable paths. Overrides participate
in build invalidation. A lock serializes concurrent checkout builds; a killed
build may leave a lock that must be inspected before removing it.

## Commands and project configuration

```sh
./trbn install
./trbn install --frozen --offline
./trbn check examples/main.trb
./trbn build
./trbn build --stdout
./trbn build --compile
./trbn build --compile --outfile bin/example
./trbn run
./trbn test
./trbn test src/domain src/requests_test.trb
./trbn test --reporter json
./trbn test --compile --outfile bin/project-tests
./trbn /path/to/standalone.trb -- "program argument"
./trbn repl
./trbn web openapi --output docs/openapi.json
./trbn web client --name PublicClient --output generated/api_client.trb
```

`build` writes QBE source to the configured `outDir` (default `build`), named
`<project-name>.ssa`. `build --compile` writes `bin/<project-name>` by default.
For standalone files, paths are relative to the entry file's directory and
its basename supplies the project name. `--outfile` is for executable builds.
`run` builds in a temporary directory, forwards arguments after `--`, and
returns the program's exit status. Failed builds preserve an existing output.

Compiled programs and the retained REPL support `trb/std/process`. `argv`
returns a fresh array, `environment` distinguishes empty and absent values,
and `working_directory` reports host errors as a typed result. `run` starts a
literal executable with separate arguments, gives it EOF on stdin, and captures
stdout and stderr independently. Output uses `Bytes#to_s()` decoding. A child
that starts and exits with status 127 is an `Ok` result. Attempting to launch an
unsupported executable format returns a typed error. REPL interruption joins
its owned child before accepting the next submission.

`test` discovers `*_test.trb` files recursively under the configured `sourceDir`.
Positional files and directories select tests relative to the current directory;
overlapping selections run each file once. Production sources remain available
for imports, but unselected test bodies are not loaded. Tests execute in file
order and then declaration order, without invoking the application's `main`.
Selected test files must declare a top-level `describe` suite, and suites must
contain a `test` or nested suite. Missing files and empty suites are errors.

The human reporter prints each outcome and a summary. `--reporter json` emits
one JSON object per event: `test_started`, `test_passed`, `test_failed`, and
`test_summary`. Test-written output still goes to the same stdout stream.
Assertion failures retain their call location; supported runtime failures use
the test declaration's location. Later cases continue after a caught failure,
and the process exits with status 1 if any case failed. Fatal runtime failures
remain terminal. Tests share one compiled process and its module state.

`test --compile` writes a standalone test executable to
`bin/<project-name>.test` by default. `--outfile` chooses another location,
relative to the current directory. Compile mode cannot be combined with path
selection or the JSON reporter. Compilation checks the complete selected source
before replacing an existing output; temporary build files are removed after
execution or a failed build. Test compilation uses the same QBE, C driver and
package dependency installations as application compilation.

Name selection with `-t` or `--test-name-pattern` is an
[approved v0.5 deferral](https://github.com/type-rb/type-rb-native/issues/643).
Both flags report an error directing the caller to file or directory selection.
`test --debug` and test execution in source-backend modes remain unavailable.
`expect` provides deep equality, inequality and scalar predicates, and
`expect_ok`/`expect_err` return typed Result payloads. Failure messages use TypeRB
names and declaration-order fields, including nested collections, enums and
classes. Hashes have deterministic key order. Rendering does not call authored
formatters; cycles, depth, element counts and total visited values are bounded.
Strings use JSON control escaping and preserve other Unicode. The
[shared display contract](https://github.com/type-rb/type-rb/blob/main/docs/guides/testing.md)
also applies to the reference source backends. Full test-command parity is tracked
by the [v0.5 contract qualification](https://github.com/type-rb/type-rb-native/issues/1221);
[#624](https://github.com/type-rb/type-rb-native/issues/624) owns the initial runner slice.
Comparisons involving opaque provider values such as `StringBuilder` remain
unavailable; their Boolean/nil predicates can report the opaque value normally.
Boolean predicates preserve the current Go behavior for nullable values; the
cross-mode inconsistency is tracked in
[#1310](https://github.com/type-rb/type-rb-native/issues/1310).
Authored `Nil` type arguments also expose a separate reference-checking
inconsistency tracked in [#1297](https://github.com/type-rb/type-rb-native/issues/1297);
they are not included in passing portable Result coverage.

Configuration discovery searches from the source file/directory, or the
current directory when no source is given, upwards for `trbconfig.jsonc`.
A discovered configuration selects the project. `--config PATH` explicitly
selects one. `--mode` may select a standalone mode; it cannot override a
configured build. `repl --mode trb` can explicitly select a REPL mode.

### SQLite models

Native projects can declare a class derived from the official `Model` and
configure `packageOptions.trb/orm` with the SQLite adapter, database source and
committed schema lock. Checking and compilation read the lock offline. A
compiled application opens its configured SQLite database when it runs a model
operation.

A model with one primary key and Integer or String columns can use
`Report.create(title: "draft")`, `Report.find(id)` and `report.delete()`.
Use the actual column names from the lock. Omitting an optional column lets
SQLite apply its default. Passing `nil` to a nullable column stores SQL NULL.
A missing key returns `Ok(nil)`. Deleting a loaded model returns `Ok(true)`
when it removes a row and `Ok(false)` when the row has already been deleted.
Use `report.update(title: "ready")` to write immediately, or prepare changes
with `report.with(title: "ready")` and call `save()` later. Each update writes
only the supplied columns and returns the refreshed model. Saving changes
after the target row has been deleted returns an `InvalidData` error.
Models can also use `count()`. These operations use the same pinned SQLite
bundle as SQLite Jobs.

Use `Model.using(tx)` inside a transaction to keep operations on its
connection. A successful block commits its final value. A propagated database
error rolls back before the caller's `catch` runs.

```trb
def publish(title: String): DbResult<Integer>
    return Database.transaction() do |tx|
        reports := Report.using(tx)
        report := try reports.create(title: title)
        report.id
    end
end
```

A nested `tx.transaction() do |inner| ... end` uses a savepoint. Catching an
inner failure lets the outer transaction continue with its earlier changes.
Unscoped model calls use their own connection.

### SQLite Jobs

Use `trbn jobs start` from a Native project with `jobs.configuration` and a
SQLite `JOBS_ADAPTER`. `--config PATH` selects the project explicitly,
`--queue NAME` limits acquisition to one queue, and `--once` processes up to
one eligible job. The initial worker dispatches Integer, String, Float and
Boolean payload arguments to `perform` and records its outcome in SQLite.

The worker updates the active lease while the handler runs in a child process.
The first interrupt or termination request stops acquisition and gives the
handler 30 seconds to finish. A repeated request or grace expiry terminates
and reaps the active handler, preserving its lease for recovery. The CLI waits
for the worker to exit before removing its temporary build directory.

SQLite Jobs links the pinned static bundle beside QBE in `sqlite/` or from
`TRBN_SQLITE_LIB_DIR`. A development checkout can build the bundle with
`python3 tools/build-sqlite-dependencies.py --output bin/sqlite`.

Configurations follow the reference schema and validation, including `lint`,
`packageOptions`, `jobs`, `db` and the mode-specific sections. `check` accepts
every declared mode (`go`, `ruby`, `typescript` and `trb`). Commands that build,
run or evaluate a program need mode `trb` and report that other modes are not
available. `install` resolves portable TypeRB local-path and Git packages in
every declared mode. It writes reference-compatible `trb.lock` metadata and
uses a content-addressed `.trb/packages` cache. Git must be installed and on
`PATH`; TypeRB passes argument vectors to it without a shell. Git handles normal
SSH/HTTPS credentials. Password-bearing URLs, URL queries/fragments and
unsupported version ranges are rejected before acquisition. Host-language
dependencies and executable package providers remain separate capabilities.

Git requirements accept exact semantic-version tags, `latest` (the greatest
semantic-version tag), or a commit revision. Exact refs retain short versions
and build metadata, such as `v1.2` and `v1.2.3+build.9`; version precedence is
used only to select `latest`. The manifest identity determines
the canonical import name; configured aliases can differ. Dependencies retain
their own alias scopes. Conflicting canonical identities, dependency cycles,
tag/manifest version mismatches and unsupported package modes are rejected.
Successful acquisition removes Git metadata and verifies every regular file's
raw bytes and relative path. Symlinks are rejected. Temporary fetch trees are
removed on success or failure; cached trees are reused by checksum.

`check`, build, run and REPL read the current lock and verify cached contents;
they never fetch a missing dependency or repair a stale lock implicitly. Local
package source edits remain live, while local manifest edits require install.
Git cache edits fail checksum verification. `localPackages` also maps names
directly to source directories without a manifest or lock. Aliases take
precedence over an identically spelled project import.

`install --frozen` requires a current lock and preserves its bytes, modification
time and file identity. It may restore a missing Git cache from the locked
revision when online. `--offline` requires cached Git packages and rejects an
unlocked direct Git requirement; the flags can be combined. **Pinned reference
exception:** fresh resolution through a local-path requirement can still fetch
its transitive Git dependencies during `--offline`. This is retained for
compatibility and tracked by [proposal #1224](https://github.com/type-rb/type-rb-native/issues/1224).
Do not treat that fresh local/transitive path as an unconditional no-Git mode.
Current locked offline installations do not invoke Git. Empty projects need no
lock. Explicit Boolean flag values such as `--frozen=false` are accepted.

`add [--source GIT | --path DIRECTORY] PACKAGE [VERSION]` changes the configured
requirement and resolves the graph. `PACKAGE@VERSION` is also accepted; an
omitted Git version means `latest`. `remove PACKAGE` removes a direct requirement.
Both follow the reference's full-resolution behavior. `update` resolves all
requirements again, while `update PACKAGE...` requires a current lock and pins
the unselected dependency graphs. A selected graph that conflicts with a pinned
graph fails without replacing the lock. Each metadata file is published by
atomic replacement after successful resolution; configuration and lock are two
separate files, not a filesystem transaction. Resolution failures preserve both.

`web openapi` generates an OpenAPI 3.1 document from checked
[endpoint contracts](native-web-endpoints.md). It accepts `--config`, `--title`,
`--api-version` and `--output`. Title and API version default to the project name
and version. Output defaults to stdout; `--output -` also selects stdout.
A file output path must be relative to and remain below the project root.
Successful file output atomically replaces the destination and prints its path;
failed generation preserves an existing file. No handler, program entry point,
server or external target tool is executed. The pinned Web integration still
requires a project with a `main` entry for contract discovery.

`web client` generates TypeRB browser-client source from the same checked
contracts. It accepts `--config`, `--name` and `--output`; the class name defaults
to `ApiClient`. It has the same stdout, file publication, configuration discovery
and target-tool independence rules as `web openapi`. Shared imported contract
types must also be available to the consuming browser project. See the endpoint
guide for generated request/response behavior and pinned consumer limitations.
This metadata command does not provide general source-backend compilation or
execution of browser APIs in Native programs.

Nonempty native-dependency/adapter manifest fields and target-language
dependency-management options remain unimplemented and fail explicitly. Configuration JSON Unicode
escapes retain their current limitations; package manifests and locks decode
JSON Unicode escapes separately.

`trb/std/path` provides `Path` and validated `RelativePath` values. Validation,
composition, parent lookup and private construction follow the portable source.
`Path#join` preserves its parent text and appends the validated descendant using
the slash separator of the supported Native targets. These value operations do
not access the filesystem. Scoped `File.open`, bounded byte/text reads and writes,
`Dir.children` and `Dir.create_all` use these values in ordinary programs and the
REPL. See [path values](native-path-values.md) and
[file and directory operations](native-filesystem.md).

The checkout root contains a small `mode: trb` example project in `examples/`.
Reference-compiler development checks use the separate
`recovery/trbconfig.jsonc` explicitly:

```sh
trb check --config recovery/trbconfig.jsonc
trb test --config recovery/trbconfig.jsonc
tools/check-native-cli.sh /path/to/trb
```

There is no special directory exemption in Native configuration discovery.
The core and benchmark subprojects retain their explicit reference-validation
configurations; opening one with `trbn` honors its configured mode too.

## Source formatting

`trbn fmt [PATH ...]` formats `.trb` files, including test files. Directory
selections skip `.git`, `.trb` and `node_modules`; overlapping selections are
deduplicated. Without paths, the configured source directory is selected,
falling back to the current directory. `--check` reports changed paths on stderr
and exits unsuccessfully without writing. `-w=false` lists changed paths without
writing; `fmt -` formats stdin to stdout. Existing file permissions are preserved.

The formatter and interactive REPL share a TypeRB implementation. It parses
supported declarations and statements before printing lossless token spellings,
preserving comments, quoted text and escapes. It verifies the formatted syntax
and normalized token stream before accepting a result. It does not execute
input, resolve imported modules, or require expressions to type-check.
Unsupported or incomplete input is rejected before that file is written; in the
editor it remains unchanged for normal diagnostics. File indentation uses tabs;
the REPL uses two spaces.

Full reference formatting remains incomplete: project-aware import
canonicalization, native-language islands, percent/heredoc/JSX literals and
spellings not accepted by the Native parser still need qualification. In
particular, raw multiline quoted strings, exponent/base numeric spellings and
keyword Hash labels
remain explicit rejected cases in the formatter comparison. Reference formatting
success alone is not evidence of Native execution support.

## REPL

No arguments start a REPL when stdin and stdout are terminals. Noninteractive
no-argument invocations print usage; use `trbn repl` to read a piped session.

The REPL uses the ordinary compiler's parser, name resolution and type checker.
A TypeRB-authored evaluator retains bindings and aggregate identities between
submissions. It executes only the new input, so earlier I/O is not replayed.
Functions, records, imports, multiline control flow, scalar values and the
ordinary compiler's supported arrays can be explored interactively.

Successful nullable assignments retain the ordinary checker's flow facts across
inputs. `:type` uses those facts without evaluation; direct assignments display
their declared type and mutable marker, with Nil for a nil assignment. Static
rejection preserves existing facts. Runtime failure or interruption discards
earlier narrowing because completed writes may have changed retained values.
A fresh guard or successful assignment can establish narrowing again.

Explicit replay rebuilds accepted inputs with their recorded checking boundaries;
it does not replay failed inputs or their partial effects. See the
[checked submission decision](decisions/0043-checked-repl-submissions.md).
As in the reference REPL, authored `return` requires a function or method.

Unique project exports are available automatically. Authored named imports
(including aliases), unaliased bare imports and local declarations suppress the
corresponding generated imports. The ordinary resolver still rejects duplicate
authored bindings. A declaration that invalidates earlier session input is
rejected before evaluation and leaves the session usable. Only explicit load or
reload replays earlier effects.

Project/prelude construction is isolated in `compiler/cli/repl/project.trb`.
The filter uses parsed declarations and preserves hidden prelude line counts;
comments and strings do not become imports. Reconstructed bindings and type
labels use visible record aliases, including nested Array/Hash values, while
stored values retain their canonical record identity. Supported standard-package
aliases resolve through the checked import binding in the evaluator.

```text
mut values := [1, 2]
values.push(3)
values
:type values
```

`:type EXPRESSION` checks without evaluating. `:load FILE` adds declarations or
statements and replays the session,
`:reload` reloads the project and replays the session, `:help` shows help, and
`:quit` (`:q` / `:exit`) or Ctrl-D exits. Explicit `:load` and `:reload`
replay earlier side effects; ordinary submissions do not. Ctrl-C cancels input or evaluation.
An interactive terminal provides:

- Live syntax colors for keywords, strings, numbers, comments, types and calls,
  with unfinished strings marked separately. Prompts, type annotations and
  diagnostic severity have distinct roles. `NO_COLOR=1` disables colors;
  `TERM=dumb` uses plain line input without terminal editing sequences.
- A single editable multiline buffer, two-space indentation on Enter, and
  canonical spacing when the complete submission is accepted (for example,
  `s:=123` becomes `s := 123`). Up/Down move between
  buffer lines and then through history at the first/last line.
- Bracketed paste, retained as one submission until Enter. Pasted control
  sequences are not interpreted as editing commands.
- Tab completion of session/project names, record fields and supported methods,
  filtered at type positions. Tab/Shift-Tab cycle ambiguous candidates with a
  type/signature description; Enter accepts a selected candidate before a
  subsequent Enter submits the buffer.
- Ctrl-R reverse history search; another Ctrl-R finds an older match. Enter
  accepts the match for editing, and Ctrl-G restores the draft.
- Left/Right, Home/End, Delete/Backspace, Ctrl-A/E/B/F/P/N, Ctrl-K/U/W to kill
  text, Ctrl-L to clear the screen, Ctrl-Y to yank it, and Ctrl-_ to undo up to 64 edits.
- UTF-8 codepoint movement and deletion, combining-mark attachment, wide
  character cell widths, and a cursor-following viewport for long input.
  Resize starts a fresh display region so old terminal reflow cannot corrupt
  cursor placement. Terminal settings are restored before evaluation and exit.

Project history is `.trb/repl_history`; standalone
history is `~/.cache/trbn/repl_history_trb`. `TRBN_HISTORY` overrides that file.
History uses the reference REPL's JSON string-array format.

The CLI currently implements part of `trb`. Full command and backend parity,
stable CLI contracts and supported distribution are [project goals](mir-consolidation.md).
See the [ordinary coverage plan](native-language-coverage.md) for path-specific
tracking; snapshot/recovery capabilities do not establish ordinary CLI support.
The ordinary compiler's language and package restrictions still apply.
Ordinary UTF-8 literals, code-point size/indexing and concatenation work in compiled programs
and the REPL; invalid source encoding is rejected before token decoding.
Language-server commands and host-language dependency management remain gaps.
The editor does not yet provide every readline/vi binding, import-repair and argument-aware completion,
or full Unicode grapheme-cluster segmentation (for example joined emoji).
Completion follows the executable Native subset. Submissions are bounded to
64 KiB, including paste; overflow is rejected without evaluating a prefix.
The evaluator has a 256-call depth bound and retains reachable session values.
It is not a sandbox and is not yet qualified for production use.

## CI artifacts and validation

Run the **Native CLI validation** workflow with `workflow_dispatch`, selecting
the desired revision. The same checkout launcher builds Darwin arm64 and
Linux arm64, verifies fixed points, runs CLI and real-terminal tests, and
uploads `trbn-<platform>` artifacts containing a tarball and checksum. Each
archive includes `trbn`, QBE, build metadata, this guide, and the project
license. The `network` directory contains qualified libuv and llhttp
binaries, dependency notices and a build receipt; `HTTP-NETWORK.md` describes
their runtime configuration. Artifacts are development CI outputs, not supported
releases.

Run the same integration tests locally after building:

```sh
python3 tools/native-cli-test.py bin/trbn
python3 -m venv .trb/repl-tests
.trb/repl-tests/bin/pip install -r tools/repl-test-requirements.txt
.trb/repl-tests/bin/python tools/native-repl-editor-test.py bin/trbn
# Optional: run the shared screen scenarios against the reference REPL too.
.trb/repl-tests/bin/python tools/native-repl-editor-test.py bin/trbn --reference /path/to/trb
```

Compiler implementation stays in `compiler/src`; CLI, terminal integration
and evaluation live in `compiler/cli`. The bootstrap stages a shared import
root without changing those canonical source trees. Typed host declarations
are backed by a compiler-owned POSIX QBE template only at the CLI compiler
entry. This is internal bootstrap machinery, not a public FFI facility.

## Builtin output

Like the reference prelude, `puts` takes one value and writes its text followed
by a newline. The Native subset accepts String, Integer and Boolean in both
compiled programs and the REPL. Integer uses decimal digits; Boolean uses
`true` or `false`. Arguments are evaluated once. This builtin output conversion
does not invoke user-defined `to_s` methods or make ordinary String parameters
accept other types. Float, collection and record output remain explicitly
unsupported; their portable formatting requires separate coverage.

`python3 tools/native-puts-test.py bin/trbn --reference /path/to/trb` compares
compiled and REPL output with the reference compiler. After a normal checkout
build, `python3 tools/native-bootstrap-test.py` checks cache invalidation, core
reuse, failed-build preservation and concurrent callers in an isolated copy.
`tools/build-native.sh --plan` prints the scope a build would take (`cached`,
`cli` or `core`) from the same content keys without building; the test uses it
for edits, additions, removals and nested inputs, and performs two real
rebuilds: a replaced core, and a CLI rebuild shared by concurrent callers.

## Diagnostics

File checking, QBE emission, executable builds and execution report frontend
errors on stderr as `path:line[:column]: error[CODE]: message`, using the actual entry or
imported file. REPL diagnostics use `(trb)` and the cumulative authored line
number of accepted submissions; generated imports, the entry function and
stored bindings do not add visible lines. Failed submissions do not advance
that line count. REPL runtime failures use `error: message` at the invoking
statement, including division by zero.

This follows the pinned reference CLI's location/severity/code presentation.
Operator failures name the operator and operand types, for example
`operator + does not support Integer and String`. For directly authored
operators, the column identifies that operator token, including compact input,
tabs and Unicode identifiers. Interpolation is currently lowered to String
concatenation, so its type errors still describe that generated operation;
generic-template diagnostics may retain only a line origin.
The reference currently points to the start of the expression instead. Other
diagnostics include a column only when an actual token origin is available;
line-only diagnostics do not fabricate one. Structured colored REPL diagnostics emphasize
the severity word and reset before the code and message. Its `TRBN` diagnostic codes and detailed messages
remain distinct where its supported subset and diagnosis differ from the
reference frontend. It reports the first error rather than accumulating the
reference compiler's complete diagnostic set. Compiled-program runtime
failures and external tool diagnostics retain their existing runtime/tool
contracts. The internal core compiler protocol remains unchanged.

Run `python3 tools/native-diagnostics-test.py bin/trbn --reference /path/to/trb`
to compare file, imported-module and cumulative REPL origins against the pinned
reference executable. The artifact workflow runs the same Native assertions
on both supported checkout platforms.

## String interpolation

Double-quoted Strings evaluate `#{expression}` through the shared Native
frontend in both compiled programs and the REPL. Expressions must return
String; use an explicit supported conversion such as `value.to_s()` for an
Integer. Multiple expressions run once each, from left to right. Grouping
preserves expression precedence and postfix operations on the resulting String.
Nested quoted Strings and nested interpolation support UTF-8 literal text
within the existing expression subset.

Use `\#` to suppress interpolation: `"\#{name}"` produces the literal text
`#{name}`. With two backslashes, `"\\#{name}"` produces one backslash followed
by the value of `name`. Undefined escapes such as `\{` and `\q` are errors,
including in literal segments next to interpolation. Escaped markers retain
literal coloring in the REPL and round-trip unchanged through its history.

The REPL colors interpolation delimiters and embedded expressions separately
from literal text, including while input is incomplete. JSON history retains
interpolation as source text and never interprets it while loading.

`python3 tools/native-interpolation-test.py bin/trbn --reference /path/to/trb`
compares evaluation, side effects and type rejection with the pinned reference.
Empty interpolation markers remain literal as in that reference. Malformed
nonempty expressions and unterminated interpolation receive diagnostics; no
implicit conversion or unchecked evaluation is introduced.

The [key-reader interruption checks](native-cli-interruption.md) document the
nonblocking read boundary, exact flag restoration and deterministic regressions.
