#!/usr/bin/env python3
"""Check Float parser rounding, fallback, syntax priority and retained values.

The C observer calls the emitted helper without substituting its arithmetic.
It supplies a String header, guard page and unchanged-output sentinel only.
"""
import argparse
import decimal
import json
import math
import os
from pathlib import Path
import random
import re
import struct
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
GRAMMAR = rb'[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?'
SENTINEL = struct.unpack('=Q', struct.pack('=d', 73.0))[0]


def expected(text):
    if not re.fullmatch(GRAMMAR, text):
        return 1, SENTINEL, 0
    value = float(text)
    mantissa, *exponent = re.split(rb'[eE]', text.lstrip(b'+-'))
    whole, *fraction = mantissa.split(b'.')
    digits = whole + (fraction[0] if fraction else b'')
    exponent_digits = exponent[0].lstrip(b'+-') if exponent else b''
    exponent_value = int(exponent[0]) if len(exponent_digits) <= 2 and exponent else 0
    scale = exponent_value - len(fraction[0] if fraction else b'')
    exact = (len(digits) <= 16 and int(digits) < 2**52 and
             len(exponent_digits) <= 2 and -22 <= scale <= 22)
    if not math.isfinite(value):
        return 2, SENTINEL, 1
    return 0, struct.unpack('=Q', struct.pack('=d', value))[0], int(not exact)


def cases():
    values = [b'', b'+', b'-', b'.', b'+.', b'-.', b'0', b'-0', b'-0.0',
              b'.5', b'1.', b'+0001.25e-1', b'1e', b'1e+', b'1e-', b'1e1.0',
              b'1_0', b' 1', b'1 ', b'NaN', b'Inf', b'0x1p1', b'++1',
              b'5e-324', b'-5e-324', b'2e-324', b'-2e-324', b'1e309',
              b'1.7976931348623157e308', b'1.7976931348623159e308']
    magnitudes = [0, 1, 5, 125, 999999999999999, 10**15, 2**52-2, 2**52-1,
                  2**52, 2**52+1, 2**53-1, 2**64, 2**64+1]
    for magnitude in magnitudes:
        for exponent in [-324, -99, -23, -22, -21, -1, 0, 1, 21, 22, 23, 99, 309]:
            for sign in ['', '+', '-']:
                values.append(f'{sign}{magnitude}e{exponent:+d}'.encode())
    rng = random.Random(131420261103)
    for _ in range(8192):
        digits = str(rng.randrange(10**18))
        point = rng.randrange(len(digits)+1)
        values.append((rng.choice(['', '+', '-']) + digits[:point] + '.' +
                       digits[point:] + f'e{rng.randrange(-25, 26):+d}').encode())
    for width in [8, 16, 17, 32, 128, 1024]:
        values += [b'0'*width, b'-'+b'0'*width, b'9'*width,
                   b'1e'+b'0'*width, b'1e-'+b'9'*width,
                   b'-0e'+b'9'*width, b'0.'+b'0'*width+b'1',
                   b'9'*width+b'x', b'1e'+b'9'*width+b'x']
    for byte in range(256):
        values += [bytes([byte]), b'1.25'+bytes([byte]),
                   b'1e9999'+bytes([byte]), b'0.5'+bytes([byte])+b'0']
    return list(dict.fromkeys(values))


