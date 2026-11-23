#!/usr/bin/env python3
"""Test project scenario replay with stub compilers."""
import importlib.util
import json
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

TOOL = Path(__file__).with_name("project-scenarios.py")
spec = importlib.util.spec_from_file_location("project_scenarios", TOOL)
scenarios = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scenarios)

REFERENCE = textwrap.dedent("""\
    #!/usr/bin/env python3
    import sys
    command = sys.argv[1]
    if command == "check":
        print("checked 1 file(s) for mode go")
    elif command == "run":
        print("hello")
    elif command == "test":
        print("PASS Suite / case\\nFAIL Suite / other\\n  <fixture>/app/src/a_test.trb:3:3: expected main.Point{X:1} to equal main.Point{X:2}\\n\\n2 test(s), 1 failure(s)\\nexit status 1")
        sys.exit(1)
    else:
        sys.exit("trb: invalid project configuration")
    """)
NATIVE = textwrap.dedent("""\
    #!/usr/bin/env python3
    import re, sys
    from pathlib import Path
    mode = re.search(r'"mode"\\s*:\\s*"([^"]*)"', Path("trbconfig.jsonc").read_text()).group(1)
    command = sys.argv[1]
    if command == "check":
        print("ok")
    elif command == "run":
        if mode != "trb":
            sys.exit("run needs mode trb")
        print("hello")
    elif command == "test":
        print("PASS Suite / case\\nFAIL Suite / other\\n  a_test.trb:3: expected Point(x: 1) to equal Point(x: 2)\\n\\n2 test(s), 1 failure(s)")
        sys.exit(1)
    elif command == "lint":
        sys.exit("trbn: unknown command lint")
    else:
        sys.exit("trbn: invalid project configuration")
    """)


class ProjectScenarioTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        root = Path(self.directory.name)
        self.fixture = root / "fixture"
        (self.fixture / "app").mkdir(parents=True)
        (self.fixture / "app" / "trbconfig.jsonc").write_text('{\n  // comment\n  "name": "demo",\n  "mode": "go"\n}\n')
        self.reference = root / "reference"
        self.native = root / "native"
        for path, source in ((self.reference, REFERENCE), (self.native, NATIVE)):
            path.write_text(source)
            path.chmod(0o755)
        self.manifest = root / "m9-manifest.json"

    def tearDown(self):
        self.directory.cleanup()

    def write(self, expectation, steps, reference):
        self.manifest.write_text(json.dumps({"schemaVersion": 1, "milestone": "M9", "typeRBRevision": "0" * 40,
            "scenarios": [{"id": "demo", "fixture": str(self.fixture), "cwd": "app", "steps": steps,
                           "expectation": expectation, "reference": reference}]}))

    def run_tool(self, role, binary, *extra):
        return subprocess.run([sys.executable, str(TOOL), role, str(binary), "--manifest", str(self.manifest), *extra],
                              capture_output=True, text=True)

    def recorded(self, *steps):
        observed = scenarios.replay(self.reference, {"id": "demo", "fixture": str(self.fixture), "cwd": "app",
                                                     "steps": [list(step) for step in steps]}, False)
        return observed

    def test_reference_replay_requires_recorded_outcomes(self):
        reference = self.recorded(("check",), ("run",))
        self.write("match-reference", [["check"], ["run"]], reference)
        self.assertEqual(self.run_tool("--reference", self.reference).returncode, 0)
        reference[1]["stdout"] = "changed\n"
        self.write("match-reference", [["check"], ["run"]], reference)
        result = self.run_tool("--reference", self.reference)
        self.assertEqual(result.returncode, 1)
        self.assertIn("M9:demo", result.stderr)

    def test_native_checks_as_written_and_runs_in_trb_mode(self):
        self.write("match-reference", [["check"], ["run"]], self.recorded(("check",), ("run",)))
        result = self.run_tool("--native", self.native, "--require")
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual((report["manifests"][0]["passed"], report["manifests"][0]["total"]), (1, 1))
        self.assertIn("mode", (self.fixture / "app" / "trbconfig.jsonc").read_text())
        self.assertIn('"go"', (self.fixture / "app" / "trbconfig.jsonc").read_text())

    def test_native_acceptance_and_output_mismatches_fail_only_when_required(self):
        reference = self.recorded(("check",), ("run",))
        reference[1]["stdout"] = "different\n"
        self.write("match-reference", [["check"], ["run"]], reference)
        report_only = self.run_tool("--native", self.native)
        self.assertEqual(report_only.returncode, 0)
        self.assertEqual(json.loads(report_only.stdout)["manifests"][0]["scenarios"][0]["status"], "fail")
        self.assertEqual(self.run_tool("--native", self.native, "--require").returncode, 1)
        self.write("match-reference-check", [["check"], ["run"]], reference)
        self.assertEqual(self.run_tool("--native", self.native, "--require").returncode, 0)

    def test_test_steps_ignore_go_exit_lines_and_status_rendering(self):
        reference = self.recorded(("test",))
        self.write("match-reference-status", [["test"]], reference)
        self.assertEqual(self.run_tool("--native", self.native, "--require").returncode, 0)
        self.write("match-reference", [["test"]], reference)
        self.assertEqual(self.run_tool("--native", self.native, "--require").returncode, 1)
        self.assertEqual(scenarios.without_go_exit("PASS a\nexit status 1"), "PASS a")

    def test_rejections_for_missing_support_do_not_match(self):
        self.write("match-reference", [["lint"]], self.recorded(("lint",)))
        report = json.loads(self.run_tool("--native", self.native).stdout)
        entry = report["manifests"][0]["scenarios"][0]
        self.assertEqual(entry["status"], "fail")
        self.assertIn("missing support", entry["mismatches"][0])
        self.write("match-reference", [["install"]], self.recorded(("install",)))
        self.assertEqual(self.run_tool("--native", self.native, "--require").returncode, 0)

    def test_triage_scenarios_are_reported_separately(self):
        self.write("triage", [["check"]], self.recorded(("check",)))
        report = json.loads(self.run_tool("--native", self.native, "--require").stdout)
        self.assertEqual(report["manifests"][0]["scenarios"][0]["status"], "triage")

    def test_current_reference_keeps_every_historical_command(self):
        historical = dict((path.name, manifest) for path, manifest in scenarios.load([]))
        current = dict((path.name, manifest) for path, manifest in scenarios.load([], True))
        self.assertIn("m2-manifest.json", historical)
        self.assertNotIn("m2-manifest.json", current)
        old = historical["m2-manifest.json"]["scenarios"]
        new = current["reference-test-manifest.json"]["scenarios"]
        self.assertEqual([row["id"] for row in old], [row["id"] for row in new])
        for before, after in zip(old, new):
            expected = json.loads(json.dumps(before))
            if expected["id"] == "assert-structural-failure":
                expected["reference"][0]["stdout"] = expected["reference"][0]["stdout"].replace(
                    "application.Point{X:1, Y:2}", "Point(x: 1, y: 2)").replace(
                    "application.Point{X:1, Y:3}", "Point(x: 1, y: 3)")
            self.assertEqual(after, expected)

    def test_repository_manifests_are_valid(self):
        loaded = scenarios.load([])
        self.assertTrue(loaded)
        for _, manifest in loaded:
            for scenario in manifest["scenarios"]:
                self.assertTrue((scenarios.ROOT / scenario["fixture"] / scenario["cwd"]).is_dir(), scenario["id"])
                self.assertLessEqual(len(manifest["scenarios"]), 20)


if __name__ == "__main__":
    unittest.main()
