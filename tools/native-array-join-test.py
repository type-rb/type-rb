#!/usr/bin/env python3
"""Check byte-preserving join, bounded allocation and retained operands."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-array-join-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False, code=0):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=environment, text=True, capture_output=True, timeout=60)
        assert result.returncode == code, (command, result.stdout, result.stderr)
        return result

    def compile_il(text):
        il, assembly, program = [root / name for name in ('program.ssa', 'program.s', 'program')]
        il.write_text(text)
        run([binary.with_name('qbe'), '-o', assembly, il])
        run(['/usr/bin/cc', assembly, '-lm', '-o', program])
        return program

    def statistics(result):
        values = {}
        for line in result.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in values, line
            values[key] = int(value)
        assert values['live-bytes'] == 0, values
        assert values['allocated-bytes'] == values['reclaimed-bytes'], values
        return values

    fixture = repository / 'compiler/conformance/valid/array-join-managed-mir'
    body = fixture.with_suffix('.trb').read_text().replace('def main()', 'def exercise()')
    body = body.replace('"a" + "b"', '"こんに" + "ちは"')
    body = body.replace('"c" + "d"', '"A" + "\\x00B"')
    source = root / 'main.trb'
    source.write_text(body + '\ndef main()\n(0...20).each { |_index| exercise() }\nend\n')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    marker = ' %result =l call $trbn_string_alloc(l %payload)'
    start = emitted.stdout.index('function l $trbn_join_fragments(')
    end = emitted.stdout.index('\n}', start) + 2
    runtime = emitted.stdout[start:end]
    assert runtime.count(marker) == 1
    # Collect after sizing, immediately before allocation, with both operands
    # dead after the join at some call sites. Their MIR roots must still hold.
    forced = runtime.replace(marker, ' call $trbn_gc_collect(w 0)\n' + marker)
    program = compile_il(emitted.stdout[:start] + forced + emitted.stdout[end:])
    result = run([program], stats=True)
    expected = fixture.with_suffix('.out').read_text().replace('ab', 'こんにちは').replace('cd', 'A\0B')
    assert result.stdout == expected * 20, result
    measured = statistics(result)
    assert measured['collections'] >= 300, measured
    assert measured['peak-heap-bytes'] < 4 * 1024 * 1024, measured

    # Force collection with empty separators, including malformed boundaries and
    # empty fragments; decoded counts must describe the completed bytes.
    literals = repository / 'compiler/conformance/valid/array-join-literal-storage'
    emitted = run([binary, '--internal-driver', 'emit-qbe', literals.with_suffix('.trb')])
    collector = '\tcall $trbn_gc_maybe_collect(l %projected)'
    assert emitted.stdout.count(collector) == 1
    program = compile_il(emitted.stdout.replace(collector, '\tcall $trbn_gc_collect(w 0)'))
    result = run([program], stats=True)
    assert result.stdout == literals.with_suffix('.out').read_text(), result
    assert statistics(result)['collections'] > 0

    # Heap-created fragments heal across an empty separator. Run a collection
    # at the allocating size fallback and verify that it actually executes.
    source.write_text(r"""def main()
mut index := 100
while index < 110
    first := index.to_s() + "\xc2"
    last := "\xa2" + (index + 1).to_s()
    puts([first, last].join("").size())
    index += 1