def canonical(raw):
    value = struct.unpack('=d', struct.pack('=Q', raw))[0]
    if value == 0:
        return '0.0'
    text = format(decimal.Decimal(repr(value)), 'f')
    return text if '.' in text else text+'.0'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path, nargs='?')
    parser.add_argument('--reference', type=Path)
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--receipt', type=Path)
    args = parser.parse_args()
    assert args.binary or args.qbe, 'Provide ordinary CLI or --qbe for emitter-only checking'
    binary = args.binary.resolve() if args.binary else None
    qbe = args.qbe.resolve() if args.qbe else binary.with_name('qbe')
    controls = cases()
    with tempfile.TemporaryDirectory(prefix='native-float-parsing-') as temporary:
        root = Path(temporary)
        env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root/'history'))

        def run(command, data=None):
            result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                    env=env, capture_output=True, timeout=180)
            assert result.returncode == 0, (command, result.stdout, result.stderr)
            return result

        source = '''import trb/std/process
import trb/std/result
import { NumberParseErrorKind } from trb/std/errors
def main()
Process.argv().each do |text|
case text.try_to_f()
when Result::Ok(value)
puts("ok")
puts(value.to_s())
when Result::Err(error)
case error.kind
when NumberParseErrorKind::InvalidFormat
puts("invalid")
when NumberParseErrorKind::OutOfRange
puts("range")
end
puts(error.input == text)
end
end
end
'''
        (root/'main.trb').write_text(source)
        if binary:
            emitted = run([binary, '--internal-driver', 'emit-qbe', root/'main.trb']).stdout.decode()
        else:
            text = (ROOT/'compiler/src/backend/qbe/runtime/number_parsing.trb').read_text()
            body = re.findall(r'^def emit_float_parse_runtime\(mut output: QbeOutput\)\n(.*?)^end$', text, re.M|re.S)
            assert len(body) == 1
            literals = re.findall(r'qbe_output_line\(output, ("(?:[^"\\]|\\.)*")\)', body[0])
            assert len(literals) == body[0].count('qbe_output_line(')
            emitted = ''.join(json.loads(x) for x in literals)
        helpers = re.findall(r'^function l \$trbn_parse_float\([^\n]*\) \{.*?^\}', emitted, re.M|re.S)
        powers = re.findall(r'^data \$trbn_parse_decimal_powers = \{.*?^\}', emitted, re.M|re.S)
        assert len(helpers) == len(powers) == 1
        # Validate every committed table entry, including scales outside the
        # dense boundary corpus. These integer powers are exactly representable.
        table_words = list(map(int, re.findall(r' l (\d+)', powers[0])))
        exact_powers = [float(10**exponent) for exponent in range(23)]
        assert all(power.as_integer_ratio() == (10**exponent, 1)
                   for exponent, power in enumerate(exact_powers))
        assert table_words == [struct.unpack('=Q', struct.pack('=d', power))[0]
                               for power in exact_powers]
        assert helpers[0].count('call $strtod(') == 1
        il = powers[0]+'\nexport '+helpers[0].replace('call $strtod(', 'call $observe_strtod(')+'\n'
        (root/'parser.ssa').write_text(il)
        run([qbe, '-o', root/'parser.s', root/'parser.ssa'])
        (root/'observer.c').write_text(r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
extern int64_t trbn_parse_float(void *, double *);
static int64_t fallback;
double observe_strtod(const char *text, char **end) {
    fallback++;
    return strtod(text, end);
}
int main(void) {
    int64_t length, status, calls;
    uint64_t wanted;
    size_t page = (size_t)sysconf(_SC_PAGESIZE);
    unsigned char *memory = mmap(NULL, page*2, PROT_READ|PROT_WRITE,
                                MAP_PRIVATE|MAP_ANON, -1, 0);
    assert(memory != MAP_FAILED);
    assert(mprotect(memory+page, page, PROT_NONE) == 0);
    while (fread(&length, sizeof length, 1, stdin) == 1) {
        assert(fread(&status, sizeof status, 1, stdin) == 1);
        assert(fread(&wanted, sizeof wanted, 1, stdin) == 1);
        assert(fread(&calls, sizeof calls, 1, stdin) == 1);
        assert(length >= 0 && (size_t)length+32 < page);
        /* Runtime Strings include a terminator for the established fallback. */
        size_t padded = ((size_t)length+1+7) & ~(size_t)7;
        unsigned char *text = memory+page-padded-24;
        memset(text, 'x', padded+24);
        memcpy(text+8, &length, sizeof length);
        memcpy(text+16, &length, sizeof length);
        assert(fread(text+24, 1, (size_t)length, stdin) == (size_t)length);
        text[24+length] = 0;
        double value = 73.0;
        uint64_t actual;
        fallback = 0;
        assert(trbn_parse_float(text, &value) == status);
        memcpy(&actual, &value, sizeof actual);
        assert(actual == wanted && fallback == calls);
    }
    assert(!ferror(stdin));
    assert(munmap(memory, page*2) == 0);
    return 0;
}
''')
        run(['/usr/bin/cc', root/'parser.s', root/'observer.c', '-o', root/'observer'])
        payload = b''.join(struct.pack('=qqQq', len(t), *expected(t))+t for t in controls)
        observed = run([root/'observer'], payload)
        assert not observed.stdout and not observed.stderr
        if binary:
            native = root/'native'
            run([binary, 'build', '--compile', root/'main.trb', '--outfile', native])
            programs = [native]
            if args.reference:
                project = root/'reference'
                (project/'src').mkdir(parents=True)
                (project/'src/main.trb').write_text(source)
                config = project/'trbconfig.jsonc'
                config.write_text(json.dumps({'name':'float-parser-contract','version':'0.1.0',
                    'mode':'go','sourceDir':'src','outDir':'build','copyFiles':False,
                    'packageManagement':'managed','go':{'module':'example.org/float-parser-contract','version':'1.27'}}))
                target = root/'reference-program'
                run([args.reference.resolve(), 'build', '--config', config, '--compile', '--outfile', target])
                programs.append(target)
            # Public paths exercise successful and failing Result values with
            # dynamic strings. Raw byte and dense random coverage lives above.
            dynamic = controls[:540]+controls[-1024:]
            dynamic = [t for t in dynamic if b'\x00' not in t]
            wanted = b''.join(('ok\n'+canonical(bits)+'\n' if status == 0 else
                              ('invalid\ntrue\n' if status == 1 else 'range\ntrue\n')).encode()
                             for status, bits, _ in map(expected, dynamic))
            for program in programs:
                # os.fsdecode preserves malformed byte inputs through argv.
                result = run([program, *map(os.fsdecode, dynamic)])
                assert result.stdout == wanted and not result.stderr, (program, result.stdout, result.stderr)
            replay = b'''saved := "1.25e-1".to_f()
zero := "-0.0".to_f()
record Later
value: Integer
end
puts(saved)
puts((1.0 / zero).to_s())
"1e999x".to_f()
"1e999".to_f()
:reload
puts(saved)
puts((1.0 / zero).to_s())
:quit
'''
            for repl in [binary]+([args.reference.resolve()] if args.reference else []):
                result = run([repl, 'repl'], replay)
                assert result.stdout.count(b'0.125\n-Infinity\n') >= 2, result.stdout
                assert b'reloaded' in result.stdout, result.stdout
                assert b'invalid Float' in result.stderr and b'Float is outside the portable range' in result.stderr, result.stderr
        receipt = {'status':'pass','rawCases':len(controls), 'ordinary':bool(binary),
                   'reference':bool(args.reference),'checks':'bitwise rounding, signed zero, fallback calls, syntax priority, failure sentinel, bounded reads; ordinary paths when requested'}
        if args.receipt:
            args.receipt.write_text(json.dumps(receipt, indent=2)+'\n')
        print(json.dumps(receipt))


if __name__ == '__main__':
    main()
