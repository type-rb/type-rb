#!/usr/bin/env python3
"""Check Hash snapshot and Range roots, accounting and retained REPL values."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-hash-range-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    # Recovery-unit fixtures use the preceding seed's literal subset. Ordinary
    # compilers additionally exercise byte-preserving Unicode and embedded NUL.
    fixture = repository / 'compiler/conformance/valid/hash-iteration-managed-mir'
    ranges = repository / 'compiler/conformance/valid/range-materialization-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def hash_exercise()')
    body = body.replace('"hel" + "lo"', '"こんに" + "ちは"')
    body = body.replace('"left"', '"あいうえ"').replace('"right"', '"A\\x00CDE"')
    body += ranges.with_suffix('.trb').read_text().replace('def main()', 'def range_exercise()')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...20).each do |_index|\nhash_exercise()\nrange_exercise()\nend\nend\n')
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
                'call $trbn_gc_alloc(', 'call $trbn_string_concat(', 'call $trbn_hash_entries(',
                'call $trbn_hash_new(', 'call $trbn_hash_set(', 'call $trbn_hash_delete(', 'call $trbnf')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 15, hooks
    il, assembly, program = [root / name for name in ('program.ssa', 'program.s', 'program')]
    il.write_text('\n'.join(forced) + '\n')
    run([binary.with_name('qbe'), '-o', assembly, il])
    run(['/usr/bin/cc', assembly, '-lm', '-o', program])
    result = run([program], stats=True)
    assert result.stdout == (fixture.with_suffix('.out').read_text() + ranges.with_suffix('.out').read_text()) * 20, result
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['collections'] >= 600, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics

    retained = run([binary, 'repl'], data='''enum Direction
Left
Right
end
mut values := {left: Direction::Left, right: Direction::Right}
span := (1..2)
enum Extra
First
Second
Third
end
mut count := 0
puts("collections")
values.each do |key, value|
if value == Direction::Left
count += 1
end
values[key] = Direction::Right
end
puts(count)
span.to_a().each { |value| puts(value) }
:quit
''')
    assert retained.stderr == '', retained
    assert retained.stdout.split('collections\n')[-1] == '1\n1\n2\n', retained

print('Hash snapshot, Range GC/accounting and retained REPL checks passed')
