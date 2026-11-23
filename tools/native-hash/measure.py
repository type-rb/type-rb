#!/usr/bin/env python3
"""One bounded Darwin diagnostic cohort; never modifies acceptance limits."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import statistics
import subprocess
import tempfile
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--candidate', type=Path, required=True)
    p.add_argument('--control', type=Path, required=True)
    p.add_argument('--frozen-source', type=Path, required=True)
    p.add_argument('--frozen-core', type=Path, required=True)
    p.add_argument('--output', type=Path, required=True)
    args = p.parse_args()
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        p.error("the registered cohort requires Darwin arm64")
    candidate, control = args.candidate.resolve(), args.control.resolve()
    frozen = args.frozen_source.resolve()
    cores = {'candidate': candidate / '.trb/bootstrap/core/compiler',
             'control': control / '.trb/bootstrap/core/compiler',
             'frozen': args.frozen_core.resolve()}
    sources = {'candidate': candidate, 'control': control, 'frozen': frozen}
    qbe = candidate / 'bin/qbe'
    cc = Path('/usr/bin/cc')
    observations = []
    report = {
        'schema': 1, 'diagnosticOnly': True, 'utc': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        'platform': {'system': platform.system(), 'release': platform.release(), 'machine': platform.machine()},
        'cache': 'warm OS caches; one warmup then five measured repetitions; uncontrolled local host',
        'target': 'darwin-arm64-v0',
        'toolchain': {'cc': subprocess.check_output([cc, '--version'], text=True).splitlines()[0],
                      'qbeSha256': digest(qbe), 'qbeBytes': qbe.stat().st_size},
        'roles': {}, 'inputs': {}, 'observations': observations,
    }
    for role, core in cores.items():
        report['roles'][role] = {'coreSha256': digest(core), 'coreBytes': core.stat().st_size,
            'sourceSha256': {str(f.relative_to(sources[role])): digest(f)
                            for f in sorted((sources[role] / 'compiler/src').rglob('*.trb'))
                            if not f.name.endswith('_test.trb')}}
    report['roles']['candidate'].update(cliBytes=(candidate / 'bin/trbn').stat().st_size,
        cliSha256=digest(candidate / 'bin/trbn'))
    if report['roles']['candidate']['coreBytes'] > 430000 or report['roles']['candidate']['cliBytes'] > 590000:
        raise RuntimeError('diagnostic size ceiling exceeded')

    def retain():
        args.output.write_text(json.dumps(report, indent=2) + '\n')

    with tempfile.TemporaryDirectory(prefix='native-hash-measure-') as directory:
        temporary = Path(directory)

        def run(command, case, role, repetition, text=None, expected=None):
            with tempfile.TemporaryFile() as out, tempfile.TemporaryFile() as err, tempfile.TemporaryFile() as inp:
                if text is not None:
                    inp.write(text.encode()); inp.seek(0)
                start = time.monotonic()
                child = subprocess.Popen(list(map(str, command)), cwd=temporary,
                    stdin=inp, stdout=out, stderr=err,
                    env=dict(os.environ, NO_COLOR='1', TRBN_HISTORY=str(temporary / 'history')))
                _, status, usage = os.wait4(child.pid, 0)
                child.returncode = os.waitstatus_to_exitcode(status)
                wall = time.monotonic() - start
                out.seek(0); stdout = out.read().decode()
                err.seek(0); stderr = err.read().decode()
            row = {'case': case, 'role': role, 'repetition': repetition, 'warmup': repetition == 0,
                   'wallSeconds': wall, 'userSeconds': usage.ru_utime, 'systemSeconds': usage.ru_stime,
                   'maxRssBytes': usage.ru_maxrss, 'status': child.returncode,
                   'stdout': stdout, 'stderr': stderr}
            observations.append(row); retain()
            if child.returncode or stderr or (expected is not None and stdout != expected):
                raise RuntimeError(f'wrong result for {case}/{role}: {row}')
            return row

        def build(role, source, output, case, repetition):
            return run([cores[role], 'build', source, '--output', output, '--qbe', qbe,
                        '--cc', cc, '--target', 'darwin-arm64-v0'], case, role, repetition, expected='')

        # Same-source comparisons isolate compiler overhead from source growth.
        for repetition in range(6):
            roles = ['control', 'candidate'] if repetition % 2 == 0 else ['candidate', 'control']
            for role in roles:
                row = build(role, candidate / 'compiler/src/compiler.trb', temporary / 'self',
                            'same-source-self-build', repetition)
                row['executableBytes'] = (temporary / 'self').stat().st_size
                row['executableSha256'] = digest(temporary / 'self')
            for role in ['frozen', 'control', 'candidate']:
                row = build(role, sources[role] / 'compiler/src/compiler.trb', temporary / 'own',
                            'own-source-self-build', repetition)
                row['executableBytes'] = (temporary / 'own').stat().st_size
                row['executableSha256'] = digest(temporary / 'own')
            for name in ['fannkuch-redux', 'n-body', 'spectral-norm']:
                source = candidate / 'benchmarks/benchmarksgame' / name / 'src/main.trb'
                report['inputs'][name] = {'sourceSha256': digest(source)}
                for role in (['frozen', 'control', 'candidate'] if repetition % 2 == 0 else ['candidate', 'control', 'frozen']):
                    row = build(role, source, temporary / name, name + '-build', repetition)
                    row['executableBytes'] = (temporary / name).stat().st_size
                    row['executableSha256'] = digest(temporary / name)

        for kind in ['scalar', 'managed']:
            for count in [1000, 10000, 100000]:
                type_name = 'Integer' if kind == 'scalar' else 'String'
                key = 'i' if kind == 'scalar' else 'i.to_s()'
                last = str(count - 1) if kind == 'scalar' else f'"{count - 1}"'
                body = (f'mut h: Hash<{type_name}, {type_name}> := {{}}\nmut i := 0\n'
                        f'while i < {count}\nh[{key}] = {key}\ni += 1\nend\n'
                        f'puts(h.size())\nputs(h[{last}])\n'
                        f'while i > 0\ni -= 1\nh.delete({key})\nend\nputs(h.size())\n')
                source = 'def main()\n' + body + 'end\n'
                submission = source.replace('def main()', 'def exercise_hash()') + '\nexercise_hash()\n:quit\n'
                name = f'{kind}-{count}'
                report['inputs'][name] = {'source': source, 'submission': submission}
                path, output = temporary / 'workload.trb', temporary / 'workload'
                path.write_text(source)
                build('candidate', path, output, name + '-build', 0)
                expected = f'{count}\n{count - 1}\n0\n'
                for repetition in range(6):
                    run([output], name, 'compiled', repetition, expected=expected)
                    run([candidate / 'bin/trbn', 'repl'], name, 'repl', repetition,
                        text=submission, expected=expected)
        summaries = []
        for case, role in sorted({(r['case'], r['role']) for r in observations if not r['warmup']}):
            rows = [r for r in observations if r['case'] == case and r['role'] == role and not r['warmup']]
            summaries.append({'case': case, 'role': role, 'runs': len(rows),
                              **{field: statistics.median(r[field] for r in rows)
                                 for field in ['wallSeconds', 'userSeconds', 'systemSeconds', 'maxRssBytes']}})
        report['medians'] = summaries
        retain()
    print(args.output)


if __name__ == '__main__':
    main()
