#!/usr/bin/env python3
"""Synchronize recovery inventories with the canonical compiler closure.

The closure is every compiler module reachable through imports from
compiler/src/compiler.trb. Three hand-read inventories must match it:

- recovery/src/compiler/layout.trb: one row per module with its exact import
  header, in the recovery flattening order;
- recovery/src/compiler/mutations.trb: one observable source mutation per
  module except the entry, whose needle occurs exactly once; and
- the module list parsed by compiler/src/tests/compiler/source_loading_test.trb's own-frontend test.

--check reports drift. --write adds and removes rows, resynchronizes import
headers and generates a default mutation for a module without a usable one.
"""

import argparse
import json
import re
from pathlib import Path


LAYOUT = "recovery/src/compiler/layout.trb"
MUTATIONS = "recovery/src/compiler/mutations.trb"
FRONTEND_TEST = "compiler/src/tests/compiler/source_loading_test.trb"
FRONTEND_TEST_NAME = "parses the checked-in compiler closure through its own frontend"
STRING = r'"(?:\\.|[^"\\])*"'
LAYOUT_ROW = re.compile(
    r'(?m)^\t\tRecoveryCompilerModule\.new\(name: "([^"]+)", imports: (' + STRING + r')\),\n'
)
MUTATION_ROW = re.compile(r'(?m)^\t\t\[(' + STRING + r'), (' + STRING + r'), (' + STRING + r')\],\n')
IMPORT = re.compile(r"import (?:\{[^}]*\} from )?([^\s]+)(?: as [A-Za-z0-9_]+)?(?:\s*#.*)?")
MODULE = re.compile(r"[a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*")
SAFE_LITERAL = re.compile(r'"([A-Za-z0-9 _.,:;!?()<>=+*/-]{3,})"')
FIELD = re.compile(r"\t[a-z_][a-z0-9_]*: [^\n#]+")


class InventoryError(ValueError):
    pass


def canonical_imports(source: str) -> str:
    lines = source.splitlines(keepends=True)
    count = 0
    while count < len(lines) and lines[count].startswith("import "):
        count += 1
    if any(line.startswith("import ") for line in lines[count:]):
        raise InventoryError("imports must form the leading block of a compiler module")
    if count == 0:
        return ""
    if count < len(lines) and lines[count] == "\n":
        count += 1
    return "".join(lines[:count])


def closure(root: Path) -> list[str]:
    """Modules reachable from the compiler entry, in discovery order."""
    directory = root / "compiler/src"
    found: list[str] = []
    pending = ["compiler"]
    while pending:
        name = pending.pop(0)
        if name in found:
            continue
        if not MODULE.fullmatch(name):
            raise InventoryError(f"invalid canonical compiler module {name}")
        path = directory / f"{name}.trb"
        if path.is_symlink() or not path.resolve().is_relative_to(directory.resolve()):
            raise InventoryError(f"canonical compiler module escapes its source root: {name}")
        if not path.is_file():
            raise InventoryError(f"missing canonical compiler module {name}")
        found.append(name)
        try:
            header = canonical_imports(path.read_text())
        except InventoryError as error:
            raise InventoryError(f"{name}: {error}") from None
        for line in header.splitlines():
            if not line:
                continue
            match = IMPORT.fullmatch(line)
            target = match and match.group(1)
            if not target or not MODULE.fullmatch(target):
                raise InventoryError(f"{name}: invalid canonical import: {line}")
            if not target.startswith("trb/"):
                pending.append(target)
    return found


