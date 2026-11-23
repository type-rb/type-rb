#!/usr/bin/env python3
"""Check stable keyed ordering, one-shot key evaluation and managed lifetime."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-keyed-sorting-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    fixture = repository / 'compiler/conformance/valid/keyed-array-sorting-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"ab"', '"日"').replace('part + "c"', 'part + "本\\x00"')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    expected = fixture.with_suffix('.out').read_text() * 30
    def check_forced_collection(expected, minimum_hooks, minimum_collections):
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
                    'call $trbn_gc_alloc(', 'call $trbn_string_concat(')):
                forced.append('\tcall $trbn_gc_collect(w 0)')
                hooks += 1
            forced.append(line)
        assert hooks >= minimum_hooks, hooks
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
        assert statistics['collections'] >= minimum_collections, statistics
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics

    check_forced_collection(expected, 16, 480)

    # Preserve the original identity on equal keys in both directions. The host
    # oracle covers strict reverse order separately from tied reverse runs and
    # late inversions, empty runs, odd tails and power-of-two boundaries to 67.
    source.write_text('''record Entry
key: Integer
identity: Integer
end

def main()
(0...7).each do |shape|
(0...68).each do |size|
mut values: Array<Entry> := []
(0...size).each do |index|
mut key := (index * 37 + size * 11) % 7 - 3
if shape == 1
key = index / 3
elsif shape == 2
key = -(index / 3)
elsif shape == 3
key = 0
elsif shape == 4
key = index / 3
if index == size - 1
key = -100
end
elsif shape == 5
key = -index
elsif shape == 6
key = -(index / 3)
if index == size - 1
key = 100
end
end
values.push(Entry.new(key: key, identity: index))
end
mut calls := 0
values.sort_by { |value| calls += 1; value.key }.each { |value| puts(value.identity) }
values.sort_by_descending { |value| calls += 1; value.key }.each { |value| puts(value.identity) }
puts(calls)
values.each { |value| puts(value.identity) }
end
end
end
''')
    result = run([binary, 'run', source])
    expected = []
    for shape in range(7):
        for size in range(68):
            def key(index):
                if shape == 1:
                    return index // 3
                if shape == 2:
                    return -(index // 3)
                if shape == 3:
                    return 0
                if shape == 4 and index == size - 1:
                    return -100
                if shape == 4:
                    return index // 3
                if shape == 5:
                    return -index
                if shape == 6:
                    return 100 if index == size - 1 else -(index // 3)
                return (index * 37 + size * 11) % 7 - 3
            values = [(key(index), index) for index in range(size)]
            expected.extend(str(identity) for _, identity in sorted(values, key=lambda value: value[0]))
            expected.extend(str(identity) for _, identity in sorted(values, key=lambda value: value[0], reverse=True))
            expected.append(str(2 * size))
            expected.extend(map(str, range(size)))
    assert result.stdout == '\n'.join(expected) + '\n' and result.stderr == '', result
    # Force collection across the insertion/merge boundary, including managed
    # records and cached keys in the keyed oracle.
    check_forced_collection('\n'.join(expected) + '\n', 3, 476)

    retained = run([binary, 'repl'], data='''mut values := [3, 1, 2]
mut calls := 0
mut ordered := values.sort_by { |value| calls += 1; value % 2 }
values[0] = 7
ordered[0] = 8
values.sort_by { |_value| true }
record Before
text: String
end
:type ordered
puts(calls)
puts(values[0])
puts(ordered[0])
:reload
puts(calls)
puts(values[0])
puts(ordered[0])
:quit
''')
    assert retained.stdout == ('[3, 1, 2] : Array<Integer> [mut]\n0 : Integer [mut]\n'
                               '[2, 3, 1] : Array<Integer> [mut]\n'
                               '7 : Integer\n8 : Integer\nArray<Integer>\n3\n7\n8\n'
                               '3\n7\n8\nreloaded\n3\n7\n8\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'sorting key' in retained.stderr, retained

print('Keyed sorting CLI, stable identities, once-only keys, replay and collection checks passed')
