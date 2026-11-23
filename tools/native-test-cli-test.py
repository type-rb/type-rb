#!/usr/bin/env python3
"""Exercise the ordinary Native test CLI, selection and generated process boundary."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--report", type=Path)
    parser.add_argument("--reference", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    env = dict(os.environ, NO_COLOR="1", TERM="dumb")
    env.setdefault("TRBN_QBE", str(binary.with_name("qbe")))
    rows = []
    with tempfile.TemporaryDirectory(prefix="native-test-runner-") as directory:
        root = Path(directory)
        (root / "src/nested").mkdir(parents=True)
        (root / "trbconfig.jsonc").write_text(json.dumps({
            "name": "runner", "mode": "trb", "sourceDir": "src", "outDir": "build",
        }))
        (root / "src/main.trb").write_text('def main()\nputs("application main must not run")\nend\n')
        header = 'import { describe, test } from trb/std/test\n'
        for path, name, text in (("a_test.trb", "one", "first"),
                                 ("nested/b_test.trb", "two", "second")):
            (root / "src" / path).write_text(
                header + f'describe("Cases") do\n test("{name}") do\n'
                f'  puts("{text}")\n end\nend\n')

        def run(name, arguments, code=0, stdout=None, contains=None, cwd=None):
            result = subprocess.run([str(binary), *arguments], cwd=cwd or root,
                                    env=env, capture_output=True, text=True, timeout=120)
            row = {"name": name, "arguments": arguments, "code": result.returncode,
                   "stdout": result.stdout.replace(str(root), "<project>"),
                   "stderr": result.stderr.replace(str(root), "<project>")}
            rows.append(row)
            assert result.returncode == code, row
            if stdout is not None:
                assert result.stdout == stdout, row
            if contains is not None:
                assert contains in result.stdout + result.stderr, row
            return result

        first = "first\nPASS Cases / one\n\n1 test(s), 0 failure(s)\n"
        second = "second\nPASS Cases / two\n\n1 test(s), 0 failure(s)\n"
        both = "first\nPASS Cases / one\nsecond\nPASS Cases / two\n\n2 test(s), 0 failure(s)\n"
        try:
            run("all", ["test"], stdout=both)
            run("file", ["test", "src/a_test.trb"], stdout=first)
            run("directory", ["test", "src/nested"], stdout=second)
            run("deduplicated", ["test", "src", "src/a_test.trb"], stdout=both)
            run("nested-cwd", ["test", "b_test.trb"], stdout=second, cwd=root / "src/nested")
            run("explicit-config", ["test", "--config", "../../trbconfig.jsonc", "../a_test.trb"],
                stdout=first, cwd=root / "src/nested")
            result = run("json", ["test", "--reporter", "json", "src/a_test.trb"])
            events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
            assert [event["type"] for event in events] == ["test_started", "test_passed", "test_summary"]
            assert events[-1] == {"failed": 0, "total": 1, "type": "test_summary"}
            for flag in ("-t", "--test-name-pattern"):
                run("deferred-" + flag, ["test", flag, "Cases"], 1,
                    contains="test name selection is deferred")
            run("missing", ["test", "src/missing_test.trb"], 1,
                contains="test path src/missing_test.trb")
            (root / "src/nested/b_test.trb").write_text("not valid source !!!")
            run("unselected-malformed", ["test", "src/a_test.trb"], stdout=first)
            (root / "src/nested/b_test.trb").unlink()
            (root / "src/a_test.trb").write_text(
                header + 'describe("Cases") do\n test("failure") do\n'
                '  divisor := 0\n  puts(1 / divisor)\n end\n'
                ' test("continued") do\n  puts("continued")\n end\nend\n')
            run("failure-continues", ["test"], 1, contains="2 test(s), 1 failure(s)")
            run("compile", ["test", "--compile", "--outfile", "tests-bin"])
            result = subprocess.run([str(root / "tests-bin")], cwd=root, env=env,
                                    capture_output=True, text=True, timeout=20)
            rows.append({"name": "standalone", "code": result.returncode,
                         "stdout": result.stdout.replace(str(root), "<project>"),
                         "stderr": result.stderr.replace(str(root), "<project>")})
            assert result.returncode == 1 and "PASS Cases / continued" in result.stdout, rows[-1]
            assert not list(root.glob("tests-bin.build-*")), "temporary build directory leaked"
            previous = (root / "tests-bin").read_bytes()
            (root / "src/a_test.trb").write_text("not valid source !!!")
            run("failed-build", ["test", "--compile", "--outfile", "tests-bin"], 1)
            assert (root / "tests-bin").read_bytes() == previous, "failed compile replaced output"
            (root / "src/a_test.trb").write_text("# An existing test file without declarations.\n")
            run("empty-file", ["test"], 1, contains="test files must declare at least one top-level describe() suite")
            run("empty-json", ["test", "--reporter", "json"], 1,
                contains="test files must declare at least one top-level describe() suite")
            (root / "src/a_test.trb").write_text(header + 'describe("Empty") do\nend\n')
            run("empty-suite", ["test"], 1, contains="describe() must contain at least one test() or nested describe()")
            (root / "src/a_test.trb").unlink()
            run("no-tests", ["test"], 1, contains="no *_test.trb files found")
            run("no-tests-before-selection", ["test", "src/missing_test.trb"], 1,
                contains="no *_test.trb files found")
            for name, value, index in (("array", "[1]", 2), ("string", '"one"', 4)):
                (root / "src/main.trb").write_text(
                    f"def main()\n value := {value}\n puts(value[{index}])\nend\n")
                result = run("terminal-bounds-" + name, ["run"], 2, stdout="")
                assert result.stderr == "panic: index is out of bounds\n", rows[-1]
            if args.reference:
                # Replay empty-file and suite rejection at the exact reference pin.
                # The Native-only invocation still needs no reference compiler.
                config = json.loads((root / "trbconfig.jsonc").read_text())
                config["mode"] = "go"
                config["go"] = {"module": "example.com/runner"}
                (root / "trbconfig.jsonc").write_text(json.dumps(config))
                reference = args.reference.resolve()
                for name, source, arguments, message in (
                    ("empty-file", "# Empty.\n", ["test"],
                     "test files must declare at least one top-level describe() suite"),
                    ("empty-json", "# Empty.\n", ["test", "--reporter", "json"],
                     "test files must declare at least one top-level describe() suite"),
                    ("empty-suite", header + 'describe("Empty") do\nend\n', ["test"],
                     "describe() must contain at least one test() or nested describe()"),
                    ("no-tests-before-selection", None, ["test", "src/missing_test.trb"],
                     "no *_test.trb files found"),
                ):
                    path = root / "src/a_test.trb"
                    if source is None:
                        path.unlink()
                    else:
                        path.write_text(source)
                    result = subprocess.run([str(reference), *arguments], cwd=root, env=env,
                                            capture_output=True, text=True, timeout=120)
                    rows.append({"name": "reference-" + name, "code": result.returncode,
                                 "stdout": result.stdout.replace(str(root), "<project>"),
                                 "stderr": result.stderr.replace(str(root), "<project>")})
                    assert result.returncode == 1 and result.stdout == "", rows[-1]
                    assert message in result.stderr, rows[-1]
        finally:
            if args.report:
                args.report.write_text(json.dumps(rows, ensure_ascii=False, indent=2) + "\n")
    print(f"PASS {len(rows)} test CLI controls")


if __name__ == "__main__":
    main()
