#!/usr/bin/env python3
from pathlib import Path
import tempfile
import unittest

from compiler_layout import check, import_header


class OwnershipTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.write('src/compiler', 'import frontend/checking/expression\n')
        self.write('src/frontend/checking/expression', 'def expression(): Integer\n\treturn 1\nend\n')

    def write(self, name, source=''):
        path = self.root / 'compiler' / (name + '.trb')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)

    def errors(self):
        return check(self.root)[0]

    def test_cross_directory_cycles_and_same_group_recursion_are_allowed(self):
        self.write('src/frontend/checking/expression', 'import mir/construction\nimport frontend/checking/statement\n')
        self.write('src/frontend/checking/statement', 'import frontend/checking/expression\n')
        self.write('src/mir/construction', 'import frontend/checking/expression\n')
        self.assertEqual(self.errors(), [])

    def test_unknown_nested_owners_and_unresolved_imports_fail(self):
        self.write('src/misc/new', 'import support/missing\n')
        errors = '\n'.join(self.errors())
        self.assertIn('missing responsibility owner', errors)
        self.assertIn('missing composed module support/missing', errors)

    def test_providers_attach_to_frontend_and_loader_without_backend_access(self):
        self.write('src/project/loader', 'import frontend/providers/declarations\n')
        self.write('src/frontend/providers/declarations', 'import project/loader\nimport frontend/resolution/types\n')
        self.write('src/frontend/resolution/types', 'import frontend/providers/declarations\n')
        self.write('src/frontend/checking/expression', 'import frontend/providers/declarations\n')
        self.assertEqual(self.errors(), [])
        self.write('src/backend/qbe/output', 'import frontend/providers/declarations\n')
        self.assertIn('backend/qbe -> frontend/providers', self.errors()[0])
        self.write('src/backend/qbe/output')
        self.write('src/frontend/providers/declarations', 'import backend/qbe/output\n')
        self.assertIn('frontend/providers -> backend/qbe', self.errors()[0])

    def test_support_and_backend_boundaries_are_enforced(self):
        self.write('src/support/path', 'import backend/qbe/output\n')
        self.write('src/backend/qbe/output', 'import frontend/checking/expression\n')
        self.assertEqual(len(self.errors()), 2)
        self.assertIn('support -> backend/qbe', self.errors()[1])

    def test_identity_exception_does_not_allow_neighboring_resolution_imports(self):
        self.write('src/backend/qbe/emit/constants', 'import frontend/resolution/entry_resolution\n')
        self.write('src/frontend/resolution/entry_resolution')
        self.assertEqual(self.errors(), [])
        self.write('src/backend/qbe/other', 'import frontend/resolution/entry_resolution\n')
        self.assertEqual(len(self.errors()), 1)

    def test_tests_cover_all_core_owners_but_production_cannot_import_them(self):
        self.write('src/support/path')
        self.write('src/support/path_test', 'import backend/qbe/output\nimport testing/pipeline\nimport trb/std/test\n')
        self.write('src/backend/qbe/output')
        self.write('src/testing/pipeline', 'import backend/qbe/output\nimport { expect } from trb/std/test\n')
        self.write('src/compiler_test')
        self.assertEqual(self.errors(), [])
        self.write('src/frontend/checking/expression', 'import support/path_test\n')
        self.assertIn('test modules cannot be imported', self.errors()[0])
        self.write('src/frontend/checking/expression', 'import trb/std/test\n')
        self.assertIn('production code imports test support', self.errors()[0])

    def test_test_case_imports_and_production_helper_dependencies_fail(self):
        self.write('src/testing/pipeline', 'import frontend/checking/expression\n')
        self.write('src/tests/arrays/execution_test', 'import testing/pipeline\n')
        self.write('src/support/path')
        self.write('src/support/path_test', 'import tests/arrays/execution_test\n')
        self.assertIn('test modules cannot be imported', self.errors()[0])
        self.write('src/support/path')
        self.write('src/support/path_test', 'import testing/pipeline\n')
        self.write('src/frontend/checking/expression', 'import testing/pipeline\n')
        self.assertIn('production code imports testing helper', self.errors()[0])
        self.write('src/frontend/checking/expression', '')
        self.write('src/testing/pipeline', 'import { describe, test } from trb/std/test\n')
        self.assertIn('test helpers cannot register cases', self.errors()[0])

    def test_unit_names_follow_the_owner_and_feature_suites_are_explicit(self):
        self.write('src/frontend/checking/expression_test')
        self.write('src/frontend/checking/expression_errors_test')
        self.write('src/tests/strings/execution_test')
        self.assertEqual(self.errors(), [])
        self.write('src/frontend/checking/unrelated_test')
        self.assertIn('unit test must name a colocated production owner', self.errors()[0])

    def test_parsed_state_exception_does_not_admit_other_syntax_dependencies(self):
        self.write('src/state/source_state', 'import frontend/syntax/iteration_syntax\n')
        self.write('src/frontend/syntax/iteration_syntax')
        self.assertEqual(self.errors(), [])
        self.write('src/state/other', 'import frontend/syntax/iteration_syntax\n')
        self.write('src/frontend/syntax/parser')
        self.write('src/state/source_state', 'import frontend/syntax/parser\n')
        self.assertEqual(len(self.errors()), 2)

    def test_cli_can_compose_core_but_core_cannot_import_cli(self):
        self.write('cli/main', 'import compiler\nimport frontend/checking/expression\n')
        self.assertEqual(self.errors(), [])
        self.write('src/compiler_test', 'import main\n')
        self.assertIn('core cannot depend on CLI', self.errors()[0])

    def test_composed_collisions_and_symlinks_fail(self):
        self.write('cli/compiler')
        self.assertIn('duplicate composed module', self.errors()[0])
        (self.root / 'compiler/cli/compiler.trb').unlink()
        (self.root / 'compiler/src/support').mkdir()
        (self.root / 'outside.trb').write_text('')
        (self.root / 'compiler/src/support/escape.trb').symlink_to(self.root / 'outside.trb')
        self.assertIn('source must stay inside', self.errors()[0])

    def test_embedded_program_imports_are_not_compiler_dependencies(self):
        source = '# Header\nimport { helper } from support/path\n\nvalue := "\nimport missing\n"\n'
        self.assertEqual(import_header(source), [(2, 'support/path')])
        with self.assertRaises(ValueError):
            import_header('import { broken\n')

    def test_recovery_is_an_independent_root_with_test_only_helpers(self):
        source = self.root / 'recovery/src'
        (source / 'driver').mkdir(parents=True)
        (source / 'testing').mkdir()
        driver = source / 'driver/main.trb'
        driver.write_text('import testing/fixture\n')
        (source / 'testing/fixture.trb').write_text('')
        self.assertIn('recovery production imports testing helper', self.errors()[0])
        driver.write_text('import frontend/checking/expression\n')
        self.assertIn('recovery import outside its source root', self.errors()[0])
        driver.write_text('')
        self.write('src/frontend/checking/expression', 'import recovery/driver/main\n')
        self.assertIn('missing composed module', self.errors()[0])


if __name__ == '__main__':
    unittest.main()
