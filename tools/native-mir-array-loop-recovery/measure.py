#!/usr/bin/env python3

"""Measure the pre-registered verified Array-loop recovery workload."""

from __future__ import annotations

import csv
import hashlib
import json
import os
from pathlib import Path
import statistics
import sys
import time


WARMUPS = 2
RETAINED = 21
RUNTIME_RATIO_LIMIT = 0.75
RSS_RATIO_LIMIT = 1.05
CATASTROPHIC_RATIO_LIMIT = 2.0


def usage() -> int:
    print(
        "usage: measure.py BASELINE_EXECUTABLE CANDIDATE_EXECUTABLE "
        "EXPECTED_STDOUT OUTPUT_DIRECTORY [RUNTIME_RATIO_LIMIT]",
        file=sys.stderr,
    )
    return 64


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def peak_rss_bytes(value: int) -> int:
    if sys.platform == "darwin":
        return value
    return value * 1024


def run_once(
    role: str,
    executable: Path,
    expected: bytes,
    phase: str,
    round_index: int,
    order: int,
    observations: Path,
) -> dict[str, object]:
    stem = f"{phase}-{round_index:02d}-order-{order}-{role}"
    stdout_path = observations / f"{stem}.stdout"
    stderr_path = observations / f"{stem}.stderr"
    stdout_fd = os.open(stdout_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
    stderr_fd = os.open(stderr_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
    started = time.monotonic_ns()
    child = os.fork()
    if child == 0:
        try:
            os.dup2(stdout_fd, 1)
            os.dup2(stderr_fd, 2)
            os.close(stdout_fd)
            os.close(stderr_fd)
            os.execve(str(executable), [str(executable)], dict(os.environ))
        except OSError as error:
            os.write(2, f"measure: {error}\n".encode())
            os._exit(127)
    os.close(stdout_fd)
    os.close(stderr_fd)
    _, wait_status, usage_record = os.wait4(child, 0)
    finished = time.monotonic_ns()
    status = os.waitstatus_to_exitcode(wait_status)
    stdout = stdout_path.read_bytes()
    stderr = stderr_path.read_bytes()
    valid = status == 0 and stdout == expected and stderr == b""
    return {
        "phase": phase,
        "round": round_index,
        "order": order,
        "role": role,
        "wallSeconds": (finished - started) / 1_000_000_000,
        "cpuSeconds": usage_record.ru_utime + usage_record.ru_stime,
        "peakRssBytes": peak_rss_bytes(usage_record.ru_maxrss),
        "exitStatus": status,
        "outputStatus": "pass" if valid else "fail",
        "stdout": stdout_path.name,
        "stderr": stderr_path.name,
    }


def median(rows: list[dict[str, object]], role: str, key: str) -> float:
    values = [float(row[key]) for row in rows if row["role"] == role]
    return statistics.median(values)


def maximum(rows: list[dict[str, object]], role: str, key: str) -> float:
    return max(float(row[key]) for row in rows if row["role"] == role)


def main(arguments: list[str]) -> int:
    if len(arguments) not in (5, 6):
        return usage()
    runtime_ratio_limit = RUNTIME_RATIO_LIMIT
    if len(arguments) == 6:
        try:
            runtime_ratio_limit = float(arguments[5])
        except ValueError:
            print("measure: runtime ratio limit must be numeric", file=sys.stderr)
            return 64
        if not 0 < runtime_ratio_limit <= CATASTROPHIC_RATIO_LIMIT:
            print("measure: runtime ratio limit is outside (0, 2]", file=sys.stderr)
            return 64
    baseline = Path(arguments[1]).resolve()
    candidate = Path(arguments[2]).resolve()
    expected_path = Path(arguments[3]).resolve()
    output = Path(arguments[4]).resolve()
    for path in (baseline, candidate, expected_path):
        if not path.is_file():
            print(f"measure: required input is not a file: {path}", file=sys.stderr)
            return 1
    if not os.access(baseline, os.X_OK) or not os.access(candidate, os.X_OK):
        print("measure: both measured inputs must be executable", file=sys.stderr)
        return 1

    output.mkdir(parents=True, exist_ok=True)
    observations = output / "observations"
    observations.mkdir(parents=True, exist_ok=True)
    expected = expected_path.read_bytes()
    rows: list[dict[str, object]] = []
    executables = {"baseline": baseline, "candidate": candidate}
    for round_index in range(1, WARMUPS + 1):
        for order, role in enumerate(("baseline", "candidate"), start=1):
            rows.append(
                run_once(
                    role,
                    executables[role],
                    expected,
                    "warmup",
                    round_index,
                    order,
                    observations,
                )
            )
    for round_index in range(1, RETAINED + 1):
        order_roles = ("baseline", "candidate")
        if round_index % 2 == 0:
            order_roles = ("candidate", "baseline")
        for order, role in enumerate(order_roles, start=1):
            rows.append(
                run_once(
                    role,
                    executables[role],
                    expected,
                    "retained",
                    round_index,
                    order,
                    observations,
                )
            )

    columns = [
        "phase",
        "round",
        "order",
        "role",
        "wallSeconds",
        "cpuSeconds",
        "peakRssBytes",
        "exitStatus",
        "outputStatus",
        "stdout",
        "stderr",
    ]
    with (output / "raw.csv").open("w", newline="", encoding="utf-8") as target:
        writer = csv.DictWriter(target, fieldnames=columns)
        writer.writeheader()
        writer.writerows(rows)

    retained = [row for row in rows if row["phase"] == "retained"]
    valid = all(row["outputStatus"] == "pass" for row in rows)
    baseline_wall = median(retained, "baseline", "wallSeconds")
    candidate_wall = median(retained, "candidate", "wallSeconds")
    baseline_cpu = median(retained, "baseline", "cpuSeconds")
    candidate_cpu = median(retained, "candidate", "cpuSeconds")
    baseline_rss = median(retained, "baseline", "peakRssBytes")
    candidate_rss = median(retained, "candidate", "peakRssBytes")
    ratios = {
        "wall": candidate_wall / baseline_wall,
        "cpu": candidate_cpu / baseline_cpu,
        "peakRss": candidate_rss / baseline_rss,
    }
    catastrophic: dict[str, dict[str, float]] = {}
    for role in ("baseline", "candidate"):
        catastrophic[role] = {
            "wall": maximum(retained, role, "wallSeconds") / baseline_wall,
            "cpu": maximum(retained, role, "cpuSeconds") / baseline_cpu,
            "peakRss": maximum(retained, role, "peakRssBytes") / baseline_rss,
        }
    thresholds_pass = (
        ratios["wall"] <= runtime_ratio_limit
        and ratios["cpu"] <= runtime_ratio_limit
        and ratios["peakRss"] <= RSS_RATIO_LIMIT
        and all(
            value <= CATASTROPHIC_RATIO_LIMIT
            for role in catastrophic.values()
            for value in role.values()
        )
    )
    summary = {
        "schemaVersion": 1,
        "clock": "time.monotonic_ns",
        "warmupsPerRole": WARMUPS,
        "retainedPerRole": RETAINED,
        "baseline": {
            "wallMedianSeconds": baseline_wall,
            "cpuMedianSeconds": baseline_cpu,
            "peakRssMedianBytes": int(baseline_rss),
            "sizeBytes": baseline.stat().st_size,
            "sha256": sha256(baseline),
        },
        "candidate": {
            "wallMedianSeconds": candidate_wall,
            "cpuMedianSeconds": candidate_cpu,
            "peakRssMedianBytes": int(candidate_rss),
            "sizeBytes": candidate.stat().st_size,
            "sha256": sha256(candidate),
        },
        "ratios": ratios,
        "limits": {
            "wall": runtime_ratio_limit,
            "cpu": runtime_ratio_limit,
            "peakRss": RSS_RATIO_LIMIT,
            "catastrophic": CATASTROPHIC_RATIO_LIMIT,
        },
        "maximumToBaselineMedianRatios": catastrophic,
        "outputsPass": valid,
        "thresholdsPass": thresholds_pass,
        "status": "pass" if valid and thresholds_pass else "fail",
    }
    (output / "summary.json").write_text(
        json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    if summary["status"] != "pass":
        print("measure: runtime contract failed", file=sys.stderr)
        return 1
    print(
        "native MIR Array-loop recovery: "
        f"wall={ratios['wall']:.6f} cpu={ratios['cpu']:.6f} "
        f"rss={ratios['peakRss']:.6f}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
