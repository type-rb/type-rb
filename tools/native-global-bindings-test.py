#!/usr/bin/env python3
"""Check mutable global roots, retained module identity and explicit replay."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-global-bindings-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    source = root / 'main.trb'
    source.write_text(r'''KEPT := ["terminal", "root"]
mut text := "日本\x00" + " first"
mut retained: Array<String>? := nil
def replace(): String
old := text
text = "日本\x00" + " next"
retained = [old]
return old
end
def exercise()
text = "日本\x00" + " first"
saved := fn(): String; return text; end
puts(replace())
text := "local"
puts(saved())
puts(text)
if retained != nil
puts(retained[0])
end
retained = nil
end
def release()
text = "released"
puts(KEPT[0])
end
def main()
(0...30).each { |_index| exercise() }
release()
end
''')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert not emitted.stderr, emitted
    forced, ordinary, hooks = [], False, 0
    for line in emitted.stdout.decode().splitlines():
        if line.startswith('function '):
            ordinary = '$trbnf' in line or '$trbn_initialize_globals(' in line
        if line == '}':
            ordinary = False
        if ordinary and any(marker in line for marker in (
                'call $trbnf', 'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                'call $trbn_array_grow_paced(', 'call $trbn_gc_alloc(',
                'call $trbn_string_concat(')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 8, hooks
    il, assembly, program = [root / name for name in ('program.ssa', 'program.s', 'program')]
    il.write_text('\n'.join(forced) + '\n')
    run([binary.with_name('qbe'), '-o', assembly, il])
    run(['/usr/bin/cc', assembly, '-lm', '-o', program])
    executed = run([program], stats=True)
    expected = ('日本\x00 first\n日本\x00 next\nlocal\n日本\x00 first\n' * 30 + 'terminal\n').encode()
    assert executed.stdout == expected, (executed.stdout, expected)
    statistics = {}
    for line in executed.stderr.decode().splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    # Require repeated collections, without fixing the emitter's helper count.
    assert statistics['collections'] >= 30, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert statistics['peak-heap-bytes'] < 1024 * 1024, statistics
    source.unlink()

    (root / 'trbconfig.jsonc').write_text('{"name":"globals","sourceDir":"src"}')
    (root / 'src').mkdir()
    (root / 'src/counter.trb').write_text('''mut count := 0
mut text := "held" + " value"
def bump(): Integer
count += 1
text = "held-" + count.to_s()
return count
end
def read(): String
return text
end
''')
    (root / 'src/other.trb').write_text('''mut count := 20
def other(): Integer
count += 1
return count
end
''')
    retained = run([binary, 'repl'], b'''puts(bump())
saved := read
puts(saved())
puts(other())
record Later
number: Integer
end
puts(bump())
puts(saved())
:reload
puts(bump())
puts(other())
puts(saved())
:quit
''')
    assert not retained.stderr, retained
    assert retained.stdout == (
        b'1\n#<callable> : () -> String\nheld-1\n21\n2\nheld-2\n'
        b'1\nheld-1\n21\n2\nheld-2\nreloaded\n3\n22\nheld-3\n'), retained

print('Mutable global identity, Unicode/NUL roots, exact reclaim and project REPL replay passed')
