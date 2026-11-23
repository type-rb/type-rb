import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import qualify


class QualificationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Qualification Test")
        self.git("config", "user.email", "test@example.invalid")
        suite_path = "tools/daily-performance/suite.json"
        suite_bytes = (qualify.ROOT / suite_path).read_bytes()
        self.suite = json.loads(suite_bytes)
        paths = {suite_path, "tools/daily-performance/prepare-compilers.sh",
                 "tools/daily-performance/measure.py", "tools/daily-performance/state.py",
                 "tools/build-reference.py", "tools/bootstrap-seed-download.sh", "tools/bootstrap-seed-manifest.sh"}
        for case in self.suite["cases"]:
            paths.update(case[key] for key in ("source", "expectedFile", "pureGoSource") if key in case)
        for path in paths:
            self.put(path, (qualify.ROOT / path).read_bytes())
        self.put("TYPE_RB_REVISION", b"a" * 40 + b"\n")
        self.put("compiler/src/compiler.trb", b"original synthetic compiler\n")
        comparison = self.commit()
        self.put("TYPE_RB_REVISION", b"b" * 40 + b"\n")
        self.put("compiler/src/compiler.trb", b"current synthetic compiler\n")
        candidate = self.commit()
        self.put("tools/cumulative-performance/marker", b"qualification tooling\n")
        self.commit()
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        self.registration = {"task": 730, "candidate": candidate, "comparison": comparison,
                             "suiteSha256": qualify.digest(suite_bytes), "rows": 67, "timedObservations": 674}
        self.selection = qualify.select_sources(self.root, self.registration)
        self.snapshot, self.raw = self.observations()

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, stderr=subprocess.DEVNULL).decode().strip()

    def put(self, path, data):
        path = self.root / path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def commit(self):
        self.git("add", ".")
        self.git("commit", "-qm", "Synthetic qualification fixture")
        return self.git("rev-parse", "HEAD")

    def observations(self):
        raw, rows = [], []
        def series(case, role, kind, warmups, retained, timeout, wall=2.0, cpu=1.0, rss=100):
            values = dict(wallSeconds=wall, cpuSeconds=cpu, memoryBytes=rss)
            for phase, count in (("warmup", warmups), ("retained", retained)):
                raw.extend(dict(case=case, role=role, kind=kind, phase=phase,
                                timeoutSeconds=timeout, status="pass", exitCode=0, **values)
                           for _ in range(count))
            return dict(wallMin=wall, wallMax=wall, **values)
        for expected in self.selection["rows"]:
            row = {key: value for key, value in expected.items() if key != "gcProbe"}
            row.update(status="pass", artifactSha256="c" * 64, artifactBytes=200, strippedBytes=190,
                       coverage="GC observed" if expected["gcProbe"] else None)
            case, role = row["case"], row["role"]
            row["build"] = series(case, role, "build", 1, 3, 90)
            value = 3.0 if role == "previous" else 1.0 if role == "pure-go" else 2.0
            row["runtime"] = series(case, role, "runtime", 1, 5, 30, wall=value, cpu=value)
            rows.append(row)
        compiler = {"status": "pass", "sourceSha256": self.selection["compilerSourceSha256"],
                    "binarySha256": "d" * 64, "binaryBytes": 300}
        compiler["ir"] = series("compiler-self", "native", "compiler-ir", 0, 1, 90)
        compiler["ir"].update(bytes=500, sha256="e" * 64)
        compiler["build"] = series("compiler-self", "native", "compiler-self", 1, 2, 120)
        snapshot = {"revision": self.registration["candidate"], "status": "measured",
                    "platform": "Linux arm64 / ubuntu-24.04-arm", "samples": 5, "buildSamples": 3,
                    "roles": {role: {"revision": revision, "sha256": "d" * 64, "bytes": 300}
                              for role, revision in self.selection["roles"].items()},
                    "rows": rows, "compilerSelf": compiler}
        return snapshot, raw

    def assess(self):
        return qualify.assess(self.selection, self.snapshot, self.raw)

    def test_exact_sources_and_separate_reference_pins(self):
        self.assertEqual(self.selection["roles"]["previous"], self.registration["comparison"])
        self.assertEqual(self.selection["comparisonReference"], "a" * 40)
        self.assertEqual(self.selection["roles"]["typerb-go"], "b" * 40)
        self.assertEqual(len(self.selection["rows"]), 67)
        self.assertEqual(self.assess()["status"], "met")

    def test_rejects_nonexact_noncommit_and_unknown_sources(self):
        self.git("tag", "-am", "Synthetic tag", "test-tag", self.registration["candidate"])
        for value in ("HEAD", "--help", "ABCDEF" * 7, "0" * 40,
                      self.registration["candidate"][:12], self.git("rev-parse", "test-tag")):
            with self.subTest(value=value), self.assertRaises((ValueError, subprocess.CalledProcessError)):
                qualify.select_sources(self.root, {**self.registration, "candidate": value})

    def test_rejects_reversed_and_unrelated_history(self):
        with self.assertRaises(subprocess.CalledProcessError):
            qualify.select_sources(self.root, {**self.registration,
                "comparison": self.registration["candidate"], "candidate": self.registration["comparison"]})
        self.git("checkout", "-q", "--detach", self.registration["comparison"])
        self.put("unrelated", b"branch\n")
        other = self.commit()
        with self.assertRaises(subprocess.CalledProcessError):
            qualify.select_sources(self.root, {**self.registration, "comparison": other})

    def test_rejects_stale_production_and_changed_registration(self):
        with self.assertRaisesRegex(ValueError, "suite changed"):
            qualify.select_sources(self.root, {**self.registration, "suiteSha256": "0" * 64})
        self.put("compiler/src/compiler.trb", b"new production\n")
        self.commit()
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        with self.assertRaisesRegex(ValueError, "stale"):
            qualify.select_sources(self.root, self.registration)

    def test_rejects_changed_adjacent_workload(self):
        path = "benchmarks/features/string-scanning/src/main.trb"
        self.put(path, (self.root / path).read_bytes() + b"\n")
        candidate = self.commit()
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        with self.assertRaisesRegex(ValueError, "workload changed"):
            qualify.select_sources(self.root, {**self.registration, "candidate": candidate})

    def test_rejects_stale_workload_outside_benchmarks(self):
        path = "tools/runtime-memory-soak/workload.trb"
        self.put(path, (self.root / path).read_bytes() + b"\n")
        self.commit()
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        with self.assertRaisesRegex(ValueError, "workload inputs are stale"):
            qualify.select_sources(self.root, self.registration)

    def test_failed_warmup_missing_duplicate_and_extra_observations(self):
        original = copy.deepcopy(self.raw)
        for mutation in (lambda: self.raw[0].update(status="timeout"),
                         lambda: self.raw.pop(), lambda: self.raw.append(self.raw[0]),
                         lambda: self.raw[0].update(phase="retained"),
                         lambda: self.raw[0].update(timeoutSeconds=91),
                         lambda: self.raw[0].update(role="unexpected")):
            self.raw = copy.deepcopy(original)
            mutation()
            with self.assertRaises(ValueError):
                self.assess()

    def test_rejects_partial_mixed_or_forged_snapshot(self):
        original = copy.deepcopy(self.snapshot)
        for mutation in (lambda: self.snapshot.update(status="measured-with-failures"),
                         lambda: self.snapshot["roles"]["previous"].update(revision="a" * 40),
                         lambda: self.snapshot["rows"].pop(),
                         lambda: self.snapshot["rows"].__setitem__(1, self.snapshot["rows"][0]),
                         lambda: self.snapshot["rows"][0].update(sourceSha256="a" * 64),
                         lambda: self.snapshot["rows"][0]["runtime"].update(wallSeconds=0.1),
                         lambda: self.snapshot["compilerSelf"].update(binarySha256="a" * 64)):
            self.snapshot = copy.deepcopy(original)
            mutation()
            with self.assertRaises(ValueError):
                self.assess()

    def test_rejects_nonfinite_negative_or_boolean_values(self):
        for value in (float("nan"), float("inf"), -1, True):
            self.raw[0]["cpuSeconds"] = value
            with self.assertRaisesRegex(ValueError, "numeric"):
                self.assess()

    def set_scan(self, role, metric, value):
        row = next(row for row in self.snapshot["rows"] if row["case"] == "string-scanning" and row["role"] == role)
        row["runtime"][metric] = value
        if metric == "wallSeconds":
            row["runtime"].update(wallMin=value, wallMax=value)
        for record in self.raw:
            if (record["case"], record["role"], record["kind"]) == ("string-scanning", role, "runtime"):
                record[metric] = value

    def test_exact_thresholds_and_unmet_target_remain_distinct(self):
        self.set_scan("previous", "wallSeconds", 3.75)
        self.set_scan("native", "wallSeconds", 3.0)
        self.set_scan("previous", "cpuSeconds", 5)
        self.set_scan("native", "cpuSeconds", 4)
        self.set_scan("native", "memoryBytes", 110)
        self.assertEqual(self.assess()["status"], "met")
        self.set_scan("native", "memoryBytes", 111)
        result = self.assess()
        self.assertEqual(result["status"], "unmet")
        self.assertFalse(result["criteria"]["rssAtMost10PercentHigher"])
        self.set_scan("previous", "cpuSeconds", 0)
        with self.assertRaises(ValueError):
            self.assess()

    def test_missing_measurement_writes_invalid_evidence(self):
        output = self.root / "qualification.json"
        missing = str(self.root / "missing.json")
        result = subprocess.run([sys.executable, str(Path(qualify.__file__)), "assess",
                                 "--selection", missing, "--snapshot", missing, "--raw", missing,
                                 "--output", str(output)], check=False)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(json.loads(output.read_text())["status"], "invalid")


class WorkflowBoundaryTests(unittest.TestCase):
    def test_workflow_uses_exact_original_controllers_without_daily_state(self):
        workflow = (qualify.ROOT / ".github/workflows/cumulative-performance.yml").read_text()
        self.assertEqual(workflow.count("python3 tools/daily-performance/measure.py"), 1)
        self.assertEqual(workflow.count("/bin/sh tools/daily-performance/prepare-compilers.sh"), 1)
        self.assertIn('"$COMPARISON" "$RUNNER_TEMP/compilers"', workflow)
        self.assertIn("working-directory: ${{ runner.temp }}/qualification-source", workflow)
        self.assertIn("github.run_attempt == 1", workflow)
        self.assertIn("name: cumulative-performance-evidence", workflow)
        self.assertNotIn("state.py", workflow)
        self.assertNotIn("daily-performance-state", workflow)
        self.assertNotIn("schedule:", workflow)


if __name__ == "__main__":
    unittest.main()
