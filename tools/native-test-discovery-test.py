#!/usr/bin/env python3
"""Run compiler-owned test discovery from runtime input through Native code."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PROBE = '''import { lex_module } from frontend/syntax/lexer
import { parse_module_with_tests } from frontend/syntax/parser
import { add_module, compiler_new, has_diagnostic } from state/state
import { load_integer, load_string } from support/storage
import { TestPathSelection, select_test_paths } from project/test_selection
import trb/std/process

def main()
    arguments := Process.argv()
    if arguments[0] == "selection"
        selection := TestPathSelection.new(authored: "src", absolute: "/project/src", directory: true, error: "")
        result := select_test_paths("/project/src", "src", ["/project/src/z_test.trb", "/project/src/a_test.trb"], [selection, selection])
        puts(result.error)
        result.paths.each do |path|
            puts(path)
        end
        return
    end
    mut state := compiler_new(16384)
    add_module(state, "sample_test", "/project/sample_test.trb")
    if lex_module(state, arguments[0], 0)
        parse_module_with_tests(state, 0, 0, true)
    end
    if has_diagnostic(state)
        puts(load_string(state.diagnostics.messages, 0))
        puts(load_integer(state.diagnostics.lines, 0))
        puts(load_integer(state.diagnostics.columns, 0))
        return
    end
    puts(state.parsed.tests.size())
    state.parsed.tests.each do |item|
        puts(item.full_name)
        puts(load_integer(state.source_tokens.token_lines, item.origin))
        puts(load_integer(state.source_tokens.token_columns, item.origin))
    end
end
'''


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    native = args.binary.resolve()
    env = dict(os.environ, NO_COLOR="1", TERM="dumb")
    if "TRBN_QBE" not in env:
        env["TRBN_QBE"] = str(native.with_name("qbe"))
    header = "import { describe, test } from trb/std/test\n"
    suite = 'describe("A") do\n  test("B") do\n    return\n  end\nend\n'
    cases = [
        (header + suite, "2\nA\n2\n1\nA / B\n3\n3\n"),
        (suite + header, "2\nA\n1\n1\nA / B\n2\n3\n"),
        (header + '  describe(label) do\nend\n',
         "describe() name must be a String literal\n2\n12\n"),
        (header + suite.replace('"A"', '"値😀"'),
         "2\n値😀\n2\n1\n値😀 / B\n3\n3\n"),
        (suite + "import { describe, test } from local/test\n",
         "describe() requires a named import from trb/std/test\n1\n1\n"),
        ("selection", "\n/project/src/a_test.trb\n/project/src/z_test.trb\n"),
    ]
    with tempfile.TemporaryDirectory(prefix="native-test-discovery-") as directory:
        tmp = Path(directory)
        for path in (ROOT / "compiler/src").iterdir():
            if path.is_dir():
                (tmp / path.name).symlink_to(path, target_is_directory=True)
        source = tmp / "probe.trb"
        source.write_text(PROBE)
        executable = tmp / "probe"
        built = subprocess.run(
            [str(native), "build", "--compile", "--outfile", str(executable), str(source)],
            env=env, text=True, capture_output=True, timeout=300,
        )
        assert built.returncode == 0, (built.returncode, built.stdout, built.stderr)
        for source_text, expected in cases:
            result = subprocess.run([str(executable), source_text], env=env,
                                    text=True, capture_output=True, timeout=30)
            assert (result.returncode, result.stdout, result.stderr) == (0, expected, ""), (
                source_text, result.returncode, result.stdout, result.stderr, expected,
            )
    print(f"PASS {len(cases)} Native discovery and selection probes")


if __name__ == "__main__":
    main()
