#!/usr/bin/env python3
"""Check streamed batch lifetime, managed aliases and retained REPL behavior."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-sliced-iteration-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    fixture = repository / 'compiler/conformance/valid/slice-managed-mir'
    source = root / 'main.trb'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"a" + "b"', '"日" + "\\x00本"').replace('"c"', '"☀"')
    expected = fixture.with_suffix('.out').read_text().replace('ab\n', '日\x00本\n').replace('c\n', '☀\n')
    source.write_text(body
                      + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert not emitted.stderr, emitted
    forced, ordinary, hooks = [], False, 0
    for line in emitted.stdout.splitlines():
        if line.startswith('function '):
            ordinary = '$trbnf' in line
        if line == '}':
            ordinary = False
        if ordinary and any(marker in line for marker in (
                'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                'call $trbn_gc_alloc(', 'call $trbn_string_concat(', 'call $trbnf')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 18, hooks
    il, assembly, program = [root / name for name in ('program.ssa', 'program.s', 'program')]
    il.write_text('\n'.join(forced) + '\n')
    run([binary.with_name('qbe'), '-o', assembly, il])
    run(['/usr/bin/cc', assembly, '-lm', '-o', program])
    result = run([program], stats=True)
    assert result.stdout == expected * 30, result
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['collections'] >= 540, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics

    # The entire portable range must not be materialized before the first batch.
    source.write_text('''def main()
(-9007199254740991..9007199254740991).each_slice(3) do |batch|
puts(batch.size())
puts(batch[0])
puts(batch[-1])
break
end
end
''')
    run([binary, 'build', '--compile', '--outfile', program, source])
    streaming = run([program], stats=True)
    assert streaming.stdout == '3\n-9007199254740991\n-9007199254740989\n', streaming
    counters = dict((parts[1], int(parts[2])) for line in streaming.stderr.splitlines()
                    if (parts := line.split(','))[0] == 'type-rb-native-gc-stat-v1')
    assert counters['peak-heap-bytes'] < 65536 and counters['live-bytes'] == 0, counters

    retained = run([binary, 'repl'], data='''mut values: Array<Integer>? := nil
mut total := 0
values&.each_slice(2) { |batch| total += batch.size() }
values = [1, 2, 3]
values&.each_slice(2) { |batch| total += batch.size() }
puts(total)
values&.each_slice(true) { |batch| total += batch.size() }
record Before
text: String
end
:type values
:reload
puts(total)
values&.each_slice(2).with_index { |batch, index| puts(batch[0] + index) }
:quit
''')
    assert retained.stdout == ('nil : Array<Integer>? [mut]\n0 : Integer [mut]\n'
                               '[1, 2, 3] : Array<Integer>? [mut]\n3\n'
                               'Array<Integer>?\n3\nreloaded\n3\n1\n4\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'found Boolean' in retained.stderr, retained

print('Sliced iteration, streamed bounds, fresh batches, managed lifetime and retained REPL checks passed')
