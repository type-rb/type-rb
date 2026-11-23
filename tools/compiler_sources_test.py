from pathlib import Path
import tempfile
import unittest

from compiler_sources import source_hashes, stage_sources


class CompilerSourceStagingTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='compiler source paths ')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.core = self.root / 'core'
        self.cli = self.root / 'cli'
        self.output = self.root / 'output'
        self.core.mkdir()
        self.cli.mkdir()

    def write(self, root, name, source):
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)

    def test_nested_paths_retain_distinct_names_and_cyclic_imports(self):
        sources = {
            'compiler.trb': 'import { first } from checking/value\n',
            'checking/value.trb': 'import { last } from mir/value\n',
            'mir/value.trb': 'import { first } from checking/value\n',
        }
        for name, source in sources.items():
            self.write(self.core, name, source)
        self.write(self.core, 'checking/value_test.trb', 'test-only')
        self.write(self.cli, 'main.trb', 'import { first } from checking/value\n')
        selected = stage_sources([self.core, self.cli], self.output)
        self.assertEqual({str(path) for path in selected}, {*sources, 'main.trb'})
        for name, source in sources.items():
            self.assertEqual((self.output / name).read_text(), source)
        self.assertFalse((self.output / 'checking/value_test.trb').exists())

    def test_collisions_fail_before_any_copy(self):
        self.write(self.core, 'first.trb', 'first')
        self.write(self.core, 'shared/value.trb', 'core')
        self.write(self.cli, 'shared/value.trb', 'cli')
        with self.assertRaisesRegex(ValueError, 'collision'):
            stage_sources([self.core, self.cli], self.output)
        self.assertFalse(self.output.exists())

    def test_helpers_never_enter_staging_or_production_content_keys(self):
        self.write(self.core, 'compiler.trb', 'entry')
        self.write(self.core, 'testing/nested/runner.trb', 'test harness')
        self.write(self.core, 'tests/arrays/execute_test.trb', 'test case')
        before = source_hashes(self.core)
        self.write(self.core, 'testing/nested/runner.trb', 'changed harness')
        self.assertEqual(source_hashes(self.core), before)
        self.assertEqual(stage_sources([self.core], self.output), [Path('compiler.trb')])
        self.write(self.core, 'compiler.trb', 'changed entry')
        self.assertNotEqual(source_hashes(self.core), before)

    def test_existing_destinations_and_symlinks_are_not_overwritten(self):
        self.write(self.core, 'nested/value.trb', 'core')
        self.write(self.output, 'nested/value.trb', 'existing')
        with self.assertRaisesRegex(ValueError, 'collision'):
            stage_sources([self.core], self.output)
        self.assertEqual((self.output / 'nested/value.trb').read_text(), 'existing')
        outside = self.root / 'outside'
        outside.mkdir()
        (self.core / 'link').symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'symlink'):
            stage_sources([self.core], self.root / 'fresh')
        (self.core / 'link').unlink()
        fresh = self.root / 'fresh'
        fresh.mkdir()
        (fresh / 'nested').symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'destination'):
            stage_sources([self.core], fresh)
        self.assertEqual(list(outside.iterdir()), [])

    def test_file_and_directory_collisions_fail_before_any_copy(self):
        self.write(self.core, 'group.trb', 'core')
        self.write(self.cli, 'group.trb/child.trb', 'cli')
        for roots in ([self.core, self.cli], [self.cli, self.core]):
            with self.assertRaisesRegex(ValueError, 'collision'):
                stage_sources(roots, self.output)
            self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
