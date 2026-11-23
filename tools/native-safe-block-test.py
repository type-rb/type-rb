#!/usr/bin/env python3
"""Exercise guarded collection blocks, retained state and managed lifetime."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-safe-blocks-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    fixture = repository / 'compiler/conformance/valid/safe-collection-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"b"', '"本"').replace('"a"', '"日\\x00"')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    expected = fixture.with_suffix('.out').read_text()
    expected = expected.replace('b1', '本1').replace('a2', '日\x002').replace('a3', '日\x003')
    expected = expected.replace('aretained', '日\x00retained') * 30
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
                'call $trbn_array_push_', 'call $trbn_array_grow_paced(', 'call $trbn_array_copy(',
                'call $trbn_gc_alloc(', 'call $trbn_string_concat(', 'call $trbn_hash_',
                'call $trbnf')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 18, hooks
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
    assert statistics['collections'] >= 540, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics

    retained = run([binary, 'repl'], data='''mut values: Array<Integer>? := nil
mut calls := 0
mut mapped := values&.map { |value| calls += 1; value + 1 }
:type mapped
values = [3, 1]
mapped = values&.map { |value| calls += 1; value + 1 }
mapped&.each { |value| puts(value) }
mapped = values&.map { |_value| true }
record Before
text: String
end
:type mapped
puts(calls)
:reload
puts(calls)
mapped&.each { |value| puts(value) }
:quit
''')
    assert retained.stdout == (
        'nil : Array<Integer>? [mut]\n0 : Integer [mut]\n'
        'nil : Array<Integer>? [mut]\nArray<Integer>?\n'
        '[3, 1] : Array<Integer>? [mut]\n[4, 2] : Array<Integer>? [mut]\n'
        '4\n2\nArray<Integer>?\n2\n4\n2\n2\nreloaded\n2\n4\n2\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'found Array<Boolean>' in retained.stderr, retained

print('Safe collection blocks, retained state, replay and managed lifetime checks passed')
