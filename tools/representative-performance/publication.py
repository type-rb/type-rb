#!/usr/bin/env python3
"""Publish an independent, bounded representative series without local paths."""
import argparse
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import subprocess
import urllib.request
import zipfile
from datetime import datetime

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("daily_state", ROOT / "tools/daily-performance/state.py")
daily = importlib.util.module_from_spec(spec)
spec.loader.exec_module(daily)
spec = importlib.util.spec_from_file_location("representative_measure", Path(__file__).with_name("measure.py"))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)
WORKFLOW = "representative-performance.yml"
ARTIFACT = "representative-performance-state"
PUBLIC_STATE = "https://type-rb.github.io/type-rb-native/benchmarks/representative-state.json"
ROLES = ("native", "typerb-go", "pure-go")
HISTORY_LIMIT = 12
HASH = re.compile(r"[0-9a-f]{64}")
SHA = re.compile(r"[0-9a-f]{40}")
RUN = re.compile(r"https://github\.com/type-rb/type-rb-native/actions/runs/[0-9]+")
SOURCES = {"sourceSha256": "benchmarks/representative/src/main.trb",
           "goSourceSha256": "benchmarks/representative/go/main.go",
           "suiteSha256": "tools/representative-performance/suite.json",
           "controllerSha256": "tools/representative-performance/measure.py"}
METRICS = ("wallSeconds", "cpuSeconds", "peakRssBytes", "unitsPerSecond", "firstProcessWallSeconds")


def catalog(profile="registered"):
    return measure.load_cases(profile=profile)[1]


def empty():
    return {**daily.empty(), "profile": "representative"}


def relevant(path):
    if path.endswith((".md", "_test.trb", "_test.py")):
        return False
    return path.startswith(("compiler/", "src/", "benchmarks/representative/", "tools/qbe/")) or path in (
        "TYPE_RB_REVISION", "trbn", "tools/build-native.sh", "tools/build-reference.py", "tools/compiler_sources.py",
        "tools/qbe_toolchain.py", "tools/bootstrap-seed.sh", "tools/bootstrap-seed-download.sh",
        "tools/representative-performance/measure.py", "tools/representative-performance/suite.json",
        ".github/workflows/representative-performance.yml")


