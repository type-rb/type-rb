#!/usr/bin/env python3
"""Reference builds reject stale source and ambiguous executable identities."""

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("build_reference", Path(__file__).with_name("build-reference.py"))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class ReferenceBuildTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / "internal/cli/cli.go"
        self.source.parent.mkdir(parents=True)
        self.source.write_text('package cli\nvar Version = "1.2.3-dev"\n')
        self.identity = {"revision": "a" * 40, "version": "1.2.3"}
        self.output = self.root / "trb"

    def build(self, revision=None, dirty="", reported="1.2.3\n", stderr="", trimpath=False):
        with patch.object(builder.subprocess, "check_output", side_effect=[
                (revision or self.identity["revision"]) + "\n", dirty]), patch.object(
                    builder.subprocess, "run", side_effect=[
                        subprocess.CompletedProcess([], 0),
                        subprocess.CompletedProcess([], 0, reported, stderr)]) as run:
            builder.build(self.root, self.output, self.identity, trimpath)
            return run.call_args_list[0].args[0]

    def test_release_source_embeds_stable_version(self):
        command = self.build()
        self.assertEqual(command[command.index("-ldflags") + 1],
                         "-X github.com/type-rb/type-rb/internal/cli.Version=1.2.3")
        self.assertNotIn("-trimpath", command)
        self.assertIn("-trimpath", self.build(trimpath=True))

    def test_rejects_wrong_revision(self):
        with self.assertRaisesRegex(builder.ValidationError, "TYPE_RB_REVISION"):
            self.build(revision="b" * 40)

    def test_rejects_modified_checkout(self):
        with self.assertRaisesRegex(builder.ValidationError, "uncommitted"):
            self.build(dirty=" M internal/cli/cli.go\n")

    def test_rejects_wrong_source_version(self):
        self.source.write_text('var Version = "1.2.4-dev"\n')
        with self.assertRaisesRegex(builder.ValidationError, "source version"):
            self.build()

    def test_rejects_wrong_executable_version(self):
        with self.assertRaisesRegex(builder.ValidationError, "executable"):
            self.build(reported="1.2.3-dev\n")
        with self.assertRaisesRegex(builder.ValidationError, "executable"):
            self.build(stderr="version warning\n")


if __name__ == "__main__":
    unittest.main()
