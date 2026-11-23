#!/usr/bin/env python3
"""Exercise literal constraints, scalar storage and retained REPL captures."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-literals-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    fixture = repository / 'compiler/conformance/valid/literal-collections-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('held("value")', 'held("日\\x00本")')
    body = body.replace('key: "a" := "a"', 'key: "日\\x00本" := "日\\x00本"')
    body = body.replace('Hash<"a", String>', 'Hash<"日\\x00本", String>')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...30).each { |_index| exercise() }\nend\n')
    expected = fixture.with_suffix('.out').read_text().replace('value!', '日\x00本!') * 30
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

    retained = run([binary, 'repl'], data=r'''alias Status = 201 | 404
mut status: Status := 201
read := fn(): Integer
return status
end
status = 500
record Earlier
text: String
end
:type status
puts(read())
status = 404
puts(read())
:reload
puts(read())
:quit
''')
    assert retained.stdout == (
        '201 : 201 | 404 [mut]\n#<fn> : () -> Integer\n201 | 404\n201\n'
        '404 : 201 | 404 [mut]\n404\n201\n404\nreloaded\n404\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'expected 201 | 404' in retained.stderr, retained

    optional = run([binary, 'repl'], data=r'''alias Code = 201
code: Code := 201
value: Code? := code
if value != nil
puts(value)
end
:reload
if value != nil
puts(value)
end
:quit
''')
    assert optional.stderr == '' and optional.stdout == (
        '201 : 201\n201 : 201?\n201\n201\nreloaded\n201\n'), optional

    queries = run([binary, 'repl'], data=r'''def query(): Integer
values: Array<1 | 2> := [2, 1, 2]
puts(values.uniq().size())
puts(values.count(2))
labels: Array<"a" | "b"> := ["b", "a", "b"]
return labels.uniq().size()
end
puts(query())
:quit
''')
    assert queries.stderr == '' and queries.stdout == '2\n2\n2\n', queries

print('Literal constraints, discriminants, retained captures and managed lifetime checks passed')
