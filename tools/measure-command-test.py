#!/usr/bin/env python3

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


CONTROLLER = Path(__file__).with_name("measure-command.py")


class CommandMeasurementTest(unittest.TestCase):
    def run_controller(
        self,
        root: Path,
        label: str,
        artifact: str,
        command: list[str],
        repetitions: int = 1,
    ) -> tuple[subprocess.CompletedProcess[bytes], Path, Path, Path]:
        record = root / f"{label}.json"
        stdout = root / f"{label}.stdout"
        stderr = root / f"{label}.stderr"
        result = subprocess.run(
            [
                sys.executable,
                str(CONTROLLER),
                str(record),
                str(stdout),
                str(stderr),
                artifact,
                str(repetitions),
                "--",
                *command,
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        return result, record, stdout, stderr

    def test_records_status_output_and_monotonic_elapsed_time(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            result, record, stdout, stderr = self.run_controller(
                root,
                "failure",
                "-",
                [
                    sys.executable,
                    "-c",
                    "import sys; print('out'); print('err', file=sys.stderr); sys.exit(3)",
                ],
            )
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, b"")
            self.assertEqual(result.stderr, b"")
            self.assertEqual(stdout.read_bytes(), b"out\n")
            self.assertEqual(stderr.read_bytes(), b"err\n")
            measurement = json.loads(record.read_text(encoding="utf-8"))
            self.assertEqual(measurement["clock"], "time.monotonic_ns")
            self.assertTrue(measurement["clockInfo"]["monotonic"])
            self.assertGreater(measurement["clockInfo"]["resolutionSeconds"], 0)
            self.assertEqual(measurement["status"], 3)
            self.assertEqual(measurement["repetitions"], 1)
            self.assertEqual(measurement["completedRepetitions"], 1)
            self.assertTrue(measurement["outputsIdentical"])
            self.assertGreater(measurement["elapsedNanoseconds"], 0)
            self.assertEqual(
                measurement["elapsedPerRepetitionSeconds"],
                measurement["elapsedSeconds"],
            )
            self.assertIsNone(measurement["artifact"])
            self.assertEqual(
                measurement["inputExecutableBefore"]["sha256"],
                measurement["inputExecutableAfter"]["sha256"],
            )

    def test_records_the_immediate_artifact_identity(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            artifact = root / "artifact"
            payload = b"measured-artifact\n"
            result, record, stdout, stderr = self.run_controller(
                root,
                "artifact",
                str(artifact),
                [
                    sys.executable,
                    "-c",
                    "from pathlib import Path; import os, sys; "
                    "path = Path(sys.argv[1]); path.write_bytes(b'measured-artifact\\n'); "
                    "os.chmod(path, 0o755)",
                    str(artifact),
                ],
            )
            self.assertEqual(result.returncode, 0)
            self.assertEqual(stdout.read_bytes(), b"")
            self.assertEqual(stderr.read_bytes(), b"")
            measurement = json.loads(record.read_text(encoding="utf-8"))
            self.assertEqual(measurement["status"], 0)
            self.assertEqual(
                measurement["artifact"],
                {
                    "path": str(artifact),
                    "exists": True,
                    "executable": True,
                    "size": len(payload),
                    "sha256": hashlib.sha256(payload).hexdigest(),
                },
            )

    def test_reports_observer_failures_without_a_false_record(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            result, record, stdout, stderr = self.run_controller(
                root,
                "missing",
                "-",
                [str(root / "missing-command")],
            )
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, b"")
            self.assertIn(b"measure-command:", result.stderr)
            self.assertFalse(record.exists())
            self.assertFalse(stdout.exists())
            self.assertFalse(stderr.exists())

    def test_batches_direct_processes_and_retains_one_exact_output(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            result, record, stdout, stderr = self.run_controller(
                root,
                "batch",
                "-",
                [sys.executable, "-c", "print('same')"],
                repetitions=4,
            )
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, b"")
            self.assertEqual(result.stderr, b"")
            self.assertEqual(stdout.read_bytes(), b"same\n")
            self.assertEqual(stderr.read_bytes(), b"")
            measurement = json.loads(record.read_text(encoding="utf-8"))
            self.assertEqual(measurement["repetitions"], 4)
            self.assertEqual(measurement["completedRepetitions"], 4)
            self.assertTrue(measurement["outputsIdentical"])
            self.assertGreater(measurement["elapsedNanoseconds"], 0)
            self.assertEqual(
                measurement["elapsedPerRepetitionSeconds"],
                f"{measurement['elapsedNanoseconds'] / 4 / 1_000_000_000:.9f}",
            )

    def test_rejects_inconsistent_repeated_output(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            counter = root / "counter"
            result, record, stdout, stderr = self.run_controller(
                root,
                "inconsistent",
                "-",
                [
                    sys.executable,
                    "-c",
                    "from pathlib import Path; import sys; "
                    "path = Path(sys.argv[1]); "
                    "value = int(path.read_text()) + 1 if path.exists() else 1; "
                    "path.write_text(str(value)); print(value)",
                    str(counter),
                ],
                repetitions=2,
            )
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, b"")
            self.assertIn(b"repeated command status or output differs", result.stderr)
            self.assertFalse(record.exists())
            self.assertFalse(stdout.exists())
            self.assertFalse(stderr.exists())

    def test_rejects_out_of_range_repetition_counts(self) -> None:
        with tempfile.TemporaryDirectory(prefix="measure-command-test-") as temporary:
            root = Path(temporary)
            for repetitions in (0, 1_001):
                result, record, stdout, stderr = self.run_controller(
                    root,
                    f"invalid-{repetitions}",
                    "-",
                    [sys.executable, "-c", "print('unreachable')"],
                    repetitions=repetitions,
                )
                self.assertEqual(result.returncode, 64)
                self.assertEqual(result.stdout, b"")
                self.assertIn(b"usage: measure-command.py", result.stderr)
                self.assertFalse(record.exists())
                self.assertFalse(stdout.exists())
                self.assertFalse(stderr.exists())


if __name__ == "__main__":
    unittest.main()
