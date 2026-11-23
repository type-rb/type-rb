#!/usr/bin/env python3
"""Focused tests for missing measurements and the weekly CI window."""

from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import unittest

from report import ci_summary, daily_summary, load_coverage, markdown, parity_summary, report


NOW = datetime(2026, 9, 24, 12, 0, tzinfo=timezone.utc)


def run(conclusion, started, finished, created=None):
    return {"event": "pull_request", "status": "completed", "conclusion": conclusion,
            "createdAt": (created or started).isoformat(), "startedAt": started.isoformat(),
            "updatedAt": finished.isoformat()}


class DevelopmentProgressTest(unittest.TestCase):
    def test_ci_window_includes_failed_runs_and_separates_queue_time(self):
        runs = [run("success", NOW - timedelta(hours=2), NOW - timedelta(hours=1),
                    NOW - timedelta(hours=2, minutes=30)),
                run("failure", NOW - timedelta(hours=4), NOW - timedelta(hours=2),
                    NOW - timedelta(hours=5)),
                run("success", NOW - timedelta(days=9), NOW - timedelta(days=8))]
        result = ci_summary(runs, NOW)
        self.assertEqual(result["runs"], 2)
        self.assertEqual(result["conclusions"], {"success": 1, "failure": 1})
        self.assertEqual(result["executionMedianSeconds"], 5400)
        self.assertEqual(result["executionP90Seconds"], 7200)
        self.assertEqual(result["feedbackMedianSeconds"], 8100)

    def test_ci_limit_marks_only_a_truncated_week(self):
        recent = run("success", NOW - timedelta(hours=2), NOW - timedelta(hours=1))
        self.assertTrue(ci_summary([recent] * 300, NOW)["sampleCapped"])
        older = run("success", NOW - timedelta(days=9), NOW - timedelta(days=8))
        self.assertFalse(ci_summary([recent] * 299 + [older], NOW)["sampleCapped"])

    def test_daily_ratio_requires_both_passing_rows_from_one_snapshot(self):
        state = {"schemaVersion": 1, "latest": {
            "revision": "a" * 40, "at": NOW.isoformat(), "status": "measured-with-failures",
            "compilerSelf": {"status": "pass", "build": {"wallSeconds": 6.25},
                             "ir": {"bytes": 1024}},
            "pureGoCases": ["n-body", "spectral-norm"],
            "rows": [{"case": "n-body", "role": "native", "status": "pass",
                      "runtime": {"wallSeconds": 2.0}},
                     {"case": "n-body", "role": "pure-go", "status": "pass",
                      "runtime": {"wallSeconds": 1.0}},
                     {"case": "spectral-norm", "role": "native", "status": "timeout"},
                     {"case": "spectral-norm", "role": "pure-go", "status": "pass",
                      "runtime": {"wallSeconds": 1.0}}]}}
        result = daily_summary(state)
        self.assertEqual(result["selfBuildSeconds"], 6.25)
        self.assertEqual(result["pureGoRatios"], {"n-body": 2.0, "spectral-norm": None})

    def test_no_daily_artifact_is_explicitly_unavailable(self):
        registry = json.loads((Path(__file__).resolve().parents[1] /
                               "native-language-cases.json").read_text())
        coverage = load_coverage()
        parity = parity_summary(registry, coverage)
        self.assertGreater(parity["probes"], 0)
        self.assertLessEqual(parity["different"], parity["probes"])
        result = report(registry, {"schemaVersion": 1, "latest": None}, [], NOW, "a" * 40)
        self.assertIsNone(result["daily"]["selfBuildSeconds"])
        self.assertIn("unavailable", markdown(result))
        self.assertIn("0 completed runs", markdown(result))
        self.assertIn("does not cover the current main revision", markdown(result))


if __name__ == "__main__":
    unittest.main()
