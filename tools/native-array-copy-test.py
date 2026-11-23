#!/usr/bin/env python3
"""Exercise Array copy storage, managed children and allocation accounting."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
qbe = binary.with_name('qbe')

SOURCE = '''record Entry
text: String
numbers: Array<Integer>
end
enum Item
Text(value: String)
Empty
end
def copies(): Boolean
mut inner := [1]
mut source: Array<Entry> := []
mut index := 0
while index < 73
 source.push(Entry.new(text: "item" + index.to_s(), numbers: inner))
 index += 1
end
mut part := source.slice(10...73)
mut copied := source.dup()
mut backward := source.reverse()
source = [Entry.new(text: "replacement", numbers: [9])]
inner.push(2)
index = 0
while index < 80
 part.push(Entry.new(text: "extra" + index.to_s(), numbers: [index]))
 index += 1
end
copied.push(source[0])
backward.push(source[0])
return part.size() == 143 && part.first().text == "item10" && part.last().text == "extra79" && part[62].text == "item72" && part.first().numbers.size() == 2 && copied.size() == 74 && copied.first().text == "item0" && copied[72].text == "item72" && backward.first().text == "item72" && backward[72].text == "item0"
end
def representations(): Boolean
mut captured := 1
read := fn(): Integer
 return captured
end
callbacks := [read].dup().reverse().slice(0..0)
callback := callbacks.first()
captured = 7
mut values: Array<Integer?> := [nil, 0, 2]
nullable := values.reverse().dup().slice(0...3)
values = [9]
first := nullable.first()
if first == nil
 return false
end
items := [Item::Empty, Item::Text("held" + " text")].reverse().dup()
mut text := ""
case items.first()
when Item::Text(value)
 text = value
when Item::Empty
 text = "wrong"
end
floats := [1.5, -2.5, 0.0].reverse().slice(0...3).dup()
flags := [false, true].reverse().dup()
mut empty: Array<String> := []
mut fresh := empty.slice(0...0)
fresh.push("first" + " value")
return callback() == 7 && first == 2 && nullable.last() == nil && text == "held text" && floats.first() == 0.0 && floats[1] == -2.5 && floats.last() == 1.5 && flags.first() && !flags.last() && empty.empty?() && fresh.first() == "first value"
end
def main()
mut index := 0
mut good := true
while index < 20
 good = copies() && representations() && good
 index += 1
end
puts(good)
end
'''

with tempfile.TemporaryDirectory(prefix='native-array-copy-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb')

    def run(command, *, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                capture_output=True, text=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    source = root / 'main.trb'
    source.write_text(SOURCE)
    checked = run([binary, 'check', source])
    assert checked.stdout == 'ok\n' and checked.stderr == '', checked
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert emitted.stderr == '', emitted
    # Collect inside the copy runtime immediately before its managed header
    # allocation. This checks that MIR retained the source and optional Range.
    marker = ' %array =l call $trbn_gc_alloc(l %descriptor, l 24, l %bytes)'
    assert emitted.stdout.count(marker) == 1
    forced = emitted.stdout.replace(marker, ' call $trbn_gc_collect(w 0)\n' + marker)
    for name, il in [('ordinary', emitted.stdout), ('forced', forced)]:
        il_path, assembly, program = [root / (name + suffix) for suffix in ('.ssa', '.s', '')]
        il_path.write_text(il)
        run([qbe, '-o', assembly, il_path])
        run(['/usr/bin/cc', assembly, '-lm', '-o', program])
        result = run([program], stats=True)
        assert result.stdout == 'true\n', result
        statistics = {}
        for line in result.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1', line
            assert key not in statistics, line
            statistics[key] = int(value)
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        assert statistics['peak-heap-bytes'] < 4 * 1024 * 1024, statistics
        if name == 'forced':
            assert statistics['collections'] >= 20, statistics

print('Array copy storage, collection and accounting checks passed')
