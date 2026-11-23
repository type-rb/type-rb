#!/usr/bin/env python3
"""Verify a bootstrap compiler against the shared cyclic-module contracts."""
import argparse
import json
from pathlib import Path
import re
import subprocess


CASES = ('module-cycle-recursion', 'module-cycle-values',
         'module-cycle-value-cycle', 'module-cycle-indirect')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--compiler', type=Path, required=True)
    parser.add_argument('--qbe', type=Path, required=True)
    parser.add_argument('--profile', required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--trace', action='store_true')
    args = parser.parse_args()
    compiler, qbe = args.compiler.resolve(), args.qbe.resolve()
    evidence = args.evidence.resolve()
    evidence.mkdir(parents=True, exist_ok=False)
    registry = json.loads(Path(__file__).with_name('native-language-cases.json').read_text())
    selected = [case for case in registry['cases'] if case['id'] in CASES]
    if sorted(case['id'] for case in selected) != sorted(CASES):
        raise ValueError('cyclic-module capability contracts are missing or duplicated')
    for case in selected:
        directory = evidence / case['id']
        directory.mkdir()
        for relative, source in case['files'].items():
            relative = Path(relative)
            if relative.is_absolute() or '..' in relative.parts:
                raise ValueError('invalid capability fixture path')
            target = directory / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(source)
        entry = directory / 'main.trb'
        entry.write_text(case['declarations'] + '\ndef main()\n' + case['body'] + '\nend\n')
        program = directory / 'program'
        commands = {
            'check': [str(compiler), 'check', str(entry)],
            'build': [str(compiler), 'build', str(entry), '--output', str(program),
                      '--qbe', str(qbe), '--cc', '/usr/bin/cc', '--target', args.profile],
            'execute': [str(program)],
        }
        for operation, command in commands.items():
            expected = case['native'][operation]
            if expected is None:
                continue
            invoked = command
            if args.trace:
                invoked = ['strace', '-f', '-e', 'trace=process', '-o',
                           str(directory / (operation + '.trace')), *command]
            result = subprocess.run(invoked, capture_output=True, text=True, timeout=120)
            if args.trace:
                trace = directory / (operation + '.trace')
                if not trace.is_file() or trace.stat().st_size == 0:
                    raise ValueError('capability process trace is missing')
            # The core emits its source:code:line:message protocol; the shared
            # registry records the CLI's presentation of those same fields.
            stderr = re.sub(r'^(.+):(TRBN[0-9]+):([0-9]+):(.*)$',
                            r'\1:\3: error[\2]: \4', result.stderr, flags=re.MULTILINE)
            observed = {'code': result.returncode, 'stdout': result.stdout,
                        'stderr': stderr.replace(str(directory), '<case>')}
            (directory / (operation + '.json')).write_text(json.dumps(
                {'command': command, 'expected': expected, 'observed': observed,
                 'coreStderr': result.stderr}, indent=2) + '\n')
            if observed != expected:
                raise ValueError(f"{case['id']} {operation} differs from the shared contract: {observed}")
            if operation == 'build' and expected['code'] != 0 and program.exists():
                raise ValueError('rejected initialization produced an executable')
    print('Module-cycle bootstrap capability checks passed')


if __name__ == '__main__':
    main()
