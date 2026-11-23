#!/usr/bin/env python3
"""Exercise prerequisite decisions, failed acquisition and owned child cleanup."""
import importlib.util
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('prerequisites', Path(__file__).with_name('ci-linux-prerequisites.py'))
prerequisites = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prerequisites)


class PrerequisiteTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='linux prerequisites ')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.bin = self.root / 'installed'
        self.bin.mkdir()
        self.export = self.root / 'github-path'
        self.export.touch()
        self.environment = patch.dict(os.environ, PATH=str(self.bin), GITHUB_PATH=str(self.export))
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.archive = self.root / 'resource/lib/linux/libclang_rt.asan-aarch64.a'
        self.archive.parent.mkdir(parents=True)
        self.archive.touch()
        self.compiler = '''import pathlib,sys
if '-print-resource-dir' in sys.argv:
 print(RESOURCE)
elif '-o' in sys.argv:
 output=pathlib.Path(sys.argv[sys.argv.index('-o')+1])
 output.write_text('#!'+sys.executable+'\\nraise SystemExit(0)\\n');output.chmod(0o700)
else: print('test compiler')
'''.replace('RESOURCE', repr(str(self.root / 'resource')))
        for name in ('cc', 'clang'):
            self.tool(name, self.compiler)
        for name in ('time', 'nm', 'objdump', 'readelf', 'file', 'jq', 'strace', 'valgrind'):
            self.tool(name, f'print({name!r} + " version")')
        self.tool('ld.lld-18', 'print("Ubuntu LLD 18.1.3")')
        self.tool('sudo', 'raise SystemExit("unexpected package acquisition")')

    def tool(self, name, source):
        path = self.bin / name
        path.write_text('#!' + sys.executable + '\n' + source + '\n')
        path.chmod(0o700)
        return path

    def invoke(self, profile='targets'):
        output = self.root / 'receipt.json'
        args = ['prerequisites', profile, '--bin-dir', str(self.root / 'job-bin'), '--receipt', str(output)]
        with patch.object(sys, 'argv', args), patch.object(prerequisites.platform, 'system', return_value='Linux'), \
             patch.object(prerequisites.platform, 'machine', return_value='aarch64'), patch('builtins.print'):
            prerequisites.main()
        return json.loads(output.read_text())

    def failure_receipt(self):
        receipt = json.loads((self.root / 'receipt.json').read_text())
        self.assertFalse(receipt['complete'])
        self.assertEqual(self.export.read_text(), '')
        return receipt

    def test_installed_versioned_linker_needs_no_package_network(self):
        receipt = self.invoke()
        self.assertTrue(receipt['complete'])
        self.assertEqual(receipt['missingPackages'], [])
        self.assertEqual((self.root / 'job-bin/ld.lld').resolve(), (self.bin / 'ld.lld-18').resolve())
        self.assertEqual(self.export.read_text(), str((self.root / 'job-bin').resolve()) + '\n')
        self.assertFalse(any('apt-get' in row['command'] for row in receipt['commands']))
        self.assertTrue(any('-fuse-ld=lld' in row['command'] for row in receipt['commands']))

    def test_existing_unversioned_linker_is_preserved(self):
        self.tool('ld.lld', 'print("LLD 18.1.3")')
        receipt = self.invoke('cli')
        self.assertTrue(receipt['complete'])
        self.assertNotIn('linkerAlias', receipt)
        self.assertEqual(receipt['tools']['ld.lld']['path'], str(self.bin / 'ld.lld'))
        self.assertEqual(self.export.read_text(), '')

    def test_only_missing_tool_is_acquired_and_rechecked(self):
        (self.bin / 'strace').unlink()
        program = '#!' + sys.executable + '\nprint("strace installed")\n'
        self.tool('sudo', f'''import sys,pathlib
if 'install' in sys.argv:
 assert sys.argv[1:]==['-n','timeout','--kill-after=10s','840s','apt-get','install','-y','strace']
 p=pathlib.Path({str(self.bin / 'strace')!r});p.write_text({program!r});p.chmod(0o700)
''')
        receipt = self.invoke()
        self.assertEqual(receipt['missingPackages'], ['strace'])
        self.assertEqual(receipt['tools']['strace']['version'], 'strace installed')
        calls = [r['command'] for r in receipt['commands'] if 'apt-get' in r['command']]
        self.assertEqual(len(calls), 2)
        self.assertIn('APT::Update::Error-Mode=any', calls[0])
        for command in calls:
            self.assertEqual(command[:6], ['sudo', '-n', 'timeout', '--kill-after=10s', '840s', 'apt-get'])

    def test_acquisition_failure_retains_diagnostics_and_does_not_retry(self):
        (self.bin / 'strace').unlink()
        self.tool('sudo', 'import sys; print("DNS unavailable",file=sys.stderr);sys.exit(100)')
        with self.assertRaisesRegex(RuntimeError, 'exited 100'):
            self.invoke()
        receipt = self.failure_receipt()
        calls = [r for r in receipt['commands'] if 'apt-get' in r['command']]
        self.assertEqual(len(calls), 1)
        self.assertIn('DNS unavailable', calls[0]['stderr'])
        self.assertEqual(calls[0]['exit'], 100)

    def test_successful_package_command_cannot_hide_missing_executable(self):
        (self.bin / 'strace').unlink()
        self.tool('sudo', 'raise SystemExit(0)')
        with self.assertRaisesRegex(RuntimeError, 'required executable still missing: strace'):
            self.invoke()
        self.failure_receipt()

    def test_invalid_versioned_linker_is_not_published(self):
        self.tool('ld.lld-18', 'print("LLD 19.0.0")')
        with self.assertRaisesRegex(RuntimeError, 'expected installed LLD 18'):
            self.invoke()
        self.failure_receipt()
        self.assertFalse((self.root / 'job-bin').exists())

    def test_memory_requires_archive_and_actual_sanitized_execution(self):
        receipt = self.invoke('memory')
        self.assertEqual(receipt['asanArchive'], str(self.archive))
        self.assertTrue(any('-fsanitize=address' in r['command'] for r in receipt['commands']))
        self.assertTrue(receipt['commands'][-1]['command'][0].endswith('/sanitized'))

    def test_missing_sanitizer_is_not_a_passing_memory_profile(self):
        self.archive.unlink()
        self.tool('sudo', 'raise SystemExit(0)')
        with self.assertRaisesRegex(RuntimeError, 'AddressSanitizer archive is missing'):
            self.invoke('memory')
        self.assertEqual(self.failure_receipt()['missingPackages'], ['libclang-rt-dev'])

    def test_link_failure_is_retained_without_package_fallback(self):
        self.tool('cc', 'import sys; print("link rejected");sys.exit(23 if "-o" in sys.argv else 0)')
        with self.assertRaisesRegex(RuntimeError, 'exited 23'):
            self.invoke('linker')
        receipt = self.failure_receipt()
        self.assertEqual(receipt['commands'][-1]['exit'], 23)
        self.assertFalse(any('apt-get' in r['command'] for r in receipt['commands']))

    def test_timeout_kills_owned_child_and_retains_partial_output(self):
        pidfile = self.root / 'child.pid'
        script = self.tool('slow', f'''import os,pathlib,time
child=os.fork()
if child==0:
 time.sleep(30);os._exit(0)
pathlib.Path({str(pidfile)!r}).write_text(str(child))
print('started',flush=True);time.sleep(30)
''')
        preparation = prerequisites.Preparation('linker', self.root / 'job-bin')
        with self.assertRaises(prerequisites.subprocess.TimeoutExpired):
            preparation.run([script], timeout=1)
        row = preparation.receipt['commands'][-1]
        self.assertEqual(row['error'], 'TimeoutExpired')
        self.assertIn('started', row['stdout'])
        self.assertLess(row['elapsedSeconds'], 5)
        pid = int(pidfile.read_text())
        # A reaped process or a non-running zombie both prove the owned child
        # cannot keep using resources after its process group is killed.
        for _ in range(50):
            try:
                os.kill(pid, 0)
            except ProcessLookupError:
                break
            stat = Path(f'/proc/{pid}/stat')
            if stat.exists() and stat.read_text().split()[2] == 'Z':
                break
            time.sleep(.02)
        else:
            self.fail('owned child survived the timeout')


if __name__ == '__main__':
    unittest.main()
