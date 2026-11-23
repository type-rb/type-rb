#!/usr/bin/env python3

"""Measure one or more direct command executions with a monotonic clock.

The measured command is executed directly, without an intermediary shell.  Peak
RSS is intentionally measured by a separate GNU time invocation owned by the
native target harness so elapsed time and orchestration-root RSS remain independent
observations. Repeated elapsed measurements retain both the raw batch duration
and its per-process normalization.
"""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time


def usage() -> int:
    print(
        "usage: measure-command.py RECORD STDOUT STDERR ARTIFACT|- REPETITIONS -- COMMAND [ARG ...]",
        file=sys.stderr,
    )
    return 64


def artifact_record(path: str) -> dict[str, object]:
    artifact = Path(path)
    if not artifact.is_file():
        return {"path": path, "exists": False}

    digest = hashlib.sha256()
    with artifact.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return {
        "path": path,
        "exists": True,
        "executable": os.access(artifact, os.X_OK),
        "size": artifact.stat().st_size,
        "sha256": digest.hexdigest(),
    }


def main(arguments: list[str]) -> int:
    if len(arguments) < 8 or arguments[6] != "--":
        return usage()

    record_path = Path(arguments[1])
    stdout_path = Path(arguments[2])
    stderr_path = Path(arguments[3])
    artifact_path = arguments[4]
    try:
        repetitions = int(arguments[5])
    except ValueError:
        return usage()
    if repetitions < 1 or repetitions > 1_000:
        return usage()
    command = arguments[7:]
    if not command:
        return usage()

    try:
        clock = time.get_clock_info("monotonic")
        input_before = artifact_record(command[0])
        first_status: int | None = None
        first_stdout: bytes | None = None
        first_stderr: bytes | None = None
        started = time.monotonic_ns()
        for _ in range(repetitions):
            process = subprocess.run(
                command,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                check=False,
            )
            if first_status is None:
                first_status = process.returncode
                first_stdout = process.stdout
                first_stderr = process.stderr
            elif (
                process.returncode != first_status
                or process.stdout != first_stdout
                or process.stderr != first_stderr
            ):
                raise ValueError("repeated command status or output differs")
        finished = time.monotonic_ns()
        input_after = artifact_record(command[0])

        assert first_status is not None
        assert first_stdout is not None
        assert first_stderr is not None
        stdout_path.write_bytes(first_stdout)
        stderr_path.write_bytes(first_stderr)
        elapsed_nanoseconds = finished - started
        elapsed_per_repetition_seconds = (
            elapsed_nanoseconds / repetitions / 1_000_000_000
        )
        record: dict[str, object] = {
            "schemaVersion": 1,
            "clock": "time.monotonic_ns",
            "clockInfo": {
                "implementation": clock.implementation,
                "resolutionSeconds": clock.resolution,
                "monotonic": clock.monotonic,
                "adjustable": clock.adjustable,
            },
            "command": command,
            "repetitions": repetitions,
            "completedRepetitions": repetitions,
            "outputsIdentical": True,
            "elapsedNanoseconds": elapsed_nanoseconds,
            "elapsedSeconds": f"{elapsed_nanoseconds / 1_000_000_000:.9f}",
            "elapsedPerRepetitionSeconds": f"{elapsed_per_repetition_seconds:.9f}",
            "status": first_status,
            "inputExecutableBefore": input_before,
            "inputExecutableAfter": input_after,
            "artifact": None if artifact_path == "-" else artifact_record(artifact_path),
        }
        record_path.write_text(
            json.dumps(record, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    except (OSError, ValueError) as error:
        print(f"measure-command: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
