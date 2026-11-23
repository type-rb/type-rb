#!/usr/bin/env python3
"""Check local frozen/offline installation without executing network tools."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def verify(binary):
    count = 0
    with tempfile.TemporaryDirectory(prefix="package install ") as temporary:
        root = Path(temporary).resolve()
        app = root / "app"
        nested = app / "src/nested"
        nested.mkdir(parents=True)
        package = root / "package"
        dependency = package / "dependency"
        for directory in (package, dependency):
            (directory / "src").mkdir(parents=True)
            (directory / "src/index.trb").write_text("record Value\nend\n")
        fake_bin = root / "tools"
        fake_bin.mkdir()
        invoked = root / "network-invoked"
        git = fake_bin / "git"
        git.write_text('#!/bin/sh\nprintf called > "$PACKAGE_INSTALL_NETWORK_MARKER"\nexit 97\n')
        git.chmod(0o755)
        env = dict(os.environ, PATH=str(fake_bin) + os.pathsep + os.environ.get("PATH", ""),
                   PACKAGE_INSTALL_NETWORK_MARKER=str(invoked), NO_COLOR="1", TERM="dumb")
        config_path = app / "trbconfig.jsonc"
        lock = app / "trb.lock"
        requirements = {"local/value": {"path": "../package"}}
        # Ruby needs no host dependency installation. Both compilers analyze
        # the same package mode; this test never evaluates Ruby source.
        config = {"name": "package-install", "mode": "ruby", "sourceDir": "src",
                  "packages": requirements}
        manifest = {"formatVersion": 1, "name": "example/value", "version": "1.0.0",
                    "sourceDir": "src", "modes": ["ruby", "trb"]}
        dependent = dict(manifest, name="example/dependency")

        def write_config(value=config):
            config_path.write_text(json.dumps(value))

        def write_manifest(value=manifest, directory=package):
            (directory / "trbpackage.json").write_text(json.dumps(value))

        def lock_state():
            if not lock.exists():
                return None
            metadata = lock.stat()
            return lock.read_bytes(), metadata.st_mtime_ns, metadata.st_ino

        def run(*arguments, success=True, unchanged=False, cwd=nested):
            nonlocal count
            before = lock_state()
            result = subprocess.run([str(binary), *arguments], cwd=cwd, env=env,
                                    capture_output=True, text=True, timeout=30)
            assert (result.returncode == 0) == success, (arguments, result)
            assert not invoked.exists(), (arguments, "invoked git")
            assert not list(app.glob("trb.lock.*")), arguments
            if unchanged:
                assert lock_state() == before, (arguments, "changed lock")
            count += 1
            return result

        write_config()
        write_manifest()
        write_manifest(dependent, dependency)
        for options in [("--frozen",), ("--offline", "--frozen")]:
            run("install", *options, success=False, unchanged=True)
        run("install", "--offline")
        assert lock.exists()
        # Semantic equality accepts reordered/whitespace-different JSON, but
        # frozen installation must not normalize or replace those bytes.
        lock.write_text(json.dumps(json.loads(lock.read_text()), sort_keys=True, indent=4) + "\n\n")
        for options in [("--frozen",), ("--frozen", "--offline"),
                        ("--offline", "--frozen"), ("--frozen=true", "--offline=false")]:
            run("install", *options, unchanged=True)
        run("install", "--frozen", "--config", str(config_path), cwd=root, unchanged=True)
        for value in ("1", "t", "T", "TRUE", "True"):
            run("install", "--frozen=" + value, unchanged=True)
        for value in ("0", "f", "F", "FALSE", "False", "false"):
            lock.unlink()
            run("install", "--offline", "--frozen=" + value)
            assert lock.exists()
        for option in ("--frozen=", "--frozen=yes", "--offline=perhaps"):
            run("install", option, success=False, unchanged=True)
        run("install", "--frozen", "false", success=False, unchanged=True)
        run("install", "src", success=False, unchanged=True)
        for command, option in [("check", "--frozen"), ("run", "--offline"),
                                ("check", "--frozen=false"), ("build", "--offline=false")]:
            run(command, option, success=False, unchanged=True)
        run("install", "--frozen", "--config", str(root / "missing.json"),
            success=False, unchanged=True)

        # Changes to source bytes do not invalidate the package manifest.
        (package / "src/index.trb").write_text("record Edited\nend\n")
        run("install", "--frozen", "--offline", unchanged=True)
        write_config(dict(config, packages={"other/value": {"path": "../package"}}))
        run("install", "--frozen", success=False, unchanged=True)
        write_config()
        write_manifest(dict(manifest, version="1.0.1"))
        run("install", "--frozen", success=False, unchanged=True)
        run("install", "--offline")
        run("install", "--frozen", unchanged=True)

        transitive = dict(manifest, packages={"nested/value": {"path": "dependency"}})
        write_manifest(transitive)
        run("install", "--offline")
        assert len(json.loads(lock.read_text())["packages"]) == 2
        write_manifest(dict(dependent, version="1.0.2"), dependency)
        run("install", "--frozen", "--offline", success=False, unchanged=True)
        run("install", "--offline")
        run("install", "--frozen", "--offline", unchanged=True)
        saved = lock.read_bytes()
        for malformed in ("{", "{}", '{"formatVersion":99}', saved.decode() + "{}"):
            lock.write_text(malformed)
            run("install", "--frozen", "--offline", success=False, unchanged=True)
        lock.write_bytes(saved)
        write_manifest(dict(transitive, packages={"cycle": {"path": "."}}))
        run("install", "--offline", success=False, unchanged=True)
        write_manifest(transitive)
        write_config(dict(config, packages={"example/remote": "v1.0.0"}))
        run("install", "--offline", success=False, unchanged=True)

        # Empty projects do not read or publish a package lock.
        write_config(dict(config, packages={}))
        lock.write_text("malformed and irrelevant")
        run("install", "--frozen", "--offline", unchanged=True)
        lock.unlink()
        run("install", "--frozen", "--offline", unchanged=True)
    return count


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--reference", type=Path)
    args = parser.parse_args()
    count = verify(args.binary.resolve())
    if args.reference:
        assert verify(args.reference.resolve()) == count
    print(f"PASS {count} frozen/offline local package checks" +
          (" for Native and pinned reference" if args.reference else ""))
