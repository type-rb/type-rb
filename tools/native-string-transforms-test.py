#!/usr/bin/env python3
"""Verify String transform bytes, Unicode lookup, retained roots and allocation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-string-transforms-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        environment = dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else env
        result = subprocess.run([item if isinstance(item, bytes) else str(item) for item in command],
                                cwd=root, env=environment, input=data, capture_output=True, timeout=90)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    def emit(source):
        (root / 'main.trb').write_text(source)
        result = run([binary, '--internal-driver', 'emit-qbe', root / 'main.trb'])
        assert result.stderr == b'', result.stderr
        return result.stdout

    def assemble(il):
        (root / 'program.ssa').write_bytes(il)
        run([binary.with_name('qbe'), '-o', root / 'program.s', root / 'program.ssa'])
        run(['/usr/bin/cc', root / 'program.s', '-lm', '-o', root / 'program'])
        return root / 'program'

    def compile_source(source, *, split_only=False, selected_fields=False):
        il = emit(source)
        checks = [
            (b'trbn_string_split', (b'trbn_string_from_bytes', b'trbn_split_fragment', b'trbn_array_push_paced')),
            (b'trbn_split_pair', (b'trbn_split_fragment', b'trbn_array_push_paced')),
            (b'trbn_split_fragment', (b'trbn_string_alloc',)),
        ]
        if selected_fields:
            assert b'call $trbn_string_split_fields(' in il
            checks.append((b'trbn_string_split_fields', (b'trbn_string_from_bytes', b'trbn_split_fragment', b'trbn_array_push_paced')))
        if not split_only:
            checks += [(b'trbn_string_replace', (b'trbn_string_alloc',)),
                       (b'trbn_string_case', (b'trbn_string_alloc',))]
        for symbol, callees in checks:
            start = il.index(b'function l $' + symbol + b'(')
            end = il.index(b'\n}', start) + 2
            body = il[start:end]
            for callee in callees:
                # Instrument every allocation/append branch, including prefix
                # fallback and the paired-fragment path, independent of names.
                pattern = rb'(?m)^.* call \$' + re.escape(callee) + rb'\(.*$'
                body, count = re.subn(pattern, lambda match: b' call $trbn_gc_collect(w 0)\n' + match[0], body)
                assert count > 0, (symbol, callee)
            il = il[:start] + body + il[end:]
        return assemble(il)

    def statistics(result):
        values = {}
        for line in result.stderr.decode().splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in values, line
            values[key] = int(value)
        assert values['live-bytes'] == 0, values
        assert values['allocated-bytes'] == values['reclaimed-bytes'], values
        return values

    # Go's complete mapping tables are independently regenerated in quick CI.
    # Compare the actual QBE binary-search implementation at every code point.
    maps = {}
    for line in (repository / 'compiler/src/backend/qbe/runtime/unicode_case_data.trb').read_text().splitlines():
        if '\tqbe_output_line(output, ' not in line:
            continue
        declaration = json.loads(line.split('output, ', 1)[1][:-1])
        name = re.search(r'data \$trbn_case_(upper|lower) =', declaration)[1]
        maps[name] = {int(a): int(b) for a, b in re.findall(r'w (\d+), w (\d+)', declaration)}
    expected = hashlib.sha256()
    for point in range(0x110000):
        expected.update(struct.pack('<II', maps['upper'].get(point, point), maps['lower'].get(point, point)))
    # Demand case support through ordinary MIR before probing its table directly.
    il = emit('def main()\nputs("a".upcase())\nend\n')
    il, renamed = re.subn(rb'(export function w \$)main\(', rb'\1checked_main(', il)
    assert renamed == 1
    il += b'''
export function w $main() {
@start
 %buffer =l call $calloc(l 1114112, l 8)
 jmp @loop
@loop
 %point =l phi @start 0, @write %next
 %done =w ceql %point, 1114112
 jnz %done, @finish, @write
@write
 %upper =l call $trbn_case_point(l %point, l 0)
 %lower =l call $trbn_case_point(l %point, l 1)
 %offset =l mul %point, 8
 %first =l add %buffer, %offset
 %second =l add %first, 4
 storew %upper, %first
 storew %lower, %second
 %next =l add %point, 1
 jmp @loop
@finish
 %written =l call $write(w 1, l %buffer, l 8912896)
 call $free(l %buffer)
 ret 0
}
'''
    result = run([assemble(il)])
    assert len(result.stdout) == 8912896 and not result.stderr, result.stderr
    assert hashlib.sha256(result.stdout).digest() == expected.digest()

    # Check managed inputs at each allocation and append, including byte-width
    # changes and UTF-8 sequences formed across replacement boundaries.
    program = compile_source('''import trb/std/process
def main()
args := Process.argv()
text := args[0]
pattern := args[1]
replacement := args[2]
puts(text.split(pattern).join("/"))
puts(text.replace_all(pattern, replacement))
puts(text.upcase())
puts(text.downcase())
end
''')
    for text, pattern, replacement, expected_output in (
        ('日本😀a😀'.encode(), '😀'.encode(), 'İ'.encode(), '日本/a/\n日本İaİ\n日本😀A😀\n日本😀a😀\n'.encode()),
        (b'\xffA\xffB', b'\xff', b'Z', b'/A/B\nZAZB\n' + '�A�B\n�a�b\n'.encode()),
        ('ȺıſK'.encode(), 'ı'.encode(), b'', 'Ⱥ/ſK\nȺſK\nȺISK\nⱥıſk\n'.encode()),
        (b'\xc2X\xa0', b'X', b'', b'\xc2/\xa0\n\xc2\xa0\n' + '�X�\n�x�\n'.encode()),
    ):
        result = run([program, text, pattern, replacement], stats=True)
        assert result.stdout == expected_output, result
        assert statistics(result)['collections'] >= 5

    program = compile_source('''def main()
text := "a\\x00b\\x00"
puts(text.split("\\x00").join("/"))
puts(text.replace_all("\\x00", "😀").size())
puts(text.replace_all("b", "\\x00").upcase())
end
''')
    result = run([program], stats=True)
    assert result.stdout == b'a/b/\n4\nA\x00\x00\x00\n', result
    statistics(result)

    program = compile_source('''import trb/std/process
def main()
text := Process.argv()[0]
parts := text.split(":")
puts(parts.size())
puts(parts.join(":") == text)
puts(text.replace_all(":", "日本").size())
puts(text.upcase().downcase() == text)
end
''')
    large = 'word:' * 2048
    result = run([program, large], stats=True)
    assert result.stdout == b'2049\ntrue\n12288\ntrue\n', result
    measured = statistics(result)
    assert measured['collections'] >= 4096 and measured['peak-heap-bytes'] < 2 * 1024 * 1024, measured
    assert measured['allocated-bytes'] < 1024 * 1024, measured

    # A projected split still owns each visible String after later allocations.
    # Force collection inside the runtime, including when a field was skipped.
    program = compile_source('''import trb/std/process
def main()
args := Process.argv()
fields := args[0].split(args[1])
puts(fields.size())
puts(fields[1])
puts(fields[2])
end
''', split_only=True, selected_fields=True)
    for source, separator in [(b",,x,", b","), (b",a,b,c,d,e,f,", b","),
                              ("α///β////δ".encode(), b"//"), (b"x\xf0\x9fX\xf0\x9fY", b"\x9f")]:
        pieces = source.split(separator)
        result = run([program, source, separator], stats=True)
        assert result.stdout == str(len(pieces)).encode() + b"\n" + b"\n".join(pieces[1:3]) + b"\n", result
        assert statistics(result)['collections'] >= len(pieces)

    for source, separator, before_error, diagnostic in [
        (b"one", b",", b"1\n", b"panic: index is out of bounds\n"),
        (b"a,b,c", b"", b"", b"panic: String split separator is empty\n"),
    ]:
        failure = subprocess.run([program, source, separator], capture_output=True, env=env, cwd=root, timeout=30)
        assert (failure.returncode, failure.stdout, failure.stderr) == (2, before_error, diagnostic), failure

    # All byte values, split boundaries and owned fragment lifetimes use the
    # dynamic path. Invalid bytes each count as one decoding unit, while valid
    # multibyte sequences must still be decoded after a separator cuts them.
    all_bytes = ''.join('\\x%02x' % byte for byte in range(256))
    split_source = '''import trb/std/process
def main()
args := Process.argv()
mut text := args[0]
mut separator := args[1]
if text == "__all_bytes__"
text = "ALL_BYTES"
end
if separator == "__nul__"
separator = "\\x00"
end
mut parts := text.split(separator)
saved := parts[0]
puts(parts.size())
parts.each { |part| puts(part.size()) }
puts(parts.join("\\x1f"))
text = "source replaced"
parts[0] = "first replaced"
puts(saved)
end
'''.replace('ALL_BYTES', all_bytes)
    program = compile_source(split_source, split_only=True)
    reference_program = None
    if args.reference:
        project = root / 'reference-split'
        (project / 'src').mkdir(parents=True)
        (project / 'src/main.trb').write_text(split_source)
        config = project / 'trbconfig.jsonc'
        config.write_text(json.dumps({
            'name': 'string-split-contract', 'version': '0.1.0', 'mode': 'go',
            'sourceDir': 'src', 'outDir': 'build', 'copyFiles': False,
            'packageManagement': 'managed',
            'go': {'module': 'example.org/string-split-contract', 'version': '1.27'},
        }))
        reference_program = root / 'reference-split-program'
        run([args.reference.resolve(), 'build', '--config', config,
             '--compile', '--outfile', reference_program])
    cases = [(bytes([byte]) + b':A:' + bytes([byte]) + b':',
              bytes([byte]) if byte % 2 else b':') for byte in range(1, 256)]
    cases += [(b'', b':'), (b'::', b':'), (b'plain', b'absent'),
              (b'aaaaa', b'aa'), (b'\xc2X\xa0', b'X'),
              (b'\xe0\x80\x80|\xed\xa0\x80|\xf4\x90\x80\x80', b'|'),
              (b'A|\xf0\x9f\x98', b'|')]
    unicode_text = 'a¢€😀z'.encode()
    for start in range(len(unicode_text)):
        for width in (1, 2, 3):
            cases.append((unicode_text, unicode_text[start:start + width]))
    cases += [(b'__all_bytes__', b'__nul__'),
              (b'__all_bytes__', b'\x80'), (b'__all_bytes__', b'\xff')]
    for text, separator in cases:
        raw = bytes(range(256)) if text == b'__all_bytes__' else text
        pattern = b'\x00' if separator == b'__nul__' else separator
        fragments = raw.split(pattern)
        sizes = [len(part.decode('utf-8', errors='surrogateescape')) for part in fragments]
        expected_output = ('\n'.join(map(str, [len(fragments), *sizes])) + '\n').encode()
        expected_output += b'\x1f'.join(fragments) + b'\n' + fragments[0] + b'\n'
        result = run([program, text, separator], stats=True)
        assert result.stdout == expected_output, (text, separator, result.stdout, expected_output)
        statistics(result)
        if reference_program:
            assert run([reference_program, text, separator]).stdout == expected_output

    retained = run([binary, 'repl'], data='''mut source := "a😀b😀"
parts := source.split("😀")
saved := source.replace_all("😀", "日本").upcase()
source = "changed"
record Later
value: String
end
puts(parts.join("/"))
puts(saved)
source.split("")
source.replace_all("", "x")
puts(saved.downcase())
:quit
'''.encode())
    assert 'a/b/\nA日本B日本\n'.encode() in retained.stdout, retained
    assert retained.stdout.endswith('a日本b日本\n'.encode()), retained
    assert len(retained.stderr.splitlines()) == 2, retained
    assert b'String split separator is empty' in retained.stderr, retained
    assert b'String replacement pattern is empty' in retained.stderr, retained

    byte_session = b'''mut source := "a\\x80:a\\xff:"
parts := source.split(":")
saved := parts[1]
source = "changed"
record LaterBytes
value: String
end
puts(parts.size())
puts(saved.size())
puts(parts.join("/"))
:quit
'''
    byte_retained = run([binary, 'repl'], data=byte_session)
    expected_bytes = b'3\n2\na\x80/a\xff/\n'
    assert expected_bytes in byte_retained.stdout and not byte_retained.stderr, byte_retained
    if args.reference:
        reference_retained = run([args.reference.resolve(), 'repl'], data=byte_session)
        assert expected_bytes in reference_retained.stdout and not reference_retained.stderr, reference_retained

print('String transform Unicode, byte, lifetime, allocation and retained REPL checks passed')
