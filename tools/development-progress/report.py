#!/usr/bin/env python3
"""Summarize reviewed language gaps and recent development feedback costs."""

import argparse
from datetime import datetime, timedelta, timezone
import importlib.util
import json
import math
from pathlib import Path
import re
import statistics


ROOT = Path(__file__).resolve().parents[2]
CI_RUN_LIMIT = 300


def load_coverage():
    path = ROOT / "tools/native-language-coverage.py"
    spec = importlib.util.spec_from_file_location("native_language_coverage", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def timestamp(value):
    if not isinstance(value, str):
        raise ValueError("Missing timestamp")
    result = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("Timestamp lacks a timezone")
    return result.astimezone(timezone.utc)


def round_seconds(value):
    return round(value, 2) if value is not None else None


def parity_summary(registry, coverage):
    cases = coverage.validate(registry)
    gaps = [case["id"] for case in cases if coverage.parity_gaps(case)]
    pending = sum(len(feature["pending"]) for feature in registry["features"]
                  if feature["scope"] == "basic")
    return {"different": len(gaps), "probes": len(cases), "ids": gaps,
            "uncoveredContracts": pending}


def ci_summary(runs, as_of):
    if not isinstance(runs, list):
        raise ValueError("PR run data must be a list")
    earliest = as_of - timedelta(days=7)
    durations, feedback = [], []
    conclusions = {}
    for run in runs:
        if run.get("event") != "pull_request" or run.get("status") != "completed":
            continue
        finished = timestamp(run.get("updatedAt"))
        if not earliest <= finished <= as_of:
            continue
        created, started = timestamp(run.get("createdAt")), timestamp(run.get("startedAt"))
        if not created <= started <= finished:
            raise ValueError("PR run timestamps are out of order")
        durations.append((finished - started).total_seconds())
        feedback.append((finished - created).total_seconds())
        conclusion = run.get("conclusion") or "unknown"
        conclusions[conclusion] = conclusions.get(conclusion, 0) + 1

    def percentile(values, proportion):
        if not values:
            return None
        ordered = sorted(values)
        return round_seconds(ordered[max(0, math.ceil(len(ordered) * proportion) - 1)])

    completed_times = [timestamp(run.get("updatedAt")) for run in runs
                       if run.get("status") == "completed"]
    sample_capped = (len(runs) >= CI_RUN_LIMIT and completed_times and
                     min(completed_times) >= earliest)
    return {"windowDays": 7, "runs": len(durations), "sampleCapped": bool(sample_capped),
            "conclusions": conclusions,
            "executionMedianSeconds": round_seconds(statistics.median(durations)) if durations else None,
            "executionP90Seconds": percentile(durations, .9),
            "feedbackMedianSeconds": round_seconds(statistics.median(feedback)) if feedback else None,
            "feedbackP90Seconds": percentile(feedback, .9)}


def daily_summary(state):
    if state.get("schemaVersion") != 1:
        raise ValueError("Unsupported daily state")
    latest = state.get("latest")
    if latest is None:
        return {"revision": None, "measuredAt": None, "status": "unavailable",
                "selfBuildSeconds": None, "selfIrBytes": None, "pureGoRatios": {}}
    revision, measured = latest.get("revision"), latest.get("at")
    if not isinstance(revision, str) or not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Invalid daily revision")
    timestamp(measured)
    compiler = latest.get("compilerSelf") or {}
    build = compiler.get("build") or {}
    ir = compiler.get("ir") or {}
    self_build = build.get("wallSeconds") if compiler.get("status") == "pass" else None
    self_ir = ir.get("bytes") if compiler.get("status") == "pass" else None
    rows = {(row["case"], row["role"]): row for row in latest.get("rows", [])}
    ratios = {}
    for case in latest.get("pureGoCases", []):
        native, go = rows.get((case, "native")), rows.get((case, "pure-go"))
        if not native or not go or native.get("status") != "pass" or go.get("status") != "pass":
            ratios[case] = None
            continue
        native_runtime, go_runtime = native.get("runtime"), go.get("runtime")
        left = native_runtime.get("wallSeconds") if native_runtime else None
        right = go_runtime.get("wallSeconds") if go_runtime else None
        if not all(isinstance(value, (int, float)) and math.isfinite(value) and value > 0
                   for value in (left, right)):
            ratios[case] = None
        else:
            ratios[case] = round(left / right, 3)
    return {"revision": revision, "measuredAt": measured, "status": latest.get("status"),
            "selfBuildSeconds": round_seconds(self_build), "selfIrBytes": self_ir,
            "pureGoRatios": ratios}


def report(registry, daily_state, runs, as_of, head_revision=None):
    coverage = load_coverage()
    if head_revision is not None and not re.fullmatch(r"[0-9a-f]{40}", head_revision):
        raise ValueError("Invalid checkout revision")
    return {"schemaVersion": 1, "at": as_of.isoformat(), "headRevision": head_revision,
            "parity": parity_summary(registry, coverage),
            "ci": ci_summary(runs, as_of), "daily": daily_summary(daily_state)}


def markdown(data):
    parity, ci, daily = data["parity"], data["ci"], data["daily"]
    def seconds(value):
        if value is None:
            return "unavailable"
        if value < 60:
            return f"{value:.2f} s"
        if value < 3600:
            return f"{value / 60:.1f} min"
        return f"{value / 3600:.1f} h"
    ci_count = f"{ci['runs']} completed runs" + ("; latest 300 only" if ci["sampleCapped"] else "")
    lines = [f"# Development progress — {data['at'][:10]}", "",
             "| Indicator | Current observation |", "| --- | --- |",
             f"| Reviewed probes with Native/reference differences | {parity['different']} / {parity['probes']} |",
             f"| Registered uncovered basic contracts | {parity['uncoveredContracts']} |",
             f"| PR CI execution, trailing 7 days | median {seconds(ci['executionMedianSeconds'])}; p90 {seconds(ci['executionP90Seconds'])} ({ci_count}) |",
             f"| PR feedback including queue, trailing 7 days | median {seconds(ci['feedbackMedianSeconds'])}; p90 {seconds(ci['feedbackP90Seconds'])} |",
             f"| Compiler self-build | {seconds(daily['selfBuildSeconds'])} |",
             f"| Compiler emitted QBE | {'unavailable' if daily['selfIrBytes'] is None else str(daily['selfIrBytes']) + ' bytes'} |"]
    for case, ratio in sorted(daily["pureGoRatios"].items()):
        value = "unavailable" if ratio is None else f"{ratio:.3f}× (Native / Pure Go; lower is better)"
        lines.append(f"| {case} runtime | {value} |")
    lines.extend(["", f"Daily measurement: {daily['measuredAt'] or 'unavailable'} "
                  f"at revision `{daily['revision'] or 'unavailable'}`; status {daily['status']}.",
                  "CI times are completed pull-request workflow observations, not only successful runs. "
                  "Daily ratios compare programs measured in the same cohort. "
                  "These indicators are diagnostics, not merge or performance acceptance thresholds.", ""])
    if data["headRevision"] and daily["revision"] != data["headRevision"]:
        lines.insert(-1, "The daily measurement does not cover the current main revision.")
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registry", type=Path, required=True)
    parser.add_argument("--daily-state", type=Path, required=True)
    parser.add_argument("--ci-runs", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--json-output", type=Path, required=True)
    parser.add_argument("--head-revision")
    args = parser.parse_args()
    result = report(json.loads(args.registry.read_text()), json.loads(args.daily_state.read_text()),
                    json.loads(args.ci_runs.read_text()), datetime.now(timezone.utc), args.head_revision)
    args.output.write_text(markdown(result))
    args.json_output.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
