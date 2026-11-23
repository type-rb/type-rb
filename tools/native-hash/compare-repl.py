#!/usr/bin/env python3
"""Registered one-cohort comparison of a bounded REPL storage refinement."""
import argparse
import hashlib
import json
import os
import platform
from pathlib import Path
import statistics
import subprocess
import tempfile
import time

p = argparse.ArgumentParser()
p.add_argument('--control', type=Path, required=True)
p.add_argument('--candidate', type=Path, required=True)
p.add_argument('--inputs', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
if platform.system() != "Darwin" or platform.machine() != "arm64":
    p.error("the registered cohort requires Darwin arm64")
binaries = {'control': a.control.resolve(), 'candidate': a.candidate.resolve()}
inputs = json.loads(a.inputs.read_text())['inputs']
report = {'diagnosticOnly': True, 'condition': 'uncontrolled local host; recovery validation overlaps',
          'binaries': {k: {'sha256': hashlib.sha256(v.read_bytes()).hexdigest(), 'bytes': v.stat().st_size}
                       for k, v in binaries.items()}, 'observations': []}
with tempfile.TemporaryDirectory(prefix='hash-repl-compare-') as temporary:
    for repetition in range(6):
        for case in ['scalar-100000', 'managed-100000']:
            for role in (['control', 'candidate'] if repetition % 2 == 0 else ['candidate', 'control']):
                with tempfile.TemporaryFile() as inp, tempfile.TemporaryFile() as out, tempfile.TemporaryFile() as err:
                    inp.write(inputs[case]['submission'].encode()); inp.seek(0)
                    start = time.monotonic()
                    child = subprocess.Popen([binaries[role], 'repl'], stdin=inp, stdout=out, stderr=err,
                        cwd=temporary, env=dict(os.environ, NO_COLOR='1', TRBN_HISTORY=temporary + '/history'))
                    _, status, usage = os.wait4(child.pid, 0)
                    child.returncode = os.waitstatus_to_exitcode(status)
                    wall = time.monotonic() - start
                    out.seek(0); stdout = out.read().decode()
                    err.seek(0); stderr = err.read().decode()
                row = {'case': case, 'role': role, 'repetition': repetition, 'warmup': repetition == 0,
                       'wallSeconds': wall, 'userSeconds': usage.ru_utime, 'systemSeconds': usage.ru_stime,
                       'maxRssBytes': usage.ru_maxrss, 'status': child.returncode, 'stdout': stdout, 'stderr': stderr}
                report['observations'].append(row)
                a.output.write_text(json.dumps(report, indent=2) + '\n')
                assert not child.returncode and not stderr and stdout == '100000\n99999\n0\n', row
report['medians'] = []
for case in ['scalar-100000', 'managed-100000']:
    for role in binaries:
        rows = [r for r in report['observations'] if r['case'] == case and r['role'] == role and not r['warmup']]
        report['medians'].append({'case': case, 'role': role,
            **{f: statistics.median(r[f] for r in rows)
               for f in ['wallSeconds', 'userSeconds', 'systemSeconds', 'maxRssBytes']}})
a.output.write_text(json.dumps(report, indent=2) + '\n')
print(a.output)
