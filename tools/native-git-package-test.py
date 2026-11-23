#!/usr/bin/env python3
"""Exercise Git package transactions against disposable local repositories."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sys
import time
import subprocess
import tempfile


def verify(binary, *, native=False):
    count = 0
    with tempfile.TemporaryDirectory(prefix="trb git packages ; quoted ' ") as temporary:
        root = Path(temporary).resolve()
        app = root / "app"
        (app / "src/nested").mkdir(parents=True)
        config_file, lock_file = app / "trbconfig.jsonc", app / "trb.lock"
        env = dict(os.environ, NO_COLOR="1", TERM="dumb", GIT_CONFIG_NOSYSTEM="1",
                   GIT_CONFIG_GLOBAL=os.devnull, GIT_TERMINAL_PROMPT="0")
        real_git = shutil.which("git")
        tools = root / "tools"
        tools.mkdir()
        marker = root / "git-calls"
        (tools / "git").write_text('#!/bin/sh\nprintf called\\n >> "$TRB_GIT_CALLS"\nexec "$TRB_REAL_GIT" "$@"\n')
        (tools / "git").chmod(0o755)
        env.update(PATH=str(tools) + os.pathsep + env.get("PATH", ""),
                   TRB_GIT_CALLS=str(marker), TRB_REAL_GIT=real_git, TRBN_HISTORY=str(root / "history"))
        config = {"name": "packages", "mode": "ruby", "sourceDir": "src", "packageManagement": "external", "packages": {}}
        (app / "src/main.trb").write_text('import { value } from value\ndef main()\nputs(value())\nend\n')

        def git(directory, *args):
            return subprocess.check_output([real_git, "-C", str(directory), *args], env=env,
                                           stderr=subprocess.STDOUT, text=True).strip()

        def repository(name, *, dependencies=None, modes=None):
            directory = root / name
            (directory / "src").mkdir(parents=True)
            git(directory, "init", "--quiet")
            git(directory, "config", "user.name", "Package fixture")
            git(directory, "config", "user.email", "fixture@example.invalid")
            manifest = {"formatVersion": 1, "name": "example/" + name,
                        "version": "1.0.0", "sourceDir": "src", "modes": modes or ["ruby", "trb"]}
            if dependencies:
                manifest["packages"] = dependencies
            publish(directory, manifest, "v1.0.0", 'def value(): String\nreturn "one"\nend\n')
            return directory, manifest

        def publish(directory, manifest, tag, source=None):
            (directory / "trbpackage.json").write_text(json.dumps(manifest))
            if source is not None:
                (directory / "src/index.trb").write_text(source)
            # Raw bytes and non-ASCII names are part of the content checksum.
            (directory / "payload-雪.bin").write_bytes(bytes([0, 255, 1, 128]))
            # Darwin filesystems reject this spelling with EILSEQ. Linux must
            # preserve it; the pure byte-relative-path unit also runs on Darwin.
            if sys.platform.startswith("linux"):
                raw_name = os.fsencode(directory) + b"/payload-\xff.bin"
                with open(raw_name, "wb") as raw_file:
                    raw_file.write(b"raw filename")
            git(directory, "add", ".")
            git(directory, "commit", "--quiet", "-m", tag)
            git(directory, "tag", tag)
            return git(directory, "rev-parse", "HEAD")

        def save(requirements=None):
            if requirements is not None:
                config["packages"] = requirements
            config_file.write_text(json.dumps(config))

        def state(path):
            if not path.exists():
                return None
            return path.read_bytes(), path.stat().st_mtime_ns, path.stat().st_ino

        def run(*args, success=True, untouched=False, offline=False, input=None, extra_env=None, closed_stdout=False):
            nonlocal count
            before = state(config_file), state(lock_file)
            calls = marker.read_bytes() if marker.exists() else b""
            p = subprocess.run([str(binary), *args], cwd=app / "src/nested", env=dict(env, **(extra_env or {})),
                               capture_output=True, text=True, errors="backslashreplace", input=input, timeout=90,
                               preexec_fn=(lambda: os.close(1)) if closed_stdout else None)
            assert (p.returncode == 0) == success, (args, p.returncode, p.stdout, p.stderr)
            if untouched:
                assert before == (state(config_file), state(lock_file)), (args, "changed config or lock")
            if offline:
                assert calls == (marker.read_bytes() if marker.exists() else b""), (args, "invoked Git")
            assert not list(app.glob("trb.lock.*")), args
            assert not list((app / ".trb/packages").glob(".fetch-*")), args
            assert not list((app / ".trb/packages").glob(".git-output.*")), args
            count += 1
            return p.stdout + p.stderr

        def locked():
            return json.loads(lock_file.read_text())

        def cache(entry):
            return app / ".trb/packages" / entry["checksum"].removeprefix("sha256:")

        def check_cache(entry):
            directory = cache(entry)
            h = hashlib.sha256()
            for path in sorted(directory.rglob("*"), key=lambda p: os.fsencode(p.relative_to(directory).as_posix())):
                if path.is_file():
                    name = os.fsencode(path.relative_to(directory).as_posix())
                    h.update(str(len(name)).encode() + b":" + name + b":" + path.read_bytes() + b"\0")
            assert entry["checksum"] == "sha256:" + h.hexdigest(), entry
            assert not (directory / ".git").exists()

        dependency, dep_manifest = repository("dependency")
        package, manifest = repository("value", dependencies={"nested": {"source": str(dependency), "version": "v1.0.0"}})
        other, other_manifest = repository("other")
        req = {"value": {"source": str(package), "version": "latest"},
               "other": {"source": str(other), "version": "latest"}}
        save(req)
        run("check", success=False, untouched=True, offline=True)
        run("install", "--frozen", success=False, untouched=True, offline=True)
        run("install", "--offline", success=False, untouched=True, offline=True)
        run("install")
        first = locked()
        assert first["imports"] == {"other": "example/other", "value": "example/value"}
        assert len(first["packages"]) == 3
        assert first["packages"]["example/value"]["dependencies"] == {"nested": "example/dependency"}
        for entry in first["packages"].values():
            check_cache(entry)
        run("check", untouched=True, offline=True)
        # Only the cache root's .git entry is excluded by the reference hash.
        # A nested directory with that name remains ordinary package content.
        cached_root = cache(first["packages"]["example/value"])
        (cached_root / ".git").write_text("ignored root metadata")
        run("check", untouched=True, offline=True)
        (cached_root / ".git").unlink()
        nested_git = cached_root / "nested/.git"
        nested_git.mkdir(parents=True)
        (nested_git / "payload").write_text("must change the package checksum")
        run("check", success=False, untouched=True, offline=True)
        shutil.rmtree(cached_root / "nested")
        run("install", "--offline", "--frozen", untouched=True, offline=True)
        run("install", untouched=True, offline=True)
        # Existing locks remain byte-for-byte frozen, even with unusual JSON layout.
        lock_file.write_text(json.dumps(first, indent=4, sort_keys=True) + "\n\n")
        run("install", "--frozen", untouched=True, offline=True)
        path = cache(first["packages"]["example/value"]) / "payload-雪.bin"
        saved_payload = path.read_bytes()
        path.write_bytes(b"tampered")
        run("check", success=False, untouched=True, offline=True)
        run("install", success=False, untouched=True, offline=True)
        path.write_bytes(saved_payload)
        outside = root / "outside"
        outside.write_bytes(b"must not enter package contents")
        link = path.parent / "linked-payload"
        link.symlink_to(outside)
        run("check", success=False, untouched=True, offline=True)
        run("install", "--offline", success=False, untouched=True, offline=True)
        link.unlink()
        shutil.rmtree(cache(first["packages"]["example/value"]))
        run("check", success=False, untouched=True, offline=True)
        run("install", "--offline", success=False, untouched=True, offline=True)
        run("install", "--frozen", untouched=True)
        check_cache(first["packages"]["example/value"])
        # Full and selected updates preserve precisely the unselected graphs.
        manifest = dict(manifest, version="1.1.0")
        new_revision = publish(package, manifest, "v1.1.0", 'def value(): String\nreturn "two"\nend\n')
        other_manifest = dict(other_manifest, version="2.0.0")
        publish(other, other_manifest, "v2.0.0")
        run("install", untouched=True, offline=True)
        run("update", "missing", success=False, untouched=True, offline=True)
        run("update", "value", "value")
        selected = locked()
        assert selected["packages"]["example/value"]["revision"] == new_revision
        assert selected["packages"]["example/other"] == first["packages"]["example/other"]
        run("update")
        assert locked()["packages"]["example/other"]["version"] == "2.0.0"
        # An unselected transitive graph remains pinned. A conflicting selected
        # graph fails without replacing the old lock; a compatible full update works.
        publish(dependency, dict(dep_manifest, version="2.0.0"), "v2.0.0")
        other_manifest = dict(other_manifest, version="2.1.0", packages={
            "nested": {"source": str(dependency), "version": "v1.0.0"}})
        publish(other, other_manifest, "v2.1.0")
        run("update", "other")
        manifest = dict(manifest, version="1.2.0", packages={
            "nested": {"source": str(dependency), "version": "v2.0.0"}})
        publish(package, manifest, "v1.2.0")
        run("update", "value", success=False, untouched=True)
        other_manifest = dict(other_manifest, version="2.2.0", packages={
            "nested": {"source": str(dependency), "version": "v2.0.0"}})
        publish(other, other_manifest, "v2.2.0")
        run("update")
        assert locked()["packages"]["example/dependency"]["version"] == "2.0.0"
        save({"value": req["value"]})
        run("update", "value", success=False, untouched=True, offline=True)
        run("install")
        run("remove", "value")
        assert locked()["packages"] == {} and json.loads("\n".join(line for line in config_file.read_text().splitlines() if not line.lstrip().startswith("//"))).get("packages", {}) == {}
        run("add", "--source", str(package), "value", "v1.1.0")
        assert locked()["packages"]["example/value"]["revision"] == new_revision
        run("remove", "value")
        run("add", "--source=" + str(package), "value@" + new_revision)
        assert locked()["packages"]["example/value"]["revision"] == new_revision
        run("add", "--source", str(package), "--path", str(package), "bad", success=False, untouched=True, offline=True)
        run("add", "--dev", "bad", success=False, untouched=True, offline=True)
        run("add", "--source", str(package), "bad", "v9.0.0", success=False, untouched=True)
        # A tag spelling must agree with the authored manifest version.
        git(package, "tag", "v9.0.0")
        run("add", "--source", str(package), "bad", "v9.0.0", success=False, untouched=True)
        run("add", "--source", "https://user:secret@example.invalid/pkg", "bad", "v1", success=False, untouched=True, offline=True)
        run("add", "--source", str(package), "bad", "branch", success=False, untouched=True, offline=True)
        # Tag spelling differs from semantic precedence: short versions and
        # build metadata remain in exact refs, while latest compares versions.
        versions, version_manifest = repository("versions")
        publish(versions, dict(version_manifest, version=" 1.2 "), "v1.2")
        run("add", "--source", str(versions), "versions", "1.2")
        assert locked()["packages"]["example/versions"]["version"] == " 1.2 "
        publish(versions, dict(version_manifest, version="1.2.3+build.9"), "v1.2.3+build.9")
        run("add", "--source", str(versions), "versions", "v1.2.3+build.9")
        assert locked()["packages"]["example/versions"]["version"] == "1.2.3+build.9"
        for version in ("2.0.0-rc.2", "2.0.0-rc.11"):
            publish(versions, dict(version_manifest, version=version), "v" + version)
        run("add", "--source", str(versions), "versions")
        assert locked()["packages"]["example/versions"]["version"] == "2.0.0-rc.11"
        run("remove", "versions")
        # Unknown lock fields and malformed input fail on every read-only path.
        current = lock_file.read_bytes()
        for malformed in (current.decode() + "{}", '{"unknown":1}',
                          json.dumps(dict(locked(), extra=True))):
            lock_file.write_text(malformed)
            run("install", "--frozen", success=False, untouched=True, offline=True)
            run("check", success=False, untouched=True, offline=True)
        lock_file.write_bytes(current)
        # Local path commands retain the existing live-source contract.
        local, _ = repository("local")
        run("add", "--path", str(local), "local")
        assert locked()["imports"]["local"] == "example/local"
        run("remove", "local")
        cyclic, _ = repository("cyclic", dependencies={"self": {
            "source": str(root / "cyclic"), "version": "v1.0.0"}})
        run("add", "--source", str(cyclic), "cycle", "v1.0.0", success=False, untouched=True)
        unsupported, _ = repository("unsupported", modes=["go"])
        run("add", "--source", str(unsupported), "unsupported", "v1.0.0", success=False, untouched=True)
        duplicate, duplicate_manifest = repository("duplicate")
        publish(duplicate, dict(duplicate_manifest, name="example/value", version="1.0.1"), "v1.0.1")
        run("add", "--source", str(duplicate), "duplicate", "v1.0.1", success=False, untouched=True)
        # Pin the observed transitive-offline exception separately from the
        # no-Git controls above. Proposal #1224 owns the paired contract decision.
        retained_config, retained_lock = config_file.read_bytes(), lock_file.read_bytes()
        transit, _ = repository("transitive", dependencies={"dependency": {
            "source": str(dependency), "version": "v1.0.0"}})
        config["packages"] = {"transitive": {"path": str(transit)}}
        save()
        lock_file.unlink()
        shutil.rmtree(app / ".trb/packages")
        before = marker.read_bytes()
        run("install", "--offline")
        assert marker.read_bytes() != before, "pinned offline exception changed; review #1224"
        run("install", "--frozen", "--offline", untouched=True, offline=True)
        config_file.write_bytes(retained_config)
        lock_file.write_bytes(retained_lock)
        run("install", "--frozen", untouched=True)
        if native:
            cfg = json.loads("\n".join(line for line in config_file.read_text().splitlines() if not line.lstrip().startswith("//")))
            cfg["mode"] = "trb"
            config_file.write_text(json.dumps(cfg))
            run("check", untouched=True, offline=True)
            assert run("run", untouched=True, offline=True) == "two\n"
            run("build", "--compile", untouched=True, offline=True)
            assert run("repl", input="import { value } from value\nputs(value())\n:quit\n", untouched=True, offline=True) == "two\n"
            # All ordinary consumers remain read-only with a missing cache.
            shutil.rmtree(cache(locked()["packages"]["example/value"]))
            run("check", success=False, untouched=True, offline=True)
            run("run", success=False, untouched=True, offline=True)
            run("build", "--compile", success=False, untouched=True, offline=True)
            run("repl", input=":quit\n", success=False, untouched=True, offline=True)
            # A closed stdout makes the capture descriptor reuse fd 1.
            # Git output must remain captured after child redirection.
            run("install", "--frozen", untouched=True, closed_stdout=True)
            shutil.rmtree(cache(locked()["packages"]["example/value"]))
            # Interrupt a live Git fetch, then require child termination and
            # removal of both the owned fetch tree and the capture file.
            slow_git = root / "slow-git"
            ready = root / "fetch-ready"
            slow_git.write_text("#!" + sys.executable + "\nimport os,sys,time\nfrom pathlib import Path\n"
                                "if 'fetch' in sys.argv[1:]:\n"
                                " Path(" + repr(str(ready)) + ").write_text(str(os.getpid()))\n"
                                " time.sleep(60)\n"
                                "os.execv(" + repr(real_git) + ", [" + repr(real_git) + "] + sys.argv[1:])\n")
            slow_git.chmod(0o755)
            before = state(config_file), state(lock_file)
            for stop_signal in (signal.SIGINT, signal.SIGTERM):
                ready.unlink(missing_ok=True)
                interrupted = subprocess.Popen([str(binary), "install", "--frozen"], cwd=app, env=dict(env, TRB_REAL_GIT=str(slow_git)),
                                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
                try:
                    deadline = time.monotonic() + 15
                    while not ready.exists() and interrupted.poll() is None and time.monotonic() < deadline:
                        time.sleep(0.02)
                    assert ready.exists(), interrupted.communicate(timeout=2)
                    interrupted.send_signal(stop_signal)
                    output = interrupted.communicate(timeout=15)
                    assert interrupted.returncode != 0, output
                    assert before == (state(config_file), state(lock_file)), "interrupt changed metadata"
                    assert not list((app / ".trb/packages").glob(".fetch-*")), "interrupted fetch retained"
                    assert not list((app / ".trb/packages").glob(".git-output.*")), "interrupted output retained"
                    try:
                        os.kill(int(ready.read_text()), 0)
                    except ProcessLookupError:
                        pass
                    else:
                        raise AssertionError("interrupted Git process is still running")
                    count += 1
                finally:
                    # Both groups belong only to this fixture. Capture isolates
                    # Git and its descendants from the CLI's process group.
                    groups = [interrupted.pid]
                    if ready.exists():
                        groups.append(int(ready.read_text()))
                    for group in groups:
                        try:
                            os.killpg(group, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                    interrupted.communicate(timeout=5)
            # A Git child may complete successfully while a signal is pending.
            # Cancellation must still win before cache/metadata publication.
            success_signal = root / "success-signal-git"
            success_signal.write_text("#!" + sys.executable + "\nimport os,signal,subprocess,sys,time\n"
                                      "result=subprocess.run([" + repr(real_git) + "] + sys.argv[1:])\n"
                                      "if 'rev-parse' in sys.argv[1:]:\n"
                                      " signal.signal(signal.SIGINT, signal.SIG_IGN)\n"
                                      " os.kill(os.getppid(), signal.SIGINT)\n"
                                      " time.sleep(0.05)\n"
                                      "sys.exit(result.returncode)\n")
            success_signal.chmod(0o755)
            run("install", "--frozen", success=False, untouched=True, extra_env={"TRB_REAL_GIT": str(success_signal)})
            run("install", "--frozen", untouched=True)
            run("check", untouched=True, offline=True)
            verify_collection_lifetimes(binary, root, env)
            count += 1
    return count


def verify_collection_lifetimes(binary, root, env):
    """A second collector result must not hide missing roots in the first."""
    first, second = root / ".trb/collector-first", root / ".trb/collector-second"
    (first / ".git").mkdir(parents=True)
    second.mkdir(parents=True)
    (first / ".git/config").write_text("ignored")
    (first / "a.txt").write_text("first")
    (first / "z.txt").write_text("last")
    (second / "payload").write_text("replacement")
    source = root / "collector.trb"
    source.write_text('record CompilerProjectSources\n paths: Array<String>\n sources: Array<String>\n error: String\nend\ndef compiler_collect_package_files(_root: String): CompilerProjectSources\n return CompilerProjectSources.new(paths: [], sources: [], error: "")\nend\ndef compiler_main(source: String, mode: String, _file: Boolean): Integer\n retained := compiler_collect_package_files(source)\n replacement := compiler_collect_package_files(mode)\n if retained.error != "" || replacement.error != "" || replacement.paths.size() != 1\n  return 7\n end\n mut allocations: Array<String> := []\n mut index := 0\n while index < 128\n  allocations.push("value-" + index.to_s())\n  index += 1\n end\n if allocations.size() != 128\n  return 8\n end\n puts(retained.paths.size())\n puts(retained.sources[0])\n puts(retained.sources[1])\n return 0\nend\ndef compiler_file_main(source: String, _path: String, mode: String, file: Boolean): Integer\n return compiler_main(source, mode, file)\nend\ndef main()\nend\n')
    def execute(command, environment=env):
        p = subprocess.run(list(map(str, command)), env=environment, cwd=root, capture_output=True, text=True, timeout=60)
        assert p.returncode == 0, (command, p.stdout, p.stderr)
        return p
    emitted = execute([binary, "--internal-driver", "emit-qbe", source])
    assert not emitted.stderr, emitted.stderr
    lines, ordinary, hooks = [], False, 0
    for line in emitted.stdout.splitlines():
        if line.startswith("function "):
            ordinary = "$trbnf" in line
        if line == "}":
            ordinary = False
        if ordinary and any(marker in line for marker in (
                "call $trbn_array_new", "call $trbn_array_push_", "call $trbn_array_grow_paced(",
                "call $trbn_gc_alloc(", "call $trbn_string_concat", "call $trbnf",
                "call $trbn_collect_package_files(")):
            lines.append("\tcall $trbn_gc_collect(w 0)")
            hooks += 1
        lines.append(line)
    assert hooks >= 5, hooks
    il, assembly, executable = (root / name for name in ("collector.ssa", "collector.s", "collector"))
    il.write_text("\n".join(lines) + "\n")
    execute([binary.with_name("qbe"), "-o", assembly, il])
    execute(["/usr/bin/cc", assembly, "-lm", "-o", executable])
    result = execute([executable, "--source-content", second, first], dict(env, TYPE_RB_NATIVE_RUNTIME_STATS="1"))
    assert result.stdout == "2\nfirst\nlast\n", result.stdout
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(",")
        assert prefix == "type-rb-native-gc-stat-v1" and key not in statistics, line
        statistics[key] = int(value)
    assert statistics["collections"] >= 128, statistics
    assert statistics["live-bytes"] == 0, statistics
    assert statistics["allocated-bytes"] == statistics["reclaimed-bytes"], statistics


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--native", action="store_true")
    args = parser.parse_args()
    print(f"PASS {verify(args.binary.resolve(), native=args.native)} Git package lifecycle observations")