end
end
""")
    emitted = run([binary, '--internal-driver', 'emit-qbe', source]).stdout
    assert emitted.count(marker) == 1
    forced = emitted.replace(marker, ' call $putchar(w 33)\n call $trbn_gc_collect(w 0)\n' + marker)
    # libc may flush the markers after Native has written the size lines.
    output = run([compile_il(forced)]).stdout
    assert output.count('!') == 10 and output.replace('!', '') == '7\n' * 10, repr(output)

    # One-byte and empty fragments must preserve raw bytes with either copy
    # path. Cover every octet, NUL and a long span after a healed UTF-8 seam.
    octets = ['"\\x%02x"' % value for value in range(256)]
    # Changing lengths and separators move the destination across word boundaries.
    sample = b'\x00\xc2\xa2ABCDEF'
    spans = [sample[:length] for length in range(1, len(sample) + 1)]
    short_values = ', '.join('"' + ''.join('\\x%02x' % byte for byte in span) + '"'
                             for span in spans)
    source.write_text('''def joined(values: Array<String>, separator: String): String
return values.join(separator)
end
def main()
values := [''' + ', '.join(octets) + ''']
puts(joined(values, ""))
puts(joined(values, "/"))
puts(joined([''' + short_values + '''], "|"))
puts(joined(["", "\\xc2", "", "\\xa2", "abcdefghijklmnopqrstuvwxyz0123456789", "\\x00", ""], ""))
puts(joined([], ""))
puts(joined([""], ""))
puts(joined(["", "", ""], ""))
end
''')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    assert emitted.stdout.count(collector) == 1
    program = compile_il(emitted.stdout.replace(collector, '\tcall $trbn_gc_collect(w 0)'))
    # Arbitrary bytes cannot be decoded by the text-mode helper above.
    raw = subprocess.run([program], capture_output=True,
                         env=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=60)
    expected = (bytes(range(256)) + b'\n' + b'/'.join(bytes([i]) for i in range(256)) +
                b'\n' + b'|'.join(spans) + b'\n\xc2\xa2abcdefghijklmnopqrstuvwxyz0123456789\x00\n\n\n\n')
    assert raw.returncode == 0 and raw.stdout == expected, raw
    raw.stderr = raw.stderr.decode()
    assert statistics(raw)['collections'] > 0

    # Cross-check both paths against a decoder independent of the runtime.
    # Each malformed byte counts separately, including around the separator.
    fragments = [b'', b'a', b'\0', b'\xc2', b'\xa2', b'\xc2\xa2', b'\xe2',
                 b'\x82\xac', b'\xf0\x9f', b'\x98\x80', b'\xff', b'/\xc2\xa2/']
    values = ', '.join('"' + ''.join('\\x%02x' % byte for byte in span) + '"'
                       for span in fragments)
    source.write_text('''def joined(a: String, b: String, sep: String): String
return [a, b].join(sep)
end
def count_joined(a: String, b: String, sep: String): Integer
return [a, b].join(sep).size()
end
def main()
values := [''' + values + ''']
values.each do |a|
    values.each do |b|
        values.each do |sep|
            puts(joined(a, b, sep).size())
            puts(count_joined(a, b, sep))
        end
    end
end
end
''')
    program = root / 'counts'
    run([binary, 'build', '--compile', source, '--outfile', program])
    expected_counts = [len((a + sep + b).decode('utf-8', 'surrogateescape'))
                       for a in fragments for b in fragments for sep in fragments]
    assert list(map(int, run([program]).stdout.splitlines())) == [
        value for count in expected_counts for value in (count, count)]

    # These joins use the runtime size entry point. Fail if it constructs a
    # temporary String even when the separator isolates a malformed fragment.
    source.write_text(r"""def count_only(a: String, b: String, sep: String): Integer
return [a, b].join(sep).size()
end
def main()
puts(count_only("β", "😀", ""))
puts(count_only("\xc2", "\xa2", "aβz"))
end
""")
    emitted = run([binary, '--internal-driver', 'emit-qbe', source]).stdout
    assert 'call $trbn_string_join_size(l %join_fragments' in emitted
    assert emitted.count(marker) == 1
    guarded = emitted.replace(marker, ' call $exit(w 42)\n' + marker)
    assert run([compile_il(guarded)]).stdout == '2\n5\n'

    # Trailing empty fields contribute no bytes. Reuse a large first String
    # even when its bytes are Unicode, and preserve ordinary empty content.
    source.write_text('''def join_padded(head: String): String
return [head, "", ""].join("")
end
def main()
puts(join_padded("雪😀"))
puts(join_padded(""))
end
''')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source]).stdout
    assert emitted.count(marker) == 1
    guarded = emitted.replace(marker, ' call $exit(w 42)\n' + marker)
    assert run([compile_il(guarded)]).stdout == '雪😀\n\n'

    # Bound total copying/allocation without a wall-clock performance assertion.
    source.write_text('''def main()
mut values: Array<String> := []
(0...4096).each { |_index| values.push("abcd") }
puts(values.join("/").size())
end
''')
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    result = run([compile_il(emitted.stdout)], stats=True)
    assert result.stdout == '20479\n', result
    assert statistics(result)['allocated-bytes'] < 512 * 1024, result.stderr

    # A reduced capacity exercises first-fragment, later-fragment and separator overflow.
    for expression in ('["1234567"].join("")', '["aaaa", "bbb"].join("")', '["aa", "bb"].join("---")'):
        source.write_text('def main()\nputs(' + expression + ')\nend\n')
        emitted = run([binary, '--internal-driver', 'emit-qbe', source])
        start = emitted.stdout.index('function l $trbn_join_fragments(')
        end = emitted.stdout.index('\n}', start) + 2
        runtime = emitted.stdout[start:end]
        assert runtime.count('9007199254740991') == 3
        limited = runtime.replace('9007199254740991', '6')
        program = compile_il(emitted.stdout[:start] + limited + emitted.stdout[end:])
        failed = run([program], code=2)
        assert failed.stdout == '' and failed.stderr == 'panic: allocation failed\n', failed

    retained = run([binary, 'repl'], data='''mut words := ["こんにちは", "😀"]
def separator(): String
return "/"
end
joined := words.join(separator())
words[0] = "changed"
puts(joined)
words.join(1)
puts(words.join("|"))
:quit
''')
    assert 'こんにちは/😀\n' in retained.stdout, retained
    assert retained.stdout.endswith('changed|😀\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and 'error[' in retained.stderr, retained

print('Array join byte, GC, allocation and retained REPL checks passed')
