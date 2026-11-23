# Ordinary compiler project layout

`compiler/src/` is the canonical ordinary compiler source closure.
`compiler/conformance/` contains its authored behavior and diagnostic cases;
`compiler/trbconfig.jsonc` is the reference-side project configuration.
The [architecture](architecture.md) and [directory map](compiler-source-layout.md)
record current responsibilities and enforced dependency rules.
There is one implementation tree, with no old-path copy, symlink or forwarding layer.
`lexer.trb` owns tokenization, String interpolation scanning and source slicing;
the compiler and CLI import those declarations directly.

Ordinary builds start at the canonical entry and follow explicit imports.
Snapshot recovery validates that closure and derives a temporary flattened input;
it does not replace normal source loading. `recovery/src/compiler/layout.trb`
records every canonical module and exact import prefix. Recovery reading, flattening
and staging consume that inventory; module mutation controls cover its dependencies. Source moves update imports, recovery,
CLI staging, tests, CI routing and operational consumers together.

CLI and snapshot staging use `tools/compiler_sources.py` to preserve every
production module's relative path below its source root. Core and CLI trees
share one derived import root without flattening basenames. Test sources,
`testing/` helpers and `tests/` suites are excluded from both staging and source
content keys. Duplicate output paths or symlinks fail before copying files.
The CLI entry composes `compiler/cli/repl/session.trb`; the other REPL modules
retain their paths under `repl/` in the composed source tree.
The staging helper is part of the CLI content key, so changing it invalidates
the CLI cache while preserving an unchanged verified core.

Recovery inventories retain complete nested module identities and exact import
headers. Their traversal admits source cycles and rejects missing or escaping
module paths. Writers create the required subdirectories only after validating
all names. CI discovery and recovery selection retain the same identities;
source moves require the complete validation lanes.

Cross-revision measurement controllers use `tools/compiler-project.sh` from the
controller checkout and resolve each source root independently. The helper accepts
the current or authenticated historical project layout, rejects ambiguous/missing
projects, and runs before timing. It is not a compiler subprocess.

The helper also selects `recovery/trbconfig.jsonc` for current recovery suites
and the root reference config for frozen historical checkouts. It rejects
ambiguous or incomplete recovery projects before execution.

The same helper resolves the configured-project corpus fixture independently
of the compiler layout. Bootstrap validation and its path-with-spaces copy use
that one directory from the source checkout. Current controllers therefore
retain frozen baseline fixtures after a corpus rename, rejecting missing,
incomplete or ambiguous layouts before generation and measurement.

Frozen baseline paths, negative layout/rename tests and immutable records remain
reproduction inputs. Their original layouts are available through the
[historical record](history.md); current source has no compatibility aliases.

The historical compiler-name seed skips the first five entry declarations when
creating its successor. Its exact accepted bridge source retains that ordering.
The cycle-capable checkout seed and current compiler use declaration identity;
source moves do not repoint or rewrite that historical transition.