def fingerprint():
    entries = subprocess.check_output(["git", "ls-tree", "-rz", "HEAD"], cwd=ROOT).split(b"\0")
    return hashlib.sha256(b"\0".join(entry for entry in entries if entry and relevant(entry.split(b"\t", 1)[1].decode()))).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_snapshot(snapshot):
    require(isinstance(snapshot, dict), "Invalid snapshot")
    require(re.fullmatch(r"[a-z0-9-]+", snapshot.get("inputProfile", "registered")), "Invalid input profile")
    require(SHA.fullmatch(snapshot.get("revision", "")) and SHA.fullmatch(snapshot.get("referenceRevision", "")), "Invalid compiler revisions")
    require(RUN.fullmatch(snapshot.get("runUrl", "")), "Invalid public evidence URL")
    require(all(HASH.fullmatch(snapshot.get(key, "")) for key in ("fingerprint", "series")), "Missing series identity")
    require(all(HASH.fullmatch(snapshot.get("sources", {}).get(key, "")) for key in SOURCES), "Missing source hashes")
    require(isinstance(snapshot.get("at"), str) and datetime.fromisoformat(snapshot["at"]).tzinfo is not None, "Missing measurement time zone")
    require(all(isinstance(snapshot.get(key), str) and 0 < len(snapshot[key]) < 512 for key in ("platform", "machine")), "Invalid environment identity")
    require(set(snapshot.get("tools", {})) == {"native", "reference", "go"}, "Missing tool identities")
    require(set(snapshot.get("binaries", {})) == set(ROLES), "Missing application identities")
    for binary in snapshot["binaries"].values():
        require(set(binary) == {"sha256", "bytes"} and HASH.fullmatch(binary["sha256"]) and type(binary["bytes"]) is int and binary["bytes"] > 0, "Invalid application identity")
    require(isinstance(snapshot.get("environment"), dict) and snapshot["environment"].get("GOMAXPROCS") == "1", "Invalid execution environment")
    environment = snapshot["environment"]
    if "peakRssMethod" in environment:
        require(environment["peakRssMethod"] in ("gnu-time-child-v1", "wait4-bytes-v1"), "Unknown RSS method")
        if environment["peakRssMethod"] == "gnu-time-child-v1":
            tool = environment.get("rssTool", {})
            require(set(tool) == {"sha256", "version"} and HASH.fullmatch(tool["sha256"]) and isinstance(tool["version"], str), "Missing RSS tool identity")
    require(isinstance(snapshot.get("samples"), int) and 1 <= snapshot["samples"] <= 100 and snapshot.get("warmups") == 1, "Invalid sampling policy")
    inputs, rows = snapshot.get("inputs"), snapshot.get("rows")
    require(isinstance(inputs, list) and 1 <= len(inputs) <= 100 and isinstance(rows, list), "Invalid case inventory")
    known = {}
    for case in inputs:
        require(isinstance(case, dict) and re.fullmatch(r"[a-z0-9-]+", case.get("id", "")), "Invalid case ID")
        require(case["id"] not in known and isinstance(case.get("count"), int) and case["count"] > 0 and isinstance(case.get("batches"), int) and case["batches"] > 0, "Invalid input dimensions")
        known[case["id"]] = case
    seen = set()
    for row in rows:
        require(row.get("case") in known and row["case"] not in seen, "Unknown or duplicate result")
        seen.add(row["case"])
        require(set(row.get("summaries", {})) == set(ROLES), "Missing comparison role")
        for metrics in row["summaries"].values():
            require(set(metrics) == set(METRICS), "Invalid metric fields")
            for key, value in metrics.items():
                require(type(value) in (int, float) and math.isfinite(value) and value >= 0, "Invalid metric")
                require(key == "cpuSeconds" or value > 0, "Invalid zero observation")
        if "sampleRanges" in row:
            require(set(row["sampleRanges"]) == set(ROLES), "Missing sample range role")
            for role, ranges in row["sampleRanges"].items():
                require(set(ranges) == set(METRICS) - {"firstProcessWallSeconds"}, "Invalid sample range metrics")
                for key, bounds in ranges.items():
                    require(set(bounds) == {"count", "minimum", "p50", "maximum"}, "Invalid sample range fields")
                    require(type(bounds["count"]) is int and bounds["count"] == snapshot["samples"], "Invalid sample count")
                    require(all(type(bounds[k]) in (int, float) and math.isfinite(bounds[k]) and bounds[k] >= 0
                                for k in ("minimum", "p50", "maximum")), "Invalid sample range")
                    require(bounds["minimum"] <= bounds["p50"] <= bounds["maximum"] and
                            bounds["p50"] == row["summaries"][role][key], "Sample range disagrees with median")
                    require(key == "cpuSeconds" or bounds["minimum"] > 0, "Invalid zero sample")
    require(len(rows) <= len(inputs), "Too many results")
    for tool in snapshot.get("tools", {}).values():
        require(set(tool) == {"sha256", "version"} and HASH.fullmatch(tool["sha256"]) and isinstance(tool["version"], str), "Invalid public tool identity")
    return snapshot


def validate(state):
    require(isinstance(state, dict), "Invalid representative state")
    require(state.get("schemaVersion") == 1 and state.get("profile") == "representative", "Unsupported representative state")
    require(isinstance(state.get("history"), list) and len(state["history"]) <= HISTORY_LIMIT, "Unbounded history")
    for snapshot in [state.get("latest"), *state["history"]]:
        if snapshot is not None:
            validate_snapshot(snapshot)
            require(len(snapshot["rows"]) == len(snapshot["inputs"]), "Complete snapshot is missing cases")
    attempt = state.get("attempt")
    if attempt is not None:
        require(attempt.get("status") in ("running", "measured", "measurement-failure", "infrastructure-failure"), "Invalid attempt status")
        require(RUN.fullmatch(attempt.get("runUrl", "")) and SHA.fullmatch(attempt.get("revision", "")), "Invalid attempt identity")
        if attempt.get("partial") is not None:
            validate_snapshot(attempt["partial"])
    return state


