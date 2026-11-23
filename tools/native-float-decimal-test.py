#!/usr/bin/env python3
"""Execute the emitted Float arithmetic on the host, independently of bootstrap.

The C harness supplies input/output and a minimal String allocation boundary.
It is not a conversion implementation or proof of ordinary GC/REPL behavior.
"""
import argparse
import decimal
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import random
import re
import struct
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
RUNTIME = ROOT / 'compiler/src/backend/qbe/runtime'
SOURCES = ['float_powers.trb', 'float_decimal.trb', 'decimal_digits.trb', 'scalar_strings.trb']
QBE_TARGETS = {'darwin-arm64': 'arm64_apple', 'darwin-amd64': 'amd64_apple',
               'linux-arm64': 'arm64', 'linux-amd64': 'amd64_sysv'}
HARNESS = r'''
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern uint64_t trbn_decimal_multiply_high(uint64_t, uint64_t);
extern void *trbn_float_to_string(double);
struct string {
  uint64_t header, bytes, points;
  char data[];
};
static void *allocation;
static size_t allocated_bytes;
void *trbn_string_alloc(int64_t payload) {
  if (payload < 0) abort();
  allocated_bytes = sizeof(uint64_t) + (size_t)payload;
  allocation = malloc(allocated_bytes);
  if (!allocation) abort();
  memset(allocation, 0xa5, allocated_bytes);
  return allocation;
}
_Noreturn void trbn_fail(const char *text, int64_t size) {
  fwrite(text, 1, (size_t)size, stderr); exit(1);
}
int main(int argc, char **argv) {
  uint64_t left, right;
  if (argc != 2) return 2;
  if (strcmp(argv[1], "products") == 0) {
    while (scanf("%" SCNx64 " %" SCNx64, &left, &right) == 2)
      printf("%016" PRIx64 "\n", trbn_decimal_multiply_high(left, right));
  } else if (strcmp(argv[1], "floats") == 0) {
    while (scanf("%" SCNx64, &left) == 1) {
      double input; memcpy(&input, &left, sizeof(input));
      struct string *value = trbn_float_to_string(input);
      if (allocation && (value != allocation || value->bytes + sizeof(*value) + 1 != allocated_bytes)) abort();
      if (value->bytes != value->points || value->data[value->bytes] != 0) abort();
      fwrite(value->data, 1, (size_t)value->bytes, stdout); putchar('\n');
      free(allocation); allocation = NULL;
    }
  } else return 2;
  return ferror(stdin) || ferror(stdout);
}
'''


def canonical(raw):
    value = struct.unpack('>d', raw.to_bytes(8, 'big'))[0]
    if math.isnan(value):
        return 'NaN'
    if math.isinf(value):
        return '-Infinity' if value < 0 else 'Infinity'
    if value == 0:
        return '0.0'
    text = format(decimal.Decimal(repr(value)), 'f')
    return text if '.' in text else text + '.0'


def float_bits(value):
    return struct.unpack('>Q', struct.pack('>d', value))[0]


def controls():
    values = []
    for exponent in range(-1074, 1024):
        value = math.ldexp(1.0, exponent)
        values.extend(float_bits(x) for x in [value, -value,
                      value * 1.0000000000000002, value * 0.9999999999999999])
    generator = random.Random(130520261102)
    finite = []
    while len(finite) < 65536:
        raw = generator.getrandbits(64)
        if (raw >> 52) & 0x7FF != 0x7FF:
            finite.append(raw)
    values.extend(finite)
    for digits in range(1, 18):
        value = float('1.' + '2345678901234567'[:digits - 1])
        values.extend([float_bits(value), float_bits(-value)])
    values.extend([0, 1 << 63, 0x7FF0000000000000, 0xFFF0000000000000,
                   0x7FF8000000000000, 0xFFF8000000000000,
                   0x7FEFFFFFFFFFFFFF, 0xFFEFFFFFFFFFFFFF])
    edges = [0, 1, 2, 3, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x100000000,
             0x100000001, 0x7FFFFFFFFFFFFFFF, 0x8000000000000000,
             0xFFFFFFFF00000000, 0xFFFFFFFF00000001, 0xFFFFFFFFFFFFFFFD,
             0xFFFFFFFFFFFFFFFE, 0xFFFFFFFFFFFFFFFF]
    products = [(left, right) for left in edges for right in edges]
    generator = random.Random(1305646420261102)
    products.extend((generator.getrandbits(64), generator.getrandbits(64))
                    for _ in range(65536))
    return values, products


def constant_emitter(name):
    content = (RUNTIME / 'decimal_digits.trb').read_text()
    bodies = re.findall(r'^def ' + re.escape(name) +
                        r'\(mut output: QbeOutput\)\n(.*?)^end$', content, re.M | re.S)
    assert len(bodies) == 1, name
    # Fail closed if the shared emitter gains control flow or interpolation.
    for line in bodies[0].splitlines():
        if line.strip() and not line.lstrip().startswith('#'):
            assert re.fullmatch(r'\s*qbe_output_line\(output, "(?:[^"\\]|\\.)*"\)', line), line
    return bodies[0]


