import tempfile
import unittest
from pathlib import Path

from recovery_layout_sync import InventoryError, closure, default_mutation, relocation_map, synchronize


LAYOUT_HEAD = "record RecoveryCompilerModule\n\tname: String\n\timports: String\nend\n\ndef compiler_recovery_layout(): Array<RecoveryCompilerModule>\n\treturn [\n"
MUTATIONS_HEAD = "def compiler_recovery_mutations(): Array<Array<String>>\n\treturn [\n"
FRONTEND = (
    'describe("Compiler") do\n\ttest("parses the checked-in compiler closure through its own frontend") do\n'
    '\t\tnames := [{names}]\n\t\tputs(names.size())\n\tend\nend\n'
)


class RecoveryInventorySyncTest(unittest.TestCase):
    def test_analysis_mutations_skip_comments_strings_and_zero_divisors(self):
        self.assertEqual(default_mutation("effects", 'return effects % 2\n'),
                         ["effects", "% 2", "% 3"])
        for body in ('return value % 0\n', '# value % 2\n', 'return "% 2"\n',
                     'return a % 2 + b % 2\n', 'return value % 9007199254740991\n'):
            self.assertIsNone(default_mutation("no_divisor", body))
        binding = "\tmut needed := false\n"
        self.assertEqual(default_mutation("scan", binding + "\tmut other := false\n"),
                         ["scan", binding, binding.replace("false", "true")])
        self.assertIsNone(default_mutation("duplicate", binding + binding))
        self.assertIsNone(default_mutation("comment", "# mut needed := false\n"))

    def test_default_mutation_covers_unique_capacity_factors(self):
        source = '\tcapacity := required * 2\n'
        self.assertEqual(default_mutation("capacity", source),
                         ["capacity", source, '\tcapacity := required * 3\n'])
        for invalid in (source + source, '# capacity := required * 2\n',
                        '\tcapacity := required * 2.5\n', '\tcapacity := required * 0\n',
                        '\tcapacity := required * 9007199254740991\n'):
            self.assertIsNone(default_mutation("invalid", invalid))

    def test_default_mutation_preserves_escaped_runtime_literals(self):
        text = r'\"panic: cleanup failed\"'
        source = 'emit("data $message = { b ' + text + ' }")\n'
        self.assertEqual(default_mutation("runtime", source),
                         ["runtime", text, r'\"panic: cleanup failed~\"'])
        self.assertIsNone(default_mutation("comment", '# ' + source))
        self.assertIsNone(default_mutation("duplicate", source + source))

    def test_default_mutation_covers_unique_integer_equality(self):
        source = 'def eligible(kind: Integer): Boolean\nreturn kind != 2\nend\n'
        self.assertEqual(default_mutation("eligible", source),
                         ["eligible", "!= 2", "!= 3"])

    def test_default_mutation_covers_literal_return_modules(self):
        source = 'def none(): Integer\nreturn 0\nend\ndef failure(): Integer\nreturn 8\nend\n'
        self.assertEqual(default_mutation("effects", source),
                         ["effects", "return 0\n", "return 1\n"])
        for body in ('# return 0\n', 'return 0.5\n', 'return -1\n',
                     'return 9007199254740991\n', 'return 0\nreturn 0\n'):
            self.assertIsNone(default_mutation("no_candidate", body))

    def test_default_mutation_uses_unique_numeric_bound_outside_strings_and_comments(self):
        source = '# bound > 2\ndef small(count: Integer): Boolean\nreturn count <= 24\nend\n'
        self.assertEqual(default_mutation("small", source), ["small", "<= 24", "<= 25"])
        self.assertIsNone(default_mutation("small", 'def f(): Boolean\nreturn 1 < 2 || 0 < 2\nend\n'))

    def test_repeated_literal_values_select_one_complete_owner(self):
        first = 'def array_none(): Integer\n\treturn 0\nend\n'
        second = 'def string_none(): Integer\n\treturn 0\nend\n'
        self.assertEqual(default_mutation("methods", first + second),
                         ["methods", first, first.replace('return 0', 'return 1')])
        self.assertIsNone(default_mutation("duplicate_definition", first + first))
        self.assertIsNone(default_mutation("portable_limit", (first + second).replace('return 0', 'return 9007199254740991')))

    def test_default_mutation_does_not_treat_code_between_strings_as_a_literal(self):
        source = ('# "comment literal" is not emitted\n'
                  'record Output\n\tlines: String\n\tflags: Integer\nend\n'
                  'def branch(condition: String): String\n'
                  '\treturn "\\tjnz " + condition + ", " + "\\\"quoted\\\""\nend\n')
        self.assertEqual(default_mutation("output", source),
                         ["output", "\tlines: String\n\tflags: Integer",
                          "\tflags: Integer\n\tlines: String"])
        self.assertEqual(default_mutation("output", source + '\ndef message(): String\nreturn "actual literal"\nend\n'),
                         ["output", '"actual literal"', '"actual literal~"'])

    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.root = Path(self.directory.name)
        (self.root / "src").mkdir()
        (self.root / "compiler/src").mkdir(parents=True)
        self.source("compiler", 'import { label } from first\n\ndef main()\n\tputs(label())\n\treturn\nend\n')
        self.source("first", 'import { Point } from second\n\ndef label(): String\n\treturn "first label"\nend\n')
        self.source("second", "record Point\n\tx: Integer\n\ty: String\nend\n")
        self.source("unrelated", 'def unused(): String\n\treturn "not in the closure"\nend\n')
        self.write("recovery/src/compiler/layout.trb", LAYOUT_HEAD +
                   '\t\tRecoveryCompilerModule.new(name: "second", imports: ""),\n'
                   '\t\tRecoveryCompilerModule.new(name: "first", imports: "import { Point } from second\\n\\n"),\n'
                   '\t\tRecoveryCompilerModule.new(name: "compiler", imports: "import { label } from first\\n\\n"),\n'
                   "\t]\nend\n")
        self.write("recovery/src/compiler/mutations.trb", MUTATIONS_HEAD +
                   '\t\t["first", "\\"first label\\"", "\\"first title\\""],\n'
                   '\t\t["second", "\\tx: Integer\\n\\ty: String", "\\ty: String\\n\\tx: Integer"],\n'
                   "\t]\nend\n")
        self.write("compiler/src/tests/compiler/source_loading_test.trb", FRONTEND.format(names='"compiler", "second", "first"'))

    def tearDown(self):
        self.directory.cleanup()

    def source(self, name, text):
        self.write(f"compiler/src/{name}.trb", text)

    def write(self, relative, text):
        (self.root / relative).parent.mkdir(parents=True, exist_ok=True)
        (self.root / relative).write_text(text)

    def read(self, relative):
        return (self.root / relative).read_text()

    def snapshot(self):
        return {path: self.read(path) for path in (
            "recovery/src/compiler/layout.trb", "recovery/src/compiler/mutations.trb", "compiler/src/tests/compiler/source_loading_test.trb")}

    def test_relocation_uses_present_identity_from_multiple_layouts(self):
        manifest = {"modules": {"first": "support/first", "second": "support/second"},
                    "previousModules": {"old/first": "first", "old/second": "second"}}
        inventory = self.read("recovery/src/compiler/mutations.trb")
        self.assertEqual(relocation_map(manifest, inventory), manifest["modules"])
        inventory = inventory.replace('["first",', '["old/first",')
        self.assertEqual(relocation_map(manifest, inventory),
                         {"old/first": "support/first", "second": "support/second"})

    def test_synchronized_inventories_are_unchanged(self):
        self.assertEqual(closure(self.root), ["compiler", "first", "second"])
        before = self.snapshot()
        self.assertEqual(synchronize(self.root, True), [])
        self.assertEqual(self.snapshot(), before)

    def test_nested_paths_and_cycles_remain_in_every_inventory(self):
        self.source("first", 'import { extra } from checking/value\n\ndef label(): String\nreturn "first label" + extra()\nend\n')
        self.source("checking/value", 'import { label } from first\nimport { last } from mir/value\n\ndef extra(): String\nreturn "nested extra"\nend\n')
        self.source("mir/value", 'import { extra } from checking/value\n\ndef last(): String\nreturn "nested last"\nend\n')
        self.assertEqual(closure(self.root), ["compiler", "first", "checking/value", "mir/value"])
        synchronize(self.root, True)
        self.assertEqual(synchronize(self.root, False), [])
        for text in self.snapshot().values():
            self.assertIn('"checking/value"', text)
            self.assertIn('"mir/value"', text)

    def test_invalid_escaping_or_missing_imports_reject_before_inventory_writes(self):
        for name in ("../outside", "/absolute", "nested/../outside", "missing/value"):
            self.source("first", f'import {{ value }} from {name}\n\ndef label(): String\nreturn "first label"\nend\n')
            before = self.snapshot()
            with self.assertRaises(InventoryError):
                synchronize(self.root, True)
            self.assertEqual(self.snapshot(), before)

    def test_explicit_relocation_preserves_reviewed_mutation_needles(self):
        self.source("compiler", 'import { label } from checking/first\n\ndef main()\nputs(label())\nend\n')
        self.source("checking/first", self.read("compiler/src/first.trb"))
        (self.root / "compiler/src/first.trb").unlink()
        synchronize(self.root, True, {"first": "checking/first"})
        mutations = self.read("recovery/src/compiler/mutations.trb")
        self.assertIn('["checking/first", "\\\"first label\\\"", "\\\"first title\\\""]', mutations)
        self.assertEqual(synchronize(self.root, False), [])

    def test_invalid_relocation_map_does_not_write_inventories(self):
        for renames in ({"first": "../outside"}, {"first": "same", "second": "same"}):
            before = self.snapshot()
            with self.assertRaises(InventoryError):
                synchronize(self.root, True, renames)
            self.assertEqual(self.snapshot(), before)

    def test_added_module_enters_every_inventory_without_reordering(self):
        self.source("first", 'import { Point } from second\nimport { extra } from third\n\n'
                             'def label(): String\n\treturn "first label" + extra()\nend\n')
        self.source("third", 'def extra(): String\n\treturn "third extra"\nend\n')
        before = self.snapshot()
        changes = synchronize(self.root, False)
        self.assertEqual(self.snapshot(), before)
        self.assertEqual(changes, ["layout: add third", "layout: imports first", "mutations: add third",
                                   "frontend test: add third"])
        synchronize(self.root, True)
        self.assertEqual(synchronize(self.root, False), [])
        layout = self.read("recovery/src/compiler/layout.trb")
        self.assertLess(layout.index('"second"'), layout.index('"compiler"'))
        self.assertLess(layout.index('name: "third"'), layout.index('name: "compiler"'))
        self.assertIn('RecoveryCompilerModule.new(name: "compiler", imports: "import { label } from first\\n\\n"),\n\t]', layout)
        self.assertIn('imports: "import { Point } from second\\nimport { extra } from third\\n\\n"', layout)
        self.assertIn('["third", "\\"third extra\\"", "\\"third extra~\\""],\n\t]',
                      self.read("recovery/src/compiler/mutations.trb"))
        self.assertIn('names := ["compiler", "second", "first", "third"]', self.read("compiler/src/tests/compiler/source_loading_test.trb"))

    def test_misplaced_compiler_entry_is_moved_to_the_end(self):
        layout = self.read("recovery/src/compiler/layout.trb")
        compiler = '\t\tRecoveryCompilerModule.new(name: "compiler", imports: "import { label } from first\\n\\n"),\n'
        self.write("recovery/src/compiler/layout.trb", layout.replace(compiler, "").replace(
            '\t\tRecoveryCompilerModule.new(name: "first", imports: "import { Point } from second\\n\\n"),\n',
            compiler + '\t\tRecoveryCompilerModule.new(name: "first", imports: "import { Point } from second\\n\\n"),\n',
        ))
        self.assertEqual(synchronize(self.root, False), ["layout: move compiler to final position"])
        synchronize(self.root, True)
        self.assertEqual(synchronize(self.root, False), [])
        self.assertIn(compiler + "\t]\n", self.read("recovery/src/compiler/layout.trb"))

    def test_module_leaving_the_closure_is_removed_everywhere(self):
        self.source("first", 'def label(): String\n\treturn "first label"\nend\n')
        self.assertEqual(synchronize(self.root, True), [
            "layout: remove second", "layout: imports first", "mutations: remove second", "frontend test: remove second"])
        self.assertNotIn('"second"', self.read("recovery/src/compiler/layout.trb"))
        self.assertNotIn('"second"', self.read("recovery/src/compiler/mutations.trb"))
        self.assertIn('names := ["compiler", "first"]', self.read("compiler/src/tests/compiler/source_loading_test.trb"))

    def test_stale_needles_regenerate_from_literals_or_record_fields(self):
        self.source("first", 'import { Point } from second\n\ndef label(): String\n\treturn "renamed label"\nend\n')
        self.source("second", "record Point\n\ty: String\n\tx: Integer\nend\n")
        self.assertEqual(synchronize(self.root, True), ["mutations: regenerate first", "mutations: regenerate second"])
        mutations = self.read("recovery/src/compiler/mutations.trb")
        self.assertIn('["first", "\\"renamed label\\"", "\\"renamed label~\\""]', mutations)
        self.assertIn('["second", "\\ty: String\\n\\tx: Integer", "\\tx: Integer\\n\\ty: String"]', mutations)

    def test_modules_without_an_observable_default_need_a_manual_mutation(self):
        self.source("second", "record Point\n\tx: Integer\nend\n")
        with self.assertRaisesRegex(InventoryError, "second"):
            synchronize(self.root, False)

    def test_unique_boolean_cell_is_a_constructor_mutation(self):
        for initial, changed in (("true", "false"), ("false", "true")):
            source = f"def fresh(): Array<Boolean>\n\treturn [{initial}]\nend\n"
            self.assertEqual(default_mutation("fresh", source),
                             ["fresh", f"[{initial}]", f"[{changed}]"])
            self.assertIsNone(default_mutation("fresh", source + source))
        self.assertIsNone(default_mutation("comments", '# [true]\n# [false]\n'))
        # These strings are not SAFE_LITERAL candidates; their contents must
        # not be mistaken for Boolean cells either.
        self.assertIsNone(default_mutation("strings", 'def text(): String\nreturn "[true]" + "[false]"\nend\n'))
        self.source("second", "def fresh(): Array<Boolean>\n\treturn [true]\nend\n")
        self.assertEqual(synchronize(self.root, True), ["mutations: regenerate second"])
        self.assertIn('["second", "[true]", "[false]"]',
                      self.read("recovery/src/compiler/mutations.trb"))
        self.assertEqual(synchronize(self.root, False), [])

    def test_imports_must_lead_and_the_entry_must_start_the_frontend_list(self):
        self.source("first", 'def label(): String\n\treturn "first label"\nend\nimport { Point } from second\n')
        with self.assertRaisesRegex(InventoryError, "leading block"):
            synchronize(self.root, False)
        self.tearDown()
        self.setUp()
        self.write("compiler/src/tests/compiler/source_loading_test.trb", FRONTEND.format(names='"first", "compiler", "second"'))
        with self.assertRaisesRegex(InventoryError, "compiler entry"):
            synchronize(self.root, False)


if __name__ == "__main__":
    unittest.main()
