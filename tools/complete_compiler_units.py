#!/usr/bin/env python3
"""Run every compiled compiler unit with bounded concurrency.

The pinned TypeRB test executable honors TRB_TEST_FILE and emits structured
events. Test files are discovered dynamically, including colocated units and feature
suites. Each selected file runs once in its own process.
Each process gets a separate temporary directory.
"""

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time


# Start the measured long files before the ordinary alphabetical queue, so a
# late source-loading suite does not leave the other worker idle. These are
# scheduling hints only: discovery still includes every current test file.
# Timing source: actions/runs/39569092444, compiler-units artifact.
EARLY_TEST_FILES = (
    "compiler/src/tests/compiler/source_loading_test.trb",
    "compiler/src/backend/qbe/emit/constants_test.trb",
    "compiler/src/mir/passes/optimize_test.trb",
    "compiler/src/mir/analysis/numeric_selection_test.trb",
)


def test_files(root: Path) -> list[Path]:
    source = root / "compiler" / "src"
    priority = {name: index for index, name in enumerate(EARLY_TEST_FILES)}
    return sorted(
        (path.resolve() for path in source.rglob("*_test.trb") if path.is_file()),
        key=lambda path: (priority.get(path.relative_to(root).as_posix(), len(priority)), path),
    )


def run_file(binary: Path, path: Path, root: Path, scratch: Path, timeout: int) -> dict:
    with tempfile.TemporaryDirectory(prefix="compiler-units-", dir=scratch) as temporary:
        environment = os.environ.copy()
        environment.pop("TRB_TEST_NAMES", None)
        environment.update({
            "TRB_TEST_FILE": str(path),
            "TRB_TEST_REPORTER": "json",
            "TMPDIR": temporary,
            "TMP": temporary,
            "TEMP": temporary,
        })
        start = time.monotonic()
        try:
            process = subprocess.run(
                [str(binary)], cwd=root, env=environment, capture_output=True, text=True,
                timeout=timeout, check=False,
            )
        except subprocess.TimeoutExpired as error:
            return {"file": path, "seconds": time.monotonic() - start,
                    "output": (error.stdout or b"").decode(errors="replace"),
                    "stderr": (error.stderr or b"").decode(errors="replace"),
                    "names": [], "error": f"timed out after {timeout}s"}
        seconds = time.monotonic() - start

    errors = []
    events = []
    for line in process.stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            # Compiler diagnostics are ordinary test output. Structured test
            # events and their final summary still have to account for all tests.
            continue
        if not isinstance(event, dict) or not isinstance(event.get("type"), str):
            errors.append("malformed test event")
            break
        events.append(event)
    started = [event for event in events if event["type"] == "test_started"]
    passed = [event for event in events if event["type"] == "test_passed"]
    failed = [event for event in events if event["type"] == "test_failed"]
    summaries = [event for event in events if event["type"] == "test_summary"]
    names = [event.get("name") for event in passed]
    if process.returncode != 0:
        errors.append(f"test executable exited {process.returncode}")
    if len(summaries) != 1:
        errors.append("expected exactly one test summary")
    elif (not isinstance(summaries[0].get("total"), int)
          or summaries[0]["total"] <= 0
          or summaries[0].get("failed") != 0
          or summaries[0]["total"] != len(passed)):
        errors.append("test summary does not match passing events")
    if failed or len(started) != len(passed):
        errors.append("started, passed, or failed event count differs")
    if (any(event.get("test_file") != str(path) for event in started + passed + failed)
            or any(not isinstance(name, str) or not name for name in names)):
        errors.append("test identity differs from selected source file")
    if (len(names) != len(set(names))
            or {event.get("name") for event in started} != set(names)):
        errors.append("test names are duplicated or incomplete")
    return {"file": path, "seconds": seconds, "output": process.stdout,
            "stderr": process.stderr, "names": names, "error": "; ".join(errors)}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, type=Path)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--scratch", type=Path, default=Path(tempfile.gettempdir()))
    parser.add_argument("--jobs", type=int, default=2)
    parser.add_argument("--timeout-seconds", type=int, default=1200)
    arguments = parser.parse_args()
    root = arguments.root.resolve()
    binary = arguments.binary.resolve()
    if arguments.jobs < 1 or arguments.jobs > 8 or arguments.timeout_seconds < 1:
        parser.error("jobs must be 1..8 and timeout-seconds must be positive")
    files = test_files(root)
    if not files or not binary.is_file() or not os.access(binary, os.X_OK):
        parser.error("compiler test files or compiled test executable are missing")
    if not arguments.scratch.is_dir():
        parser.error("scratch directory does not exist")
    tasks = [(path, "whole file") for path in files]

    started = time.monotonic()
    results = []
    with ThreadPoolExecutor(max_workers=arguments.jobs) as executor:
        futures = {
            executor.submit(run_file, binary, path, root, arguments.scratch,
                            arguments.timeout_seconds): (path, label)
            for path, label in tasks
        }
        for future in as_completed(futures):
            path, label = futures[future]
            try:
                result = future.result()
            except Exception as error:
                result = {"file": path, "seconds": 0, "output": "",
                          "stderr": "", "names": [], "error": str(error)}
            results.append(result)
            print(f"=== {path.relative_to(root)} [{label}] ({result['seconds']:.2f}s) ===", flush=True)
            print(result["output"], end="" if str(result["output"]).endswith("\n") else "\n", flush=True)
            if result["stderr"]:
                print(result["stderr"], file=sys.stderr, flush=True)
            if result["error"]:
                print(f"FAIL {path}: {result['error']}", file=sys.stderr, flush=True)

    names = [name for result in results for name in result["names"]]
    if len(names) != len(set(names)):
        print("FAIL duplicate test names across source files", file=sys.stderr)
        return 1
    if any(result["error"] for result in results):
        print(f"FAIL compiler units: {len(files)} files, {len(names)} passing tests", file=sys.stderr)
        return 1
    print(f"PASS compiler units: {len(files)} files, {len(names)} tests, "
          f"{time.monotonic() - started:.2f}s execution", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
