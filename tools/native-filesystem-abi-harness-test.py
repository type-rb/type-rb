#!/usr/bin/env python3
"""Exercise ABI qualification failure retention and owned-process cleanup."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import unittest

SCRIPT = Path(__file__).with_name('native-filesystem-abi-test.py')
spec = importlib.util.spec_from_file_location('filesystem_abi', SCRIPT)
abi = importlib.util.module_from_spec(spec)
spec.loader.exec_module(abi)


class QualificationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='filesystem-abi-controls-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.tools = self.root / 'tools'
        self.tools.mkdir()
        self.receipt = self.root / 'evidence' / 'abi.json'
        self.compiler = shutil.which(os.environ.get('CC', 'cc'))
        self.inspector = shutil.which('nm')
        self.assertTrue(self.compiler and self.inspector, 'real C compiler and nm required')
        self.environment = dict(os.environ, CC=self.compiler,
                                PATH=str(self.tools) + os.pathsep + os.environ['PATH'])

    def tool(self, name, source):
        path = self.tools / name
        path.write_text('#!' + sys.executable + '\n' + source)
        path.chmod(0o755)
        return path

    def qualify(self, success):
        result = subprocess.run([sys.executable, str(SCRIPT), '--receipt', str(self.receipt)],
                                env=self.environment, text=True, capture_output=True, timeout=90)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        receipt = json.loads(self.receipt.read_text())
        self.assertEqual(receipt['complete'], success)
        return receipt

    def test_delayed_inspection_still_checks_actual_symbols(self):
        self.tool('nm', 'import os, sys, time\n'
                  'time.sleep(10.25)\n'
                  'print("controlled inspector startup delay", file=sys.stderr, flush=True)\n'
                  'os.execv(' + repr(self.inspector) + ', ["nm", *sys.argv[1:]])\n')
        receipt = self.qualify(True)
        inspection = receipt['checks'][2]
        self.assertGreaterEqual(inspection['elapsed_seconds'], 10)
        self.assertIn('controlled inspector startup delay', inspection['stderr'])
        self.assertTrue(receipt['undefined_symbols'])
        self.assertTrue(all(check['complete'] for check in receipt['checks']))
        self.assertEqual(receipt['checks'][1]['timeout_seconds'], 10)

    def test_failed_inspector_keeps_output_and_does_not_retry(self):
        self.tool('nm', 'import sys\nprint("partial symbols", flush=True)\n'
                  'print("inspection failure", file=sys.stderr, flush=True)\nsys.exit(23)\n')
        receipt = self.qualify(False)
        self.assertEqual(len(receipt['checks']), 3)
        inspection = receipt['checks'][-1]
        self.assertEqual(inspection['code'], 23)
        self.assertEqual(inspection['stdout'], 'partial symbols\n')
        self.assertEqual(inspection['stderr'], 'inspection failure\n')
        self.assertFalse(inspection['complete'])
        self.assertNotIn('undefined_symbols', receipt)

    def test_failed_compilation_stops_before_probe_or_inspection(self):
        compiler = self.tool('broken-cc', 'import sys\n'
                             'print("ABI compile failure", file=sys.stderr)\nsys.exit(24)\n')
        self.environment['CC'] = str(compiler)
        receipt = self.qualify(False)
        self.assertEqual(len(receipt['checks']), 1)
        self.assertEqual(receipt['checks'][0]['code'], 24)
        self.assertIn('ABI compile failure', receipt['checks'][0]['stderr'])

    def test_missing_inspector_is_not_success(self):
        self.environment['PATH'] = str(self.tools)
        receipt = self.qualify(False)
        self.assertIsNone(receipt['tools']['symbols'])
        self.assertEqual(receipt['checks'], [])

    def test_timeout_retains_output_and_kills_owned_descendants(self):
        pid_file = self.root / 'child.pid'
        child = ('import os, pathlib, time; pathlib.Path(' + repr(str(pid_file)) +
                 ').write_text(str(os.getpid())); time.sleep(30)')
        parent = ('import subprocess, sys, time; subprocess.Popen([sys.executable, "-c", ' +
                  repr(child) + ']); print("partial output", flush=True); '
                  'print("waiting", file=sys.stderr, flush=True); time.sleep(30)')
        checks = []
        with self.assertRaises(subprocess.TimeoutExpired):
            abi.run_command([sys.executable, '-c', parent], self.root, checks, timeout=1)
        self.assertEqual(len(checks), 1)
        self.assertTrue(checks[0]['timed_out'])
        self.assertFalse(checks[0]['complete'])
        self.assertEqual(checks[0]['stdout'], 'partial output\n')
        self.assertEqual(checks[0]['stderr'], 'waiting\n')
        self.assertGreaterEqual(checks[0]['elapsed_seconds'], 1)
        pid = int(pid_file.read_text())
        deadline = time.monotonic() + 2
        while True:
            status = subprocess.run(['ps', '-o', 'stat=', '-p', str(pid)],
                                    capture_output=True, text=True, timeout=5).stdout.strip()
            if not status or status.startswith('Z'):
                break
            self.assertLess(time.monotonic(), deadline, 'owned descendant survived timeout: ' + status)
            time.sleep(0.05)


if __name__ == '__main__':
    unittest.main()
