#!/usr/bin/env python3
"""Exercise byte encodings and request ID propagation across modules and forced GC."""
import argparse
from native_network_link import static_network_flags
import base64
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
parser.add_argument('--reference-only', action='store_true')
args = parser.parse_args()
if args.reference_only and not args.reference:
    parser.error('--reference-only requires --reference')
binary = args.binary.resolve()

producer = r'''import trb/std/encoding/hex
import trb/std/encoding/base64
import trb/std/secure_random
import trb/std/result
import { Context, ContextKey, Next, Request, Response } from trb/web
import { Body, Headers, HttpMethod } from trb/http
import { RequestID } from trb/web/middleware/request_id
KEY := ContextKey<Bytes>.new("payload")
def make_bytes(): Bytes
case Hex.decode("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
when Result::Ok(value)
return value
when Result::Err(_error)
return "failed".to_bytes()
end
end
INITIAL_BYTES := make_bytes()
def check_bytes(value: Bytes)
puts(value.size() == 256)
puts(Hex.encode(value) == "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
case Base64.decode(Base64.encode(value))
when Result::Ok(decoded)
puts(Hex.encode(decoded) == Hex.encode(value))
when Result::Err(_error)
puts(false)
end
case Base64.url_decode(Base64.url_encode(value))
when Result::Ok(decoded)
puts(Hex.encode(decoded) == Hex.encode(value))
when Result::Err(_error)
puts(false)
end
end
class Terminal implements Next
def call(context: Context): Response
case context.fetch(KEY)
when Result::Ok(value)
check_bytes(value)
when Result::Err(_error)
puts("lost bytes")
end
case context.request.header_value("x-request-id")
when Result::Ok(value)
case Hex.decode(value)
when Result::Ok(decoded)
puts(decoded.size() == 16)
when Result::Err(_error)
puts(false)
end
return Response.text(value)
when Result::Err(_error)
return Response.text("missing")
end
end
end
def probe()
check_bytes(INITIAL_BYTES)
value := make_bytes()
context := Context.new(request: Request.new(method: HttpMethod.get(), path: "/example", query_string: "", headers: Headers.new(), body: Body.empty()), path_parameters: {}).with(KEY, value)
response := RequestID.call(context, Terminal.new())
case response.header_value("x-request-id")
when Result::Ok(id)
puts(id == response.body.to_s())
when Result::Err(_error)
puts(false)
end
puts(SecureRandom.bytes(257).size() == 257)
check_classifications()
end
'''
# Check every possible input octet against Python's codecs and the reference.
# Raw literals include invalid UTF-8 so table indexes 128..255 are exercised too.
classification_source = ['CLASSIFICATION_INPUTS := [']
classification_source.extend(f'"\\x{point:02x}",' for point in range(256))
classification_source += [']', 'def check_classifications()',
                          'CLASSIFICATION_INPUTS.each do |input|',
                          'check_hex_digit(input)', 'check_base64_digit(input)',
                          'check_url_digit(input)', 'end', 'end']
for name, module, method in [('hex', 'Hex', 'decode'),
                             ('base64', 'Base64', 'decode'),
                             ('url', 'Base64', 'url_decode')]:
    suffix = '0' if name == 'hex' else 'AAA'
    message = ('invalid hexadecimal character' if name == 'hex' else
               'invalid base64url character' if name == 'url' else
               'invalid base64 character')
    classification_source += [f'def check_{name}_digit(input: String)',
        f'text := input + "{suffix}"', f'case {module}.{method}(text)',
        'when Result::Ok(value)', 'puts("ok")', 'puts(Hex.encode(value))',
        'when Result::Err(error)', 'puts("error")',
        f'mut kind := {module}::DecodeErrorKind::InvalidCharacter',
        f'mut message := "{message}"']
    if name != 'hex':
        padding_message = ('base64url input must not contain padding' if name == 'url'
                           else 'invalid base64 padding')
        classification_source += ['if input == "="',
            'kind = Base64::DecodeErrorKind::InvalidPadding',
            f'message = "{padding_message}"', 'end']
    classification_source += [
        'puts(error.kind == kind && error.input == text && error.index == 0 && error.message == message)',
        'end', 'end']