def encode(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def decode(literal: str) -> str:
    # Inventory strings use the JSON-compatible subset of TypeRB escapes.
    return json.loads(literal)


def default_mutation(name: str, source: str) -> list[str] | None:
    """A well-typed edit that must change the emitted compiler QBE."""
    body = source[len(canonical_imports(source)):]
    # Consume complete strings before considering a safe literal. Otherwise the
    # closing quote of one string and opening quote of the next can make code
    # such as `" + condition + "` look like a literal and produce invalid source.
    for match in re.finditer(STRING + r"|#[^\n]*", body):
        needle = match.group(0)
        if SAFE_LITERAL.fullmatch(needle) and source.count(needle) == 1:
            return [name, needle, needle[:-1] + '~"']
    lines = body.splitlines()
    for index in range(len(lines) - 1):
        first, second = lines[index], lines[index + 1]
        if FIELD.fullmatch(first) and FIELD.fullmatch(second) and first != second:
            needle = first + "\n" + second
            if source.count(needle) == 1:
                return [name, needle, second + "\n" + first]
    # Constructor-only modules may have neither text literals nor record fields.
    # Flip a unique Boolean cell, consuming strings/comments before candidates.
    for match in re.finditer(STRING + r"|#[^\n]*|\[(?:true|false)\]", body):
        needle = match.group(0)
        if needle in ("[true]", "[false]") and source.count(needle) == 1:
            return [name, needle, "[false]" if needle == "[true]" else "[true]"]
    # Optimization-only modules can contain no strings or records. A unique
    # integer comparison bound is still a typed, observable source mutation.
    # Consume strings/comments first and retain the operator in the needle.
    for match in re.finditer(STRING + r"|#[^\n]*|(?:>=|<=|==|!=|>|<) [0-9]+\b", body):
        needle = match.group(0)
        bound = re.fullmatch(r"(>=|<=|==|!=|>|<) ([0-9]+)", needle)
        if bound and int(bound[2]) < 9007199254740991 and source.count(needle) == 1:
            return [name, needle, f"{bound[1]} {int(bound[2]) + 1}"]
    # Bit-mask analyses may have no unique comparison literal. A positive
    # remainder divisor is a typed control mutation without division by zero.
    for match in re.finditer(STRING + r"|#[^\n]*|% [0-9]+\b", body):
        needle = match.group(0)
        divisor = re.fullmatch(r"% ([0-9]+)", needle)
        if divisor and 0 < int(divisor[1]) < 9007199254740991 and source.count(needle) == 1:
            return [name, needle, f"% {int(divisor[1]) + 1}"]
    # Use the complete binding to distinguish repeated Boolean values without
    # editing comments, strings, or unrelated initializers.
    binding = r"^\t*mut [a-z_][a-z0-9_]* := (true|false)\n"
    for match in re.finditer(STRING + r"|#[^\n]*|" + binding, body, re.MULTILINE):
        needle = match.group(0)
        boolean = re.fullmatch(binding, needle)
        if boolean and source.count(needle) == 1:
            opposite = "false" if boolean[1] == "true" else "true"
            return [name, needle, needle[:boolean.start(1)] + opposite + "\n"]
    # Named Integer literals need neither strings nor comparisons. Include the
    # line ending so a shorter value cannot also match a longer/Float return.
    for match in re.finditer(STRING + r"|#[^\n]*|\breturn [0-9]+\n", body):
        needle = match.group(0)
        literal = re.fullmatch(r"return ([0-9]+)\n", needle)
        if literal and int(literal[1]) < 9007199254740991 and source.count(needle) == 1:
            return [name, needle, f"return {int(literal[1]) + 1}\n"]
    # Several independent semantic domains can legitimately share every value.
    # Use the complete nullary definition to select just one literal owner.
    definition = r"^def [A-Za-z_][A-Za-z_0-9]*\(\): Integer\n[ \t]*return ([0-9]+)\nend\n"
    for match in re.finditer(STRING + r"|#[^\n]*|" + definition, body, re.MULTILINE):
        needle = match.group(0)
        literal = re.fullmatch(definition, needle)
        if literal and int(literal[1]) < 9007199254740991 and source.count(needle) == 1:
            replacement = needle[:literal.start(1)] + str(int(literal[1]) + 1) + needle[literal.end(1):]
            return [name, needle, replacement]
    # Sparse capacity helpers can contain only assignments and variable bounds.
    # Select a complete unique binding so the mutation cannot touch a comment,
    # string, fractional literal or an unrelated occurrence of the same factor.
    product = r"^\t*[a-z_][a-z0-9_]* := [a-z_][a-z0-9_]* \* ([0-9]+)\n"
    for match in re.finditer(STRING + r"|#[^\n]*|" + product, body, re.MULTILINE):
        needle = match.group(0)
        factor = re.fullmatch(product, needle)
        if factor and 0 < int(factor[1]) < 9007199254740991 and source.count(needle) == 1:
            replacement = needle[:factor.start(1)] + str(int(factor[1]) + 1) + "\n"
            return [name, needle, replacement]
    # Runtime emitters can consist entirely of IL string literals. As a final
    # fallback, alter one quoted emitted diagnostic without changing TypeRB
    # escape syntax or the established priority of ordinary source mutations.
    for match in re.finditer(STRING + r"|#[^\n]*", body):
        literal = match.group(0)
        if literal.startswith('"'):
            for emitted in re.finditer(r'\\"[A-Za-z0-9 _.,:;!?()-]{3,}\\"', literal):
                text = emitted.group(0)
                if source.count(text) == 1:
                    return [name, text, text[:-2] + '~\\"']
    return None


def sync_layout(root: Path, modules: list[str], sources: dict[str, str], write: bool) -> list[str]:
    path = root / LAYOUT
    original = path.read_text()
    rows = list(LAYOUT_ROW.finditer(original))
    if len(rows) != original.count("RecoveryCompilerModule.new(name:") or not rows:
        raise InventoryError("recovery layout contains an unrecognized module row")
    if any(left.end() != right.start() for left, right in zip(rows, rows[1:])):
        raise InventoryError("recovery layout rows must be contiguous")
    names = [row.group(1) for row in rows]
    if len(set(names)) != len(names):
        raise InventoryError("duplicate recovery module")
    changes = [f"layout: add {name}" for name in modules if name not in names]
    changes += [f"layout: remove {name}" for name in names if name not in modules]
    if "compiler" not in modules:
        raise InventoryError("compiler entry is missing from the closure")
    if "compiler" in names and names[-1] != "compiler":
        changes.append("layout: move compiler to final position")

    entries: dict[str, str] = {}
    for row in rows:
        name, encoded = row.group(1), row.group(2)
        if name not in modules:
            continue
        expected = encode(canonical_imports(sources[name]))
        if encoded != expected:
            changes.append(f"layout: imports {name}")
        entries[name] = row.group(0).replace(encoded, expected)
    for name in modules:
        if name not in entries:
            entries[name] = (
                f'\t\tRecoveryCompilerModule.new(name: "{name}", '
                f'imports: {encode(canonical_imports(sources[name]))}),\n'
            )
    # The matched-Go recovery driver replaces the compiler entry's final main().
    # Keep all newly discovered modules before it, including on later syncs.
    ordered = [name for name in names if name in modules and name != "compiler"]
    ordered += [name for name in modules if name not in names and name != "compiler"]
    ordered.append("compiler")
    updated = original[:rows[0].start()] + "".join(entries[name] for name in ordered) + original[rows[-1].end():]
    if write and updated != original:
        path.write_text(updated)
    return changes


def sync_mutations(root: Path, modules: list[str], sources: dict[str, str], write: bool, renames: dict[str, str]) -> list[str]:
    path = root / MUTATIONS
    original = path.read_text()
    rows = MUTATION_ROW.findall(original)
    if len(rows) != original.count('\t\t["'):
        raise InventoryError("recovery mutations contain an unrecognized row")
    changes: list[str] = []
    seen: set[str] = set()
    targets = [name for name in modules if name != "compiler"]
    kept: list[str] = []
    for match in MUTATION_ROW.finditer(original):
        previous, needle, replacement = (decode(literal) for literal in match.groups())
        name = renames.get(previous, previous)
        if name not in targets or name in seen:
            changes.append(f"mutations: remove {name}")
            continue
        seen.add(name)
        if needle == replacement or sources[name].count(needle) != 1:
            generated = default_mutation(name, sources[name])
            if generated is None:
                raise InventoryError(f"mutation for {name} no longer matches once; add one by hand")
            changes.append(f"mutations: regenerate {name}")
            kept.append("\t\t[" + ", ".join(encode(value) for value in generated) + "],\n")
            continue
        if name != previous:
            changes.append(f"mutations: relocate {previous} to {name}")
            kept.append("\t\t[" + ", ".join(encode(value) for value in (name, needle, replacement)) + "],\n")
        else:
            kept.append(match.group(0))
    for name in targets:
        if name not in seen:
            generated = default_mutation(name, sources[name])
            if generated is None:
                raise InventoryError(f"no default mutation for {name}; add one by hand")
            changes.append(f"mutations: add {name}")
            kept.append("\t\t[" + ", ".join(encode(value) for value in generated) + "],\n")
    first = MUTATION_ROW.search(original)
    closing = original.rindex("\t]\n")
    updated = original[:first.start()] + "".join(kept) + original[closing:] if first else original
    if write and updated != original:
        path.write_text(updated)
    return changes


def sync_frontend_test(root: Path, modules: list[str], write: bool) -> list[str]:
    path = root / FRONTEND_TEST
    original = path.read_text()
    test = original.find(FRONTEND_TEST_NAME)
    start = original.find("names := [", test)
    finish = original.find("]", start)
    if test < 0 or start < 0 or finish < 0:
        raise InventoryError("own-frontend module list is missing")
    names = [decode(value) for value in re.findall(STRING, original[start:finish])]
    if any(not MODULE.fullmatch(name) for name in names) or len(set(names)) != len(names):
        raise InventoryError("own-frontend module list has invalid or duplicated paths")
    if not names or names[0] != "compiler":
        raise InventoryError("own-frontend module list must start with the compiler entry")
    kept = [name for name in names if name in modules]
    kept += [name for name in modules if name not in kept]
    changes = [f"frontend test: add {name}" for name in modules if name not in names]
    changes += [f"frontend test: remove {name}" for name in names if name not in modules]
    updated = original[:start] + "names := [" + ", ".join(f'"{name}"' for name in kept) + original[finish:]
    if write and updated != original:
        path.write_text(updated)
    return changes


def synchronize(root: Path, write: bool, renames: dict[str, str] | None = None) -> list[str]:
    renames = {} if renames is None else renames
    if (not isinstance(renames, dict) or
            any(not isinstance(old, str) or not isinstance(new, str) or not MODULE.fullmatch(old) or not MODULE.fullmatch(new) for old, new in renames.items()) or
            len(set(renames.values())) != len(renames)):
        raise InventoryError("invalid or colliding module relocation map")
    modules = closure(root)
    sources = {name: (root / "compiler/src" / f"{name}.trb").read_text() for name in modules}
    return (sync_layout(root, modules, sources, write) +
            sync_mutations(root, modules, sources, write, renames) +
            sync_frontend_test(root, modules, write))


def relocation_map(manifest: dict, inventory: str) -> dict[str, str]:
    """Select only present historical identities before checking collisions."""
    modules = manifest["modules"]
    aliases = dict(modules)
    for previous, owner in manifest.get("previousModules", {}).items():
        if owner not in modules:
            raise InventoryError(f"previous module has no current owner: {previous}")
        aliases[previous] = modules[owner]
    present = {decode(match[1]) for match in MUTATION_ROW.finditer(inventory)}
    return {old: new for old, new in aliases.items() if old in present}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument("--check", action="store_true")
    action.add_argument("--write", action="store_true")
    parser.add_argument("--module-map", type=Path, help="reviewed migration manifest; preserve matching mutation needles across module moves")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    try:
        renames = relocation_map(json.loads(args.module_map.read_text()), (root / MUTATIONS).read_text()) if args.module_map else {}
        changes = synchronize(root, args.write, renames)
    except InventoryError as error:
        print(f"Recovery inventory error: {error}")
        return 1
    if changes:
        print("Recovery inventories differ from the compiler closure:")
        for change in changes:
            print(f"  {change}")
        if args.check:
            print("Run python3 tools/recovery_layout_sync.py --write")
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
