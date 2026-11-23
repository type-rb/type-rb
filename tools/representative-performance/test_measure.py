"""Controller guarantees: wrong output, child failures and sparse tails stay visible."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("representative_measure", Path(__file__).with_name("measure.py"))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)
report_spec = importlib.util.spec_from_file_location("representative_report", Path(__file__).with_name("report.py"))
report = importlib.util.module_from_spec(report_spec)
report_spec.loader.exec_module(report)


class MeasurementTests(unittest.TestCase):
    def test_summary_preserves_every_batch_and_final_retained_checksum(self):
        _, cases = measure.load_cases(smoke=True)
        for original in cases:
            case = dict(original, batches=12)
            with self.subTest(case=case["id"]):
                lines = measure.expected(case).decode().splitlines()
                total = sum(int(line.split(",")[1]) for line in lines[1:-1]) % 1000000007
                expected = f"ready\nsummary,{total}\n{lines[-1]}\n".encode()
                self.assertEqual(measure.expected(case, "summary"), expected)
        with self.assertRaises(ValueError):
            measure.expected(cases[0], "unknown")

    def test_summary_output_remains_bounded_for_long_repetition(self):
        case = {"workload": "arrays", "variant": "reverse", "count": 3, "batches": 200000}
        self.assertEqual(measure.expected(case, "summary"), b"ready\nsummary,2800000\ndone,0\n")

    def test_profiles_preserve_registered_cases_and_fix_batches_before_execution(self):
        suite, original = measure.load_cases()
        _, sustained = measure.load_cases(profile="sustained-v1")
        self.assertEqual(original, suite["cases"])
        for before, after in zip(original, sustained):
            self.assertEqual({k: v for k, v in before.items() if k != "batches"},
                             {k: v for k, v in after.items() if k != "batches"})
            multiplier = 1 if before["id"] == "worker-medium-mixed" else {"small": 200, "medium": 40, "large": 8}[before["size"]]
            self.assertEqual(after["batches"], before["batches"] * multiplier)
        self.assertEqual({"schemaVersion": 1, "cases": suite["cases"], "inputProfiles": suite["inputProfiles"]},
                         json.loads((measure.ROOT / "docs/capabilities/benchmarks/representative-suite.json").read_text()))
        for kwargs in ({"profile": "typo"}, {"profile": "sustained-v1", "smoke": True}):
            with self.assertRaises(ValueError):
                measure.load_cases(**kwargs)

    def test_sample_ranges_preserve_every_retained_value_and_exclude_first(self):
        def record(phase, wall, cpu, rss):
            return {"phase": phase, "status": "pass", "wallSeconds": wall, "cpuSeconds": cpu,
                    "peakRssBytes": rss, "arrivals": [{"line": "ready", "seconds": 0, "coalesced": False}]}
        records = [record("warmup", 100, 99, 10000), record("retained", 2, 0, 10), record("retained", 4, 3, 40)]
        summary = measure.summarize(records, 16)
        self.assertEqual(summary["firstProcessWallSeconds"], 100)
        self.assertEqual(summary["sampleRanges"]["wallSeconds"], {"count": 2, "minimum": 2, "p50": 2, "maximum": 4})
        self.assertEqual(summary["sampleRanges"]["cpuSeconds"], {"count": 2, "minimum": 0, "p50": 1.5, "maximum": 3})
        self.assertEqual(summary["sampleRanges"]["unitsPerSecond"], {"count": 2, "minimum": 4, "p50": 8, "maximum": 8})
        records[-1]["status"] = "nonzero-exit"
        self.assertIsNone(measure.summarize(records, 16))

    def test_tail_resolution(self):
        self.assertIsNone(measure.percentile(list(range(19)), 0.95))
        self.assertIsNone(measure.percentile(list(range(99)), 0.99))
        self.assertEqual(measure.percentile(list(range(1, 101)), 0.99), 99)
        self.assertEqual(measure.percentile(list(range(1, 21)), 0.95), 19)

    def test_smoke_covers_every_path_without_large_inputs(self):
        _, full = measure.load_cases()
        _, smoke = measure.load_cases(True)
        self.assertEqual({(c["workload"], c["variant"]) for c in full},
                         {(c["workload"], c["variant"]) for c in smoke})
        self.assertTrue(all(c["count"] == 23 and c["batches"] == 2 for c in smoke))
        self.assertEqual(len({c["id"] for c in full}), len(full))
        with self.assertRaises(ValueError):
            measure.load_cases(selected=["typo"])

    def test_known_small_outputs(self):
        base = {"count": 3, "batches": 1}
        self.assertEqual(measure.expected(dict(base, workload="hash", variant="hits")), b"ready\n0,3\ndone,0\n")
        self.assertEqual(measure.expected(dict(base, workload="hash", variant="miss-heavy")), b"ready\n0,2\ndone,0\n")
        self.assertEqual(measure.expected(dict(base, workload="arrays", variant="reverse")), b"ready\n0,14\ndone,0\n")
        self.assertEqual(measure.expected(dict(base, workload="json-transform", variant="mixed")), b'ready\n0,47\ndone,0\n')

    def test_document_width_oracles(self):
        base = {"count": 2, "batches": 1, "workload": "json-document"}
        for variant, length in (("ascii", 14), ("unicode", 8), ("escaped", 60),
                                ("wide-array", 13), ("wide-object", 29),
                                ("integer-array", 9), ("fractional-array", 11),
                                ("mixed-number-object", 25),
                                ("precise-fractional-array", 41)):
            with self.subTest(variant=variant):
                self.assertEqual(measure.expected(dict(base, variant=variant)),
                                 f"ready\n0,{length}\ndone,0\n".encode())
        self.assertEqual(measure.expected(dict(base, count=3, variant="mixed-number-object")),
                         b"ready\n0,39\ndone,0\n")

    def test_output_and_exit_failures(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            good = measure.observe([sys.executable, "-c", "print('ok')"], root / "good", 3)
            self.assertEqual(measure.validate(good, root / "good", b"ok\n")["status"], "pass")
            self.assertGreater(good["peakRssBytes"], 0)
            self.assertEqual(measure.validate(good, root / "good", b"wrong\n")["status"], "output-mismatch")
            failed = measure.observe([sys.executable, "-c", "raise SystemExit(7)"], root / "exit", 3)
            self.assertEqual(failed["status"], "nonzero-exit")
            self.assertEqual(failed["exitCode"], 7)
            noisy = measure.observe([sys.executable, "-c", "import sys; print('warning', file=sys.stderr)"], root / "noise", 3)
            self.assertEqual(measure.validate(noisy, root / "noise", b"")["status"], "unexpected-stderr")

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux exec RSS accounting")
    def test_linux_rss_excludes_inflated_observer_and_still_tracks_child_memory(self):
        # Touch real pages in the observer. A direct wait4 measurement of an
        # exec child incorrectly reports this memory as the child's RSS floor.
        observer = bytearray(128 * 1024 * 1024)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            small = measure.observe([sys.executable, "-c", "data=bytearray(4*1024*1024);print('ok')"], root / "small", 5)
            large = measure.observe([sys.executable, "-c", "data=bytearray(64*1024*1024);print('ok')"], root / "large", 5)
            self.assertEqual(small["status"], "pass")
            self.assertEqual(large["status"], "pass")
            self.assertEqual(small["peakRssMethod"], "gnu-time-child-v1")
            self.assertLess(small["peakRssBytes"], len(observer) // 2)
            self.assertGreater(large["peakRssBytes"], small["peakRssBytes"] + 32 * 1024 * 1024)

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux resource sidecar")
    def test_linux_missing_rss_is_not_a_successful_measurement(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(measure.Path, "exists", return_value=False):
            result = measure.observe([sys.executable, "-c", "print('ok')"], Path(temporary) / "missing", 3)
        self.assertEqual(result["status"], "missing-resource-observation")
        self.assertIsNone(result["peakRssBytes"])

    def test_descendant_with_open_pipe_has_a_deadline(self):
        with tempfile.TemporaryDirectory() as temporary:
            code = "import subprocess,sys; subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)'])"
            result = measure.observe([sys.executable, "-c", code], Path(temporary) / "timeout", 0.15)
            self.assertEqual(result["status"], "timeout")
            self.assertLess(result["wallSeconds"], 3)

    def test_stdout_observer_delay_is_not_application_wall_time(self):
        selector = measure.selectors.DefaultSelector()
        original = selector.select

        def delayed_select(timeout):
            events = original(timeout)
            time.sleep(0.3)
            return events

        with tempfile.TemporaryDirectory() as temporary, patch.object(selector, "select", delayed_select), \
                patch.object(measure.selectors, "DefaultSelector", return_value=selector):
            result = measure.observe([sys.executable, "-c", "print('finished')"], Path(temporary) / "delayed", 3)
            self.assertEqual(result["status"], "pass")
            # The observation thread deliberately cannot read the line for
            # 300 ms. Process completion is captured independently before it.
            self.assertLess(result["wallSeconds"], result["arrivals"][0]["seconds"])

    def test_coalesced_markers_are_not_zero_latency_samples(self):
        record = {"phase": "retained", "status": "pass", "wallSeconds": 1,
                  "cpuSeconds": 0.5, "peakRssBytes": 123,
                  "arrivals": [{"line": "ready", "seconds": 0.1, "coalesced": True},
                               {"line": "0,3", "seconds": 0.1, "coalesced": True}]}
        result = measure.summarize([record], 10)
        self.assertIsNone(result["batchArrivalIntervalSeconds"])
        self.assertIsNone(result["processWallSeconds"]["p99"])
        record["status"] = "output-mismatch"
        self.assertIsNone(measure.summarize([record], 10))

    def test_failed_or_unrun_cases_are_visible_in_report(self):
        result = {"status": "failed", "platform": "synthetic", "cases": [],
                  "plannedCases": [{"id": "failed-case"}, {"id": "unrun-case"}],
                  "error": "budget exhausted"}
        rendered = report.render(result)
        self.assertIn("failed-case | unknown", rendered)
        self.assertIn("unrun-case | unknown", rendered)
        self.assertIn("budget exhausted", rendered)

    def test_report_excludes_legacy_linux_rss_but_preserves_timing(self):
        native = {"processWallSeconds": {"p50": 0.5}, "peakRssBytes": 64 * 1024 ** 2,
                  "batchArrivalIntervalSeconds": None}
        result = {"status": "pass", "platform": "Linux", "plannedCases": [{"id": "sample"}],
                  "cases": [{"input": {"id": "sample"}, "summaries": {"native": native}}]}
        self.assertIn("sample | 500.000 | unknown | unknown | unknown |", report.render(result))
        self.assertIn("Legacy Linux RSS", report.render(result))
        self.assertEqual(native["peakRssBytes"], 64 * 1024 ** 2)
        result["environment"] = {"peakRssMethod": "gnu-time-child-v1"}
        self.assertIn("sample | 500.000 | unknown | unknown | 64.00 |", report.render(result))
        result.update(platform="Darwin", environment={})
        self.assertIn("sample | 500.000 | unknown | unknown | 64.00 |", report.render(result))


if __name__ == "__main__":
    unittest.main()
