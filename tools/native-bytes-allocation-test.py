#!/usr/bin/env python3
"""Check exact decoded String sizes and final reclamation through ordinary Bytes."""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
with tempfile.TemporaryDirectory(prefix='native-bytes-allocation-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
    env.pop('TYPE_RB_NATIVE_RUNTIME_TRACE', None)

    def run(command, *, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                capture_output=True, timeout=60)
        assert result.returncode == 0, result
        return result

    fixtures = [('"' + 'x' * n + '"', b'x' * n) for n in (0, 1, 10, 11, 12, 30, 31, 32, 64, 96)]
    fixtures.append(('"a\\x00あ\\xe3\\x81z\\xf0\\x90\\x80"', 'a\0あ�z�'.encode()))
    # Sweep word boundaries after valid and maximal-subpart malformed input.
    payloads = [b'\xc2\xa2', b'\xe6\x97\xa5', b'\xf0\x9f\x98\x80',
                b'\xff', b'\xe3\x81', b'\xed\xa0\x80', b'\x00']
    for prefix in range(9):
        for tail in (0, 1, 7, 8, 9, 31, 64):
            for payload in payloads:
                data = b'a' * prefix + payload + b'b' * tail + payload
                literal = '"' + ''.join('\\x%02x' % byte for byte in data) + '"'
                fixtures.append((literal, data.decode('utf-8', errors='replace').encode()))
    data = bytes(range(256))
    fixtures.append(('"' + ''.join('\\x%02x' % byte for byte in data) + '"',
                     data.decode('utf-8', errors='replace').encode()))
    body = '\n'.join('puts((' + literal + ').to_bytes().to_s())' for literal, _ in fixtures)
    source = root / 'main.trb'
    source.write_text('def exercise()\n' + body + '\nend\ndef main()\n'
                      '(0...30).each { |_index| exercise() }\nend\n')
    expected = b''.join(value + b'\n' for _, value in fixtures) * 30
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert not emitted.stderr, emitted
    il = emitted.stdout.decode()
    match = re.search(r'^function l \$trbn_bytes_to_string\([^\n]*\) \{.*?^\}', il, re.M | re.S)
    assert match
    hook = ' %result =l call $trbn_string_alloc(l %size)'
    assert match[0].count(hook) == 1
    forced = il.replace(match[0], match[0].replace(hook, ' call $trbn_gc_collect(w 0)\n' + hook))
    for name, text in [('ordinary', il), ('forced', forced)]:
        ssa, assembly, program = [root / (name + suffix) for suffix in ('.ssa', '.s', '')]
        ssa.write_text(text)
        assert not run([binary.with_name('qbe'), '-o', assembly, ssa]).stderr
        assert not run(['/usr/bin/cc', assembly, '-lm', '-o', program]).stderr
        result = run([program], stats=True)
        assert result.stdout == expected, result
        statistics = {}
        for line in result.stderr.decode().splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        if name == 'forced':
            assert statistics['collections'] >= len(fixtures) * 30, statistics

    # Receiver and index evaluation precede the same terminal checked failure.
    source.write_text('''import trb/std/process
def receiver(): Bytes
puts("receiver")
return "A\\x00\\xff".to_bytes()
end
def index(value: Integer): Integer
puts("index")
return value
end
def main()
puts(receiver().at(index(Process.argv()[0].to_i())))
puts("after")
end
''')
    program = root / 'checked-index'
    run([binary, 'build', '--compile', source, '--outfile', program])
    for value, expected in [(0, 65), (1, 0), (2, 255)]:
        result = run([program, value])
        assert result.stdout == f'receiver\nindex\n{expected}\nafter\n'.encode(), result
    for value in (-9007199254740991, -3, -1, 3, 4, 9007199254740991):
        result = subprocess.run([str(program), str(value)], capture_output=True, env=env, timeout=60)
        assert result.returncode != 0, result
        assert result.stdout == b'receiver\nindex\n', result
        assert result.stderr == b'panic: Bytes index is out of bounds\n', result
print('Ordinary Bytes decoding, forced collection and complete reclamation passed')
