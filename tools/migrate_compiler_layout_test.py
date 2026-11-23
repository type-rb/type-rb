#!/usr/bin/env python3
from pathlib import Path
import tempfile
import unittest

from migrate_compiler_layout import apply, plan, rewrite_imports


class MigrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.manifest = {
            'modules': {'compiler': 'compiler', 'checked_program': 'frontend/checking/checked_program',
                        'checked_statements': 'frontend/checking/checked_statements',
                        'lambda_checking': 'frontend/checking/lambda_checking',
                        'support': 'support/storage', 'checking_test': 'frontend/checking/checking_test'},
            'checkerFunctions': {'expression': 'checked_program', 'statement': 'checked_statements'},
        }
        self.expression = 'def expression(value: Integer): Integer\n\treturn statement(value)\nend\n\n'
        self.statement = 'def statement(value: Integer): Integer\n\treturn expression(identity(value))\nend\n'
        self.write('compiler/src/checked_program.trb', 'import { identity } from support\n\n' + self.expression + self.statement)
        self.write('compiler/src/compiler.trb', 'import { expression, statement as body } from checked_program\n')
        self.write('compiler/src/lambda_checking.trb', '')
        self.write('compiler/src/support.trb', 'def identity(value: Integer): Integer\n\treturn value\nend\n')
        self.write('compiler/src/checking_test.trb', 'import { expression } from checked_program\n\n# Embedded input stays authored\nvalue := "import { statement } from checked_program"\n')
        self.write('compiler/cli/main.trb', 'import { statement } from checked_program\n')
        self.write('src/independent.trb', 'import { statement } from checked_program\n')

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def snapshot(self):
        return {p.relative_to(self.root).as_posix(): p.read_bytes() for p in self.root.rglob('*') if p.is_file()}

    def test_split_moves_complete_functions_and_updates_aliases_and_cli(self):
        apply(self.root, self.manifest)
        source = self.root / 'compiler/src/frontend/checking'
        self.assertIn(self.expression, (source / 'checked_program.trb').read_text())
        self.assertIn(self.statement, (source / 'checked_statements.trb').read_text())
        self.assertIn('from frontend/checking/checked_statements', (source / 'checked_program.trb').read_text())
        self.assertIn('from frontend/checking/checked_program', (source / 'checked_statements.trb').read_text())
        self.assertIn('from support/storage', (source / 'checked_statements.trb').read_text())
        entry = (self.root / 'compiler/src/compiler.trb').read_text()
        self.assertIn('import { statement as body } from frontend/checking/checked_statements', entry)
        self.assertIn('from frontend/checking/checked_statements', (self.root / 'compiler/cli/main.trb').read_text())
        self.assertIn('value := "import { statement } from checked_program"', (source / 'checking_test.trb').read_text())
        self.assertEqual((self.root / 'src/independent.trb').read_text(), 'import { statement } from checked_program\n')
        self.assertFalse((self.root / 'compiler/src/checked_program.trb').exists())

    def test_second_application_is_a_byte_identical_noop(self):
        apply(self.root, self.manifest)
        before = self.snapshot()
        self.assertEqual(plan(self.root, self.manifest), ({}, []))
        self.assertEqual(apply(self.root, self.manifest), (0, 0))
        self.assertEqual(self.snapshot(), before)

    def test_unassigned_new_source_fails_before_any_write(self):
        self.write('compiler/src/new_owner.trb', '')
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, 'unassigned compiler module'):
            apply(self.root, self.manifest)
        self.assertEqual(self.snapshot(), before)

    def test_source_collision_fails_before_any_write(self):
        self.write('compiler/src/' + self.manifest['modules']['checked_program'] + '.trb', 'different')
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, 'flat/nested source collision'):
            apply(self.root, self.manifest)
        self.assertEqual(self.snapshot(), before)

    def test_new_extraction_owner_cannot_overwrite_an_existing_module(self):
        self.write('compiler/src/checked_statements.trb', 'def unrelated()\nend\n')
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, 'extracted owner already exists'):
            apply(self.root, self.manifest)
        self.assertEqual(self.snapshot(), before)

    def test_new_function_requires_explicit_ownership(self):
        path = self.root / 'compiler/src/checked_program.trb'
        path.write_text(path.read_text() + '\ndef new_helper()\nend\n')
        before = self.snapshot()
        with self.assertRaisesRegex(ValueError, 'declarations changed'):
            apply(self.root, self.manifest)
        self.assertEqual(self.snapshot(), before)

    def test_partial_migration_and_escaping_assignment_fail(self):
        apply(self.root, self.manifest)
        path = self.root / ('compiler/src/' + self.manifest['modules']['checked_statements'] + '.trb')
        path.write_text('')
        with self.assertRaisesRegex(ValueError, 'partially applied'):
            plan(self.root, self.manifest)
        self.manifest['modules']['compiler'] = '../outside'
        with self.assertRaisesRegex(ValueError, 'invalid or colliding'):
            plan(self.root, self.manifest)

    def test_reviewed_consumers_move_but_history_stays_unchanged(self):
        self.manifest['pathConsumers'] = ['docs/current.md']
        self.manifest['moduleConsumers'] = ['tools/probe.py']
        self.manifest['lookupConsumers'] = {'compiler/src/compiler.trb': ['support']}
        self.write('docs/current.md', 'compiler/src/support.trb')
        self.write('docs/history.md', 'compiler/src/support.trb')
        self.write('tools/probe.py', 'import { identity } from support\n' + '"import { identity } from support\\n"')
        entry = self.root / 'compiler/src/compiler.trb'
        entry.write_text(entry.read_text() + '\nfind_module(state, "support")\n')
        apply(self.root, self.manifest)
        self.assertEqual((self.root / 'docs/current.md').read_text(), 'compiler/src/support/storage.trb')
        self.assertEqual((self.root / 'docs/history.md').read_text(), 'compiler/src/support.trb')
        self.assertEqual((self.root / 'tools/probe.py').read_text().count('from support/storage'), 2)
        self.assertIn('find_module(state, "support/storage")', entry.read_text())
        self.assertEqual(plan(self.root, self.manifest), ({}, []))

    def test_previous_nested_layout_and_default_import_identity(self):
        apply(self.root, self.manifest)
        previous = self.manifest['modules']['support']
        self.manifest['previousModules'] = {previous: 'support'}
        self.manifest['modules']['support'] = 'support/values'
        self.manifest['defaultImports'] = {'support': 'Storage'}
        self.write('compiler/cli/main.trb', 'import support/storage as Value # retained name\n')
        apply(self.root, self.manifest)
        self.assertEqual((self.root / 'compiler/cli/main.trb').read_text(),
                         'import { Storage as Value } from support/values # retained name\n')
        self.assertFalse((self.root / 'compiler/src/support/storage.trb').exists())
        self.assertIn('from support/values', (self.root / 'compiler/src/frontend/checking/checked_statements.trb').read_text())
        self.assertEqual(plan(self.root, self.manifest), ({}, []))

    def test_import_order_is_preserved(self):
        source = 'import second\nimport { First as Alias } from first\n\ndef main()\nend\n'
        expected = source.replace('import second', 'import support/second').replace('from first', 'from mir/first')
        self.assertEqual(rewrite_imports(source, {'second': 'support/second', 'first': 'mir/first'}, {}), expected)


if __name__ == '__main__':
    unittest.main()
