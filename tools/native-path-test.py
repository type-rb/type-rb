#!/usr/bin/env python3
"""Retain validated path values across allocation and forced collection."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument("binary", type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent
registry = json.loads((repository / "tools/native-language-cases.json").read_text())
identities = {"path-value-collections", "path-value-parent", "path-value-callback"}
cases = [case for case in registry["cases"] if case["id"] in identities]
assert {case["id"] for case in cases} == identities

with tempfile.TemporaryDirectory(prefix="native-path-values-") as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR="1", TERM="dumb")

    def run(command, *, statistics=False):
        env = dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS="1") if statistics else environment
        result = subprocess.run(list(map(str, command)), cwd=root, env=env,
                                text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    for case in cases:
        source = root / "main.trb"
        source.write_text(case["declarations"] + "\ndef main()\n" + case["body"] + "\nend\n")
        emitted = run([binary, "--internal-driver", "emit-qbe", source])
        assert not emitted.stderr, emitted.stderr
        forced, ordinary, hooks = [], False, 0
        for line in emitted.stdout.splitlines():
            if line.startswith("function "):
                ordinary = "$trbnf" in line
            if line == "}":
                ordinary = False
            if ordinary and any(marker in line for marker in (
                    "call $trbn_array_new", "call $trbn_array_push_",
                    "call $trbn_array_grow_paced(", "call $trbn_gc_alloc(",
                    "call $trbn_string_concat", "call $trbn_hash_", "call $trbnf")):
                forced.append("\tcall $trbn_gc_collect(w 0)")
                hooks += 1
            forced.append(line)
        assert hooks >= 10, (case["id"], hooks)
        il, assembly, program = [root / name for name in ("program.ssa", "program.s", "program")]
        il.write_text("\n".join(forced) + "\n")
        run([binary.with_name("qbe"), "-o", assembly, il])
        run(["/usr/bin/cc", assembly, "-lm", "-o", program])
        result = run([program], statistics=True)
        expected = case["reference"]["execute"]
        assert expected["code"] == 0 and not expected["stderr"]
        assert result.stdout == expected["stdout"], (case["id"], result.stdout, expected)
        statistics = {}
        for line in result.stderr.splitlines():
            prefix, key, value = line.split(",")
            assert prefix == "type-rb-native-gc-stat-v1" and key not in statistics, line
            statistics[key] = int(value)
        assert statistics["collections"] >= 10, (case["id"], statistics)
        assert statistics["live-bytes"] == 0, (case["id"], statistics)
        assert statistics["allocated-bytes"] == statistics["reclaimed-bytes"], (case["id"], statistics)

print("Native path values: forced-GC retention passed")
