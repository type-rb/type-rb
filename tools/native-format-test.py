#!/usr/bin/env python3
"""Qualify source formatting against frozen reference results and file effects."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('binary', type=Path, nargs='?')
parser.add_argument('--reference', type=Path)
parser.add_argument('--reference-only', action='store_true')
parser.add_argument('--receipt', type=Path)
args = parser.parse_args()
if args.reference_only and not args.reference or not args.reference_only and not args.binary:
    parser.error('select a Native binary, or --reference-only with --reference')
binary = args.binary.resolve() if args.binary else None
reference = args.reference.resolve() if args.reference else None
manifest = Path(__file__).with_name('native-format-cases.json')
data = json.loads(manifest.read_text())
assert data['reference_revision'] == (manifest.parent.parent / 'TYPE_RB_REVISION').read_text().strip()
observations = []

with tempfile.TemporaryDirectory(prefix='native format ') as directory:
    root = Path(directory)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(tool, *arguments, source=None):
        return subprocess.run([str(tool), *map(str, arguments)], cwd=root, env=env,
                              input=source, text=True, capture_output=True, timeout=30)

    for case in data['cases']:
        if reference:
            control = run(reference, 'fmt', '-', source=case['source'])
            assert (control.returncode, control.stdout, control.stderr) == (
                case['reference_status'], case['formatted'], case['reference_error']), (case['id'], control)
        if args.reference_only:
            continue
        result = run(binary, 'fmt', '-', source=case['source'])
        actual = (result.returncode, result.stdout, result.stderr)
        wanted = (0, case['formatted'], '')
        if case['native_status'] == 'unsupported':
            observations.append({'id': case['id'], 'disposition': 'unsupported',
                                 'safely_rejected': result.returncode != 0 and not result.stdout,
                                 'status': result.returncode, 'stderr': result.stderr})
            assert result.returncode != 0 and not result.stdout, (case['id'], result)
            continue
        observations.append({'id': case['id'], 'passed': actual == wanted,
                             'status': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr})
        if actual != wanted:
            print(case['id'], 'expected', repr(wanted), 'actual', repr(actual), flush=True)
            continue
        second = run(binary, 'fmt', '-', source=result.stdout)
        assert (second.returncode, second.stdout, second.stderr) == wanted, ('idempotence', case['id'], second)

    if args.reference_only:
        print(f"{len(data['cases'])} frozen reference formatting observations verified")
        raise SystemExit(0)

    # Validation must precede writes. A rejected file remains byte-identical.
    for index, source in enumerate(['s := "unterminated\n', 'if true\nputs(1)\n', 'puts(,)\n']):
        file = root / f'invalid-{index}.trb'
        file.write_text(source)
        result = run(binary, 'fmt', file)
        assert result.returncode != 0 and file.read_text() == source, result
        file.unlink()

    file = root / 'source with spaces.trb'
    file.write_text('def main()\nputs( 123 )\nend\n')
    file.chmod(0o640)
    original = file.read_bytes()
    check = run(binary, 'fmt', '--check', file)
    assert check.returncode == 1 and file.read_bytes() == original and not check.stdout, check
    dry = run(binary, 'fmt', '-w=false', file)
    assert dry.returncode == 0 and file.read_bytes() == original and str(file) in dry.stdout, dry
    write = run(binary, 'fmt', file)
    assert write.returncode == 0 and file.read_text() == 'def main()\n\tputs(123)\nend\n', write
    assert file.stat().st_mode & 0o777 == 0o640, 'file mode changed'
    check = run(binary, 'fmt', '--check', file)
    assert (check.returncode, check.stdout, check.stderr) == (0, '', ''), check
    before = file.read_bytes()
    bad = run(binary, 'fmt', '--unknown', file)
    assert bad.returncode != 0 and file.read_bytes() == before, bad

    # Directory selection includes test files, excludes generated metadata, and
    # deduplicates overlapping selections without executing any source.
    sub = root / 'sources'
    sub.mkdir()
    (sub / 'a.trb').write_text('puts( 1 )\n')
    (sub / 'a_test.trb').write_text('puts( 2 )\n')
    (sub / 'README.txt').write_text('s:=123\n')
    for excluded in ('.git', '.trb', 'node_modules'):
        (sub / excluded).mkdir()
        (sub / excluded / 'untouched.trb').write_text('invalid := "unterminated\n')
    selected = run(binary, 'fmt', sub, sub / 'a.trb')
    assert selected.returncode == 0, selected
    assert (sub / 'a.trb').read_text() == 'puts(1)\n'
    assert (sub / 'a_test.trb').read_text() == 'puts(2)\n'
    assert (sub / 'README.txt').read_text() == 's:=123\n'
    assert len(selected.stdout.splitlines()) == 2, selected
    for excluded in ('.git', '.trb', 'node_modules'):
        assert (sub / excluded / 'untouched.trb').read_text() == 'invalid := "unterminated\n'

receipt = {'reference_revision': data['reference_revision'],
           'manifest_sha256': hashlib.sha256(manifest.read_bytes()).hexdigest(),
           'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
           'cases': observations, 'file_effects': 'passed'}
if args.receipt:
    args.receipt.parent.mkdir(parents=True, exist_ok=True)
    args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
assert all(row.get('passed', row.get('safely_rejected', False)) for row in observations), 'source formatting differs from the frozen reference'
passed = sum(row.get('passed', False) for row in observations)
unsupported = sum(row.get('disposition') == 'unsupported' for row in observations)
print(f'{passed} paired source formats passed; {unsupported} explicit unsupported spellings safely rejected; idempotence and file effects passed')
