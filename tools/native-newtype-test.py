#!/usr/bin/env python3
"""Exercise erased nominal storage, managed lifetime and retained REPL identity."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-newtypes-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    fixture = repository / 'compiler/conformance/valid/newtype-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"he" + "ld"', '"日\\x00" + "本"')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    expected = fixture.with_suffix('.out').read_text().replace('held', '日\x00本') * 30
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert emitted.stderr == '', emitted
    forced, ordinary, hooks = [], False, 0
    for line in emitted.stdout.splitlines():
        if line.startswith('function '):
            ordinary = '$trbnf' in line
        if line == '}':
            ordinary = False
        if ordinary and any(marker in line for marker in (
                'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                'call $trbn_gc_alloc(', 'call $trbn_string_concat(', 'call $trbn_hash_',
                'call $trbnf')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 10, hooks
    il, assembly, program = [root / name for name in ('program.ssa', 'program.s', 'program')]
    il.write_text('\n'.join(forced) + '\n')
    run([binary.with_name('qbe'), '-o', assembly, il])
    run(['/usr/bin/cc', assembly, '-lm', '-o', program])
    result = run([program], stats=True)
    assert result.stdout == expected, (result.stdout, expected, result.stderr)
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['collections'] >= 300, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics

    retained = run([binary, 'repl'], data=r'''newtype Label = String do
private new
def self.make(text: String): Label
return self.new(text)
end
def reader(): () -> String
return fn(): String
return value()
end
end
end
mut saved := Label.make("日\x00本")
read := saved.reader()
saved = 3
newtype Earlier = Float
record Before
text: String
end
:type saved
puts(read())
puts(saved.value())
:reload
puts(read())
:quit
''')
    assert retained.stdout == (
        '"日\\x00本" : Label [mut]\n#<fn> : () -> String\nLabel\n'
        '日\x00本\n日\x00本\n日\x00本\n日\x00本\nreloaded\n日\x00本\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'Label' in retained.stderr, retained

print('Nominal newtypes, retained identity, replay and managed lifetime checks passed')
