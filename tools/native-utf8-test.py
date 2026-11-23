#!/usr/bin/env python3
"""Exercise ordinary UTF-8 input, output and managed lifetimes, including GC."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
qbe = binary.with_name('qbe')

with tempfile.TemporaryDirectory(prefix='native UTF-8 日本語 ') as directory:
    root = Path(directory)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history.json'))

    def run(command, *, data=None, success=True):
        result = subprocess.run([item if isinstance(item, bytes) else str(item) for item in command], input=data, capture_output=True,
                                cwd=root, env=env, timeout=30)
        assert (result.returncode == 0) == success, (command, result)
        return result

    def compile_source(text, expected, *, force_gc=False, arguments=()):
        source = root / 'main.trb'
        source.write_text(text)
        result = run([binary, 'check', source])
        assert result.stdout == b'ok\n' and result.stderr == b'', result
        output = root / 'program'
        if force_gc:
            emitted = run([binary, '--internal-driver', 'emit-qbe', source])
            assert emitted.stderr == b'', emitted
            marker = b'\t%result =l call $trbn_gc_alloc(l $trbn_desc_string, l %size, l 0)'
            assert emitted.stdout.count(marker) == 1
            il = emitted.stdout.replace(marker, b'\tcall $trbn_gc_collect(w 0)\n' + marker)
            (root / 'program.ssa').write_bytes(il)
            run([qbe, '-o', root / 'program.s', root / 'program.ssa'])
            run(['/usr/bin/cc', root / 'program.s', '-lm', '-o', output])
        else:
            result = run([binary, 'build', '--compile', '--outfile', output, source])
            assert result.stdout == result.stderr == b'', result
        result = run([output, *arguments])
        assert result.stdout == expected and result.stderr == b'', result

    # Escaped byte fragments form a scalar only when joined. Retain earlier
    # results while later joins allocate, including an embedded NUL.
    compile_source(r'''def join(left: String, right: String): String
return left + right
end
def main()
lead := "\xc2"
tail := "\242"
saved := join(lead, tail)
astral := join("\xf0\x9f", "\230\200")
text := join(saved, "\000\u65e5\U0001f600")
puts(saved)
puts(saved.size())
puts(astral)
puts(astral.size())
puts(text)
puts(text.size())
puts(text.codepoints()[1])
puts("\a\b\f\n\r\t\v".size())
puts(lead.size())
end
''', '¢\n1\n😀\n1\n¢\0日😀\n4\n0\n7\n1\n'.encode(), force_gc=True)

    # Stress decoding and literal emission with every byte, repeatedly. These
    # bytes remain exact even when String indexing would replace invalid UTF-8.
    escaped_bytes = ''.join('\\x%02x' % value for value in range(256)) * 8
    compile_source('def main()\ntext := "' + escaped_bytes + '"\nputs(text)\nputs(text.size())\nend\n',
                   bytes(range(256)) * 8 + b'\n2048\n', force_gc=True)

    # Both MIR String functions and the nominal/Hash adapter must retain indexed
    # values and their owners when every String allocation collects the heap.
    compile_source('''record Box
text: String
end
def join(left: String, right: String): String
return left + right
end
def indexed(text: String): String
return join(text[1], text[-1])
end
def stored(): String
mut values := ["あ" + "😀"]
saved := values[0][0]
values[0] = "replacement"
mut table := {"日本" => Box.new(text: saved)}
table["日本語"] = Box.new(text: indexed("Aあ😀"))
return table["日本"].text + table["日本" + "語"].text
end
def main()
puts(stored())
puts(indexed("Aあ😀"))
end
''', 'ああ😀\nあ😀\n'.encode(), force_gc=True)

    # A query retains its captured receiver while evaluating an allocating
    # argument. Search offsets count code points and survive later allocations.
    compile_source('''def join(left: String, right: String): String
return left + right
end
def show(value: Integer?)
if value == nil
puts("nil")
else
puts(value)
end
end
def main()
value := join("x", "😀y😀y")
show(value.index(join("😀", "y")))
show(value.rindex(join("😀", "y")))
puts(value.include?(join("😀", "y")))
puts(value.start_with?(join("x", "😀")))
puts(value.end_with?(join("😀", "y")))
puts(value)
end
''', '1\n3\ntrue\ntrue\ntrue\nx😀y😀y\n'.encode(), force_gc=True)

    compile_source('def join(left: String, right: String): String\nreturn left + right\nend\ndef main()\nvalue := join("x", "😀日本")\npoints := value.codepoints()\ncharacters := value.chars()\nreversed := value.reverse()\npoints.each { |point| puts(point) }\ncharacters.each { |character| puts(character) }\nputs(reversed)\nputs(value)\nend\n', '120\n128512\n26085\n26412\nx\n😀\n日\n本\n本日😀x\nx😀日本\n'.encode(), force_gc=True)

    compile_source('def main()\nvalue := "A\x00😀"\nputs(value.codepoints()[1])\nputs(value.chars()[1].size())\nputs(value.reverse())\nend\n', '0\n1\n😀\0A\n'.encode(), force_gc=True)

    # More than one backing-buffer growth while earlier results stay live.
    compile_source('def join(left: String, right: String): String\nreturn left + right\nend\ndef main()\nvalue := join("' + '日本😀' * 32 + '", "!")\ncharacters := value.chars()\npoints := value.codepoints()\nreversed := value.reverse()\nputs(characters.size())\nputs(points.size())\nputs(characters[0])\nputs(characters[-1])\nputs(points[-1])\nputs(reversed.start_with?("!😀"))\nputs(value.end_with?("😀!"))\nend\n',
                   '97\n97\n日\n!\n33\ntrue\ntrue\n'.encode(), force_gc=True)

    # Capture the original receiver while allocating/evaluating Range endpoints,
    # retain sliced results, and keep NUL within the code-point span.
    compile_source('''def main()
mut value := "A" + "日本😀"
first := fn(): Integer
 value = "replacement" + "!"
 return 1
end
part := value.slice(first()...3)
empty := value.slice(value.size()...value.size())
puts(part)
puts(empty.size())
puts(value)
end
''', '日本\n0\nreplacement!\n'.encode(), force_gc=True)
    compile_source('def main()\ntext := "A\0😀\0B"\npart := text.slice(1..3)\nputs(part)\nputs(part.size())\nputs(text.slice(5...5).size())\nend\n',
                   '\0😀\0\n3\n0\n'.encode(), force_gc=True)

    # Count runtime input across short tails and ASCII runs after valid or
    # malformed UTF-8. Literal headers cannot supply these cached counts.
    count_inputs, count_output = [], []
    payloads = [(b'\xc2\xa2', 1), (b'\xe6\x97\xa5', 1), (b'\xf0\x9f\x98\x80', 1),
                (b'\xff', 1), (b'\xe3\x81', 2), (b'\xed\xa0\x80', 3)]
    for prefix in range(9):
        for tail in (0, 1, 7, 8, 9, 31, 64):
            for payload, points in payloads:
                count_inputs.append(b'a' * prefix + payload + b'b' * tail + payload)
                count_output.append(str(prefix + tail + points * 2))
    compile_source('''import trb/std/process
def main()
Process.argv().each { |text| puts(text.size()) }
end
''', ('\n'.join(count_output) + '\n').encode(), force_gc=True, arguments=count_inputs)

    slice_source = '''import trb/std/process
def main()
value := Process.argv()[0]
puts(value.slice(0...value.size()))
puts(value.slice(1..1))
puts(value.slice(value.size()...value.size()).size())
end
'''
    compile_source(slice_source, 'A�B\n�\n0\n'.encode(), arguments=(b'A\xffB',), force_gc=True)
    compile_source(slice_source, '��\n�\n0\n'.encode(), arguments=(b'\xe3\x81',), force_gc=True)

    sequence_source = """import trb/std/process
