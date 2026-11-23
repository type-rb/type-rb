#!/usr/bin/env python3
"""Build the exact reference identity independently of local Git tag metadata."""

import argparse
from pathlib import Path
import re
import subprocess
import sys

from compatibility_manifest import load_json_strict, validate_manifest_data, ValidationError


def build(reference, output, identity, trimpath=False):
    revision = subprocess.check_output(
        ["git", "-C", str(reference), "rev-parse", "HEAD"], text=True).strip()
    if revision != identity["revision"]:
        raise ValidationError("reference checkout differs from TYPE_RB_REVISION")
    if subprocess.check_output(
            ["git", "-C", str(reference), "status", "--porcelain"], text=True):
        raise ValidationError("reference checkout has uncommitted changes")
    versions = re.findall(r'^var Version = "([^"]+)"$',
                          (reference / "internal/cli/cli.go").read_text(), re.MULTILINE)
    version = identity["version"]
    if versions not in ([version], [version + "-dev"]):
        raise ValidationError("reference source version differs from compatibility metadata")
    command = ["go", "build", "-C", str(reference)]
    if trimpath:
        command.append("-trimpath")
    command += ["-ldflags", f"-X github.com/type-rb/type-rb/internal/cli.Version={version}",
                "-o", str(output), "./cmd/trb"]
    subprocess.run(command, check=True)
    reported = subprocess.run([str(output), "version"], capture_output=True, text=True, check=True)
    if reported.stdout != version + "\n" or reported.stderr:
        raise ValidationError("built reference executable reports a different version")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("reference", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--trimpath", action="store_true")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    try:
        manifest = load_json_strict(root / "compatibility/current.json")
        schema = load_json_strict(root / "compatibility/schema-v3.json")
        validate_manifest_data(root, manifest, schema)
        build(args.reference.resolve(), args.output.resolve(), manifest["typeRB"], args.trimpath)
    except (OSError, subprocess.CalledProcessError, ValidationError) as error:
        print(f"build-reference: {error}", file=sys.stderr)
        return 1
    print(f"build-reference: TypeRB {manifest['typeRB']['version']}@{manifest['typeRB']['revision']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
