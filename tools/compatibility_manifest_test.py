#!/usr/bin/env python3

from __future__ import annotations

import copy
import re
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


TOOLS = Path(__file__).resolve().parent
ROOT = TOOLS.parent
sys.path.insert(0, str(TOOLS))

from compatibility_manifest import (  # noqa: E402
    REFERENCE_WORKFLOWS,
    ValidationError,
    load_json_strict,
    load_json_text_strict,
    validate_manifest_data,
    validate_reference_checkouts,
)


class CompatibilityManifestTest(unittest.TestCase):
    def setUp(self) -> None:
        self.manifest = load_json_strict(ROOT / "compatibility/current.json")
        self.schema = load_json_strict(ROOT / "compatibility/schema-v3.json")

    def validate(self, manifest: dict) -> None:
        validate_manifest_data(ROOT, manifest, self.schema)

    def test_current_manifest_is_valid(self) -> None:
        self.validate(self.manifest)

    def test_bootstrap_snapshot_command_matches_the_manifest(self) -> None:
        checks = [
            (ROOT / ".github/workflows/native-validation.yml", 'tools/check-bootstrap-snapshot.sh "$RUNNER_TEMP/trb"'),
            (ROOT / "tools/check-bootstrap-snapshot.sh", "--snapshot-version 4"),
        ]
        read_text = Path.read_text
        for path, command in checks:
            with self.subTest(path=path):
                def read_source(candidate, *args, **kwargs):
                    source = read_text(candidate, *args, **kwargs)
                    return source.replace(command, "missing-snapshot-command") if candidate == path else source

                with patch.object(Path, "read_text", read_source):
                    with self.assertRaisesRegex(ValidationError, "snapshotSchemaVersion"):
                        self.validate(self.manifest)

    def test_target_tokens_are_checked_in_the_runtime_source_owner(self) -> None:
        runtime = ROOT / "compiler/src/backend/qbe/runtime/system.trb"
        read_text = Path.read_text
        source = read_text(runtime, encoding="utf-8")
        profile = self.manifest["targets"][0]["profile"]
        self.assertIn(profile, source)
        for changed in ("", source.replace(profile, "unknown-profile")):
            with self.subTest(empty=not changed):
                def read_source(path, *args, **kwargs):
                    return changed if path == runtime else read_text(path, *args, **kwargs)

                with patch.object(Path, "read_text", read_source):
                    with self.assertRaisesRegex(ValidationError, "self-hosted target profile"):
                        self.validate(self.manifest)

    def test_unknown_and_missing_members_are_rejected(self) -> None:
        unknown = copy.deepcopy(self.manifest)
        unknown["implicitSupport"] = True
        with self.assertRaisesRegex(ValidationError, "unknown member"):
            self.validate(unknown)

        missing = copy.deepcopy(self.manifest)
        del missing["runtimeABI"]
        with self.assertRaisesRegex(ValidationError, "missing required member"):
            self.validate(missing)

    def test_duplicate_members_are_rejected_before_validation(self) -> None:
        duplicate = '{"schemaVersion":1,"schemaVersion":2}'
        with self.assertRaisesRegex(ValidationError, "duplicate JSON object member"):
            load_json_text_strict(duplicate, "duplicate fixture")

    def test_only_exact_typerb_support_is_allowed(self) -> None:
        ranged = copy.deepcopy(self.manifest)
        ranged["typeRB"]["supportMode"] = "range"
        with self.assertRaisesRegex(ValidationError, "expected constant 'exact'"):
            self.validate(ranged)

    def test_canonical_repository_identities_must_match(self) -> None:
        mutations = [
            ("Native version", lambda value: value.update(nativeVersion="0.1.1-dev"), "NATIVE_VERSION"),
            (
                "TypeRB revision",
                lambda value: value["typeRB"].update(revision="a" * 40),
                "TYPE_RB_REVISION",
            ),
            (
                "snapshot schema",
                lambda value: value["bootstrap"].update(snapshotSchemaVersion=5),
                "bootstrap snapshot",
            ),
            (
                "target profile",
                lambda value: value["targets"][0].update(profile="darwin-arm64-v1"),
                "target profiles",
            ),
            (
                "backend identity",
                lambda value: value["backend"].update(version="1.4"),
                "backend identity",
            ),
        ]
        for label, mutate, message in mutations:
            with self.subTest(label=label):
                changed = copy.deepcopy(self.manifest)
                mutate(changed)
                with self.assertRaisesRegex(ValidationError, message):
                    self.validate(changed)

    def test_evidence_paths_cannot_escape_the_repository(self) -> None:
        escaped = copy.deepcopy(self.manifest)
        escaped["evidence"]["compatibility"]["result"] = "results/../../README.md"
        with self.assertRaisesRegex(ValidationError, "escapes the repository"):
            self.validate(escaped)

    def test_current_target_profiles_are_unique(self) -> None:
        duplicated = copy.deepcopy(self.manifest)
        duplicate = copy.deepcopy(duplicated["targets"][-1])
        duplicate["qbeTarget"] = "amd64_apple"
        duplicated["targets"].append(duplicate)
        with self.assertRaisesRegex(ValidationError, "target profiles must be unique"):
            self.validate(duplicated)

    def test_recovered_targets_require_exact_target_chain_evidence(self) -> None:
        mutations = [
            lambda value: value["evidence"]["targetChains"].clear(),
            lambda value: value["evidence"]["targetChains"][0].update(
                profile="linux-riscv64-v0"
            ),
        ]
        for mutate in mutations:
            changed = copy.deepcopy(self.manifest)
            mutate(changed)
            with self.assertRaisesRegex(ValidationError, "exactly cover"):
                self.validate(changed)


class ReferenceCheckoutTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.revision = (ROOT / "TYPE_RB_REVISION").read_text().strip()
        self.files = {}
        for relative in [*(f".github/workflows/{name}" for name in REFERENCE_WORKFLOWS),
                         "tools/linux-amd64-targets.sh"]:
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / relative, path)
            self.files[path] = path.read_text()

    def validate(self) -> None:
        validate_reference_checkouts(self.root, self.revision)

    def reject(self, path: Path, source: str) -> None:
        with self.subTest(path=path.name, source=source[:80]):
            try:
                path.write_text(source)
                with self.assertRaises(ValidationError):
                    self.validate()
            finally:
                path.write_text(self.files[path])

    def test_all_current_and_frozen_consumers_are_accepted(self) -> None:
        self.validate()

    def test_current_builds_cannot_lose_explicit_version_identity(self) -> None:
        for name, (mode, _) in REFERENCE_WORKFLOWS.items():
            if mode not in ("direct", "derived", "environment"):
                continue
            path = self.root / ".github/workflows" / name
            source = self.files[path]
            self.reject(path, source.replace('tools/build-reference.py', 'tools/old-build-reference.py'))

    def test_each_checkout_ref_cannot_change_or_disappear(self) -> None:
        checked = 0
        for name, (_, count) in REFERENCE_WORKFLOWS.items():
            path = self.root / ".github/workflows" / name
            source = self.files[path]
            refs = list(re.finditer(
                r"(?m)^ +repository: type-rb/type-rb\n +ref: ([^\n]+)", source))
            self.assertEqual(len(refs), count)
            for ref in refs:
                for value in ("main", "a" * 40, ""):
                    self.reject(path, source[:ref.start(1)] + value + source[ref.end(1):])
                self.reject(path, source[:ref.start(1)].rsplit("\n", 1)[0] +
                            source[ref.end(1):])
                checked += 1
        self.assertEqual(checked, sum(count for _, count in REFERENCE_WORKFLOWS.values()))

    def test_environment_pins_retain_current_or_historical_identity(self) -> None:
        for name, (mode, _) in REFERENCE_WORKFLOWS.items():
            if mode in ("direct", "derived"):
                continue
            path = self.root / ".github/workflows" / name
            source = self.files[path]
            for value in ("a" * 40, ""):
                self.reject(path, re.sub(r"(?m)^( *TYPE_RB_REVISION: *)\S+", rf"\g<1>{value}", source))
            if mode != "environment":
                self.reject(path, source.replace(mode, self.revision))

    def test_derived_pins_require_the_canonical_producer_before_checkout(self) -> None:
        producer = 'run: echo "revision=$(cat TYPE_RB_REVISION)" >> "$GITHUB_OUTPUT"'
        for name, (mode, _) in REFERENCE_WORKFLOWS.items():
            if mode != "derived":
                continue
            path = self.root / ".github/workflows" / name
            source = self.files[path]
            for old, new in (("id: reference", "id: other"),
                             (producer, producer.replace("cat TYPE_RB_REVISION", "echo main")),
                             (producer, "# " + producer)):
                self.reject(path, source.replace(old, new))
            self.reject(path, source.replace(producer, "run: true") + "\n" + producer + "\n")

    def test_identity_checks_cannot_be_removed_or_commented_out(self) -> None:
        for path, source in self.files.items():
            identities = list(re.finditer(
                r"(?m)^([ \t]*(?:run: )?)(test [^\n]* = [^\n]*)$",
                source))
            self.assertTrue(identities, path.name)
            for identity in identities:
                if identity[2].endswith("\\"):
                    continue
                if not any(token in identity[2] for token in
                           ("TYPE_RB_REVISION", "git -C .type-rb rev-parse HEAD")):
                    continue
                for prefix in ("# ", ": removed # "):
                    # A shell no-op containing an old assertion is not a check.
                    self.reject(path, source[:identity.start(2)] + prefix + source[identity.start(2):])

    def test_missing_consumers_and_controller_fail_deterministically(self) -> None:
        for path, source in self.files.items():
            with self.subTest(path=path.name):
                try:
                    path.unlink()
                    with self.assertRaises(ValidationError):
                        self.validate()
                finally:
                    path.write_text(source)
        controller = self.root / "tools/linux-amd64-targets.sh"
        self.reject(controller, self.files[controller].replace(self.revision, "a" * 40))

    def test_added_and_duplicate_checkout_consumers_require_registration(self) -> None:
        path = self.root / ".github/workflows/new.yml"
        for source in ('repository: type-rb/type-rb\nref: main\n',
                       'env:\n  ORACLE_REPOSITORY: type-rb/type-rb\n'):
            path.write_text(source)
            with self.assertRaisesRegex(ValidationError, "inventory differs"):
                self.validate()
        path.unlink()
        known = self.root / ".github/workflows/native-validation.yml"
        self.reject(known, self.files[known] + '\n      - with:\n          repository: type-rb/type-rb\n          ref: ' + self.revision + '\n')


if __name__ == "__main__":
    unittest.main()