def main()
arguments := Process.argv()
value := arguments[0]
points := value.codepoints()
characters := value.chars()
reversed := value.reverse()
points.each { |point| puts(point) }
characters.each { |character| puts(character) }
puts(reversed)
end
"""
    compile_source(sequence_source, '65\n65533\nA\n�\n�A\n'.encode(), arguments=(b'A\xff',), force_gc=True)
    compile_source(sequence_source, '65533\n65533\n�\n�\n��\n'.encode(), arguments=(b'\xe3\x81',), force_gc=True)

    query_source = '''import trb/std/process
def show(value: Integer?)
if value == nil
puts("nil")
else
puts(value)
end
end
def main()
arguments := Process.argv()
value := arguments[0]
part := arguments[1]
puts(value.include?(part))
puts(value.start_with?(part))
puts(value.end_with?(part))
show(value.index(part))
show(value.rindex(part))
end
'''
    # Indexed search follows decoded code points in external byte input, while
    # literal predicates keep the reference's byte-exact matching semantics.
    for value, part, expected in (
        (b'\xc2x\xa2', '�'.encode(), b'false\nfalse\nfalse\n0\n2\n'),
        ('¢'.encode(), b'\xa2', b'true\nfalse\ntrue\nnil\nnil\n'),
        (b'\xff', b'\xff', b'true\ntrue\ntrue\n0\n0\n'),
        (b'\xff', '�'.encode(), b'false\nfalse\nfalse\n0\n0\n'),
        ('�'.encode(), b'\xff', b'false\nfalse\nfalse\n0\n0\n'),
    ):
        compile_source(query_source, expected, arguments=(value, part))

    compile_source('def main()\nvalue := "A\0😀\0B"\nfirst := value.index("\0")\nlast := value.rindex("\0")\nif first != nil\nputs(first)\nend\nif last != nil\nputs(last)\nend\nputs(value.include?("😀\0"))\nend\n',
                   b'1\n3\ntrue\n')

    # A real NUL in a literal must retain the full byte and code-point lengths.
    compile_source('def main()\ntext := "あ\0😀"\nputs(text)\nputs(text.size())\nputs(text[1])\nend\n',
                   'あ\0😀\n3\n\0\n'.encode())
    long = '日本語😀' * 90
    compile_source('def main()\ntext := "' + long + '"\nputs(text)\nputs(text.size())\nend\n',
                   (long + '\n360\n').encode())
    compile_source('''import trb/std/process
def main()
arguments := Process.argv()
puts(arguments[0])
puts(arguments[0].size())
puts(arguments[0][-1])
end
''', 'こんにちは😀\n6\n😀\n'.encode(), arguments=('こんにちは😀',))

    # External byte fragments can form one code point only after concatenation.
    compile_source('import trb/std/process\ndef main()\nargs := Process.argv()\nvalue := args[0] + args[1]\nputs(value)\nputs(value.size())\nputs(value[-1])\nend\n',
                   '¢\n1\n¢\n'.encode(), arguments=(b'\xc2', b'\xa2'))

    # Invalid source bytes must be rejected before token decoding can replace
    # them with U+FFFD. Valid U+FFFD remains an ordinary character.
    compile_source('def main()\nputs("�")\nend\n', '�\n'.encode())
    for malformed in (b'\xff', b'\xc0\xaf', b'\xe3\x81', b'\xed\xa0\x80', b'\xf4\x90\x80\x80'):
        (root / 'main.trb').write_bytes(b'def main()\nputs("' + malformed + b'")\nend\n')
        for command in ('check', 'build'):
            result = run([binary, command, root / 'main.trb'], success=False)
            assert result.stdout == b'' and b'source is not valid UTF-8' in result.stderr, result
        # Avoid loading the invalid project entry before the REPL submission.
        (root / 'main.trb').unlink()
        result = run([binary, 'repl'], data=b'"' + malformed + b'"\n:quit\n')
        assert result.stdout == b'' and b'source is not valid UTF-8' in result.stderr, result

print('UTF-8: literals, NUL, long data, argv, Hash/record lifetimes, forced GC and invalid source passed')