producer += '\n'.join(classification_source) + '\n'
classification_expected = []
for point in range(256):
    octet = bytes([point])
    for name in ['hex', 'base64', 'url']:
        alphabet = (b'0123456789abcdefABCDEF' if name == 'hex' else
                    b'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789' +
                    (b'-_' if name == 'url' else b'+/'))
        if point in alphabet:
            value = (bytes.fromhex((octet + b'0').decode('ascii')) if name == 'hex' else
                     base64.b64decode(octet + b'AAA',
                                      altchars=b'-_' if name == 'url' else None,
                                      validate=True))
            classification_expected += ['ok\n', value.hex() + '\n']
        else:
            classification_expected += ['error\n', 'true\n']
expected = 'true\n' * 11 + ''.join(classification_expected)
retained_input = r'''import { make_bytes, check_bytes } from encoding_values
stored := make_bytes()
record Hex
other: Integer
end
record Base64
other: String
end
check_bytes(stored)
record Later
end
check_bytes(stored)
:quit
'''

# The byte-oriented fast path must retain code-point indexes and the reference
# precedence between length, character, padding and canonical-bit failures.
decode_errors = [
    ('hex', 'abc', 'OddLength', 3, 'hex input has odd length'),
    ('hex', '0g0', 'InvalidCharacter', 1, 'invalid hexadecimal character'),
    ('hex', '0é0', 'InvalidCharacter', 1, 'invalid hexadecimal character'),
    ('hex', '00😀0', 'InvalidCharacter', 2, 'invalid hexadecimal character'),
    ('hex', '00\x000', 'InvalidCharacter', 2, 'invalid hexadecimal character'),
    ('base64', 'é', 'InvalidLength', 1, 'base64 input length must be a multiple of 4'),
    ('base64', 'A=é', 'InvalidLength', 3, 'base64 input length must be a multiple of 4'),
    ('base64', 'AAAé', 'InvalidCharacter', 3, 'invalid base64 character'),
    ('base64', 'AA😀=', 'InvalidCharacter', 2, 'invalid base64 character'),
    ('base64', 'AA=é', 'InvalidPadding', 3, 'invalid base64 padding'),
    ('base64', '=éAA', 'InvalidPadding', 0, 'invalid base64 padding'),
    ('base64', 'AB==', 'NonCanonical', 1, 'non-canonical base64 encoding'),
    ('base64', 'AAB=', 'NonCanonical', 2, 'non-canonical base64 encoding'),
    ('url', 'AAAAé', 'InvalidLength', 5, 'base64url input has invalid length'),
    ('url', 'AAAé', 'InvalidCharacter', 3, 'invalid base64url character'),
    ('url', 'AA+é', 'InvalidCharacter', 2, 'invalid base64url character'),
    ('url', 'AA=é', 'InvalidPadding', 2, 'base64url input must not contain padding'),
    ('url', 'AB', 'NonCanonical', 1, 'non-canonical base64url encoding'),
    ('url', 'AAB', 'NonCanonical', 2, 'non-canonical base64url encoding'),
]


def error_source():
    lines = ['import trb/std/encoding/hex', 'import trb/std/encoding/base64',
             'import trb/std/result', 'def main()']
    for decoder, value, kind, index, message in decode_errors:
        module = 'Hex' if decoder == 'hex' else 'Base64'
        method = 'url_decode' if decoder == 'url' else 'decode'
        # TypeRB uses \x00 rather than JSON's \u0000 spelling.
        literal = json.dumps(value, ensure_ascii=False).replace('\\u0000', '\\x00')
        lines.extend([
            f'case {module}.{method}({literal})',
            'when Result::Ok(_value)', 'puts(false)', 'when Result::Err(error)',
            f'puts(error.kind == {module}::DecodeErrorKind::{kind}'
            f' && error.input == {literal} && error.index == {index}'
            f' && error.message == {json.dumps(message)})', 'end',
        ])
    return '\n'.join(lines + ['end', ''])