def emitted_runtime():
    parts = []
    pattern = r'qbe_output_line\(output, ("(?:[^"\\]|\\.)*")\)'
    for name in SOURCES:
        content = (constant_emitter('emit_decimal_pair_table') if name == 'decimal_digits.trb'
                   else (RUNTIME / name).read_text())
        if name == 'scalar_strings.trb':
            call = '\temit_decimal_digits(output)\n'
            assert content.count(call) == 1
            content = content.replace(call, constant_emitter('emit_decimal_digits'))
        literals = re.findall(pattern, content)
        assert len(literals) == content.count('qbe_output_line(output, '), name
        parts.extend(json.loads(literal) for literal in literals)
    result = ''.join(parts)
    for symbol in ('trbn_decimal_multiply_high', 'trbn_float_to_string'):
        declaration = 'function l $' + symbol + '('
        assert result.count(declaration) == 1
        result = result.replace(declaration, 'export ' + declaration)
    assert '$snprintf' not in result and '$strtod' not in result
    return result


def runtime_definitions(source):
    definitions, body, name = {}, [], None
    for line in source.splitlines(keepends=True):
        if name is not None:
            body.append(line)
            if line.strip() == '}':
                assert name not in definitions, name
                # Ordinary QbeOutput inserts blank separators after function
                # headers. Compare every nonempty line, including whitespace,
                # without changing instructions, labels or data declarations.
                definitions[name] = ''.join(row for row in body if row.strip())
                name, body = None, []
        elif line.startswith('data $'):
            symbol = line.split('$', 1)[1].split(' ', 1)[0]
            assert symbol not in definitions, symbol
            definitions[symbol] = line
        elif re.match(r'^(?:export )?function(?: [a-z])? \$', line):
            name = line.split('$', 1)[1].split('(', 1)[0]
            body = [line.removeprefix('export ')]
    assert name is None
    return definitions


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qbe', type=Path, default=Path(os.environ.get('TRBN_QBE', ROOT / 'bin/qbe')))
    parser.add_argument('--receipt', type=Path)
    parser.add_argument('--compiler', type=Path, help='Optional ordinary core compiler: verify extracted definitions match real emission')
    args = parser.parse_args()
    system = platform.system().lower()
    machine = {'aarch64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine(), platform.machine())
    target = system + '-' + machine
    assert target in QBE_TARGETS, target
    compiler = os.environ.get('CC', 'cc')
    receipt = {'status': 'running', 'target': target,
               'scope': 'Emitted arithmetic and fixed decimal layout; ordinary GC/REPL require separate qualification',
               'sourceHashes': {name: hashlib.sha256((RUNTIME / name).read_bytes()).hexdigest() for name in SOURCES},
               'outputControls': 0, 'productControls': 0}
    try:
        values, products = controls()
        source = emitted_runtime()
        receipt['qbeSHA256'] = hashlib.sha256(source.encode()).hexdigest()
        with tempfile.TemporaryDirectory(prefix='native-float-decimal-') as temporary:
            work = Path(temporary)
            (work / 'runtime.ssa').write_text(source)
            (work / 'probe.c').write_text(HARNESS)
            receipt['phase'] = 'compile'
            subprocess.run([str(args.qbe.resolve()), '-t', QBE_TARGETS[target], '-o', str(work / 'runtime.s'), str(work / 'runtime.ssa')], check=True, timeout=60)
            subprocess.run([compiler, '-std=c11', '-Werror', str(work / 'probe.c'), str(work / 'runtime.s'), '-o', str(work / 'probe')], check=True, timeout=60)
            for mode, inputs, expected in [
                ('products', ''.join(f'{a:016x} {b:016x}\n' for a, b in products), [f'{(a*b)>>64:016x}' for a, b in products]),
                ('floats', ''.join(f'{raw:016x}\n' for raw in values), [canonical(raw) for raw in values]),
            ]:
                receipt['phase'] = mode
                actual = subprocess.run([str(work / 'probe'), mode], input=inputs, capture_output=True, text=True, check=True, timeout=60)
                assert not actual.stderr, actual.stderr
                rows = actual.stdout.splitlines()
                assert len(rows) == len(expected), (mode, len(rows), len(expected))
                for index, (got, wanted) in enumerate(zip(rows, expected)):
                    assert got == wanted, (mode, index, got, wanted)
                receipt['productControls' if mode == 'products' else 'outputControls'] = len(rows)
            if args.compiler:
                receipt['phase'] = 'ordinary-emission'
                example = work / 'ordinary.trb'
                example.write_text('def main()\nputs(1.2345.to_s())\nend\n')
                actual = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(example)],
                                        capture_output=True, text=True, check=True, timeout=120)
                assert not actual.stderr, actual.stderr
                expected = runtime_definitions(source)
                emitted = runtime_definitions(actual.stdout)
                for name, definition in expected.items():
                    assert emitted.get(name) == definition, name
                receipt['ordinaryDefinitions'] = len(expected)
                receipt['ordinaryQbeSHA256'] = hashlib.sha256(actual.stdout.encode()).hexdigest()
        receipt['status'] = 'pass'
    except BaseException as error:
        receipt.update(status='failed', error=repr(error))
        raise
    finally:
        if args.receipt:
            args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
        print(json.dumps(receipt))


if __name__ == '__main__':
    main()
