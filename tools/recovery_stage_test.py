import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('recovery_stage', Path(__file__).with_name('recovery-stage.py'))
stage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)


class RecoveryStageTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.path = Path(self.directory.name) / 'stages.jsonl'

    def test_complete_ordered_monotonic_evidence(self):
        with patch.object(stage.time, 'monotonic_ns', side_effect=range(100, 200)):
            for name in stage.STAGES:
                stage.record(self.path, 'start', name)
                stage.record(self.path, 'end', name)
            self.assertTrue(stage.finalize(self.path, 'success'))
        summary = json.loads(self.path.with_suffix('.summary.json').read_text())
        self.assertEqual([row['name'] for row in summary['stages']], list(stage.STAGES))
        self.assertTrue(all(row['elapsedSeconds'] == 1e-9 for row in summary['stages']))
        self.assertTrue(all(row['state'] == 'completed' for row in summary['stages']))

    def test_missing_incomplete_failure_and_cancellation_never_complete(self):
        self.assertFalse(stage.finalize(self.path, 'success'))
        stage.record(self.path, 'start', stage.STAGES[0])
        for outcome in ('success', 'failed', 'cancelled'):
            self.assertFalse(stage.finalize(self.path, outcome))
            summary = json.loads(self.path.with_suffix('.summary.json').read_text())
            self.assertEqual(summary['stages'][0]['state'], 'incomplete' if outcome == 'success' else outcome)

    def test_stale_reordered_duplicate_and_malformed_receipts_fail(self):
        for event, name in [('end', stage.STAGES[0]), ('start', stage.STAGES[1])]:
            with self.assertRaises(ValueError):
                stage.record(self.path, event, name)
        stage.record(self.path, 'start', stage.STAGES[0])
        with self.assertRaises(ValueError):
            stage.record(self.path, 'start', stage.STAGES[0])
        with patch.object(stage.time, 'monotonic_ns', return_value=0):
            with self.assertRaises(ValueError):
                stage.record(self.path, 'end', stage.STAGES[0])
        for invalid in ('{', '{}\n', '[]\n', 'null\n', '{"stage":"x"}\n'):
            self.path.write_text(invalid)
            self.assertFalse(stage.finalize(self.path, 'success'))

    def test_completed_phases_do_not_mask_later_failure(self):
        for name in stage.STAGES:
            stage.record(self.path, 'start', name)
            stage.record(self.path, 'end', name)
        self.assertFalse(stage.finalize(self.path, 'failed'))

    def test_independent_invocations_and_write_failures(self):
        other = self.path.with_name('other.jsonl')
        stage.record(self.path, 'start', stage.STAGES[0])
        stage.record(other, 'start', stage.STAGES[0])
        stage.record(other, 'end', stage.STAGES[0])
        self.assertEqual(len(stage.read_events(self.path)), 1)
        self.assertEqual(len(stage.read_events(other)), 2)
        with self.assertRaises(OSError):
            stage.record(self.path / 'absent', 'start', stage.STAGES[0])


if __name__ == '__main__':
    unittest.main()
