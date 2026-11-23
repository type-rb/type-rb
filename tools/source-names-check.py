#!/usr/bin/env python3
"""Keep retired experiment-stage names out of current source and documentation."""
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
IDENTIFIER = re.compile(r"(?<![A-Za-z0-9])_*(?:Gate[0-6]|GATE[0-6]_+|gate[0-6]|qbe[23]_|trbn_g[0-6])[A-Za-z0-9_]*\b")
PATH_NAME = re.compile(r"(?:gate[-_]?[0-6][a-z]*|gate-zero|qbe[23])(?:[_.-]|$)", re.I)
PROSE = re.compile(r"\bgate(?:[-_ ]?(?:[0-6][a-z]*|zero|specific|derived|numbered)|N)\b|\bqbe[23]\b", re.I)
# Source-era links identify immutable history; their labels are still current prose.
LINK_TARGET = re.compile(r"https?://[^\s)>]+|\]\([^)]+\)")


def retired_names(relative, content):
    path = Path(relative)
    if path.parts[0] == "results":
        return []
    errors = []
    if any(PATH_NAME.match(part) for part in path.parts):
        errors.append(f"{relative}: retired stage name in active path")
    for number, line in enumerate(content.splitlines(), 1):
        if path.suffix == ".trb":
            for match in IDENTIFIER.finditer(line):
                errors.append(f"{relative}:{number}: retired identifier {match.group()}")
            visible = line.split("#", 1)[1] if "#" in line else ""
        elif path.suffix == ".md":
            visible = LINK_TARGET.sub("", line)
        else:
            continue
        for match in PROSE.finditer(visible):
            errors.append(f"{relative}:{number}: retired stage description {match.group()}")
    return errors


def main():
    paths = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode().split("\0")
    errors = []
    for relative in paths:
        path = ROOT / relative
        if not relative or not path.is_file():
            continue
        content = path.read_text() if path.suffix in {".trb", ".md"} else ""
        errors.extend(retired_names(relative, content))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("Current source and documentation names passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
