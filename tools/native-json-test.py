#!/usr/bin/env python3
"""Verify JSON source ownership, retained values and managed recursive payloads."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
source = r'''import trb/std/json
import trb/std/jsonc
import trb/std/result
import trb/std/url

def display(value: JSON::Value)
case JSON.stringify(value)
when Result::Ok(text)
puts(text)
when Result::Err(error)
puts(error.message)
end
end

def probe()
held := JSON::Value::Object({"held" => JSON::Value::Array([JSON::Value::String("retained"), JSON::Value::Integer(7)])})
mut index := 0
while index < 32
case JSONC.parse("/* comment */{\"z\":[null,true,{\"text\":\"\\ud83d\\ude00\"}],\"a\":1}")
when Result::Ok(value)
case JSON.field(value, "z")
when Result::Ok(items)
if index == 31
display(items)
end
when Result::Err(error)
puts(error.message)
end
when Result::Err(error)
puts(error.message)
end
index += 1
end
display(held)
puts(URL.encode_component("json + url"))
end

def main()
probe()
end
'''
expected = '[null,true,{"text":"😀"}]\n{"held":["retained",7]}\njson%20%2B%20url\n'

with tempfile.TemporaryDirectory(prefix='native-json-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
    env.pop('TYPE_RB_NATIVE_RUNTIME_TRACE', None)
    env.pop('TYPE_RB_NATIVE_RUNTIME_STATS', None)

    def run(command, *, data=None, success=True, environment=env):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=120)
        assert (result.returncode == 0) == success, (command, result.stdout, result.stderr)
        return result

    file_root = root / 'file root'
    file_root.mkdir()
    main = file_root / 'main.trb'
    main.write_text(source)
    shadow = file_root / 'trb/std/json/index.trb'
    shadow.parent.mkdir(parents=True)
    shadow.write_text('module JSON\ndef wrong()\nunknown()\nend\nend\n')
    assert run([binary, 'check', main]).stdout == 'ok\n'
    program = root / 'program'
    run([binary, 'build', '--compile', main, '--outfile', program])
    result = run([program], environment=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
    assert result.stdout == expected, result
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert run([binary, 'run', main]).stdout == expected

    # Collect at each decoded string boundary while raw nodes, recursive values
    # and an unrelated retained object are live in ordinary checked MIR.
    emitted = run([binary, '--internal-driver', 'emit-qbe', main]).stdout
    match = re.search(r'^function l \$trbn_bytes_to_string\([^\n]*\) \{.*?^\}', emitted, re.M | re.S)
    assert match
    hook = ' %result =l call $trbn_string_alloc(l %size)'
    assert match[0].count(hook) == 1
    forced = emitted.replace(match[0], match[0].replace(hook, ' call $trbn_gc_collect(w 0)\n' + hook))
    ssa, assembly, forced_program = root / 'forced.ssa', root / 'forced.s', root / 'forced'
    ssa.write_text(forced)
    run([binary.with_name('qbe'), '-o', assembly, ssa])
    run(['/usr/bin/cc', assembly, '-lm', '-o', forced_program])
    assert run([forced_program]).stdout == expected

    project = root / 'project'
    (project / 'src').mkdir(parents=True)
    config = project / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'json-import', 'mode': 'trb', 'sourceDir': 'src'}))
    (project / 'src/main.trb').write_text(source)
    assert run([binary, 'check', '--config', config]).stdout == 'ok\n'
    run([binary, 'build', '--compile', '--config', config, '--outfile', program])
    assert run([program]).stdout == expected

    declarations = source.split('def main()')[0]
    retained = run([binary, 'repl'], data=declarations + 'probe()\nclass Later\nend\nprobe()\n:quit\n')
    assert not retained.stderr and retained.stdout == expected * 2, retained

    # Alias and default importers must share recursive enum identity.
    (file_root / 'helper.trb').write_text('import trb/std/json as OtherJSON\n'
        'def identity(value: OtherJSON::Value): OtherJSON::Value\nreturn value\nend\n')
    main.write_text('import trb/std/json\nimport trb/std/result\nimport { identity } from helper\n'
        'def main()\ncase JSON.stringify(identity(JSON::Value::Array([JSON::Value::Integer(7)])))\n'
        'when Result::Ok(text)\nputs(text)\nwhen Result::Err(error)\nputs(error.message)\nend\nend\n')
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == '[7]\n'

    # Cover every raw non-control octet, UTF-8 width boundary and invalid
    # prefix. JSON decoding replaces each invalid byte, unlike Bytes#to_s's
    # maximal-subpart replacement. String escaping must also preserve HTML
    # and JavaScript safety after a long, otherwise unescaped prefix.
    def quoted(text):
        encoded = json.dumps(text, ensure_ascii=False)
        for character in ('<', '>', '&', '\u2028', '\u2029'):
            encoded = encoded.replace(character, f'\\u{ord(character):04x}')
        return encoded

    pairs = [(bytes([octet]), chr(octet) if octet < 128 else '\ufffd')
             for octet in range(32, 256) if octet not in (34, 92)]
    for point in (0x80, 0x7ff, 0x800, 0xd7ff, 0xe000, 0xffff,
                  0x10000, 0x10ffff, 0x2028, 0x2029, 0xfffd):
        pairs.append((chr(point).encode(), chr(point)))
    for raw in (b'\xc0\xaf', b'\xc2', b'\xe0\x9f\xbf', b'\xed\xa0\x80',
                b'\xe2\x82', b'\xf0\x8f\xbf\xbf', b'\xf4\x90\x80\x80',
                b'\xf0\x90\x80', b'\xf5\x80\x80\x80'):
        pairs.append((raw, '\ufffd' * len(raw)))
    # A decoded escape can precede raw UTF-8 octets in the same join. Cached
    # code-point counts must not mistake these split bytes for separate runes.
    pairs.extend([
        (b'\\u00e9' + 'β😀'.encode(), 'éβ😀'),
        (b'\\ud83d\\ude00' + 'éβ'.encode(), '😀éβ'),
        (b'\\u00e9' + b'\xf0\x90\x80' + 'β'.encode(), 'é' + '\ufffd' * 3 + 'β'),
    ])
    statements, outputs = [], []
    for raw, decoded in pairs:
        literal = '"' + ''.join(f'\\x{octet:02x}' for octet in b'"' + raw + b'"') + '"'
        statements.append(f'parse_and_display({literal})')
        outputs.append(quoted(decoded))
    # Exercise the original String and both copied spans around a masked comment.
    for document, expected_value in [
        (b'"\xe2\x82"', quoted('\ufffd\ufffd')),
        (b'"/\xe2\x82"', quoted('/\ufffd\ufffd')),
        (b'["\xe2\x82",/*\xff*/"\xed\xa0\x80"]', '["\ufffd\ufffd","\ufffd\ufffd\ufffd"]'),
    ]:
        literal = '"' + ''.join(f'\\x{octet:02x}' for octet in document) + '"'
        statements.append(f'parse_jsonc_and_display({literal})')
        outputs.append(expected_value)
    # Encoding raw String bytes must preserve the same replacement boundaries
    # without relying on the parser to have normalized invalid UTF-8 first.
    encoded_pairs = [(bytes([octet]), chr(octet) if octet < 128 else '\ufffd')
                     for octet in range(256)]
    encoded_pairs.extend((raw, decoded) for raw, decoded in pairs
                         if len(raw) > 1 and not raw.startswith(b'\\'))
    for raw, decoded in encoded_pairs:
        literal = '"' + ''.join(f'\\x{octet:02x}' for octet in raw) + '"'
        statements.append(f'display(JSON::Value::String({literal}))')
        outputs.append(quoted(decoded))
        if len(raw) > 1:
            # Raw bytes reach both key and value writers without parser repair.
            statements.append(f'display(JSON::Value::Object({{{literal} => '
                              f'JSON::Value::Array([JSON::Value::Null, JSON::Value::String({literal})])}}))')
            outputs.append('{' + quoted(decoded) + ':[null,' + quoted(decoded) + ']}')
    # Exercise accumulated fragments across long ASCII/Unicode/escaped strings.
    for unit, count, tail, decoded in [
            (b'a', 8192, b'\xf0\x90\x80', 'a' * 8192 + '\ufffd' * 3),
            (b'\\ud83d\\ude00', 512, b'', '😀' * 512),
            (b'\\ud800x', 512, b'', '\ufffdx' * 512)]:
        def octets(raw):
            return '"' + ''.join(f'\\x{octet:02x}' for octet in raw) + '"'
        delimiter = bytes([34])
        expression = f'{octets(delimiter)} + repeated({octets(unit)}, {count}) + {octets(tail + delimiter)}'
        statements.append(f'parse_and_display({expression})')
        outputs.append(quoted(decoded))
    for suffix in ('', '<>&', '\u2028\u2029', '"\\\n\t\x00'):
        value = 'aβ😀' * 256 + suffix
        statements.append(f'display(JSON::Value::String({json.dumps(value, ensure_ascii=False)}))')
        outputs.append(quoted(value))
        statements.append(f'parse_and_display({json.dumps(quoted(value), ensure_ascii=False)})')
        outputs.append(quoted(value))
    # Force collection while a failed Unicode escape reconstructs its raw
    # diagnostic. Earlier object members must remain rooted across that join.
    for digits in ("x000", "0x00", "00x0", "000x", "12xz"):
        sequence = "\\u" + digits
        document = '{"held":"aβ😀","bad":"' + sequence + '"}'
        statements.append(f'parse_and_display({json.dumps(document, ensure_ascii=False)})')
        outputs.append("invalid escape sequence `" + sequence + "` in string")
    # Sibling containers and scalar variants must remain independent while the
    # parser stores their raw nodes and the conversion stack revisits parents.
    structures = [
        [{"id": index, "name": "β😀", "items": [index, None, index % 2 == 0],
          "empty": {}} for index in range(256)],
        {f"key-{index}": "aβ😀\n<>&" for index in range(256)},
        {"left": [[], {}, [False, True]], "right": {"nested": [{}, [], None]}},
    ]
    # Alternating containers and unrelated sibling keys expose a resumed parent
    # accidentally using a child's position or sorted-key list.
    def mixed_tree(depth, seed):
        if depth == 0:
            return [None, False, seed, 'β😀\n<>&', [], {}][seed % 6]
        children = [mixed_tree(depth - 1, seed * 3 + index) for index in range(3)]
        if seed % 2:
            return [[], children[0], {}, children[1], children[2]]
        return {'z\n': children[0], '': children[1], 'aβ😀<': children[2]}

    structures.append(mixed_tree(5, 2))
    structures.append([{'z': [[], {'nested': index}, {}], 'a': {}, '': [index]}
                       for index in range(32)])
    # Exercise longer multibyte text beside escapes and in object keys.
    for width in (15, 16, 17, 31, 32, 33):
        structures.extend([
            'a' * width + 'β😀' + 'z' * width,
            'a' * width + '\n"<>&\u2028' + 'β😀' * width,
            {'a' * width + 'β/~/😀': ['β😀' * width, 'a' * width]},
        ])
    nested = 0
    for _ in range(128):
        nested = {"z": nested, "a": []}
    structures.append(nested)
    for value in structures:
        document = json.dumps(value, ensure_ascii=False)
        statements.append(f'parse_and_display({json.dumps(document, ensure_ascii=False)})')
        encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'))
        for character in ('<', '>', '&', '\u2028', '\u2029'):
            encoded = encoded.replace(character, f'\\u{ord(character):04x}')
        outputs.append(encoded)
    item = json.dumps('"aβ😀",', ensure_ascii=False)
    last = json.dumps('"aβ😀"]', ensure_ascii=False)
    statements.append(f'parse_and_display("[" + repeated({item}, 2047) + {last})')
    outputs.append('[' + ','.join(['"aβ😀"'] * 2048) + ']')
    for prefix, unit, count, suffix, expected_document in [
            ('/*', 'β😀* ', 1024, '*/["aβ😀",true]', '["aβ😀",true]'),
            ('// ', 'β😀', 1024, '\r\n{"value":"a\\nβ😀"}', '{"value":"a\\nβ😀"}'),
            ('{"slashes":"/* text */ // text",/*', 'x', 4096, '*/"z":0}',
             '{"slashes":"/* text */ // text","z":0}')]:
        expression = (f'{json.dumps(prefix, ensure_ascii=False)} + repeated({json.dumps(unit, ensure_ascii=False)}, {count})'
                      f' + {json.dumps(suffix, ensure_ascii=False)}')
        statements.append(f'parse_jsonc_and_display({expression})')
        outputs.append(expected_document)
    # Independent integer output controls include decimal carry boundaries and
    # the largest portable exact values. The same corpus runs with forced GC.
    portable_limit = 9007199254740991
    portable_values = {0, 1, -1, portable_limit, -portable_limit,
                       portable_limit - 1, 1 - portable_limit}
    for power in range(16):
        for offset in (-1, 0, 1):
            value = 10 ** power + offset
            portable_values.update((value, -value))
    numeric_pairs = [(str(value), str(value)) for value in sorted(portable_values)]
    numeric_pairs += [('-0', '0'), ('-0.0', '0'), ('1.0', '1'), ('1e2', '100'),
                      ('1.25e-1', '0.125'), ('-0.5', '-0.5'),
                      ('9007199254740990.5', '9007199254740990'),
                      ('{"n":1e309,"n":7}', '{"n":7}'),
                      ('{"n":7,"n":1e309}', 'JSON number is not finite')]
    for token in ('9007199254740992', '-9007199254740992', '9' * 80):
        numeric_pairs.append((token, 'JSON integer is outside the portable range'))
    numeric_pairs.append(('9' * 400, 'JSON number is not finite'))
    for document, expected_document in numeric_pairs:
        statements.append(f'parse_and_display({json.dumps(document)})')
        outputs.append(expected_document)
    for token, kind in [('0', 'integer'), ('-0', 'integer'), ('1e2', 'integer'),
                        (str(portable_limit), 'integer'), ('0.5', 'float'), ('1.25e-1', 'float')]:
        statements.append(f'report_number_kind({json.dumps(token)})')
        outputs.append(kind)
    # Construct Float values directly: JSON decoding has a narrower portable
    # integer range. Cover both sides of notation thresholds and signed zero.
    float_output_pairs = [
        ('0', '0'),
        ('-0', '-0'),
        ('0.5', '0.5'),
        ('0.125', '0.125'),
        ('1', '1'),
        ('1.5', '1.5'),
        ('1e-6', '0.000001'),
        ('9.999999999999997e-7', '9.999999999999997e-7'),
        ('1.0000000000000002e-6', '0.0000010000000000000002'),
        ('1e-10', '1e-10'),
        ('1e20', '100000000000000000000'),
        ('9.999999999999999e20', '999999999999999900000'),
        ('1e21', '1e+21'),
        ('1.0000000000000001e21', '1.0000000000000001e+21'),
        ('1e22', '1e+22'),
        ('5e-324', '5e-324'),
        ('2.2250738585072014e-308', '2.2250738585072014e-308'),
        ('1.7976931348623157e308', '1.7976931348623157e+308'),
        ('0.12345678901234568', '0.12345678901234568'),
        ('0.8765432109876543', '0.8765432109876543'),
        ('12345.678901234567', '12345.678901234567'),
        ('0.0012345678901234567', '0.0012345678901234567'),
    ]
    float_output_pairs += [('-' + token, '-' + expected_text)
                           for token, expected_text in float_output_pairs[2:]]
    float_statements = [f'display(JSON::Value::Float({json.dumps(token)}.to_f()))'
                        for token, _expected_text in float_output_pairs]
    statements.extend(float_statements)
    outputs.extend(expected_text for _token, expected_text in float_output_pairs)
    declarations = source.split('def probe()')[0].replace('import trb/std/url\n', '')
    main.write_text(declarations + '''
def report_number_kind(text: String)
case JSON.parse(text)
when Result::Ok(value)
case value
when JSON::Value::Integer(_integer)
puts("integer")
when JSON::Value::Float(_float)
puts("float")
else
puts("unexpected value")
end
when Result::Err(error)
puts(error.message)
end
end
# Construct large runtime inputs without embedding large compiler constants.
def repeated(text: String, count: Integer): String
mut parts: Array<String> := []
mut index := 0
while index < count
parts.push(text)
index += 1
end
return parts.join("")
end
def parse_jsonc_and_display(text: String)
case JSONC.parse(text)
when Result::Ok(value)
display(value)
when Result::Err(error)
puts(error.message)
end
end
def parse_and_display(text: String)
case JSON.parse(text)
when Result::Ok(value)
display(value)
when Result::Err(error)
puts(error.message)
end
end
def main()
''' + '\n'.join(statements) + '\nend\n')
    run([binary, 'build', '--compile', main, '--outfile', program])
    actual = run([program]).stdout
    assert actual == '\n'.join(outputs) + '\n', {
        'expectedLines': len(outputs), 'actualLines': len(actual.splitlines()),
        'differences': [(index + 1, expected[:160], observed[:160])
                        for index, (expected, observed) in enumerate(zip(outputs, actual.splitlines()))
                        if expected != observed][:3],
    }
    emitted = run([binary, '--internal-driver', 'emit-qbe', main]).stdout
    join = re.search(r'^function l \$trbn_array_join\([^\n]*\) \{.*?^\}', emitted, re.M | re.S)
    assert join
    assert join[0].count('@start\n') == 1
    forced = emitted.replace(join[0], join[0].replace('@start\n', '@start\n call $trbn_gc_collect(w 0)\n'))
    # Collect during parser-owned pool and object-member creation as well as
    # joins. Partially initialized state and retained ancestors must stay live.
    for allocator in ('trbn_array_new', 'trbn_hash_new'):
        allocation = re.search(r'^function l \$' + allocator + r'\([^\n]*\) \{.*?^\}', forced, re.M | re.S)
        assert allocation and allocation[0].count('@start\n') == 1
        forced = forced.replace(allocation[0], allocation[0].replace('@start\n', '@start\n call $trbn_gc_collect(w 0)\n'))
    # Collection after the span helper installs both roots exercises buffer
    # growth with a live input Bytes value and a retained builder owner.
    span = re.search(r'^function \$trbn_builder_append_span\([^\n]*\) \{.*?^\}', forced, re.M | re.S)
    assert span
    root_hook = ' call $trbn_gc_temp_push(l %value)\n'
    assert span[0].count(root_hook) == 1
    forced = forced.replace(span[0], span[0].replace(
        root_hook, root_hook + ' call $trbn_gc_collect(w 0)\n'))
    ssa.write_text(forced)
    run([binary.with_name('qbe'), '-o', assembly, ssa])
    run(['/usr/bin/cc', assembly, '-lm', '-o', forced_program])
    assert run([forced_program]).stdout == '\n'.join(outputs) + '\n'

    float_source = declarations.replace('import trb/std/jsonc\n', '') + 'def main()\n' + '\n'.join(float_statements) + '\nend\n'
    float_output = ''.join(text + '\n' for _token, text in float_output_pairs)
    float_main = root / 'float-output.trb'
    float_main.write_text(float_source)
    if args.reference:
        assert run([args.reference.resolve(), 'run', float_main]).stdout == float_output
    retained_float_source = float_source.replace('def main()', 'def report_floating()')
    retained = run([binary, 'repl'], data=retained_float_source +
                   'report_floating()\nclass LaterNumbers\nend\nreport_floating()\n:quit\n')
    assert not retained.stderr and retained.stdout == float_output * 2, retained
    if args.reference:
        assert run([args.reference.resolve(), 'run', main]).stdout == '\n'.join(outputs) + '\n'

    # Cursor synchronization must preserve metadata as well as the error text.
    # Long ordinary spans precede every kind of decoder exit, and number runs
    # end at both grammar delimiters and malformed fraction/exponent tokens.
    if args.reference:
        cursor_documents = []
        for width in (0, 1, 31, 32, 33, 4096):
            prefix = '"' + 'a' * width
            cursor_documents.extend(prefix + tail for tail in (
                '"', '', '\\', '\\q"', '\\u12', '\\u12xz"', '\\ud83d\\ude00"',
                '\\ud83dX"', '\\nβ😀"', '\x01"', 'β😀\\q"'))
        for number in ('0', '-0', '123456789', '-123456789', '1.25', '-1.25e+2',
                       '1E-2', '-', '01', '1.', '1.e2', '1e', '1e+', '1e-',
                       '1eX', '1.2.3', '9007199254740992', '1e309'):
            cursor_documents.extend((number, '[0,' + number + ']',
                                     '{"held":"β😀","n":' + number + '}'))
        for number in (str(portable_limit), str(-portable_limit), str(portable_limit + 1),
                       str(-portable_limit - 1), '9' * 80, '9' * 400, '-0', '00', '-01',
                       '1e-400', '9007199254740990.5', '9007199254740991.5'):
            cursor_documents.extend((number, 'true ' + number, '[' + number + ',]',
                                     '{"n":' + number + ',"n":3}',
                                     '{"n":3,"n":' + number + '}'))
        cursor_documents.extend([' \t\r\n', '[ \r\n 0 , \t -2E+3 \n ]', '{} []',
                                 '{ \t "a" \r\n : \t [false, ]}', '[0, - ]'])
        # The reference checks both parsed values and diagnostic positions after
        # comment masking, including individual UTF-8 bytes, quotes and CR/LF.
        comment_documents = [
            '{"path":"/a//b/*c*/"}', '/*β😀*/{ "x": true } //end',
            '[0,/*β😀\r\n more*/ X]', '[0,//β😀\r still a comment\n X]',
            '[0,/*x*//**/false,]', '0/* unterminated', '/* only', '/x',
            'true//x\rfalse', ' /*x*/ true false', '"unterminated //',
            json.dumps('quoted " slash // and trailing backslash \\') + '/*ok*/',
        ]
        cursor_source = declarations + '''
def report_cursor(result: Result<JSON::Value, JSON::Error>)
case result
when Result::Ok(value)
display(value)
when Result::Err(error)
puts(error.message)
puts(error.path)
line := error.line
column := error.column
if line != nil
puts(line.to_s())
else
puts("nil")
end
if column != nil
puts(column.to_s())
else
puts("nil")
end
end
end
def main()
''' + '\n'.join(f'report_cursor({module}.parse({json.dumps(text, ensure_ascii=False)}))'
                      for module, documents in [('JSON', cursor_documents), ('JSONC', comment_documents)]
                      for text in documents) + '\nend\n'
        main.write_text(cursor_source)
        expected_cursor = run([args.reference.resolve(), 'run', main]).stdout
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected_cursor

    # Decode failures still report escaped JSON Pointer paths after successful
    # traversal, including overwritten keys and nested array/object boundaries.
    pointer_cases = [
        ('9007199254740992', ''),
        ('{"":9007199254740992}', '/'),
        ('{"~/":[0,{"β😀/~/":9007199254740992}]}', '/~0~1/1/β😀~1~0~1'),
        ('{"~1":9007199254740992}', '/~01'),
        ('{"z":9007199254740992,"a/":9007199254740992}', '/a~1'),
        ('{"a":9007199254740992,"a":1}', None),
        ('{"a":1,"a":9007199254740992}', '/a'),
        ('[0,{"/~":[0,9007199254740992]}]', '/1/~1~0/1'),
        ('{"z":{"a":9007199254740992},"a":[{"z":9007199254740992,"a":9007199254740992}]}', '/a/0/a'),
        ('{"a":{"z":9007199254740992},"a":{"b":1},"b":{"a":9007199254740992}}', '/b/a'),
        ('[' + '0,' * 64 + '9007199254740992]', '/64'),
        ('{"old":[{"bad":9007199254740992}],"old":0,"live":[0,{"~/":9007199254740992}]}', '/live/1/~0~1'),
        ('{"old":{"bad":[9007199254740992]},"old":{"ok":[]}}', None),
    ]
    pointer_source = declarations.replace('import trb/std/jsonc\n', '') + '''
def report_path(source: String)
case JSON.parse(source)
when Result::Ok(_value)
puts("ok")
when Result::Err(error)
puts(error.path)
puts(error.message)
end
end
def main()
''' + '\n'.join(f'report_path({json.dumps(text, ensure_ascii=False)})' for text, _ in pointer_cases) + '\nend\n'
    main.write_text(pointer_source)
    run([binary, 'build', '--compile', main, '--outfile', program])
    pointer_output = ''.join('ok\n' if path is None else path + '\nJSON integer is outside the portable range\n'
                             for _, path in pointer_cases)
    assert run([program]).stdout == pointer_output

    # Path metadata can be constructed lazily after discovering an invalid
    # number. Retain the original traversal order, mixed keys and array indexes.
    encoding_source = '''import trb/std/json
import trb/std/result
def report_encoding(value: JSON::Value)
case JSON.stringify(value)
when Result::Ok(_text)
puts("ok")
when Result::Err(error)
puts(error.path)
puts(error.message)
end
end
def main()
bad := JSON::Value::Float(1.0 / 0.0)
negative := JSON::Value::Float(-1.0 / 0.0)
nan := JSON::Value::Float(0.0 / 0.0)
report_encoding(bad)
report_encoding(JSON::Value::Array([JSON::Value::Integer(0), negative, nan]))
report_encoding(JSON::Value::Object({"~/β😀" => JSON::Value::Array([JSON::Value::Null, bad])}))
report_encoding(JSON::Value::Array([JSON::Value::Object({"" => nan}), bad]))
mut items: Array<JSON::Value> := []
mut index := 0
while index < 64
items.push(JSON::Value::Integer(index))
index += 1
end
items.push(bad)
report_encoding(JSON::Value::Array(items))
report_encoding(JSON::Value::Object({"valid" => JSON::Value::Array([JSON::Value::Float(-0.0), JSON::Value::Null])}))
end
'''
    encoding_output = ''.join(path + '\nJSON Float must be finite\n'
                              for path in ('', '/1', '/~0~1β😀/1', '/0/', '/64')) + 'ok\n'
    main.write_text(encoding_source)
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == encoding_output
    if args.reference:
        # These objects contain only one failing member, avoiding the separate
        # unresolved multi-member precedence and REPL-message specifications.
        assert run([args.reference.resolve(), 'run', main]).stdout == encoding_output

    for regression_source, regression_output in ((pointer_source, pointer_output),
                                                  (encoding_source, encoding_output),
                                                  (float_source, float_output)):
        main.write_text(regression_source)
        emitted = run([binary, '--internal-driver', 'emit-qbe', main]).stdout
        conversion = re.search(r'^function l \$trbn_integer_to_string\([^\n]*\) \{.*?^\}', emitted, re.M | re.S)
        assert conversion and conversion[0].count('@start\n') == 1
        forced = emitted.replace(conversion[0], conversion[0].replace(
            '@start\n', '@start\n call $trbn_gc_collect(w 0)\n'))
        ssa.write_text(forced)
        run([binary.with_name('qbe'), '-o', assembly, ssa])
        run(['/usr/bin/cc', assembly, '-lm', '-o', forced_program])
        assert run([forced_program]).stdout == regression_output

    # Loading JSON's shared octets must not grant authored modules access to
    # the internal dependency, even after the verified provider has loaded it.
    for declarations in ('import trb/internal/encoding_bytes as Hidden\n',
                         'import trb/std/json\nimport trb/internal/encoding_bytes as Hidden\n',
                         'module EncodingBytes\nend\nimport trb/internal/encoding_bytes as Hidden\n'):
        main.write_text(declarations + 'def main()\nputs("unreachable")\nend\n')
        result = run([binary, 'check', main], success=False)
        assert 'internal encoding and entropy sources are reserved for verified standard sources' in result.stderr, result

    for declarations in ('import trb/internal/json as Hidden\n',
                         'module JSON\nend\nimport trb/internal/json as Hidden\n'):
        main.write_text(declarations + 'def main()\nputs("unreachable")\nend\n')
        result = run([binary, 'check', main], success=False)
        assert 'internal package imports are reserved for verified standard sources' in result.stderr, result

print('PASS JSON file/project ownership, recursive values, forced collection and retained REPL')
