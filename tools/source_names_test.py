"""Regressions for current prose versus immutable historical references."""
import importlib.util
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("source_names", Path(__file__).with_name("source-names-check.py"))
NAMES = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(NAMES)


class SourceNamesTests(unittest.TestCase):
    def test_source_comment_and_identifier(self):
        for text in ("# gate-specific module models", "def Gate4SymbolIndex()", "# Gate 6M compiler"):
            with self.subTest(text=text):
                self.assertTrue(NAMES.retired_names("recovery/src/scalar/qbe.trb", text))

    def test_current_markdown_including_labels(self):
        self.assertTrue(NAMES.retired_names("docs/architecture.md", "[Gate 4](https://example.test/history)"))
        self.assertTrue(NAMES.retired_names("docs/gate-4-model.md", "Current model"))
        self.assertTrue(NAMES.retired_names("tools/gate4-verify.sh", ""))

    def test_history_identity_and_evidence_are_unchanged(self):
        self.assertFalse(NAMES.retired_names("docs/history.md", "[Original record](https://github.com/type-rb/type-rb-native/blob/7726ff18/docs/gate-4-self-hosting.md)"))
        self.assertFalse(NAMES.retired_names("results/old/README.md", "Gate4 measurements"))
        self.assertFalse(NAMES.retired_names("docs/ci-validation.md", "A correctness gate remains required."))


if __name__ == "__main__":
    unittest.main()
