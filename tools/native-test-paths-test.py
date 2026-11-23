#!/usr/bin/env python3
"""Exercise filename-only compiler discovery without opening test bodies."""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile


SOURCE = '''record CompilerProjectSources
    paths: Array<String>
    sources: Array<String>
    error: String
end

def compiler_collect_test_paths(_root: String, _out: String): CompilerProjectSources
    return CompilerProjectSources.new(paths: [], sources: [], error: "unadapted")
end

def compiler_main(source: String, mode: String, _file: Boolean): Integer
    files := compiler_collect_test_paths(source, mode)
    puts(files.error)
    puts(files.sources.size())
    files.paths.each do |path|
        puts(path)
    end
    return 0
end

def compiler_file_main(_source: String, _path: String, _mode: String, _file: Boolean): Integer
    return 1
end

def main()
end
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    env = dict(os.environ, NO_COLOR="1", TERM="dumb")
    env.setdefault("TRBN_QBE", str(binary.with_name("qbe")))
    with tempfile.TemporaryDirectory(prefix="native-test-paths-") as directory:
        temporary = Path(directory)
        source = temporary / "probe.trb"
        source.write_text(SOURCE)
        executable = temporary / "probe"
        build = subprocess.run(
            [str(binary), "build", "--compile", "--outfile", str(executable), str(source)],
            env=env, text=True, capture_output=True, timeout=300,
        )
        assert build.returncode == 0, (build.returncode, build.stdout, build.stderr)
        emitted = subprocess.run(
            [str(binary), "--internal-driver", "emit-qbe", str(source)],
            env=env, text=True, capture_output=True, timeout=300,
        )
        assert emitted.returncode == 0, emitted.stderr
        instrumented = emitted.stdout
        for function in ("trbn_collect_test_paths", "trbn_test_walk_callback"):
            pattern = r"^function [lw] \$" + function + r"\([^\n]*\) \{.*?^\}"
            match = re.search(pattern, instrumented, re.M | re.S)
            assert match, function
            instrumented = instrumented.replace(match[0], match[0].replace(
                "@start\n", "@start\n call $trbn_gc_collect(w 0)\n", 1))
        # The hidden compiler-input protocol normally exits without application
        # GC finalization. Close that test-only entry explicitly for accounting.
        finish = re.compile(r"(%hst =l call \$trbnf\d+\(l %hs, l %hm, l 0\)\n)(\tret 0\n@fargs)")
        instrumented, replacements = finish.subn(r"\1\tcall $trbn_gc_finish()\n\2", instrumented)
        assert replacements == 1, "compiler probe entry must be finalized exactly once"
        il = temporary / "forced.ssa"
        il.write_text(instrumented)
        assembly = temporary / "forced.s"
        forced_executable = temporary / "forced"
        subprocess.run([env["TRBN_QBE"], "-o", str(assembly), str(il)],
                       check=True, capture_output=True, timeout=60)
        subprocess.run(["cc", str(assembly), "-lm", "-o", str(forced_executable)],
                       check=True, capture_output=True, timeout=60)
        root = temporary / "project with spaces"
        root.mkdir()
        expected = []
        for name in ("main.trb", "bad_test.trb", "case_TEST.trb", "nested/値😀_test.trb"):
            path = root / name
            path.parent.mkdir(exist_ok=True)
            path.write_bytes(b"\xff not a valid program\n")
            expected.append(path)
        for index in range(270):
            path = root / f"growth_{index:03}_test.trb"
            path.write_text("not parsed")
            expected.append(path)
        unreadable = root / "bad_test.trb"
        unreadable.chmod(0)
        # A traversal which reads every file hangs here. A dangling link also
        # belongs to discovery; only reading a selected source can reject it.
        fifo = root / "pipe_test.trb"
        os.mkfifo(fifo)
        expected.append(fifo)
        dangling = root / "missing_test.trb"
        dangling.symlink_to(root / "does-not-exist")
        expected.append(dangling)
        linked = root / "linked_test.trb"
        linked.symlink_to(root / "main.trb")
        expected.append(linked)
        (root / "directory-link").symlink_to(root / "nested", target_is_directory=True)
        for name in ("case_test.TRB", "notes.txt", ".git/ignored_test.trb",
                     ".trb/ignored_test.trb", "node_modules/ignored_test.trb",
                     "nested/node_modules/ignored_test.trb", "out/ignored_test.trb"):
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("ignored")
        observations = 0
        try:
            for program in (executable, forced_executable):
                execution_env = dict(env)
                if program == forced_executable:
                    execution_env["TYPE_RB_NATIVE_RUNTIME_STATS"] = "1"
                result = subprocess.run(
                    [str(program), "--source-content", str(root / "out"), str(root)],
                    env=execution_env, text=True, capture_output=True, timeout=15,
                )
                wanted = "\n0\n" + "".join(str(path) + "\n" for path in sorted(expected))
                assert (result.returncode, result.stdout) == (0, wanted), (
                    program.name, result.returncode, result.stdout, result.stderr, wanted,
                )
                if program == forced_executable:
                    statistics = {line.split(",")[1]: int(line.split(",")[2])
                                  for line in result.stderr.splitlines()
                                  if line.startswith("type-rb-native-gc-stat-v1,")}
                    assert statistics["live-bytes"] == 0, statistics
                    assert statistics["allocated-bytes"] == statistics["reclaimed-bytes"], statistics
                else:
                    assert result.stderr == "", result.stderr
                observations += 1
            absent = root / "absent"
            result = subprocess.run(
                [str(executable), "--source-content", str(root / "out"), str(absent)],
                env=env, text=True, capture_output=True, timeout=15,
            )
            assert (result.returncode, result.stdout, result.stderr) == (0, str(absent) + "\n0\n", ""), (
                result.returncode, result.stdout, result.stderr,
            )
            observations += 1
        finally:
            unreadable.chmod(0o600)
    print(f"PASS {observations} filename-only discovery observations")


if __name__ == "__main__":
    main()
