#!/usr/bin/env python3
"""Exercise the actual String puts QBE at the POSIX scatter/gather boundary."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--output', type=Path)
parser.add_argument('--source', type=Path)
args = parser.parse_args()
repo = Path(__file__).resolve().parent.parent
source = args.source or repo / 'compiler/src/backend/qbe/runtime/system.trb'
functions = []
for literal in re.findall(r'"(?:[^"\\]|\\.)*"', source.read_text()):
    decoded = json.loads(literal)
    functions.extend(re.findall(r'^function \$trbn_puts\(l %string\) \{.*?^\}', decoded, re.M | re.S))
assert len(functions) == 1, 'expected one canonical String puts body'
body = functions[0]
il = 'data $trbn_newline = { b 10 }\nexport ' + body.replace('call $writev(', 'call $test_writev(') + '\n'
# This C code observes the platform ABI and injects syscall results. Production
# output logic remains the extracted TypeRB-owned QBE, not a model of its loop.
helper = r'''
#include <assert.h>
#include <errno.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/uio.h>
#include <unistd.h>
struct native_string { uint64_t descriptor; int64_t length; int64_t points; unsigned char bytes[]; };
_Static_assert(sizeof(void *) == 8 && sizeof(size_t) == 8, "LP64 boundary");
_Static_assert(sizeof(struct iovec) == 16 && offsetof(struct iovec, iov_len) == 8, "iovec layout");
_Static_assert(offsetof(struct native_string, bytes) == 24, "String payload layout");
extern void trbn_puts(struct native_string *);
static const char *mode;
static size_t limit;
static int calls;
ssize_t test_writev(int fd, const struct iovec *vectors, int count) {
    assert(fd == 1 && count >= 1 && count <= 2 && ++calls < 100000);
    if (!strcmp(mode, "zero") || (!strcmp(mode, "prefix-zero") && calls > 1)) return 0;
    if (!strcmp(mode, "error") || (!strcmp(mode, "prefix-error") && calls > 1)) { errno = EIO; return -1; }
    struct iovec limited[2];
    size_t remaining = limit;
    for (int i = 0; i < count; ++i) {
        limited[i] = vectors[i];
        if (limited[i].iov_len > remaining) limited[i].iov_len = remaining;
        remaining -= limited[i].iov_len;
    }
    return writev(fd, limited, count);
}
int main(int argc, char **argv) {
    assert(argc == 4);
    mode = argv[1]; limit = (size_t)strtoull(argv[2], NULL, 10);
    FILE *input = fopen(argv[3], "rb"); assert(input);
    assert(fseek(input, 0, SEEK_END) == 0);
    long length = ftell(input); assert(length >= 0);
    rewind(input);
    struct native_string *value = malloc(25 + (size_t)length); assert(value);
    value->descriptor = 0; value->length = length; value->points = 0;
    assert(fread(value->bytes, 1, (size_t)length, input) == (size_t)length);
    value->bytes[length] = 0; fclose(input);
    trbn_puts(value);
    fprintf(stderr, "{\"calls\":%d}\n", calls);
    free(value);
    return 0;
}
'''
observations = []
with tempfile.TemporaryDirectory(prefix='native puts boundary ') as directory:
    root = Path(directory)
    (root / 'puts.ssa').write_text(il)
    (root / 'observer.c').write_text(helper)
    subprocess.run([str(args.qbe.resolve()), '-o', str(root / 'puts.s'), str(root / 'puts.ssa')], check=True, capture_output=True, timeout=30)
    subprocess.run(['/usr/bin/cc', '-O2', str(root / 'puts.s'), str(root / 'observer.c'), '-o', str(root / 'probe')], check=True, capture_output=True, timeout=30)
    for name, payload in [('empty', b''), ('text', b'abcdef'), ('utf8', 'こんにちは😀'.encode()), ('newlines', b'a\nb\n'), ('nul', b'a\0b'), ('large', b'x' * 16384)]:
        (root / 'input').write_bytes(payload)
        complete = payload + b'\n'
        for mode, limit in [('normal', 32768), ('partial-one', 1), ('partial-seven', 7), ('boundary', max(1, len(payload))), ('zero', 3), ('error', 3), ('prefix-zero', 3), ('prefix-error', 3)]:
            result = subprocess.run([str(root / 'probe'), mode, str(limit), str(root / 'input')], capture_output=True, timeout=10)
            assert result.returncode == 0, (name, mode, result)
            calls = json.loads(result.stderr)['calls']
            expected = complete
            expected_calls = math.ceil(len(complete) / limit)
            if mode in ('zero', 'error'):
                expected, expected_calls = b'', 1
            elif mode in ('prefix-zero', 'prefix-error'):
                expected = complete[:limit]
                expected_calls = 1 if len(complete) <= limit else 2
            assert result.stdout == expected and calls == expected_calls, (name, mode, calls, expected_calls, result.stdout)
            observations.append({'case': name, 'mode': mode, 'bytes': len(expected), 'calls': calls})
report = {'sourceSha256': hashlib.sha256(source.read_bytes()).hexdigest(), 'putsSha256': hashlib.sha256(body.encode()).hexdigest(), 'observations': observations}
if args.output:
    args.output.write_text(json.dumps(report, indent=2) + '\n')
print(f'String puts: {len(observations)} exact-byte, call-count and short-write observations passed')
