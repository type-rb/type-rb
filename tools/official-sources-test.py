#!/usr/bin/env python3
"""The bundle comparison derives its authority from committed reference objects."""

import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("official_sources", Path(__file__).with_name("official-sources.py"))
bundle = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bundle)


class OfficialSourceTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name) / "native"
        self.reference = Path(self.temporary.name) / "reference checkout"
        self.root.mkdir()
        self.reference.mkdir()
        self.git("init", "-q")
        self.git("config", "user.name", "Source Test")
        self.git("config", "user.email", "source-test@example.invalid")
        self.git("config", "core.autocrlf", "false")
        self.source = self.reference / bundle.SOURCE_ROOT / "trb/sample/src/index.trb"
        self.source.parent.mkdir(parents=True)
        self.source.write_bytes(b"# preserved source\r\ndef sample(): String\n\treturn \"sample\"\nend\n")
        self.manifest = self.source.parent.parent / "trbpackage.json"
        self.manifest.write_text('{"name":"trb/sample","module":"trb/sample/index","source":"src/index.trb"}\n')
        url = self.reference / bundle.URL_SOURCE
        url.parent.mkdir(parents=True, exist_ok=True)
        url.write_text('package stdlib\n\nfunc urlSource() string {\n\treturn `module URL\nend\n`\n}\n')
        adapter = self.root / bundle.URL_ADAPTER
        adapter.parent.mkdir(parents=True, exist_ok=True)
        adapter.write_text("module URL\nend\n")
        for name in (name for name in bundle.STDLIB_SOURCES if name != "url"):
            source = self.reference / bundle.STDLIB_SOURCES[name]
            if name == "test":
                source.write_text('package stdlib\n\n\t"trb/std/test": {\n\t\tSource: `class Expectation<T>\nend\n`,\n\t\tKind: Portable,\n\t},\n\t"trb/internal/test": {\n\t},\n')
            else:
                source.write_text('package stdlib\n\nfunc ' + name + 'Source() string {\n\treturn `module ' + name.upper() + '\nend\n`\n}\n')
        (self.root / bundle.JSON_ADAPTER).write_text("module JSON\nend\n")
        for name in bundle.NATIVE_ADAPTERS:
            (self.root / "compiler/packages/internal" / (name + ".trb")).write_text("module TimeAdapter\nend\n")
        (self.reference / "LICENSE").write_text("Synthetic license\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Synthetic reference")
        self.revision = self.git("rev-parse", "HEAD")
        (self.root / "TYPE_RB_REVISION").write_text(self.revision + "\n")
        bundle.sync(self.root, self.reference)
        self.vendored = self.root / bundle.BUNDLE / "packages/trb/sample/src/index.trb"

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.reference), *args], text=True).strip()

    def test_exact_bytes_and_deterministic_sync(self):
        first = bundle.bundle_files(self.root)
        self.assertEqual(self.vendored.read_bytes(), self.source.read_bytes())
        self.assertEqual(bundle.check(self.root, self.reference), 3 + len(bundle.STDLIB_SOURCES))
        self.assertEqual(bundle.sync(self.root, self.reference), 3 + len(bundle.STDLIB_SOURCES))
        self.assertEqual(first, bundle.bundle_files(self.root))
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_registry_wrapper_has_exact_boundary_and_actual_provenance(self):
        source = (self.reference / bundle.STDLIB_SOURCES["test"]).read_bytes()
        self.assertEqual(bundle.pinned_standard_source(source, "test"),
                         "class Expectation<T>\nend\n")
        self.assertEqual(bundle.pinned_standard_source(b"// Registry documentation.\n" + source, "test"),
                         "class Expectation<T>\nend\n")
        for changed in (source + source, source.replace(b"Source: `", b"Source: build(`"),
                        source.replace(b'"trb/internal/test"', b'"trb/internal/other"')):
            with self.assertRaisesRegex(bundle.ValidationError, "TEST source boundary"):
                bundle.pinned_standard_source(changed, "test")
        provenance = json.loads((self.root / bundle.BUNDLE / "provenance.json").read_text())
        entry = next(row for row in provenance["files"] if row["path"] == "stdlib/test.go")
        self.assertEqual(entry["source"], "internal/stdlib/registry.go")

    def test_uses_pin_instead_of_head_or_dirty_working_files(self):
        self.source.write_text("replacement\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Later reference")
        self.source.write_text("uncommitted replacement\n")
        before = self.git("status", "--porcelain")
        original = self.vendored.read_bytes()
        bundle.sync(self.root, self.reference)
        self.assertEqual(original, self.vendored.read_bytes())
        bundle.check(self.root, self.reference)
        self.assertEqual(self.git("status", "--porcelain"), before)

    def test_rejects_modified_missing_and_additional_files(self):
        self.vendored.write_text("changed\n")
        with self.assertRaisesRegex(bundle.ValidationError, "differs"):
            bundle.check(self.root, self.reference)
        self.vendored.unlink()
        with self.assertRaisesRegex(bundle.ValidationError, "missing"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        (self.vendored.parent / "extra.trb").write_text("extra\n")
        with self.assertRaisesRegex(bundle.ValidationError, "unexpected"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        bundle.check(self.root, self.reference)

    def test_rejects_forged_provenance_even_with_matching_local_hash(self):
        self.vendored.write_text("forged\n")
        provenance = self.root / bundle.BUNDLE / "provenance.json"
        data = json.loads(provenance.read_text())
        entry = next(row for row in data["files"] if row["path"].endswith("index.trb"))
        entry.update(bytes=self.vendored.stat().st_size,
                     sha256=hashlib.sha256(self.vendored.read_bytes()).hexdigest())
        provenance.write_text(json.dumps(data, indent=2) + "\n")
        with self.assertRaisesRegex(bundle.ValidationError, "differs"):
            bundle.check(self.root, self.reference)

    def test_rejects_stale_pin_and_unavailable_commit(self):
        self.source.write_text("new committed source\n")
        self.git("add", ".")
        self.git("commit", "-qm", "Changed pin")
        (self.root / "TYPE_RB_REVISION").write_text(self.git("rev-parse", "HEAD") + "\n")
        with self.assertRaisesRegex(bundle.ValidationError, "differs"):
            bundle.check(self.root, self.reference)
        (self.root / "TYPE_RB_REVISION").write_text("f" * 40 + "\n")
        with self.assertRaises(bundle.ValidationError):
            bundle.check(self.root, self.reference)

    def test_rejects_provenance_only_tampering(self):
        provenance = self.root / bundle.BUNDLE / "provenance.json"
        provenance.write_text(provenance.read_text().replace(bundle.REPOSITORY, "https://example.invalid/source"))
        with self.assertRaisesRegex(bundle.ValidationError, "provenance.json"):
            bundle.check(self.root, self.reference)

    def test_ignores_mutable_git_object_replacements(self):
        relative = self.source.relative_to(self.reference).as_posix()
        original_blob = self.git("rev-parse", self.revision + ":" + relative)
        self.source.write_text("replacement object\n")
        replacement_blob = self.git("hash-object", "-w", str(self.source))
        self.git("replace", original_blob, replacement_blob)
        original = self.vendored.read_bytes()
        bundle.sync(self.root, self.reference)
        self.assertEqual(self.vendored.read_bytes(), original)
        bundle.check(self.root, self.reference)

    def test_rejects_file_and_directory_symlinks_without_touching_target(self):
        self.vendored.unlink()
        self.vendored.symlink_to(self.source)
        original = self.source.read_bytes()
        for operation in (bundle.check, bundle.sync):
            with self.assertRaisesRegex(bundle.ValidationError, "symlink"):
                operation(self.root, self.reference)
        self.vendored.unlink()
        bundle.sync(self.root, self.reference)
        (self.root / bundle.BUNDLE / "external").symlink_to(self.reference, target_is_directory=True)
        with self.assertRaisesRegex(bundle.ValidationError, "symlink"):
            bundle.check(self.root, self.reference)
        self.assertEqual(original, self.source.read_bytes())

    def test_rejects_symlink_in_reference_commit(self):
        self.source.unlink()
        self.source.symlink_to("../../../../LICENSE")
        self.git("add", ".")
        self.git("commit", "-qm", "Invalid source object")
        (self.root / "TYPE_RB_REVISION").write_text(self.git("rev-parse", "HEAD") + "\n")
        with self.assertRaisesRegex(bundle.ValidationError, "regular file"):
            bundle.sync(self.root, self.reference)

    def test_rejects_modified_and_missing_catalog(self):
        catalog = self.root / bundle.CATALOG
        catalog.write_text(catalog.read_text().replace('return 100', 'return 101', 1))
        with self.assertRaisesRegex(bundle.ValidationError, "catalog differs"):
            bundle.check(self.root, self.reference)
        catalog.unlink()
        with self.assertRaisesRegex(bundle.ValidationError, "catalog differs"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        bundle.check(self.root, self.reference)

    def test_pinned_url_source_and_native_adapter_are_verified(self):
        url = self.root / bundle.BUNDLE / "stdlib/url.go"
        original = url.read_bytes()
        self.assertEqual(bundle.pinned_url_source(original), "module URL\nend\n")
        url.write_bytes(original.replace(b"module URL", b"module Forged"))
        with self.assertRaisesRegex(bundle.ValidationError, "differs"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        (self.root / bundle.URL_ADAPTER).write_text("module Changed\nend\n")
        with self.assertRaisesRegex(bundle.ValidationError, "catalog differs"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        bundle.check(self.root, self.reference)
        with self.assertRaisesRegex(bundle.ValidationError, "unrecognized"):
            bundle.pinned_url_source(b"package stdlib\nfunc urlSource() string { return injected() }\n")

    def test_standard_sources_and_adapter_keep_exact_provenance(self):
        for name in (name for name in bundle.STDLIB_SOURCES if name != "url"):
            path = self.root / bundle.BUNDLE / ("stdlib/" + name + ".go")
            expected = "class Expectation<T>\nend\n" if name == "test" else "module " + name.upper() + "\nend\n"
            self.assertEqual(bundle.pinned_standard_source(path.read_bytes(), name), expected)
            path.write_bytes(path.read_bytes().replace(b"end", b"changed"))
            with self.assertRaisesRegex(bundle.ValidationError, "differs"):
                bundle.check(self.root, self.reference)
            bundle.sync(self.root, self.reference)
        (self.root / bundle.JSON_ADAPTER).write_text("module Changed\nend\n")
        with self.assertRaisesRegex(bundle.ValidationError, "catalog differs"):
            bundle.check(self.root, self.reference)
        bundle.sync(self.root, self.reference)
        bundle.check(self.root, self.reference)

    def test_catalog_does_not_follow_symlinks(self):
        catalog = self.root / bundle.CATALOG
        catalog.unlink()
        catalog.symlink_to(self.source)
        original = self.source.read_bytes()
        for operation in (bundle.check, bundle.sync):
            with self.assertRaisesRegex(bundle.ValidationError, "symlink"):
                operation(self.root, self.reference)
        self.assertEqual(self.source.read_bytes(), original)

    def test_catalog_preserves_strings_and_manifest_boundaries(self):
        raw = 'puts("#{value} \\\\ 😀")\r\n'
        files = {'packages/trb/sample/trbpackage.json': json.dumps({
            'name': 'trb/sample', 'aliases': ['trb/old/sample'],
            'module': 'trb/sample/index', 'source': 'src/index.trb'}).encode(),
            'packages/trb/sample/src/index.trb': raw.encode(),
            **{'stdlib/' + name + '.go': (self.reference / source).read_bytes() for name, source in bundle.STDLIB_SOURCES.items()}}
        catalog = bundle.catalog_source(files).decode()
        self.assertIn('path == "trb/sample" || path == "trb/old/sample"', catalog)
        encoded = catalog.split('def official_source_text')[1].split('return ', 1)[1].splitlines()[0]
        self.assertEqual(json.loads(encoded.replace('\\#', '#')), raw)
        self.assertIn('return true', catalog)
        for key, value in [('kind', 'platform'), ('targets', ['typescript']),
                           ('semanticProvider', 'provider'), ('typeProvider', 'provider'),
                           ('projectProvider', 'provider')]:
            manifest = json.loads(files['packages/trb/sample/trbpackage.json'])
            manifest[key] = value
            changed = dict(files, **{'packages/trb/sample/trbpackage.json': json.dumps(manifest).encode()})
            self.assertNotIn('return true', bundle.catalog_source(changed).decode())

    def test_web_value_source_admission_keeps_other_provider_boundaries(self):
        manifest = {'name': 'trb/web', 'module': 'trb/web/index', 'source': 'src/index.trb',
                    'semanticProvider': 'trb.web', 'typeProvider': 'trb.web',
                    'projectProvider': 'trb.web.routes'}
        raw = 'class Response\nend\n'
        files = {'packages/trb/web/trbpackage.json': json.dumps(manifest).encode(),
                 'packages/trb/web/src/index.trb': raw.encode(),
                 **{'stdlib/' + name + '.go': (self.reference / source).read_bytes()
                    for name, source in bundle.STDLIB_SOURCES.items()}}
        catalog = bundle.catalog_source(files).decode()
        self.assertIn('return true', catalog.split('def official_source_supported')[1])
        encoded = catalog.split('def official_source_text')[1].split('return ', 1)[1].splitlines()[0]
        self.assertEqual(json.loads(encoded), raw)
        for key, value in [('name', 'trb/other'), ('kind', 'platform'),
                           ('targets', ['typescript']), ('semanticProvider', 'unknown'),
                           ('typeProvider', 'unknown'), ('projectProvider', 'unknown')]:
            changed = dict(manifest, **{key: value})
            candidate = dict(files, **{'packages/trb/web/trbpackage.json': json.dumps(changed).encode()})
            self.assertNotIn('return true', bundle.catalog_source(candidate).decode())


    def test_web_testing_admission_preserves_bytes_and_provider_boundaries(self):
        manifest = {'name': 'trb/web/testing', 'module': 'trb/web/testing/index',
                    'source': 'src/index.trb', 'semanticProvider': 'trb.web.testing'}
        raw = 'record _DispatchContract\nend\n'
        path = 'packages/trb/web/testing/trbpackage.json'
        files = {path: json.dumps(manifest).encode(),
                 'packages/trb/web/testing/src/index.trb': raw.encode(),
                 **{'stdlib/' + name + '.go': (self.reference / source).read_bytes()
                    for name, source in bundle.STDLIB_SOURCES.items()}}
        catalog = bundle.catalog_source(files).decode()
        self.assertIn('return true', catalog.split('def official_source_supported')[1])
        encoded = catalog.split('def official_source_text')[1].split('return ', 1)[1].splitlines()[0]
        self.assertEqual(json.loads(encoded), raw)
        for key, value in [('name', 'trb/other/testing'), ('kind', 'platform'),
                           ('targets', ['go']), ('semanticProvider', 'unknown'),
                           ('typeProvider', 'trb.web'), ('projectProvider', 'trb.web.routes')]:
            changed = dict(manifest, **{key: value})
            candidate = dict(files, **{path: json.dumps(changed).encode()})
            self.assertNotIn('return true', bundle.catalog_source(candidate).decode())


if __name__ == "__main__":
    unittest.main()
