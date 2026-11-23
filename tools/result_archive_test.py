import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

from tools import result_archive as archive


class EvidenceArchiveTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "repo"
        self.root.mkdir()
        self.call("init", "-q")
        self.call("config", "user.name", "Archive test")
        self.call("config", "user.email", "archive@example.invalid")
        self.result = self.root / "results" / "test-result"
        self.result.mkdir(parents=True)
        (self.result / "README.md").write_text("# Original result\n")
        (self.result / "raw.csv").write_bytes(b"status,value\r\npass,1\r\nfail,2\r\n")
        (self.result / "empty.stderr").write_bytes(b"")
        (self.result / "program.raw").write_bytes(b"\x7fELF\x00payload")
        (self.result / "detail.log").write_text("unfiltered diagnostic\n")
        self.registry = self.root / "results" / "active.json"
        self.registry.write_text(json.dumps({"schemaVersion": 1, "entries": [{
            "slot": "candidate", "directory": "test-result", "reason": "Synthetic test candidate",
            "retireWhen": "The test completes"}]}))
        self.commit()
        self.base = self.call("rev-parse", "HEAD").decode().strip()
        self.output = Path(self.temp.name) / "evidence.tar.gz"

    def call(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args])

    def commit(self):
        self.call("add", ".")
        self.call("commit", "-qm", "Synthetic evidence")

    def packed(self):
        return archive.pack(self.root, self.output, ["test-result"])

    def test_deterministic_complete_archive_including_empty_and_failed_data(self):
        info = self.packed()
        second = Path(self.temp.name) / "second.tar.gz"
        self.assertEqual(info, archive.pack(self.root, second, ["test-result"]))
        manifest = archive.verify(self.output, info["sha256"])
        self.assertEqual(len(manifest["files"]), 5)
        self.assertEqual(manifest["sourceRevision"], self.base)
        self.assertEqual(manifest["files"][2]["bytes"], 0)

    def test_rejects_wrong_checksum_dirty_missing_or_symlink_input(self):
        info = self.packed()
        with self.assertRaises(ValueError):
            archive.verify(self.output, "0" * 64)
        (self.result / "detail.log").write_text("changed")
        with self.assertRaises(ValueError):
            archive.pack(self.root, Path(self.temp.name) / "dirty.tar.gz", ["test-result"])
        (self.result / "detail.log").unlink()
        (self.result / "detail.log").symlink_to(self.result / "README.md")
        with self.assertRaises(ValueError):
            archive.compact(self.root, self.output, info["sha256"],
                            archive.REPOSITORY + "/releases/download/test/evidence.tar.gz")
        self.assertTrue((self.result / "program.raw").exists())

    def test_pack_rejects_in_repo_archive_unknown_or_unsafe_selection(self):
        for output, names in [(self.root / "evidence.tar.gz", ["test-result"]),
                              (self.output, ["absent"]), (self.output, ["../escape"]),
                              (self.output, ["test-result", "test-result"])]:
            with self.assertRaises(ValueError):
                archive.pack(self.root, output, names)

    def test_pack_rejects_symlinked_directory_inside_checkout(self):
        original = self.root / "results" / "original"
        self.result.rename(original)
        self.result.symlink_to(original, target_is_directory=True)
        with self.assertRaises(ValueError):
            self.packed()

    def test_compaction_preserves_tables_and_originals_in_archive(self):
        info = self.packed()
        before = (self.result / "raw.csv").read_bytes()
        summary = archive.compact(self.root, self.output, info["sha256"],
                                  archive.REPOSITORY + "/releases/download/test/evidence.tar.gz")
        self.assertEqual(summary["removedFiles"], 3)
        self.assertEqual((self.result / "raw.csv").read_bytes(), before)
        self.assertFalse((self.result / "program.raw").exists())
        self.assertEqual(json.loads((self.result / "ARCHIVE.json").read_text())["sha256"], info["sha256"])
        with tarfile.open(self.output) as packed:
            self.assertEqual(packed.extractfile("results/test-result/README.md").read(), b"# Original result\n")
        self.commit()
        self.assertEqual(archive.check(self.root, self.base, "HEAD"), [])

    def test_compaction_rejects_added_evidence_and_wrong_url_without_deletion(self):
        info = self.packed()
        with self.assertRaises(ValueError):
            archive.compact(self.root, self.output, info["sha256"], "https://example.invalid/archive.tar.gz")
        (self.result / "new.csv").write_text("new evidence\n")
        self.commit()
        with self.assertRaises(ValueError):
            archive.compact(self.root, self.output, info["sha256"],
                            archive.REPOSITORY + "/releases/download/test/evidence.tar.gz")
        self.assertTrue((self.result / "program.raw").exists())

    def test_verify_rejects_missing_duplicate_traversal_and_changed_members(self):
        info = self.packed()
        with tarfile.open(self.output) as source:
            originals = [(m.name, source.extractfile(m).read()) for m in source.getmembers()]
        variants = [originals[:-1], originals + [originals[-1]],
                    originals + [("../../escape", b"bad")],
                    originals[:-1] + [(originals[-1][0], b"different")],
                    [originals[0], originals[0]] + originals[1:]]
        for index, rows in enumerate(variants):
            path = Path(self.temp.name) / f"invalid-{index}.tar.gz"
            with tarfile.open(path, "w:gz") as target:
                for name, content in rows:
                    archive.add_file(target, name, content)
            with self.assertRaises(ValueError):
                archive.verify(path, archive.digest_file(path))

    def test_budget_grandfathers_old_files_but_blocks_new_bulk_and_binary(self):
        self.assertEqual(archive.check(self.root, self.base, self.base), [])
        for name, data in [("another.stderr", b""), ("large.log", b"x" * (256 * 1024 + 1)),
                           ("binary.txt", b"\0binary"), ("generated.ssa", b"generated")]:
            (self.result / name).write_bytes(data)
        self.commit()
        self.assertEqual(len(archive.check(self.root, self.base, "HEAD")), 4)

    def test_budget_caps_result_count_and_total_bytes(self):
        directory = self.root / "results" / "new-result"
        directory.mkdir()
        for index in range(101):
            (directory / f"{index}.txt").write_bytes(b"x" * 22000)
        self.commit()
        errors = archive.check(self.root, self.base, "HEAD")
        self.assertTrue(any("new-result" in error for error in errors))
        self.assertTrue(any("active slot" in error for error in errors))

    def test_lifecycle_rejects_unregistered_replaced_and_unexplained_results(self):
        original = json.loads(self.registry.read_text())
        for mutate in [lambda m: m.update(entries=[]),
                       lambda m: m['entries'].append(dict(m['entries'][0])),
                       lambda m: m['entries'][0].update(reason=''),
                       lambda m: m['entries'][0].update(retireWhen=''),
                       lambda m: m['entries'][0].update(slot='another-permanent-snapshot'),
                       lambda m: m['entries'][0].update(directory='replacement-result')]:
            value = json.loads(json.dumps(original))
            mutate(value)
            self.registry.write_text(json.dumps(value))
            self.commit()
            self.assertTrue(archive.check(self.root, self.base, 'HEAD'))

    def test_lifecycle_allows_replacement_only_after_old_directory_is_removed(self):
        replacement = self.result.with_name('replacement-result')
        replacement.mkdir()
        (replacement / 'README.md').write_text('New reviewed result\n')
        value = json.loads(self.registry.read_text())
        value['entries'][0]['directory'] = replacement.name
        self.registry.write_text(json.dumps(value))
        self.commit()
        self.assertTrue(archive.check(self.root, self.base, 'HEAD'))
        self.call('rm', '-r', '--quiet', 'results/test-result')
        self.commit()
        self.assertEqual(archive.check(self.root, self.base, 'HEAD'), [])

    def test_global_budget_and_registry_are_not_grandfathered(self):
        entries = archive.tree(self.root, 'HEAD')
        for number in range(513):
            entries[f'results/test-result/{number}.txt'] = ('100644', 'unused', 1)
        self.assertTrue(any('global' in e for e in archive.lifecycle_errors(self.root, 'HEAD', entries)))
        entries = archive.tree(self.root, 'HEAD')
        entries['results/test-result/large.txt'] = ('100644', 'unused', 4 * 1024**2)
        self.assertTrue(any('global' in e for e in archive.lifecycle_errors(self.root, 'HEAD', entries)))
        del entries['results/active.json']
        self.assertTrue(any('registry' in e for e in archive.lifecycle_errors(self.root, 'HEAD', entries)))

    def test_retired_references_require_available_pinned_history(self):
        name = '2020-01-01-old-result'
        historical = self.root / 'results' / name
        historical.mkdir()
        (historical / 'README.md').write_text('Rejected synthetic experiment\n')
        self.commit()
        revision = self.call('rev-parse', 'HEAD').decode().strip()
        self.call('rm', '-r', '--quiet', f'results/{name}')
        note = self.root / 'history.md'
        note.write_text(f'[old](results/{name}/README.md)\n')
        self.commit()
        self.assertTrue(any('historical revision' in e for e in archive.check(self.root, self.base, 'HEAD')))
        note.write_text(f'[old]({archive.REPOSITORY}/blob/{revision}/results/{name}/README.md)\n')
        self.commit()
        self.assertEqual(archive.check(self.root, self.base, 'HEAD'), [])
        note.write_text(note.read_text().replace(revision, '0' * 40))
        self.commit()
        self.assertTrue(any('unavailable' in e for e in archive.check(self.root, self.base, 'HEAD')))


if __name__ == "__main__":
    unittest.main()
