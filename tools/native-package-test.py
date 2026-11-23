#!/usr/bin/env python3
"""Exercise local source maps, path-package identities and atomic package locks."""
import hashlib
import json
from pathlib import Path
import stat
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()
repository = Path(__file__).resolve().parent.parent


def compact(value):
    # The pinned Go JSON encoder leaves HTML characters intact and escapes the
    # two JavaScript line separators even when other UTF-8 remains literal.
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")


def checksum(value):
    return "sha256:" + hashlib.sha256(compact(value).encode()).hexdigest()


with tempfile.TemporaryDirectory(prefix="native packages ") as temporary:
    root = Path(temporary)
    app = root / "app"
    (app / "src").mkdir(parents=True)

    def run(*arguments, success=True, cwd=app):
        result = subprocess.run([str(binary), *arguments], cwd=cwd, text=True,
                                capture_output=True, timeout=60)
        assert (result.returncode == 0) == success, (arguments, result.returncode, result.stdout, result.stderr)
        return result.stdout + result.stderr

    def config(requirements=None, local=None):
        value = {"name": "package-probe", "mode": "trb", "sourceDir": "src"}
        if requirements is not None:
            value["packages"] = requirements
        if local is not None:
            value["localPackages"] = local
        (app / "trbconfig.jsonc").write_text(json.dumps(value), encoding="utf-8")

    def package(directory, name, source, dependencies=None, source_dir="src"):
        (directory / source_dir).mkdir(parents=True, exist_ok=True)
        manifest = {"formatVersion": 1, "name": name, "version": "1.0.0", "sourceDir": source_dir,
                    "modes": ["go", "ruby", "typescript", "trb"]}
        if dependencies:
            manifest["packages"] = dependencies
        (directory / "trbpackage.json").write_text(json.dumps(manifest), encoding="utf-8")
        (directory / source_dir / "index.trb").write_text(source, encoding="utf-8")
        return manifest

    # A local source map has no manifest or lock. Paths are lexical and may
    # contain spaces, repeated separators and parent components.
    local = root / "local source"
    local.mkdir()
    (local / "index.trb").write_text('def label(): String\nreturn "local"\nend\n')
    config(local={"local/widgets": "../local source/./"})
    (app / "src/main.trb").write_text('import { label } from local/widgets\ndef main()\nputs(label())\nend\n')
    assert run("check") == "ok\n"
    assert run("run") == "local\n"
    assert not (app / "trb.lock").exists()
    config(local={"local/widgets": "../empty"})
    (root / "empty").mkdir()
    assert "no .trb files" in run("check", success=False)

    # Native-written locks have the same content and checksums as the pinned
    # reference serialization, including escaped Unicode input and HTML text.
    directory = root / "path package"
    manifest = package(directory, "acme/widgets", 'def label(): String\nreturn "package"\nend\n',
                       source_dir="src/日本\u2028<&>")
    requirements = {"local/widgets": {"path": "../path package"}}
    config(requirements)
    assert "not installed" in run("check", success=False)
    assert "resolved 1 TypeRB package(s)" in run("install")
    lock_path = app / "trb.lock"
    original = lock_path.read_bytes()
    lock = json.loads(original)
    assert lock == {
        "formatVersion": 1, "configChecksum": checksum(requirements),
        "imports": {"local/widgets": "acme/widgets"},
        "packages": {"acme/widgets": {"version": "1.0.0", "path": "../path package",
                                      "manifestChecksum": checksum(manifest)}}}, lock
    assert stat.S_IMODE(lock_path.stat().st_mode) == 0o644
    assert original.endswith(b"\n")
    assert run("check") == "ok\n"
    assert run("run") == "package\n"
    source_file = directory / manifest["sourceDir"] / "index.trb"
    source_file.write_text('def label(): String\nreturn "package"\nend\ndef main()\nputs("wrong")\nend\n')
    assert "more than one top-level main" in run("check", success=False)
    source_file.write_text('def label(): String\nreturn "package"\nend\n')
    run("install")
    assert lock_path.read_bytes() == original
    (directory / manifest["sourceDir"] / "index.trb").write_text('def label(): String\nreturn "edited"\nend\n')
    assert run("run") == "edited\n"  # source edits do not invalidate the manifest checksum

    # Compare semantic lock content, not whitespace or member order.
    lock_path.write_text(json.dumps(lock, sort_keys=True))
    assert run("check") == "ok\n"
    changed_manifest = dict(manifest, version="1.0.1")
    (directory / "trbpackage.json").write_text(json.dumps(changed_manifest))
    assert "run trb install" in run("check", success=False)
    run("install")
    saved = lock_path.read_bytes()
    (directory / "trbpackage.json").write_text(json.dumps(changed_manifest) + "{}")
    assert "trailing JSON content" in run("install", success=False)
    assert lock_path.read_bytes() == saved
    assert not list(app.glob("trb.lock.*"))
    (directory / "trbpackage.json").write_text(json.dumps(changed_manifest))
    config({"local/renamed": {"path": "../path package"}})
    assert "run trb install" in run("check", success=False)

    # The root alias wins over the same authored project import path. Alias
    # matching happens before directory-index fallback and uses whole segments.
    config(requirements)
    (app / "src/local").mkdir()
    (app / "src/local/widgets.trb").write_text('def label(): String\nreturn "project"\nend\n')
    run("install")
    assert run("run") == "edited\n"

    # Each installed package resolves its own dependency aliases, independently
    # of the project's alias with the same spelling.
    first = root / "first"
    second = root / "second"
    package(first / "shared", "acme/shared-a", 'def value(): String\nreturn "a"\nend\n')
    package(second / "shared", "acme/shared-b", 'def value(): String\nreturn "b"\nend\n')
    package(first, "acme/first", 'import { value } from shared\ndef first(): String\nreturn value()\nend\n',
            {"shared": {"path": "shared"}})
    package(second, "acme/second", 'import { value } from shared\ndef second(): String\nreturn value()\nend\n',
            {"shared": {"path": "shared"}})
    package(root / "top-shared", "acme/top-shared", 'def value(): String\nreturn "top"\nend\n')
    config({"second": {"path": "../second"}, "first": {"path": "../first"}, "shared": {"path": "../top-shared"}})
    (app / "src/main.trb").write_text('import { first } from first\nimport { second } from second\nimport { value } from shared\ndef main()\nputs(first() + second() + value())\nend\n')
    assert "resolved 5 TypeRB package(s)" in run("install")
    assert run("run") == "abtop\n"
    graph_lock = json.loads(lock_path.read_text())
    assert graph_lock["packages"]["acme/first"]["dependencies"] == {"shared": "acme/shared-a"}
    assert graph_lock["packages"]["acme/second"]["dependencies"] == {"shared": "acme/shared-b"}

    # A longer alias prefix wins, but an index fallback is not mapped twice.
    (app / "src/fallback").mkdir()
    (app / "src/fallback/index.trb").write_text('def fallback(): String\nreturn "fallback"\nend\n')
    config({"first": {"path": "../first"}, "first/extra": {"path": "../second"},
            "fallback/index": {"path": "../second"}})
    (app / "src/main.trb").write_text('import { second } from first/extra\nimport { fallback } from fallback\ndef main()\nputs(second() + fallback())\nend\n')
    run("install")
    assert run("run") == "bfallback\n"

    # A true cycle uses an in-root path, so manifest validation cannot mask it.
    cyclic = root / "cyclic"
    package(cyclic, "acme/cyclic", 'def value(): Integer\nreturn 1\nend\n', {"self": {"path": "."}})
    config({"cycle": {"path": "../cyclic"}})
    saved = lock_path.read_bytes()
    assert "dependency cycle" in run("install", success=False)
    assert lock_path.read_bytes() == saved
    config({"first": {"path": "../first"}, "duplicate": {"path": "../second"}})
    package(second, "acme/first", 'def value(): Integer\nreturn 1\nend\n')
    assert "incompatible requirements" in run("install", success=False)
    assert lock_path.read_bytes() == saved
    config({"remote": "v1.0.0"})
    assert "cannot resolve an unlocked TypeRB package while offline" in run("install", "--offline", success=False)
    assert lock_path.read_bytes() == saved

subprocess.run([sys.executable, str(repository / "tools/native-package-install-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-git-package-test.py"), str(binary), "--native"], check=True)
print("Native local source maps, path packages, alias scopes and atomic lock checks passed")
