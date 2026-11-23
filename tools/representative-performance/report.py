#!/usr/bin/env python3
"""Render all registered observations, including uncompleted or unknown cases."""
import argparse
import json
from pathlib import Path


def render(result):
    invalid_rss = (result["platform"].startswith("Linux") and
                   result.get("environment", {}).get("peakRssMethod") != "gnu-time-child-v1")
    lines = ["# Representative performance diagnostic", "",
             f"Status: {result['status']}. Platform: {result['platform']}. Profile: {result.get('inputProfile', 'registered')}.",
             "", "Local diagnostic only. Ratios below 1 mean shorter Native process wall time.",
             "Missing or failed observations are unknown; no victory count is derived.", "",
             "| Input | Native wall ms | Native/Go | Native/TypeRB Go | Native peak RSS MiB | Batch arrival p95 / p99 ms |",
             "| --- | ---: | ---: | ---: | ---: | ---: |"]
    observed = {row["input"]["id"]: row for row in result["cases"]}
    for case in result["plannedCases"]:
        row = observed.get(case["id"], {})
        native = row.get("summaries", {}).get("native")
        if native is None:
            values = ["unknown"] * 5
        else:
            ratios = row.get("nativeWallRatio", {})
            intervals = native["batchArrivalIntervalSeconds"]
            def tail(key):
                return f"{intervals[key] * 1000:.3f}" if intervals and intervals[key] is not None else "unknown"
            values = [f"{native['processWallSeconds']['p50'] * 1000:.3f}",
                      f"{ratios['pure-go']:.3f}" if "pure-go" in ratios else "unknown",
                      f"{ratios['typerb-go']:.3f}" if "typerb-go" in ratios else "unknown",
                      "unknown" if invalid_rss else f"{native['peakRssBytes'] / 1024 ** 2:.2f}",
                      f"{tail('p95')} / {tail('p99')}"]
        lines.append("| " + case["id"] + " | " + " | ".join(values) + " |")
    lines.extend(["", "Batch arrival intervals include pipe/scheduling overhead; they are not request latency.",
                  "Coalesced markers or insufficient tail observations remain unknown."])
    lines.extend(["", "## Retained sample ranges", "",
                  "Observed minimum–maximum, with the original median; not confidence intervals.",
                  "First-process observations remain separate. Missing historical ranges are not reconstructed.", "",
                  "| Input | Role | Metric | Count | Minimum | Median | Maximum |",
                  "| --- | --- | --- | ---: | ---: | ---: | ---: |"])
    for row in result["cases"]:
        for role, summary in row.get("summaries", {}).items():
            for metric, bounds in (summary or {}).get("sampleRanges", {}).items():
                if metric == "peakRssBytes" and invalid_rss:
                    continue
                values = [str(bounds[key]) for key in ("count", "minimum", "p50", "maximum")]
                lines.append("| " + " | ".join([row["input"]["id"], role, metric, *values]) + " |")
    if invalid_rss:
        lines.extend(["", "Legacy Linux RSS includes observer memory and is unavailable; other metrics and raw observations are retained."])
    if result.get("smoke"):
        lines.extend(["", "This is correctness smoke; its timings do not establish a performance comparison."])
    if result.get("error"):
        lines.extend(["", "Failure: " + result["error"]])
    if result.get("coverageGaps"):
        lines.extend(["", "Uncovered: " + "; ".join(result["coverageGaps"]) + "."])
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    args = parser.parse_args()
    print(render(json.loads(args.manifest.read_text())), end="")