def restore(path):
    warning = None
    try:
        daily.restore(path, workflow=WORKFLOW, artifact_name=ARTIFACT, validator=validate)
        state = daily.read(path)
        # No artifact exists before the first run, or after the 90-day expiry.
        if state.get("profile") == "representative" and state.get("latest"):
            return
    except (OSError, ValueError, KeyError, TypeError, zipfile.BadZipFile, subprocess.SubprocessError):
        state = empty()
        warning = "Artifact retrieval failed. Showing the last available public measurement."
    try:
        with urllib.request.urlopen(PUBLIC_STATE, timeout=20) as response:
            data = response.read(8_000_001)
        require(len(data) <= 8_000_000, "Public state exceeds its limit")
        previous = validate(json.loads(data))
        # Preserve a newer failed first attempt even if a previous deployment is empty.
        if previous.get("latest"):
            newer_attempt = state.get("attempt")
            state = previous
            if newer_attempt and (not state.get("attempt") or newer_attempt["at"] >= state["attempt"]["at"]):
                state["attempt"] = newer_attempt
        elif not state.get("attempt"):
            state = previous
    except (OSError, ValueError, KeyError, TypeError):
        if state.get("profile") != "representative":
            state = empty()
        warning = warning or "No retained measurement is available yet. All registered cases remain listed."
    if warning:
        state["publicationWarning"] = warning
    daily.write(path, validate(state))


def start(path):
    state = validate(daily.read(path))
    require(os.environ.get("GITHUB_REPOSITORY") == "type-rb/type-rb-native", "Unexpected publication repository")
    run_url = f"https://github.com/type-rb/type-rb-native/actions/runs/{os.environ['GITHUB_RUN_ID']}"
    state["attempt"] = {"at": daily.now(), "revision": daily.git("rev-parse", "HEAD"), "runUrl": run_url,
                        "status": "running", "fingerprint": fingerprint()}
    state.pop("publicationWarning", None)
    daily.write(path, validate(state))


def snapshot(manifest, attempt):
    require(not manifest.get("smoke") and manifest.get("repositoryRevision") == attempt["revision"], "Wrong measurement source or smoke data")
    require(manifest.get("outputMode", "stream") == "stream", "Summary diagnostics cannot enter the streaming publication series")
    profile = manifest.get("inputProfile", "registered")
    require(manifest.get("plannedCases") == catalog(profile), "Publication needs the complete registered matrix")
    sources = {key: manifest.get(key) for key in SOURCES}
    for key, relative in SOURCES.items():
        require(sources[key] == hashlib.sha256((ROOT / relative).read_bytes()).hexdigest(), "Measurement source changed")
    reference_revision = (ROOT / "TYPE_RB_REVISION").read_text().strip()
    rows = []
    for case in manifest["cases"]:
        require(case["input"] in manifest["plannedCases"], "Unregistered result input")
        summaries = {}
        for role in ROLES:
            data = case["summaries"][role]
            require(data["processWallSeconds"]["count"] == manifest["samples"], "Missing retained samples")
            summaries[role] = {"wallSeconds": data["processWallSeconds"]["p50"],
                               **{key: data[key] for key in METRICS if key != "wallSeconds"}}
        row = {"case": case["input"]["id"], "summaries": summaries}
        if any("sampleRanges" in case["summaries"][role] for role in ROLES):
            require(all("sampleRanges" in case["summaries"][role] for role in ROLES), "Missing sample ranges")
            row["sampleRanges"] = {role: case["summaries"][role]["sampleRanges"] for role in ROLES}
        rows.append(row)
    # Explicit allowlist: raw manifests also contain machine-local tool paths.
    value = {"at": manifest["startedAt"], "revision": attempt["revision"], "referenceRevision": reference_revision,
             "fingerprint": attempt["fingerprint"], "runUrl": attempt["runUrl"], "sources": sources,
             "platform": manifest["platform"], "machine": manifest["machine"],
             "samples": manifest["samples"], "warmups": manifest["warmups"],
             "inputs": manifest["plannedCases"], "rows": rows, "inputProfile": profile,
             "tools": {name: {key: manifest["tools"][name][key] for key in ("sha256", "version")}
                       for name in ("native", "reference", "go")},
             "binaries": {role: {key: manifest["binaries"][role][key] for key in ("sha256", "bytes")} for role in ROLES},
             "environment": {key: manifest["environment"][key] for key in ("GOMAXPROCS", "affinity", "cpuPinning", "filesystemCache")}}
    for key in ("peakRssMethod", "timingMethod"):
        if key in manifest["environment"]:
            value["environment"][key] = manifest["environment"][key]
    if "rssTool" in manifest["environment"]:
        value["environment"]["rssTool"] = {key: manifest["environment"]["rssTool"][key] for key in ("sha256", "version")}
    value["series"] = hashlib.sha256(json.dumps([sources, reference_revision, value["samples"], value["warmups"],
                                               value["platform"], value["machine"], value["environment"],
                                               profile, value["inputs"]], sort_keys=True).encode()).hexdigest()
    return validate_snapshot(value)


