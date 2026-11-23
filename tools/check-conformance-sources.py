#!/usr/bin/env python3
"""Quickly check every reviewed conformance source with an existing compiler.

This checks the source/diagnostic boundary before the slower recovery and
runtime checks. It never updates expectations or replaces those checks.
"""

import argparse
from pathlib import Path
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent / "compiler" / "conformance"


def cases():
    for group in ("valid", "runtime-invalid"):
        for path in sorted((ROOT / group).glob("*.trb")):
            yield path, "ok\n"
    for path in sorted((ROOT / "invalid").glob("*.source")):
        yield path, path.with_suffix(".diag").read_text()


def check(compiler, path, expected):
    try:
        result = subprocess.run(
            [str(compiler), "--source-content", "check", path.read_text()],
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        return str(error)
    if result.returncode != 0 or result.stderr or result.stdout != expected:
        return (f"status={result.returncode} stdout={result.stdout!r} "
                f"stderr={result.stderr!r} expected={expected!r}")
    return ""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("compiler", type=Path, nargs="+", help="existing Native compiler executable")
    args = parser.parse_args()
    reviewed = list(cases())
    failed = 0
    for compiler in args.compiler:
        for path, expected in reviewed:
            difference = check(compiler, path, expected)
            if difference:
                print(f"{compiler}: {path.relative_to(ROOT)}: {difference}", file=sys.stderr)
                failed += 1
    print(f"Checked {len(reviewed)} reviewed sources with {len(args.compiler)} compiler(s); {failed} mismatch(es)")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
