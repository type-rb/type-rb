"""Check strict non-transitional UTS46 against the fixed official Unicode corpus."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from network_artifact_smoke import imports
from network_artifacts import target_name

CORPUS_SHA256 = 'beb5d0be20e896189b03209a82fdc34f06351502bbd4b8e2523583fc2954d9cf'
DIRECTORY = Path(__file__).with_name('fixtures') / 'idna'


def decode(value):
    if value == '""':
        return ''

    def scalar(match):
        number = int(match[1] or match[2], 16)
        return '\ufffd' if 0xd800 <= number <= 0xdfff or number > 0x10ffff else chr(number)

    return re.sub(r'\\u([0-9a-fA-F]{4})|\\x\{([0-9a-fA-F]+)\}', scalar, value)


def rows():
    source = (DIRECTORY / 'IdnaTestV2.txt').read_bytes()
    assert hashlib.sha256(source).hexdigest() == CORPUS_SHA256
    cases = []
    for number, line in enumerate(source.decode().splitlines(), 1):
        line = line.split('#', 1)[0].strip()
        if not line:
            continue
        fields = [field.strip() for field in line.split(';')]
        value = decode(fields[0])
        unicode = decode(fields[1]) if fields[1] else value
        ascii = decode(fields[3]) if fields[3] else unicode
        status = fields[4] or fields[2]
        cases.append({'line': number, 'input': value, 'output': ascii, 'valid': status in ('', '[]')})
    return cases


def qualify(libraries, root):
    root.mkdir()
    source = Path(__file__).with_name('fixtures') / 'oidc_idna.c'
    executable = root / 'probe'
    target = target_name()
    system = ['-lc++'] if target.startswith('darwin-') else ['-lstdc++', '-pthread', '-ldl', '-lm']
    subprocess.run(['/usr/bin/cc', '-std=c11', '-D_POSIX_C_SOURCE=200809L', '-Werror', str(source),
                    str(libraries / 'libicuuc.a'), str(libraries / 'libicu-uts46.a'), *system,
                    '-o', str(executable)], check=True, timeout=60)
    fifo = root / 'no-data-files'
    os.mkfifo(fifo)
    cases = rows()
    result = subprocess.run([str(executable)], input=''.join(row['input'].encode().hex() + '\n' for row in cases),
                            capture_output=True, text=True, timeout=30, env={'PATH': '', 'ICU_DATA': str(fifo)})
    assert result.returncode == 0, result.stderr
    outputs = result.stdout.splitlines()
    assert len(outputs) == len(cases), len(outputs)
    mismatches = []
    for row, actual in zip(cases, outputs):
        valid = actual.startswith('ok:')
        output = bytes.fromhex(actual[3:]).decode() if valid else ''
        if valid != row['valid'] or valid and output != row['output']:
            mismatches.append(dict(row, actual=actual))
    assert not mismatches, mismatches[:20]
    linked = imports(executable)
    assert not any('libicu' in name for name in linked), linked
    return {'cases': len(cases), 'corpus_sha256': CORPUS_SHA256, 'probe_sha256': hashlib.sha256(source.read_bytes()).hexdigest(),
            'imports': linked, 'runtime_data_files': False, 'executable_bytes': executable.stat().st_size}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--libraries', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='oidc-idna-corpus-') as temporary:
        result = qualify(args.libraries.resolve(), Path(temporary) / 'qualification')
    args.receipt.write_text(json.dumps(result, indent=2) + '\n')
    print('Unicode 17 strict UTS46:', result['cases'], 'cases passed')