def finish(path, manifest_path, evidence_status):
    state = validate(daily.read(path))
    attempt = state["attempt"]
    attempt["status"] = "infrastructure-failure"
    if evidence_status == "success" and Path(manifest_path).is_file():
        manifest = daily.read(manifest_path)
        attempt["status"] = "measurement-failure"
        attempt["completedCases"] = len(manifest.get("cases", []))
        failure = manifest.get("failedObservation")
        if failure and failure.get("case") in {case["id"] for case in catalog()} and failure.get("role") in ROLES:
            attempt["failedObservation"] = {key: failure[key] for key in ("case", "role", "status")}
        # Build/preparation failures have no complete source/tool identity yet.
        if "repositoryRevision" in manifest:
            result = snapshot(manifest, attempt)
            if manifest.get("status") == "pass":
                require(len(result["rows"]) == len(catalog()), "Successful run omitted cases")
                state["latest"] = result
                state["history"] = [*state["history"], result][-HISTORY_LIMIT:]
                attempt["status"] = "measured"
            else:
                attempt["partial"] = result
    daily.write(path, validate(state))


def site(path, output):
    state = validate(daily.read(path))
    trigger = os.environ.get("TRIGGER_RUN_ID")
    if trigger and os.environ.get("TRIGGER_CONCLUSION") in ("failure", "cancelled", "timed_out"):
        run_url = f"https://github.com/type-rb/type-rb-native/actions/runs/{trigger}"
        if (state.get("attempt") or {}).get("runUrl") != run_url or state["attempt"]["status"] == "running":
            state["attempt"] = {"at": daily.now(), "revision": os.environ["TRIGGER_HEAD_SHA"],
                                "status": "infrastructure-failure", "runUrl": run_url}
    state["siteRevision"] = daily.git("rev-parse", "HEAD")
    state["pendingChanges"] = not state["latest"] or state["latest"]["fingerprint"] != fingerprint()
    daily.write(output, validate(state))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("restore", "start", "finish", "site"))
    parser.add_argument("--state", required=True)
    parser.add_argument("--manifest")
    parser.add_argument("--evidence-status", default="failure")
    parser.add_argument("--output")
    args = parser.parse_args()
    if args.command == "restore":
        restore(args.state)
    elif args.command == "start":
        start(args.state)
    elif args.command == "finish":
        finish(args.state, args.manifest, args.evidence_status)
    else:
        site(args.state, args.output)


if __name__ == "__main__":
    main()
