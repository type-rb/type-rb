#!/usr/bin/env python3
"""Check stable natural Array ordering, retained copies and managed lifetime."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-array-sorting-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    # The CLI is built by the current compiler. Include literal byte cases that
    # the preceding seed's recovery source does not itself construct.
    fixture = repository / 'compiler/conformance/valid/array-sorting-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"a"\n', '"日"\n', 1).replace('part + "b"', 'part + "本"')
    body = body.replace('"aa"', '"a\\x00"').replace('"zz"', '"😀"').replace('"x"', '"é"')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    expected = ('|a|a|a\0|z|é|日本|😀\n日本\ntrue\n'
                '😀|日本|é|z|changed|a\0|a|a\n😀\ntrue\na\n') * 30
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
                    'call $trbn_gc_alloc(', 'call $trbn_string_concat(', 'call $trbn_array_join(')):
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

    # Strict reverse order can use reversal; repeated keys and a last-element
    # inversion must preserve stable merging. Cover odd tails and power-of-two
    # boundaries with an independent host oracle and one authored sort body.
    source.write_text('''def main()
(0...7).each do |shape|
(0...68).each do |size|
mut values: Array<Integer> := []
(0...size).each do |index|
mut value := (index * 37 + size * 11) % 23 - 11
if shape == 1
value = index / 3
elsif shape == 2
value = -(index / 3)
elsif shape == 3
value = 0
elsif shape == 4
value = index
if index == size - 1
value = -100
end
elsif shape == 5
value = -index
elsif shape == 6
value = -(index / 3)
if index == size - 1
value = 100
end
end
values.push(value)
end
values.sort().each { |value| puts(value) }
values.sort_descending().each { |value| puts(value) }
puts(values.size())
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
                    return index
                if shape == 5:
                    return -index
                if shape == 6:
                    return 100 if index == size - 1 else -(index // 3)
                return (index * 37 + size * 11) % 23 - 11
            values = [key(index) for index in range(size)]
            expected.extend(map(str, sorted(values)))
            expected.extend(map(str, sorted(values, reverse=True)))
            expected.append(str(size))
    assert result.stdout == '\n'.join(expected) + '\n' and result.stderr == '', result
    # Force collection across the insertion/merge boundary, including managed
    # records and cached keys in the keyed oracle.
    check_forced_collection('\n'.join(expected) + '\n', 3, 476)

    # Keep this small ordinary program separate: folding a dead branch in its
    # generated code once moved the input copy ahead of the Float pushes.
    cases = json.loads((repository / 'tools/native-language-cases.json').read_text())['cases']
    specials = next(case for case in cases if case['id'] == 'array-sort-float-specials')
    source.write_text('def main()\n' + specials['body'] + 'end\n')
    result = run([binary, 'run', source])
    assert result.stdout == specials['native']['execute']['stdout'] and result.stderr == '', result

    # A rejected call preserves retained source/copy storage. Replay rebuilds
    # those same independent bindings and nominal declarations cannot disturb it.
    retained = run([binary, 'repl'], data='''mut values := [3, 1, 2]
mut ordered := values.sort()
values[0] = 7
ordered[0] = 8
values.sort(1)
record Before
text: String
end
:type ordered
puts(values[0])
puts(ordered[0])
puts(values.sort()[0])
:reload
puts(values[0])
puts(ordered[0])
puts(ordered.sort_descending()[0])
:quit
''')
    assert retained.stdout == ('[3, 1, 2] : Array<Integer> [mut]\n'
                               '[1, 2, 3] : Array<Integer> [mut]\n'
                               '7 : Integer\n8 : Integer\nArray<Integer>\n7\n8\n1\n'
                               '7\n8\n1\nreloaded\n7\n8\n8\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'argument count' in retained.stderr, retained

print('Array sorting CLI, replay, ordering and collection checks passed')
