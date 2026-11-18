#!/usr/bin/env python3
"""Sync or verify the bundled official sources against pinned reference Git objects."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SOURCE_ROOT = "internal/official/packages"
BUNDLE = Path("vendor/type-rb/official")
CATALOG = Path("compiler/src/project/official_catalog.trb")
URL_SOURCE = "internal/stdlib/url.go"
URL_ADAPTER = Path("compiler/packages/internal/url.trb")
JSON_ADAPTER = Path("compiler/packages/internal/json.trb")
STDLIB_SOURCES = {"url": URL_SOURCE, "json": "internal/stdlib/json.go", "jsonc": "internal/stdlib/jsonc.go", "time": "internal/stdlib/time.go", "hex": "internal/stdlib/hex.go", "base64": "internal/stdlib/base64.go", "path": "internal/stdlib/path.go", "hash": "internal/stdlib/hash.go", "hmac": "internal/stdlib/hmac.go", "file": "internal/stdlib/file.go", "dir": "internal/stdlib/dir.go", "errors": "internal/stdlib/errors.go"}
NATIVE_ADAPTERS = ["time", "time_calendar", "time_parse", "time_zones", "time_host", "web_context", "hex", "base64", "encoding_bytes", "secure_random", "entropy", "web_dispatch", "web_timeout", "network_host", "web_server", "oidc", "oidc_json", "oidc_host", "oidc_http"]
STDLIB_SOURCES["test"] = "internal/stdlib/registry.go"
STDLIB_SOURCES["process"] = "internal/stdlib/process.go"
NATIVE_ADAPTERS.append("process")
NATIVE_ADAPTERS.append("sqlite")
NATIVE_ADAPTERS.append("jobs_sql")
NATIVE_ADAPTERS.append("jobs_worker")
REPOSITORY = "https://github.com/type-rb/type-rb"


class ValidationError(Exception):
    pass


def git(checkout, *args):
    result = subprocess.run(["git", "--no-replace-objects", "-C", str(checkout), *args], capture_output=True)
    if result.returncode:
        raise ValidationError("cannot read pinned reference Git objects: " +
                              result.stderr.decode("utf-8", "replace").strip())
    return result.stdout


def reference_files(root, checkout):
    revision = (root / "TYPE_RB_REVISION").read_text().strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValidationError("TYPE_RB_REVISION must be an exact commit id")
    if git(checkout, "cat-file", "-t", revision).strip() != b"commit":
        raise ValidationError("TYPE_RB_REVISION must identify a commit")
    files = {}
    entries = git(checkout, "ls-tree", "-rz", "--full-tree", revision, "--", SOURCE_ROOT)
    for entry in entries.split(b"\0"):
        if not entry:
            continue
        metadata, raw_path = entry.split(b"\t", 1)
        mode, kind, oid = metadata.split()
        source_path = raw_path.decode("utf-8")
        relative = source_path.removeprefix(SOURCE_ROOT + "/")
        path = PurePosixPath(relative)
        if (source_path == relative or path.is_absolute() or ".." in path.parts or
                path.as_posix() != relative or "\\" in relative):
            raise ValidationError("invalid reference package path: " + source_path)
        if mode != b"100644" or kind != b"blob":
            raise ValidationError("reference package must be a regular file: " + source_path)
        files["packages/" + relative] = git(checkout, "cat-file", "blob", oid.decode("ascii"))
    if not files or not any(name.endswith("/trbpackage.json") for name in files):
        raise ValidationError("pinned reference has no official package manifests")
    for name, source_path in STDLIB_SOURCES.items():
        entry = git(checkout, "ls-tree", revision, "--", source_path).split()
        if len(entry) != 4 or entry[:2] != [b"100644", b"blob"]:
            raise ValidationError("pinned " + name.upper() + " source must be a regular file")
        data = git(checkout, "cat-file", "blob", entry[2].decode("ascii"))
        pinned_standard_source(data, name)
        files["stdlib/" + name + ".go"] = data
    license_entry = git(checkout, "ls-tree", revision, "--", "LICENSE").split()
    if len(license_entry) != 4 or license_entry[:2] != [b"100644", b"blob"]:
        raise ValidationError("pinned reference license must be a regular file")
    files["LICENSE"] = git(checkout, "cat-file", "blob", license_entry[2].decode("ascii"))
    provenance = {
        "schemaVersion": 1,
        "repository": REPOSITORY,
        "revision": revision,
        "sourceRoot": SOURCE_ROOT,
        "files": [
            {"path": name,
             "source": ("LICENSE" if name == "LICENSE" else STDLIB_SOURCES[PurePosixPath(name).stem] if name.startswith("stdlib/")
                        else SOURCE_ROOT + "/" + name[len("packages/"):]),
             "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}
            for name, data in sorted(files.items())
        ],
    }
    files["provenance.json"] = (json.dumps(provenance, indent=2) + "\n").encode("utf-8")
    return files


def bundle_files(root):
    bundle = root / BUNDLE
    for path in (root / "vendor", root / "vendor/type-rb", bundle):
        if path.is_symlink():
            raise ValidationError("bundle contains a symlink: " + str(path))
    if not bundle.is_dir():
        raise ValidationError("official source bundle is missing")
    files = {}
    for directory, directories, names in os.walk(bundle, followlinks=False):
        for name in directories + names:
            path = Path(directory) / name
            if path.is_symlink():
                raise ValidationError("bundle contains a symlink: " + str(path.relative_to(bundle)))
        for name in names:
            path = Path(directory) / name
            if not path.is_file():
                raise ValidationError("bundle entry must be a regular file: " + str(path))
            files[path.relative_to(bundle).as_posix()] = path.read_bytes()
    return files


def typerb_string(value):
    # Escape interpolation after escaping backslashes, preserving the raw value.
    return json.dumps(value, ensure_ascii=False).replace("#{", "\\#{")


def pinned_standard_source(data, name):
    if name == "test":
        # The wrapper is a single raw literal inside the registry, rather than
        # a source-returning function. Anchor both package entries so an added
        # or moved literal cannot silently select a different provider.
        start = b'\t"trb/std/test": {\n'
        finish = b'\t"trb/internal/test": {\n'
        if (len(re.findall(rb"^package stdlib\n", data, re.M)) != 1 or
                data.count(start) != 1 or data.count(finish) != 1):
            raise ValidationError("unrecognized pinned TEST source boundary")
        entry = data.split(start, 1)[1].split(finish, 1)
        if len(entry) != 2:
            raise ValidationError("unrecognized pinned TEST source boundary")
        matches = re.findall(rb"\t\tSource: `([^`]*)`,\n\t\tKind: Portable,", entry[0])
        if len(matches) != 1:
            raise ValidationError("unrecognized pinned TEST source boundary")
        return matches[0].decode("utf-8")
    boundary = rb"package stdlib\n\nfunc " + re.escape(name.encode()) + rb"Source\(\) string \{\n\treturn `([^`]*)`\n\}\n"
    if name in {"time", "path", "dir", "errors"}:
        # The pinned file also owns the reference intrinsic registry. Extract
        # only its final raw TypeRB wrapper, preserving every source byte.
        marker = b"func " + name.encode() + b"Source() string {\n\treturn `"
        if data.count(marker) != 1 or not data.startswith(b"package stdlib\n"):
            raise ValidationError("unrecognized pinned " + name.upper() + " source boundary")
        boundary = rb"[\s\S]*" + re.escape(marker) + rb"([^`]*)`\n\}\n"
    match = re.fullmatch(boundary, data)
    if match is None:
        raise ValidationError("unrecognized pinned " + name.upper() + " source boundary")
    return match[1].decode("utf-8")


def pinned_url_source(data):
    return pinned_standard_source(data, "url")


def adapter_source(root, relative=URL_ADAPTER):
    path = root / relative
    for parent in (path, *path.parents):
        if parent == root:
            break
        if parent.is_symlink():
            raise ValidationError("standard adapter contains a symlink")
    return path.read_text()


def catalog_source(files, adapter="", json_adapter="", extra_adapters=None):
    packages = []
    names = set()
    for path, data in sorted(files.items()):
        if not path.endswith("/trbpackage.json"):
            continue
        manifest = json.loads(data)
        name, module, source = (manifest[key] for key in ("name", "module", "source"))
        if not all(isinstance(value, str) and value for value in (name, module, source)):
            raise ValidationError("official catalog requires name, module and source: " + path)
        source_path = PurePosixPath(path).parent / source
        if source_path.is_absolute() or ".." in source_path.parts or source_path.as_posix() not in files:
            raise ValidationError("invalid official catalog source: " + path)
        aliases = manifest.get("aliases", [])
        if not isinstance(aliases, list) or not all(isinstance(alias, str) and alias for alias in aliases):
            raise ValidationError("invalid official catalog aliases: " + path)
        for key in [name, *aliases]:
            if key in names:
                raise ValidationError("duplicate official catalog name: " + key)
            names.add(key)
        # Web's portable value source is checked in full. Generated semantic
        # APIs remain unavailable until their providers are implemented; this
        # admits source loading, not routes, server or codec-provider support.
        portable_value_source = (name == "trb/web" and
                                 manifest.get("semanticProvider") == "trb.web" and
                                 manifest.get("typeProvider") == "trb.web" and
                                 manifest.get("projectProvider") == "trb.web.routes")
        testing_source = (name == "trb/web/testing" and
                          manifest.get("semanticProvider") == "trb.web.testing" and
                          not manifest.get("typeProvider") and not manifest.get("projectProvider"))
        jobs_source = (name == "trb/jobs" and manifest.get("typeProvider") == "trb.jobs" and
                       manifest.get("projectProvider") == "trb.jobs.manifest" and not manifest.get("semanticProvider"))
        sqlite_jobs_source = (name == "trb/jobs/sql" and manifest.get("projectProvider") == "trb.jobs.sql" and
                              not manifest.get("typeProvider") and not manifest.get("semanticProvider"))
        supported = (manifest.get("kind", "portable") == "portable" and
                     not manifest.get("targets") and
                     (portable_value_source or testing_source or jobs_source or sqlite_jobs_source or
                      not any(manifest.get(key) for key in ("semanticProvider", "typeProvider", "projectProvider"))))
        packages.append((name, aliases, module, files[source_path.as_posix()].decode("utf-8"), supported))
    lines = ["# Generated by tools/official-sources.py from the pinned reference bundle.",
             "# Source String values preserve the upstream bytes; do not edit this catalog.", "",
             "def official_source_kind(path: String): Integer"]
    for index, (name, aliases, *_rest) in enumerate(packages, 100):
        condition = " || ".join("path == " + typerb_string(key) for key in [name, *aliases])
        lines += ["\tif " + condition, "\t\treturn " + str(index), "\tend"]
    lines += ["\treturn 0", "end", ""]
    for function, type_name, column, default in (("official_source_name", "String", 2, '""'),
                                               ("official_source_text", "String", 3, '""'),
                                               ("official_source_supported", "Boolean", 4, "false")):
        lines += [f"def {function}(kind: Integer): {type_name}"]
        for index, package in enumerate(packages, 100):
            value = package[column]
            literal = ("true" if value else "false") if isinstance(value, bool) else typerb_string(value)
            lines += ["\tif kind == " + str(index), "\t\treturn " + literal, "\tend"]
        lines += ["\treturn " + default, "end", ""]
    sources = [("pinned_" + name + "_source", pinned_standard_source(files["stdlib/" + name + ".go"], name))
               for name in STDLIB_SOURCES]
    sources += [("native_url_source", adapter), ("native_json_source", json_adapter)]
    sources += [("native_" + name + "_source", source) for name, source in (extra_adapters or {}).items()]
    for name, source in sources:
        lines += [f"def {name}(): String", "\treturn " + typerb_string(source), "end", ""]
    return "\n".join(lines).encode("utf-8")


def catalog_path(root):
    destination = root / CATALOG
    for path in [destination, *destination.parents]:
        if path == root:
            break
        if path.is_symlink():
            raise ValidationError("official catalog contains a symlink: " + str(path))
    return destination


def check(root, checkout):
    expected = reference_files(root, checkout)
    actual = bundle_files(root)
    missing = sorted(expected.keys() - actual.keys())
    extra = sorted(actual.keys() - expected.keys())
    if missing:
        raise ValidationError("official source bundle is missing: " + missing[0])
    if extra:
        raise ValidationError("official source bundle has an unexpected file: " + extra[0])
    for name in sorted(expected):
        if actual[name] != expected[name]:
            raise ValidationError("official source bundle differs from TYPE_RB_REVISION: " + name)
    catalog = catalog_path(root)
    if not catalog.is_file() or catalog.read_bytes() != catalog_source(expected, adapter_source(root), adapter_source(root, JSON_ADAPTER),
                        {name: adapter_source(root, Path("compiler/packages/internal") / (name + ".trb")) for name in NATIVE_ADAPTERS}):
        raise ValidationError("official source catalog differs from TYPE_RB_REVISION")
    return len(expected) - 1


def sync(root, checkout):
    expected = reference_files(root, checkout)
    catalog_bytes = catalog_source(expected, adapter_source(root), adapter_source(root, JSON_ADAPTER),
                        {name: adapter_source(root, Path("compiler/packages/internal") / (name + ".trb")) for name in NATIVE_ADAPTERS})
    catalog = catalog_path(root)
    bundle = root / BUNDLE
    for path in (root / "vendor", root / "vendor/type-rb", bundle):
        if path.is_symlink():
            raise ValidationError("bundle contains a symlink: " + str(path))
    if bundle.exists():
        bundle_files(root)
    bundle.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="official-sources-", dir=bundle.parent) as tmp:
        staging = Path(tmp) / "bundle"
        for name, data in expected.items():
            destination = staging / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(data)
        backup = Path(tmp) / "previous"
        if bundle.exists():
            bundle.rename(backup)
        try:
            staging.rename(bundle)
        except OSError:
            if backup.exists():
                backup.rename(bundle)
            raise
    catalog.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(prefix="official-catalog-", dir=catalog.parent, delete=False) as staged:
        staged.write(catalog_bytes)
        staged_path = Path(staged.name)
    try:
        staged_path.replace(catalog)
    finally:
        staged_path.unlink(missing_ok=True)
    return len(expected) - 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reference-checkout", required=True, type=Path)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument("--check", action="store_true")
    action.add_argument("--write", action="store_true")
    args = parser.parse_args()
    try:
        count = (sync if args.write else check)(ROOT, args.reference_checkout)
    except (ValidationError, OSError, UnicodeError, ValueError) as error:
        print("official sources: " + str(error), file=sys.stderr)
        return 1
    print(f"official sources: {'synced' if args.write else 'verified'} {count} pinned files")
    return 0


if __name__ == "__main__":
    sys.exit(main())
