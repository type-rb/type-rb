"""Publication keeps coverage, failed attempts and source-qualified results distinct."""
import copy
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("publication", Path(__file__).with_name("publication.py"))
publication = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publication)


def attempt():
    return {"at": "2026-01-01T00:00:00+00:00", "revision": "a" * 40, "fingerprint": "f" * 64,
            "runUrl": "https://github.com/type-rb/type-rb-native/actions/runs/123", "status": "running"}


def manifest():
    summary = {"processWallSeconds": {"count": 3, "p50": 2}, "cpuSeconds": 1,
               "peakRssBytes": 4096, "unitsPerSecond": 100, "firstProcessWallSeconds": 3}
    go_release = "go" + (publication.ROOT / ".go-version").read_text().strip()
    return {"smoke": False, "status": "pass", "goBuildVersions": dict.fromkeys(("reference", "typerb-go", "pure-go"), go_release), "repositoryRevision": "a" * 40,
            "plannedCases": publication.catalog(),
            "cases": [{"input": case, "summaries": {role: copy.deepcopy(summary) for role in publication.ROLES}}
                      for case in publication.catalog()],
            **{key: hashlib.sha256((publication.ROOT / path).read_bytes()).hexdigest() for key, path in publication.SOURCES.items()},
            "samples": 3, "warmups": 1, "startedAt": "2026-01-01T00:00:00+00:00", "platform": "synthetic", "machine": "arm64",
            "tools": {name: {"path": "/synthetic/private/tool", "sha256": "c" * 64, "version": "go version " + go_release + " linux/arm64" if name == "go" else "synthetic 1.0"}
                      for name in ("native", "reference", "go")},
            "binaries": {role: {"sha256": "d" * 64, "bytes": 1000} for role in publication.ROLES},
            "environment": {"GOMAXPROCS": "1", "affinity": [0], "cpuPinning": "inherited single CPU", "filesystemCache": "warm"}}