with tempfile.TemporaryDirectory(prefix='native-encoding-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=90)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    (root / 'encoding_values.trb').write_text(producer)
    main = root / 'main.trb'
    main.write_text('import { probe } from encoding_values\ndef main()\nprobe()\nend\n')
    program = root / 'program'
    if args.reference:
        reference = args.reference.resolve()
        config = root / 'reference.trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'encoding-values', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/encoding-values'}}))
        run([reference, 'check', '--config', config])
        run([reference, 'build', '--config', config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected

    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'encoding-values',
        'mode': 'go' if args.reference_only else 'trb', 'sourceDir': '.',
        'go': {'module': 'example.com/encoding-values'}}))
    repl_binary = reference if args.reference_only else binary
    retained = run([repl_binary, 'repl', '--config', config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.endswith('true\n' * 8), retained

    if not args.reference_only:
        main.write_text('import { probe } from encoding_values\ndef main()\n'
                        '(0...8).each do |_index|\nprobe()\nend\nend\n')
        emitted = run([binary, '--internal-driver', 'emit-qbe', main])
        forced, ordinary, hooks = [], False, 0
        for line in emitted.stdout.splitlines():
            if line.startswith('function '):
                ordinary = '$trbnf' in line
            if line == '}':
                ordinary = False
            if ordinary and any(marker in line for marker in (
                    'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                    'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                    'call $trbn_gc_alloc(', 'call $trbn_string_concat(')):
                forced.append('\tcall $trbn_gc_collect(w 0)')
                hooks += 1
            forced.append(line)
        assert hooks >= 8, hooks
        il, assembly = root / 'input.ssa', root / 'input.s'
        il.write_text('\n'.join(forced) + '\n')
        run([binary.with_name('qbe'), '-o', assembly, il])
        run(['/usr/bin/cc', assembly, *static_network_flags(binary.with_name('qbe')), '-lm', '-o', program])
        collected = subprocess.run([str(program)], cwd=root, text=True, capture_output=True,
            env=dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=90)
        assert collected.returncode == 0 and collected.stdout == expected * 8, collected
        statistics = {}
        for line in collected.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['collections'] >= 8, statistics
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        print('Encoding GC hooks:', hooks, 'statistics:', json.dumps(statistics, sort_keys=True))

    if not args.reference_only:
        shim = root / 'entropy-fixture.c'
        shim.write_text(r"""#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
static int calls;
int getentropy(void *buffer, size_t length) {
    ++calls;
    fprintf(stderr, "entropy-chunk:%zu\n", length);
    if (length == 0 || length > 256) abort();
    if (getenv("ENTROPY_FIXTURE_FAIL") && calls == 2) return -1;
    unsigned char *bytes = buffer;
    for (size_t i = 0; i < length; ++i) bytes[i] = (unsigned char)i;
    return 0;
}
""")
        for length in (0, 1, 256, 257, 513, 65536, -1, 65537):
            main.write_text('import trb/std/secure_random\ndef main()\n'
                            f'puts(SecureRandom.bytes({length}).size())\nend\n')
            il.write_text(run([binary, '--internal-driver', 'emit-qbe', main]).stdout)
            run([binary.with_name('qbe'), '-o', assembly, il])
            run(['/usr/bin/cc', assembly, shim, '-lm', '-o', program])
            observed = subprocess.run([str(program)], cwd=root, env=environment,
                                      text=True, capture_output=True, timeout=30)
            if 0 <= length <= 65536:
                chunks = [256] * (length // 256) + ([length % 256] if length % 256 else [])
                assert observed.returncode == 0 and observed.stdout == str(length) + '\n', observed
                assert observed.stderr == ''.join(f'entropy-chunk:{n}\n' for n in chunks), observed
                if length == 513:
                    failed = subprocess.run([str(program)], cwd=root, text=True, capture_output=True,
                        env=dict(environment, ENTROPY_FIXTURE_FAIL='1'), timeout=30)
                    assert failed.returncode != 0 and not failed.stdout, failed
                    assert failed.stderr.splitlines() == ['entropy-chunk:256', 'entropy-chunk:256',
                        'panic: secure_random.bytes could not read operating system entropy'], failed
            else:
                assert observed.returncode != 0 and not observed.stdout, observed
                assert observed.stderr == 'panic: secure_random.bytes length must be between 0 and 65536\n', observed
        print('Entropy OS chunk bounds and failure checks passed.')

    (root / 'encoding_values.trb').unlink()
    main.write_text(error_source())
    error_expected = 'true\n' * len(decode_errors)
    if args.reference:
        reference_config = root / 'reference.trbconfig.jsonc'
        run([reference, 'check', '--config', reference_config])
        run([reference, 'build', '--config', reference_config, '--compile', '--outfile', program])
        assert run([program]).stdout == error_expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == error_expected
    print('Decode Unicode offsets and error precedence checks passed.')

print('Encoding, request ID, retained-session and storage checks passed.')
