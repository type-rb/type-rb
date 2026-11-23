#!/usr/bin/env python3
"""Bounded, opt-in application coverage; never part of the daily victory score."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import selectors
import shutil
import signal
import statistics
import subprocess
import sys
import tempfile
import threading
import time

ROOT = Path(__file__).resolve().parents[2]
SUITE = Path(__file__).with_name("suite.json")
SOURCE = ROOT / "benchmarks/representative/src/main.trb"
GO_SOURCE = ROOT / "benchmarks/representative/go/main.go"
OUTPUT_LIMIT = 1024 * 1024
GNU_TIME = Path("/usr/bin/time")


def rss_method():
    return "gnu-time-child-v1" if sys.platform.startswith("linux") else "wait4-bytes-v1"


def write(path, value):
    path.write_text(json.dumps(value, indent=2, allow_nan=False) + "\n")


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def percentile(values, fraction):
    """Nearest rank; sparse tails remain unknown, never a favorable score."""
    minimum = {0.5: 1, 0.95: 20, 0.99: 100}[fraction]
    if len(values) < minimum:
        return None
    return sorted(values)[math.ceil(len(values) * fraction) - 1]


def distribution(values):
    return {"count": len(values), "p50": percentile(values, 0.5),
            "p95": percentile(values, 0.95), "p99": percentile(values, 0.99),
            "maximum": max(values) if values else None}


def observe(command, directory, timeout, env=None):
    """Observe one owned process, including CPU/RSS and stdout arrival times.

    Linux wait4 includes memory inherited before exec. GNU time forks the
    command from its small address space and records that child's peak RSS.
    Wall/CPU include this launcher; raw records identify the collection method.
    Batch timestamps are observer-side completion arrivals, including pipe and
    scheduler overhead. Coalesced lines are recorded and invalidate interval
    percentiles; they cannot be interpreted as zero-latency work.
    """
    directory.mkdir(parents=True)
    started = time.monotonic()
    record = {"command": list(map(str, command)), "timeoutSeconds": timeout,
              "status": "pass", "arrivals": [], "peakRssMethod": rss_method()}
    invocation = record["command"]
    rss_file = directory / "rss-kib.txt"
    if sys.platform.startswith("linux"):
        invocation = [str(GNU_TIME), "-q", "-f", "%M", "-o", str(rss_file), "--", *invocation]
    process = subprocess.Popen(invocation, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True, env=env)
    completion = {}
    finished = threading.Event()

    def reap():
        _, status, usage = os.wait4(process.pid, 0)
        completion.update(status=status, usage=usage, wallSeconds=time.monotonic() - started)
        process.returncode = os.waitstatus_to_exitcode(status)
        finished.set()

    # Timestamp child completion independently of stdout draining/polling. A
    # selector timeout after EOF must not become apparent application runtime.
    waiter = threading.Thread(target=reap, daemon=True)
    waiter.start()
    streams = {"stdout": bytearray(), "stderr": bytearray()}
    pending = bytearray()
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ, "stdout")
    selector.register(process.stderr, selectors.EVENT_READ, "stderr")
    terminated = False

    def terminate():
        nonlocal terminated
        if terminated:
            return
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        terminated = True

    try:
        while selector.get_map() or not finished.is_set():
            elapsed = time.monotonic() - started
            if elapsed >= timeout:
                record["status"] = "timeout"
                terminate()
                break
            for key, _ in selector.select(min(0.02, timeout - elapsed)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                streams[key.data].extend(chunk)
                if sum(map(len, streams.values())) > OUTPUT_LIMIT:
                    record["status"] = "output-limit"
                    terminate()
                    break
                if key.data == "stdout":
                    pending.extend(chunk)
                    lines = pending.split(b"\n")
                    pending = bytearray(lines.pop())
                    arrival = time.monotonic() - started
                    batch_lines = sum(line.split(b",")[0].isdigit() for line in lines)
                    coalesced = batch_lines > 1 or (batch_lines > 0 and b"ready" in lines)
                    for line in lines:
                        record["arrivals"].append({"line": line.decode("utf-8", errors="replace"),
                                                   "seconds": arrival, "coalesced": coalesced})
            if record["status"] != "pass":
                break
        waiter.join()
        usage = completion["usage"]
        record.update(exitCode=process.returncode, wallSeconds=completion["wallSeconds"],
                      cpuSeconds=usage.ru_utime + usage.ru_stime,
                      peakRssBytes=int(usage.ru_maxrss) if sys.platform == "darwin" else None)
        if process.returncode and record["status"] == "pass":
            record["status"] = "nonzero-exit"
        if sys.platform.startswith("linux"):
            value = rss_file.read_text().strip() if rss_file.exists() else ""
            if value.isdigit() and int(value) > 0:
                record["peakRssBytes"] = int(value) * 1024
            elif record["status"] == "pass":
                record["status"] = "missing-resource-observation"
    finally:
        terminate()
        waiter.join()
        selector.close()
        process.stdout.close()
        process.stderr.close()
        process.wait()
        for name, data in streams.items():
            (directory / name).write_bytes(data)
        write(directory / "observation.json", record)
    return record


def expected(case, output_mode="stream"):
    """Independent correctness oracle for every registered input, not timings."""
    if output_mode not in ("stream", "summary"):
        raise ValueError("Unknown output mode: " + output_mode)
    count, variant, workload = case["count"], case["variant"], case["workload"]
    cache, retained = [""] * 128, [""] * count if variant == "retained" else []
    completed = 0
    lines = ["ready"]
    repeat_checksum = None
    total_checksum = 0
    for batch in range(case["batches"]):
        if repeat_checksum is not None:
            checksum = repeat_checksum
        elif workload == "log-pipeline":
            totals, rejected = {}, 0
            for index in range(count):
                if variant == "mixed" and index % 17 == 0:
                    rejected += 1
                else:
                    key = f"group-{index % 127}"
                    totals[key] = totals.get(key, 0) + index % 1000
            checksum = len("\n".join(f"{key}={totals[key]}" for key in sorted(totals))) + rejected
        elif workload == "json-transform":
            checksum = sum(1 if variant == "mixed" and index % 17 == 0 else
                           len(json.dumps([index + 1, "aβ😀" * 4, True], ensure_ascii=False,
                                          separators=(",", ":"))) for index in range(count))
        elif workload == "json-document":
            if variant == "wide-array":
                value = ["aβ😀"] * count
            elif variant == "wide-object":
                value = {f"key-{index}": "aβ😀" for index in range(count)}
            elif variant == "integer-array":
                value = [index % 127 - 63 for index in range(count)]
            elif variant == "fractional-array":
                value = [0.5 if index % 2 == 0 else 0.125 for index in range(count)]
            elif variant == "precise-fractional-array":
                numbers = (0.12345678901234568, -0.8765432109876543,
                           12345.678901234567, 0.0012345678901234567)
                value = [numbers[index % len(numbers)] for index in range(count)]
            elif variant == "mixed-number-object":
                value = {f"key-{index}": (index % 127 - 63 if index % 3 == 0 else
                                         0.5 if index % 3 == 1 else 0.125)
                         for index in range(count)}
            else:
                value = {"ascii": "abcdef", "unicode": "aβ😀",
                         "escaped": 'a\n"<>&\u2028'}[variant] * count
            encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
            for character in ("<", ">", "&", "\u2028", "\u2029"):
                encoded = encoded.replace(character, f"\\u{ord(character):04x}")
            checksum = len(encoded)
        elif workload == "worker":
            checksum = 0
            for index in range(count):
                if index % 17 == 0:
                    checksum += 1
                    continue
                value = f"job-{batch}-{index}-" + "aβ😀" * 8 + ("-retry" if index % 11 == 0 else "")
                cache[(index + batch) % 128] = value
                completed += 1
                checksum += len(value)
            checksum += sum(map(len, cache)) + completed
        elif workload == "hash":
            checksum = sum(1 if variant == "miss-heavy" and index % 10 else index for index in range(count))
        elif workload == "strings":
            checksum = count * (3 * (512 if variant == "long" else 4) + 5)
        elif workload == "arrays":
            values = {"sorted": lambda i: i, "reverse": lambda i: count - i,
                      "duplicates": lambda i: i % 8, "random": lambda i: (i * 7919 + 17) % 100003}
            checksum = sum(value * (index % 7 + 1) for index, value in
                           enumerate(sorted(values[variant](i) for i in range(count)))) % 1000000007
        elif workload == "allocation":
            values = [f"object-{batch}-{index}-" + "aβ😀" * 16 for index in range(count)]
            checksum = sum(map(len, values))
            if variant == "retained":
                retained = values
        else:
            raise ValueError("Unknown workload: " + workload)
        if workload not in ("worker", "allocation"):
            repeat_checksum = checksum
        if output_mode == "summary":
            total_checksum = (total_checksum + checksum) % 1000000007
        else:
            lines.append(f"{batch},{checksum}")
    if output_mode == "summary":
        lines.append(f"summary,{total_checksum}")
    lines.append(f"done,{sum(map(len, retained))}")
    return ("\n".join(lines) + "\n").encode()


def validate(record, directory, oracle):
    if record["status"] == "pass":
        if (directory / "stdout").read_bytes() != oracle:
            record["status"] = "output-mismatch"
        elif (directory / "stderr").stat().st_size:
            record["status"] = "unexpected-stderr"
    write(directory / "observation.json", record)
    return record


def summarize(records, units):
    retained = [r for r in records if r["phase"] == "retained"]
    if not retained or any(r["status"] != "pass" for r in records):
        return None
    walls = [r["wallSeconds"] for r in retained]
    intervals, coalesced = [], False
    for record in retained:
        arrivals = record["arrivals"]
        ready = next(a for a in arrivals if a["line"] == "ready")
        previous = ready["seconds"]
        for arrival in arrivals:
            if arrival["line"].split(",")[0].isdigit():
                coalesced |= arrival["coalesced"]
                intervals.append(arrival["seconds"] - previous)
                previous = arrival["seconds"]
    medians = {"wallSeconds": percentile(walls, 0.5),
               "cpuSeconds": statistics.median(r["cpuSeconds"] for r in retained),
               "peakRssBytes": statistics.median(r["peakRssBytes"] for r in retained)}
    medians["unitsPerSecond"] = units / medians["wallSeconds"]
    ranges = {}
    for metric, median in medians.items():
        values = [units / r["wallSeconds"] if metric == "unitsPerSecond" else r[metric] for r in retained]
        ranges[metric] = {"count": len(values), "minimum": min(values), "p50": median, "maximum": max(values)}
    return {"processWallSeconds": distribution(walls),
            "sampleRanges": ranges,
            "cpuSeconds": statistics.median(r["cpuSeconds"] for r in retained),
            "peakRssBytes": statistics.median(r["peakRssBytes"] for r in retained),
            "unitsPerSecond": units / statistics.median(walls),
            "batchArrivalIntervalSeconds": None if coalesced else distribution(intervals),
            "batchIntervalUnavailableReason": "coalesced stdout markers" if coalesced else None,
            "firstProcessWallSeconds": records[0]["wallSeconds"]}


def load_cases(smoke=False, selected=None, profile="registered"):
    suite = json.loads(SUITE.read_text())
    if profile not in suite["inputProfiles"]:
        raise ValueError("Unknown input profile: " + profile)
    if smoke and profile != "registered":
        raise ValueError("Correctness smoke uses only the registered profile")
    policy = suite["inputProfiles"][profile]
    cases = [dict(case, batches=case["batches"] * policy.get("caseMultipliers", {}).get(
        case["id"], policy["batchMultipliers"][case["size"]])) for case in suite["cases"]]
    if selected:
        known = {c["id"] for c in cases}
        if set(selected) - known:
            raise ValueError("Unknown case IDs: " + ", ".join(sorted(set(selected) - known)))
        cases = [c for c in cases if c["id"] in selected]
    if smoke:
        # Cover each path once with small inputs; timing has no acceptance role.
        unique = {}
        for case in cases:
            key = (case["workload"], case["variant"])
            unique.setdefault(key, dict(case, count=23, batches=2))
        cases = list(unique.values())
    return suite, cases


def build_programs(args, workspace, evidence, env):
    binaries = {}
    if args.native:
        binaries["native"] = workspace / "native"
        command = [args.native, "build", "--compile", SOURCE, "--outfile", binaries["native"]]
        result = observe(command, evidence / "build-native", 120, env)
        if result["status"] != "pass":
            raise RuntimeError("Native build failed; see build-native")
    if args.go:
        binaries["pure-go"] = workspace / "pure-go"
        result = observe([args.go, "build", "-trimpath", "-o", binaries["pure-go"], GO_SOURCE],
                         evidence / "build-pure-go", 120, env)
        if result["status"] != "pass":
            raise RuntimeError("Pure Go build failed; see build-pure-go")
    if args.reference:
        project = workspace / "reference"
        (project / "src").mkdir(parents=True)
        shutil.copyfile(SOURCE, project / "src/main.trb")
        config = project / "trbconfig.jsonc"
        write(config, {"name": "representative-performance", "version": "0.1.0", "mode": "go",
                       "sourceDir": "src", "outDir": "build", "copyFiles": False,
                       "packageManagement": "managed", "go": {"module": "example.org/representative", "version": "1.27"}})
        binaries["typerb-go"] = workspace / "typerb-go"
        result = observe([args.reference, "build", "--config", config, "--compile", "--outfile", binaries["typerb-go"]],
                         evidence / "build-typerb-go", 120, env)
        if result["status"] != "pass":
            raise RuntimeError("TypeRB Go build failed; see build-typerb-go")
    return binaries


def gc_probe(binary, case, evidence, env, output_mode="stream"):
    """Untimed Native managed-heap counters; not a Go heap comparison."""
    directory = evidence / case["id"] / "native-gc-probe"
    command = [binary, case["workload"], str(case["count"]), case["variant"], str(case["batches"])]
    if output_mode == "summary":
        command.append("summary")
    record = observe(command, directory, 30, dict(env, TYPE_RB_NATIVE_RUNTIME_STATS="1"))
    counters = {}
    if record["status"] == "pass":
        if (directory / "stdout").read_bytes() != expected(case, output_mode):
            record["status"] = "output-mismatch"
        else:
            for line in (directory / "stderr").read_text().splitlines():
                fields = line.split(",")
                if len(fields) != 3 or fields[0] != "type-rb-native-gc-stat-v1" or fields[1] in counters or not fields[2].isdigit():
                    record["status"] = "invalid-gc-counters"
                    break
                counters[fields[1]] = int(fields[2])
            if not {"allocated-bytes", "reclaimed-bytes", "live-bytes"} <= counters.keys():
                record["status"] = "missing-gc-counters"
    record.update(instrumented=True, comparisonScope="Native managed allocations only", counters=counters)
    write(directory / "observation.json", record)
    if record["status"] != "pass":
        raise RuntimeError(f"{case['id']}: {record['status']}")
    return {"input": case, "status": record["status"], "counters": counters}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", type=Path, help="Already built current Native CLI; never builds a compiler")
    parser.add_argument("--reference", type=Path, help="Pinned TypeRB Go CLI")
    parser.add_argument("--go", type=Path, help="Go executable for the hand-written control")
    parser.add_argument("--evidence", required=True, type=Path, help="New output directory")
    parser.add_argument("--smoke", action="store_true", help="Small correctness-only inputs")
    parser.add_argument("--profile", default="registered", help="Fixed input profile from suite.json (default: registered)")
    parser.add_argument("--output-mode", choices=("stream", "summary"), default="stream",
                        help="Summary bounds diagnostic output; it has no batch-arrival latency measurements")
    parser.add_argument("--case", action="append", help="Select an exact case ID (repeatable)")
    parser.add_argument("--samples", type=int, help="Fixed retained process count, 1..100")
    parser.add_argument("--gc-probes", action="store_true", help="Separate untimed Native worker/allocation counters")
    args = parser.parse_args()
    if not (args.native or args.reference or args.go):
        parser.error("Provide at least one compiler")
    if args.samples is not None and not 1 <= args.samples <= 100:
        parser.error("--samples must be between 1 and 100")
    if args.gc_probes and (not args.native or args.smoke):
        parser.error("--gc-probes requires --native without --smoke")
    for name in ("native", "reference", "go"):
        if getattr(args, name):
            setattr(args, name, getattr(args, name).resolve(strict=True))
    suite, cases = load_cases(args.smoke, args.case, args.profile)
    evidence = args.evidence.resolve()
    evidence.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOMAXPROCS="1", NO_COLOR="1", TERM="dumb")
    env.pop("TYPE_RB_NATIVE_RUNTIME_STATS", None)
    env.pop("TYPE_RB_NATIVE_RUNTIME_TRACE", None)
    result = {"schemaVersion": 1, "diagnosticOnly": True, "smoke": args.smoke,
              "startedAt": datetime.now(timezone.utc).isoformat(),
              "platform": platform.platform(), "machine": platform.machine(),
              "sourceSha256": digest(SOURCE), "goSourceSha256": digest(GO_SOURCE),
              "suiteSha256": digest(SUITE), "cases": [], "status": "running",
              "plannedCases": cases, "inputProfile": args.profile, "outputMode": args.output_mode,
              "coverageGaps": ["file/network input", "request queueing and service latency", "parallel scaling"],
              "measurementScope": "fresh-process wall/CPU/peak RSS; observer-side batch completion intervals include pipe/scheduler overhead"}
    if args.output_mode == "summary":
        result["measurementScope"] = "fresh-process wall/CPU/peak RSS with aggregate output; no batch-arrival latency measurements"
    write(evidence / "manifest.json", result)
    try:
        with tempfile.TemporaryDirectory(prefix="representative-performance-") as temporary:
            binaries = build_programs(args, Path(temporary), evidence, env)
            result["binaries"] = {role: {"sha256": digest(binary), "bytes": binary.stat().st_size}
                                  for role, binary in binaries.items()}
            result["tools"] = {name: {"path": str(getattr(args, name)), "sha256": digest(getattr(args, name))}
                               for name in ("native", "reference", "go") if getattr(args, name)}
            for name, tool in result["tools"].items():
                version = subprocess.run([tool["path"], "--version" if name == "native" else "version"],
                                         capture_output=True, text=True, timeout=10)
                if version.returncode:
                    raise RuntimeError(name + " version query failed")
                tool["version"] = version.stdout.strip()
            result["repositoryRevision"] = subprocess.check_output(["git", "-C", ROOT, "rev-parse", "HEAD"], text=True).strip()
            result["controllerSha256"] = digest(__file__)
            result["environment"] = {"GOMAXPROCS": "1", "cpuCount": os.cpu_count(),
                                     "affinity": sorted(os.sched_getaffinity(0)) if hasattr(os, "sched_getaffinity") else None,
                                     "filesystemCache": "uncontrolled warm cache",
                                     "cpuPinning": "inherited single CPU" if hasattr(os, "sched_getaffinity") and len(os.sched_getaffinity(0)) == 1 else "none",
                                     "peakRssMethod": rss_method(),
                                     "timingMethod": "process wall and wait4 CPU; Linux includes GNU time launcher"}
            if sys.platform.startswith("linux"):
                version = subprocess.check_output([GNU_TIME, "--version"], text=True).splitlines()[0]
                result["environment"]["rssTool"] = {"sha256": digest(GNU_TIME), "version": version}
            roles = list(binaries)
            started = time.monotonic()
            samples = 1 if args.smoke else args.samples or suite["samples"]
            warmups = 0 if args.smoke else suite["warmups"]
            result.update(samples=samples, warmups=warmups)
            for case in cases:
                oracle = expected(case, args.output_mode)
                rows = {role: [] for role in roles}
                for index in range(samples + warmups):
                    order = roles[index % len(roles):] + roles[:index % len(roles)]
                    for role in order:
                        remaining = suite["suiteTimeoutSeconds"] - (time.monotonic() - started)
                        if remaining <= 0:
                            raise TimeoutError("Suite time budget exhausted; partial evidence retained")
                        directory = evidence / case["id"] / role / str(index)
                        command = [binaries[role], case["workload"], str(case["count"]), case["variant"], str(case["batches"])]
                        if args.output_mode == "summary":
                            command.append("summary")
                        record = observe(command, directory, min(remaining, suite["timeoutSeconds"]), env)
                        record.update(phase="warmup" if index < warmups else "retained", role=role)
                        validate(record, directory, oracle)
                        rows[role].append(record)
                        if record["status"] != "pass":
                            result["failedObservation"] = {"case": case["id"], "role": role, "status": record["status"]}
                            raise RuntimeError(f"{case['id']}/{role}: {record['status']}; observations retained")
                summaries = {role: summarize(records, case["count"] * case["batches"]) for role, records in rows.items()}
                ratios = {}
                if "native" in summaries:
                    for role in roles:
                        if role != "native":
                            ratios[role] = summaries["native"]["processWallSeconds"]["p50"] / summaries[role]["processWallSeconds"]["p50"]
                result["cases"].append({"input": case, "summaries": summaries, "nativeWallRatio": ratios})
                write(evidence / "manifest.json", result)
                print(case["id"] + ": pass", flush=True)
            if args.gc_probes:
                result["nativeGcProbes"] = []
                for case in cases:
                    if case["size"] == "medium" and case["workload"] in ("worker", "allocation"):
                        result["nativeGcProbes"].append(gc_probe(binaries["native"], case, evidence, env, args.output_mode))
                        write(evidence / "manifest.json", result)
            result["status"] = "pass"
    except Exception as error:
        result.update(status="failed", error=str(error))
        raise
    finally:
        result["finishedAt"] = datetime.now(timezone.utc).isoformat()
        write(evidence / "manifest.json", result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