class PublicationTests(unittest.TestCase):
    def test_canonical_run_preserves_historical_evidence(self):
        previous = publication.snapshot(manifest(), attempt())
        with tempfile.TemporaryDirectory() as temporary, patch.dict(publication.os.environ, {
                "GITHUB_REPOSITORY": "type-rb/type-rb", "GITHUB_RUN_ID": "456"}):
            path = Path(temporary) / "state.json"
            publication.daily.write(path, {**publication.empty(), "latest": previous, "history": [previous]})
            publication.start(path)
            state = publication.daily.read(path)
            self.assertEqual(state["attempt"]["runUrl"], "https://github.com/type-rb/type-rb/actions/runs/456")
            self.assertEqual(state["latest"], previous)
            self.assertEqual(state["history"], [previous])
            with patch.dict(publication.os.environ, {"GITHUB_REPOSITORY": "other/type-rb"}):
                with self.assertRaisesRegex(ValueError, "Unexpected publication repository"):
                    publication.start(path)

    def test_go_toolchain_identity_separates_measurement_series(self):
        data = manifest()
        original = publication.snapshot(data, attempt())
        data["tools"]["go"]["sha256"] = "e" * 64
        updated = publication.snapshot(data, attempt())
        self.assertNotEqual(original["series"], updated["series"])
        del data["goBuildVersions"]["pure-go"]
        with self.assertRaisesRegex(ValueError, "all three Go binaries"):
            publication.snapshot(data, attempt())
        legacy = copy.deepcopy(original)
        del legacy["goBuildVersions"]
        self.assertEqual(publication.validate_snapshot(legacy), legacy)

    def test_summary_diagnostics_cannot_replace_streaming_results(self):
        data = manifest()
        data["outputMode"] = "summary"
        with self.assertRaisesRegex(ValueError, "Summary diagnostics"):
            publication.snapshot(data, attempt())
        data["outputMode"] = "stream"
        publication.snapshot(data, attempt())

    def test_profile_identity_separates_series_and_rejects_relabelled_inputs(self):
        original = publication.snapshot(manifest(), attempt())
        data = manifest()
        data["inputProfile"] = "sustained-v1"
        with self.assertRaises(ValueError):
            publication.snapshot(data, attempt())
        cases = publication.catalog("sustained-v1")
        data["plannedCases"] = cases
        for row, case in zip(data["cases"], cases):
            row["input"] = case
        sustained = publication.snapshot(data, attempt())
        self.assertNotEqual(original["series"], sustained["series"])
        self.assertEqual(sustained["inputProfile"], "sustained-v1")
        legacy = copy.deepcopy(original)
        legacy.pop("inputProfile")
        state = {**publication.empty(), "latest": sustained, "history": [legacy, sustained]}
        self.assertEqual(publication.validate(state)["history"][0], legacy)
        self.assertNotEqual(legacy["inputs"][0]["batches"], sustained["inputs"][0]["batches"])

    def test_public_sample_ranges_are_validated_and_match_medians(self):
        data = manifest()
        for row in data["cases"]:
            for summary in row["summaries"].values():
                values = {"wallSeconds": summary["processWallSeconds"]["p50"],
                          **{key: summary[key] for key in ("cpuSeconds", "peakRssBytes", "unitsPerSecond")}}
                summary["sampleRanges"] = {key: {"count": 3, "minimum": value / 2, "p50": value, "maximum": value * 2}
                                           for key, value in values.items()}
        value = publication.snapshot(data, attempt())
        self.assertEqual(value["rows"][0]["sampleRanges"]["native"]["cpuSeconds"]["minimum"], .5)
        for mutation in (lambda r: r.update(count=2), lambda r: r.update(minimum=9),
                         lambda r: r.update(maximum=float("inf")), lambda r: r.update(p50=1.2),
                         lambda r: r.update(path="/synthetic/private")):
            changed = copy.deepcopy(value)
            mutation(changed["rows"][0]["sampleRanges"]["native"]["cpuSeconds"])
            with self.assertRaises(ValueError):
                publication.validate_snapshot(changed)
        changed = copy.deepcopy(value)
        changed["rows"][0]["sampleRanges"].pop("pure-go")
        with self.assertRaises(ValueError):
            publication.validate_snapshot(changed)
        data["cases"][0]["summaries"]["pure-go"].pop("sampleRanges")
        with self.assertRaises(ValueError):
            publication.snapshot(data, attempt())

    def test_snapshot_preserves_all_cases_without_tool_paths(self):
        value = publication.snapshot(manifest(), attempt())
        self.assertEqual(len(value["rows"]), len(publication.catalog()))
        self.assertNotIn("/synthetic/private", json.dumps(value))
        self.assertEqual(set(value["rows"][0]["summaries"]), set(publication.ROLES))

    def test_rss_method_and_tool_identity_survive_without_local_paths(self):
        data = manifest()
        data["environment"].update(peakRssMethod="gnu-time-child-v1", timingMethod="process tree",
                                   rssTool={"path": "/synthetic/private/time", "sha256": "e" * 64, "version": "GNU time"})
        value = publication.snapshot(data, attempt())
        self.assertEqual(value["environment"]["peakRssMethod"], "gnu-time-child-v1")
        self.assertNotIn("/synthetic/private", json.dumps(value))
        value["environment"].pop("rssTool")
        with self.assertRaises(ValueError):
            publication.validate_snapshot(value)

    def test_smoke_wrong_source_and_selected_subsets_cannot_be_complete_results(self):
        for mutate in (lambda d: d.update(smoke=True), lambda d: d.update(repositoryRevision="b" * 40),
                       lambda d: d.update(sourceSha256="d" * 64), lambda d: d["plannedCases"].pop(),
                       lambda d: d["cases"][0]["summaries"]["native"]["processWallSeconds"].update(count=2)):
            data = manifest()
            mutate(data)
            with self.assertRaises(ValueError):
                publication.snapshot(data, attempt())

    def test_duplicate_missing_roles_and_invalid_metrics_are_rejected(self):
        value = publication.snapshot(manifest(), attempt())
        for mutate in (lambda d: d["rows"].append(copy.deepcopy(d["rows"][0])),
                       lambda d: d["rows"][0]["summaries"].pop("pure-go"),
                       lambda d: d["rows"][0]["summaries"]["native"].update(wallSeconds=float("nan")),
                       lambda d: d.update(runUrl="https://example.invalid/run")):
            changed = copy.deepcopy(value)
            mutate(changed)
            with self.assertRaises(ValueError):
                publication.validate_snapshot(changed)

    def test_failed_cohort_keeps_previous_complete_and_partial_observations(self):
        with tempfile.TemporaryDirectory() as temporary:
            path, source = Path(temporary) / "state.json", Path(temporary) / "manifest.json"
            previous = publication.snapshot(manifest(), attempt())
            publication.daily.write(path, {**publication.empty(), "latest": previous, "history": [previous], "attempt": attempt()})
            data = manifest()
            data.update(status="failed", cases=data["cases"][:2],
                        failedObservation={"case": publication.catalog()[2]["id"], "role": "native", "status": "output-mismatch"})
            publication.daily.write(source, data)
            publication.finish(path, source, "success")
            result = publication.daily.read(path)
            self.assertEqual(result["latest"], previous)
            self.assertEqual(len(result["history"]), 1)
            self.assertEqual(result["attempt"]["status"], "measurement-failure")
            self.assertEqual(len(result["attempt"]["partial"]["rows"]), 2)
            self.assertEqual(result["attempt"]["failedObservation"]["status"], "output-mismatch")

    def test_evidence_upload_failure_never_publishes_favorable_results(self):
        with tempfile.TemporaryDirectory() as temporary:
            path, source = Path(temporary) / "state.json", Path(temporary) / "manifest.json"
            publication.daily.write(path, {**publication.empty(), "attempt": attempt()})
            publication.daily.write(source, manifest())
            publication.finish(path, source, "failure")
            result = publication.daily.read(path)
            self.assertIsNone(result["latest"])
            self.assertEqual(result["attempt"]["status"], "infrastructure-failure")

    def test_complete_runs_bound_history_without_changing_daily_state(self):
        with tempfile.TemporaryDirectory() as temporary:
            path, source = Path(temporary) / "state.json", Path(temporary) / "manifest.json"
            previous = publication.snapshot(manifest(), attempt())
            publication.daily.write(path, {**publication.empty(), "latest": previous,
                                          "history": [previous] * publication.HISTORY_LIMIT, "attempt": attempt()})
            publication.daily.write(source, manifest())
            publication.finish(path, source, "success")
            result = publication.daily.read(path)
            self.assertEqual(result["attempt"]["status"], "measured")
            self.assertEqual(len(result["history"]), publication.HISTORY_LIMIT)
            self.assertEqual(publication.daily.WORKFLOW, "daily-performance.yml")
            self.assertEqual(publication.daily.ARTIFACT, "daily-performance-state")

    def test_canonical_cancelled_run_links_to_its_repository(self):
        previous = publication.snapshot(manifest(), attempt())
        with tempfile.TemporaryDirectory() as temporary, patch.dict(publication.os.environ, {
                "TRIGGER_RUN_ID": "456", "TRIGGER_HEAD_SHA": "b" * 40, "TRIGGER_CONCLUSION": "cancelled"}):
            path, output = Path(temporary) / "state.json", Path(temporary) / "site.json"
            publication.daily.write(path, {**publication.empty(), "latest": previous})
            publication.site(path, output)
            state = publication.daily.read(output)
            self.assertEqual(state["attempt"]["runUrl"], "https://github.com/type-rb/type-rb/actions/runs/456")
            self.assertEqual(state["attempt"]["status"], "infrastructure-failure")
            self.assertEqual(state["latest"], previous)

    def test_first_canonical_deployment_inherits_public_native_history(self):
        previous = {**publication.empty(), "latest": publication.snapshot(manifest(), attempt())}
        missing = publication.urllib.error.HTTPError(publication.PUBLIC_STATE, 404, "Not found", {}, None)
        with patch.object(publication.urllib.request, "urlopen", side_effect=[
                missing, io.BytesIO(json.dumps(previous).encode())]) as fetch:
            self.assertEqual(publication.read_public_state(), previous)
            self.assertEqual([call.args[0] for call in fetch.call_args_list],
                             [publication.PUBLIC_STATE, publication.LEGACY_STATE])

    def test_expired_artifacts_fall_back_to_last_public_result(self):
        previous = {**publication.empty(), "latest": publication.snapshot(manifest(), attempt())}
        def restore(path, **kwargs):
            self.assertEqual(kwargs["workflow"], publication.WORKFLOW)
            self.assertEqual(kwargs["artifact_name"], publication.ARTIFACT)
            publication.daily.write(path, publication.daily.empty())
        with tempfile.TemporaryDirectory() as temporary, patch.object(publication.daily, "restore", restore), \
                patch.object(publication.urllib.request, "urlopen", return_value=io.BytesIO(json.dumps(previous).encode())):
            path = Path(temporary) / "state.json"
            publication.restore(path)
            self.assertEqual(publication.daily.read(path)["latest"], previous["latest"])

    def test_api_failure_and_unavailable_public_data_are_not_empty_success(self):
        with tempfile.TemporaryDirectory() as temporary, \
                patch.object(publication.daily, "restore", side_effect=subprocess.CalledProcessError(1, "gh")), \
                patch.object(publication.urllib.request, "urlopen", side_effect=OSError("unavailable")):
            path = Path(temporary) / "state.json"
            publication.restore(path)
            result = publication.daily.read(path)
            self.assertIsNone(result["latest"])
            self.assertIn("Artifact retrieval failed", result["publicationWarning"])

    def test_public_fallback_does_not_hide_a_newer_failed_attempt(self):
        previous = {**publication.empty(), "latest": publication.snapshot(manifest(), attempt()), "attempt": dict(attempt(), status="measured")}
        failed = dict(attempt(), at="2026-02-01T00:00:00+00:00", status="measurement-failure")
        def restore(path, **kwargs):
            publication.daily.write(path, {**publication.empty(), "attempt": failed})
        with tempfile.TemporaryDirectory() as temporary, patch.object(publication.daily, "restore", restore), \
                patch.object(publication.urllib.request, "urlopen", return_value=io.BytesIO(json.dumps(previous).encode())):
            path = Path(temporary) / "state.json"
            publication.restore(path)
            result = publication.daily.read(path)
            self.assertEqual(result["latest"], previous["latest"])
            self.assertEqual(result["attempt"], failed)

    def test_short_canonical_artifact_recovers_older_history_and_preserves_failed_attempt(self):
        original = publication.snapshot(manifest(), attempt())
        old = [dict(original, at=f"2026-01-0{day}T00:00:00+00:00",
                    runUrl=f"https://github.com/type-rb/type-rb-native/actions/runs/{day}") for day in (1, 2, 3)]
        latest = dict(original, at="2026-02-01T00:00:00+00:00",
                      runUrl="https://github.com/type-rb/type-rb/actions/runs/4")
        failed = dict(attempt(), at="2026-02-02T00:00:00+00:00", status="measurement-failure",
                      runUrl="https://github.com/type-rb/type-rb/actions/runs/5")
        current = {**publication.empty(), "latest": latest, "history": [latest], "attempt": failed}
        previous = {**publication.empty(), "latest": old[-1], "history": old}
        with tempfile.TemporaryDirectory() as tmp, patch.object(publication, "HISTORY_LIMIT", 3):
            path = Path(tmp) / "state.json"
            publication.daily.write(path, current)
            with patch.object(publication.daily, "restore"), patch.object(publication, "read_state_url", return_value=previous) as read:
                publication.restore(path)
                restored = publication.daily.read(path)
                self.assertEqual(restored, {**current, "history": [old[1], old[2], latest]})
                # A full window is self-contained on the next restore.
                publication.restore(path)
                self.assertEqual(publication.daily.read(path), restored)
                read.assert_called_once_with(publication.LEGACY_STATE)

    def test_archive_failure_retains_canonical_measurement_and_recovers_on_retry(self):
        old = publication.snapshot(manifest(), attempt())
        latest = dict(old, at="2026-02-01T00:00:00+00:00", runUrl="https://github.com/type-rb/type-rb/actions/runs/4")
        current = {**publication.empty(), "latest": latest, "history": [latest]}
        previous = {**publication.empty(), "latest": old, "history": [old]}
        with tempfile.TemporaryDirectory() as tmp, patch.object(publication.daily, "restore"):
            path = Path(tmp) / "state.json"
            publication.daily.write(path, current)
            with patch.object(publication, "read_state_url", side_effect=OSError("unavailable")):
                publication.restore(path)
            restored = publication.daily.read(path)
            self.assertIn("archived Native site", restored.pop("publicationWarning"))
            self.assertEqual(restored, current)
            with patch.object(publication, "read_state_url", return_value=previous):
                publication.restore(path)
                publication.restore(path)
            self.assertEqual(publication.daily.read(path), {**current, "history": [old, latest]})

    def test_main_refresh_keeps_original_measurement_identity_and_reports_pending_changes(self):
        previous = publication.snapshot(manifest(), attempt())
        with tempfile.TemporaryDirectory() as temporary, patch.object(publication.daily, "git", return_value="b" * 40), \
                patch.object(publication, "fingerprint", return_value="e" * 64), patch.dict(publication.os.environ, {"TRIGGER_RUN_ID": ""}):
            path, output = Path(temporary) / "state.json", Path(temporary) / "site.json"
            publication.daily.write(path, {**publication.empty(), "latest": previous})
            publication.site(path, output)
            result = publication.daily.read(output)
            self.assertTrue(result["pendingChanges"])
            self.assertEqual(result["latest"], previous)
            self.assertEqual(result["siteRevision"], "b" * 40)

    def test_freshness_ignores_docs_but_covers_compiler_workloads_and_controller(self):
        for path in ("compiler/src/value.trb", "src/main.trb", "benchmarks/representative/go/main.go",
                     "tools/representative-performance/measure.py", ".go-version", ".github/workflows/representative-performance.yml"):
            self.assertTrue(publication.relevant(path), path)
        for path in ("docs/capabilities/benchmarks/representative.html", "compiler/src/value_test.trb", "docs/performance-policy.md"):
            self.assertFalse(publication.relevant(path), path)

    def test_manual_workflow_does_not_add_a_schedule_or_pr_trigger(self):
        text = (publication.ROOT / ".github/workflows/representative-performance.yml").read_text()
        self.assertIn("  workflow_dispatch:", text)
        self.assertNotIn("  schedule:", text)
        self.assertNotIn("  pull_request:", text)
        self.assertIn("github.ref == 'refs/heads/main'", text)


if __name__ == "__main__":
    unittest.main()
