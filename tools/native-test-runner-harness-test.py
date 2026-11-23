#!/usr/bin/env python3
"""The M2 acceptance gate must reject false parity and preserve deferrals."""

import copy
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("native_test_runner", Path(__file__).with_name("native-test-runner-test.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
values_spec = importlib.util.spec_from_file_location("native_test_values", Path(__file__).with_name("native-test-values-test.py"))
values = importlib.util.module_from_spec(values_spec)
values_spec.loader.exec_module(values)


class AcceptanceTest(unittest.TestCase):
    def setUp(self):
        self.manifest = runner.load_manifest()
        self.rows = {row["id"]: row for row in self.manifest["scenarios"]}

    def altered(self, name, **values):
        row = self.rows[name]
        observations = copy.deepcopy(row["reference"])
        observations[0].update(values)
        return runner.compare(row, observations)

    def test_reviewed_observations_pass(self):
        for row in self.rows.values():
            with self.subTest(row=row["id"]):
                self.assertEqual(runner.compare(row, row["reference"]), [])

    def test_unrelated_rejection_never_passes_a_negative(self):
        for name in ("test-compile-error", "test-namespaced-import-rejected", "test-nonliteral-name-rejected", "test-no-tests", "test-selection-invalid"):
            with self.subTest(row=name):
                self.assertTrue(self.altered(name, stderr="trbn: cannot load standard module\n"))
                self.assertTrue(self.altered(name, code=2))

    def test_origin_is_required_even_with_same_failure_status(self):
        row = self.rows["assert-structural-failure"]
        stdout = row["reference"][0]["stdout"]
        self.assertTrue(self.altered(row["id"], stdout=stdout.replace(":15:3:", ":1:1:")))
        self.assertTrue(self.altered(row["id"], stdout="FAIL Structure / record\n\n1 test(s), 1 failure(s)\n"))
        self.assertTrue(self.altered(row["id"], stdout=stdout.replace("Point(x: 1, y: 2)", "application.Point{X:1, Y:2}")))
        self.assertTrue(self.altered(row["id"], stdout=stdout.replace("Point(x: 1, y: 2)", "Point(x: 9, y: 2)")))

    def test_json_allows_key_order_but_not_lost_origins_or_events(self):
        name = "test-json-reporter"
        events = [json.loads(line) for line in self.rows[name]["reference"][0]["stdout"].splitlines()]
        encode = lambda items: "\n".join(json.dumps(item) for item in items) + "\n"
        self.assertEqual(self.altered(name, stdout=encode(events)), [])
        self.assertTrue(self.altered(name, stdout=encode(events[:-1])))
        events[0].pop("column")
        self.assertTrue(self.altered(name, stdout=encode(events)))

    def test_steps_and_command_cannot_be_substituted(self):
        row = self.rows["test-positional-file"]
        self.assertTrue(runner.compare(row, []))
        self.assertTrue(runner.compare(row, row["reference"] * 2))
        self.assertTrue(self.altered(row["id"], args=["check"]))

    def test_missing_case_extra_deferral_and_changed_fixture_fail_closed(self):
        variants = []
        removed = copy.deepcopy(self.manifest)
        removed["scenarios"].pop()
        variants.append(removed)
        waived = copy.deepcopy(self.manifest)
        waived["deferrals"].append({"id": "test-no-tests"})
        variants.append(waived)
        changed = copy.deepcopy(self.manifest)
        changed["fixtureHashes"][next(iter(changed["fixtureHashes"]))] = "0" * 64
        variants.append(changed)
        outcome = copy.deepcopy(self.manifest)
        outcome["scenarios"][0]["reference"][0]["code"] = 1
        variants.append(outcome)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            for variant in variants:
                path.write_text(json.dumps(variant))
                with self.assertRaises(ValueError):
                    runner.load_manifest(path)


class ValueContractTest(unittest.TestCase):
    def test_frozen_value_groups_retain_exact_sources_and_counts(self):
        contract = values.load_contract()
        self.assertEqual(set(contract), {row[0] for row in values.GROUPS})

    def test_missing_group_changed_source_and_status_fail_closed(self):
        baseline = json.loads(values.CONTRACT.read_text())
        variants = []
        missing = copy.deepcopy(baseline)
        del missing["groups"]["results"]
        variants.append(missing)
        changed = copy.deepcopy(baseline)
        changed["groups"]["scalars"]["sourceSha256"] = "0" * 64
        variants.append(changed)
        status = copy.deepcopy(baseline)
        status["groups"]["scalars"]["code"] = 0
        variants.append(status)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "contract.json"
            for variant in variants:
                path.write_text(json.dumps(variant))
                with self.assertRaises(ValueError):
                    values.load_contract(path)

    def test_events_origins_and_native_stderr_are_required(self):
        expected = values.load_contract()["scalars"]
        observed = dict(copy.deepcopy(expected), stderr="")
        values.compare_observation("scalars", observed, expected, native=True)
        del observed["events"][0]["column"]
        with self.assertRaises(ValueError):
            values.compare_observation("scalars", observed, expected, native=True)
        observed = dict(copy.deepcopy(expected), stderr="exit status 1\n")
        with self.assertRaises(ValueError):
            values.compare_observation("scalars", observed, expected, native=True)

    def test_native_only_controller_needs_no_reference_executable(self):
        # Synthetic orchestration control, not compiler/runtime feature evidence.
        expected = values.load_contract()["scalars"]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fake = root / "native"
            fake.write_text("#!" + sys.executable + "\nimport hashlib,json,sys\nfrom pathlib import Path\n"
                            "assert sys.argv[1:] == ['test','--reporter','json']\n"
                            "assert json.loads(Path('trbconfig.jsonc').read_text())['mode'] == 'trb'\n"
                            "assert hashlib.sha256(Path('src/sample_test.trb').read_bytes()).hexdigest() == "
                            + repr(expected["sourceSha256"]) + "\n"
                            "events = " + repr(expected["events"]) + "\n"
                            "for event in events: print(json.dumps(event))\n"
                            "raise SystemExit(" + str(expected["code"]) + ")\n")
            fake.chmod(0o755)
            report = root / "report.json"
            result = subprocess.run([sys.executable, str(values.ROOT / "tools/native-test-values-test.py"),
                                     "--native", str(fake), "--group", "scalars", "--report", str(report)],
                                    capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            row = json.loads(report.read_text())["observations"][0]
            self.assertNotIn("reference", row)
            self.assertEqual(row["native"]["events"], expected["events"])


if __name__ == "__main__":
    unittest.main()
