#!/usr/bin/env python3
"""Regenerate the vendored Ruby IDNA tables from fixed Unicode 17 inputs."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1] / "internal/oidchost"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="replace checked-in tables")
    parser.add_argument("--ruby", default="ruby")
    args = parser.parse_args()
    version = subprocess.check_output(
        [args.ruby, "-rrbconfig", "-e", 'print RbConfig::CONFIG["UNICODE_VERSION"]'],
        text=True,
    )
    if version != "17.0.0":
        raise SystemExit("table generation requires Ruby with Unicode 17.0.0")
    inputs = json.loads((ROOT / "unicode-inputs.json").read_text())
    with tempfile.TemporaryDirectory(prefix="trb-idna-tables-") as temporary:
        data = Path(temporary) / "data"
        output = Path(temporary) / "output"
        data.mkdir()
        output.mkdir()
        for name, pin in inputs.items():
            with urllib.request.urlopen(pin["url"], timeout=30) as response:
                contents = response.read(pin["bytes"] + 1)
            if len(contents) != pin["bytes"] or hashlib.sha256(contents).hexdigest() != pin["sha256"]:
                raise SystemExit("Unicode input identity mismatch: " + name)
            (data / name).write_bytes(contents)
        env = dict(os.environ, TRB_UNICODE_DATA=str(data), TRB_IDNA_OUTPUT=str(output))
        subprocess.run([args.ruby, str(ROOT / "ruby-generator/generate.rb")], env=env, check=True, timeout=120)
        expected = ROOT / "ruby/uri/idna/data"
        generated = sorted(output.glob("*.rb"))
        if {p.name for p in generated} != {p.name for p in expected.glob("*.rb")}:
            raise SystemExit("generated table inventory differs")
        for path in generated:
            destination = expected / path.name
            if args.write:
                destination.write_bytes(path.read_bytes())
            elif destination.read_bytes() != path.read_bytes():
                raise SystemExit("generated table differs: " + path.name)
        print(f"Verified {len(generated)} Unicode 17.0.0 tables from {len(inputs)} pinned inputs")


if __name__ == "__main__":
    main()
