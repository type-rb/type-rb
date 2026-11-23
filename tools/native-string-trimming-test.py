#!/usr/bin/env python3
"""Check Unicode trimming, exact retained bytes, allocation and GC lifetimes."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-string-trimming-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run([item if isinstance(item, bytes) else str(item) for item in command],
                                input=data, cwd=root, env=environment,
                                capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    def compile_source(text, *, force_gc=True):
        source = root / 'main.trb'
        source.write_text(text)
        emitted = run([binary, '--internal-driver', 'emit-qbe', source])
        assert emitted.stderr == b'', emitted
        il = emitted.stdout
        if force_gc:
            start = il.index(b'function l $trbn_string_trim(')
            end = il.index(b'\n}', start) + 2
            runtime = il[start:end]
            marker = b' %result =l call $trbn_string_alloc(l %payload)'
            assert runtime.count(marker) == 1
            # Collect after the UTF-8 scan, before copying the retained bytes.
            forced = runtime.replace(marker, b' call $trbn_gc_collect(w 0)\n' + marker)
            il = il[:start] + forced + il[end:]
        assembly, program = root / 'program.s', root / 'program'
        (root / 'program.ssa').write_bytes(il)
        run([binary.with_name('qbe'), '-o', assembly, root / 'program.ssa'])
        run(['/usr/bin/cc', assembly, '-lm', '-o', program])
        return program

    def statistics(result):
        values = {}
        for line in result.stderr.decode().splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in values, line
            values[key] = int(value)
        assert values['live-bytes'] == 0, values
        assert values['allocated-bytes'] == values['reclaimed-bytes'], values
        return values

    fixture = repository / 'compiler/conformance/valid/string-trimming-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('" before "', '"\u3000こんにちは\\x00😀\u00a0"')
    body = body.replace('" one "', '"\u202f日本\u2009"')
    body = body.replace('" two "', '"\u0085語\u1680"')
    program = compile_source(body + '\ndef main()\n(0...20).each { |_index| exercise() }\nend\n')
    result = run([program], stats=True)
    expected = 'こんにちは\0😀/日本/語\nafter\nheld\n'.encode()
    assert result.stdout == expected * 20, result
    measured = statistics(result)
    assert measured['collections'] >= 100, measured
    assert measured['peak-heap-bytes'] < 4 * 1024 * 1024, measured

    # Every byte survives when it is inside the retained span, including NUL
    # and malformed UTF-8. Conversion through code-point arrays would lose this.
    escaped = ''.join('\\x%02x' % value for value in range(256))
    program = compile_source('def main()\ntext := "\u3000' + escaped + '\u00a0"\n'
                             'puts(text.strip())\nputs(text.lstrip())\nputs(text.rstrip())\nend\n')
    result = run([program], stats=True)
    raw = bytes(range(256))
    assert result.stdout == raw + b'\n' + raw + '\u00a0\n\u3000'.encode() + raw + b'\n', result
    statistics(result)

    program = compile_source('''import trb/std/process
def main()
text := Process.argv()[0]
puts(text.strip())
puts(text.lstrip())
puts(text.rstrip())
end
''')
    for value in (b'\xff', b'\xe3\x81', b'\xc0\xa0', b'\xc2', '\ufeff\u180e\u200b\u2060'.encode()):
        left, right = '\u3000'.encode(), '\u00a0'.encode()
        result = run([program, left + value + right], stats=True)
        assert result.stdout == value + b'\n' + value + right + b'\n' + left + value + b'\n', result
        statistics(result)

    # Bound allocation by input size, without a wall-clock performance claim.
    program = compile_source('''import trb/std/process
def main()
text := Process.argv()[0]
result := text.strip()
puts(result.size())
puts(text.size())
end
''')
    large = ('\u3000' + '日本😀' * 8192 + '\t ').encode()
    result = run([program, large], stats=True)
    assert result.stdout == b'24576\n24579\n', result
    assert statistics(result)['allocated-bytes'] < 4 * len(large) + 4096, result.stderr

    retained = run([binary, 'repl'], data='''mut original := "　こんにちは😀　"
saved := original.strip()
unchanged := saved.strip()
original = "replacement"
def suffix(): String
return "!"
end
puts(saved + suffix())
saved.strip(1)
puts(unchanged)
puts(original.lstrip().rstrip())
:quit
'''.encode())
    assert 'こんにちは😀!\n'.encode() in retained.stdout, retained
    assert retained.stdout.endswith('こんにちは😀\nreplacement\n'.encode()), retained
    assert len(retained.stderr.splitlines()) == 1 and b'error[' in retained.stderr, retained

print('String trimming byte, GC, allocation and retained REPL checks passed')
