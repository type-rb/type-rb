#!/usr/bin/env python3
"""Check Bytes word reads/writes against protected pages and an independent oracle."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
args = parser.parse_args()
repo = Path(__file__).resolve().parent.parent
source = repo / 'compiler/src/backend/qbe/emit/bytes.trb'
decoded = '\n'.join(json.loads(s) for s in re.findall(r'"(?:[^"\\]|\\.)*"', source.read_text()))
names = ['trbn_bytes_utf8_span', 'trbn_bytes_valid_utf8', 'trbn_bytes_to_string']
bodies = []
for name in names:
    matches = re.findall(r'^function l \$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    bodies.append('export ' + matches[0])
il = '\n'.join(bodies) + '\n'

# Only the observer is C. The tested functions are the TypeRB-owned runtime.
# Header fields use memcpy so deliberately unaligned input is valid C as well.
observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
extern int64_t trbn_bytes_valid_utf8(void *);
extern void *trbn_bytes_to_string(void *);
int64_t trbn_gc_temp_count;
const char trbn_allocation_error[26] = "allocation failure";
static unsigned char *target_region;
static size_t page;
void trbn_gc_temp_push(void *value) { assert(value); ++trbn_gc_temp_count; }
void trbn_fatal(const void *message, int64_t size) { (void)message; (void)size; abort(); }
static unsigned char *region(void) {
    unsigned char *p = mmap(NULL, page * 2, PROT_READ | PROT_WRITE,
                           MAP_PRIVATE | MAP_ANON, -1, 0);
    assert(p != MAP_FAILED && mprotect(p + page, page, PROT_NONE) == 0);
    return p;
}
void *trbn_string_alloc(int64_t size) {
    assert(!target_region && size >= 17 && (uint64_t)size + 8 < page);
    target_region = region();
    return target_region + page - size - 8;
}
int main(void) {
    page = (size_t)sysconf(_SC_PAGESIZE);
    uint32_t length;
    while (fread(&length, sizeof length, 1, stdin) == 1) {
        assert(length + 24 < page);
        unsigned char *input_region = region();
        unsigned char *input = input_region + page - length - 24;
        int64_t bytes = length;
        memcpy(input + 8, &bytes, 8);
        assert(fread(input + 24, 1, length, stdin) == length);
        int64_t valid = trbn_bytes_valid_utf8(input);
        unsigned char *output = trbn_bytes_to_string(input);
        int64_t written, points;
        memcpy(&written, output + 8, 8); memcpy(&points, output + 16, 8);
        assert(written >= 0 && written < (int64_t)page && !output[24 + written]);
        assert(trbn_gc_temp_count == 0);
        printf("%lld %lld %lld ", (long long)valid, (long long)written, (long long)points);
        for (int64_t i = 0; i < written; ++i) printf("%02x", output[24 + i]);
        puts("");
        assert(munmap(input_region, page * 2) == 0);
        assert(munmap(target_region, page * 2) == 0); target_region = NULL;
    }
    return 0;
}
'''

def request(data):
    import struct
    return struct.pack('=I', len(data)) + data

cases = [b'a' * n for n in range(80)] + [bytes(range(256))]
payloads = [b'\xc2\xa2', b'\xe6\x97\xa5', b'\xf0\x9f\x98\x80', b'\xff',
            b'\xe3\x81', b'\xed\xa0\x80', b'\xf0\x90\x80', b'\x00']
for prefix in range(9):
    for tail in (0, 1, 7, 8, 9, 31, 64):
        cases += [b'a' * prefix + payload + b'b' * tail + payload for payload in payloads]
expected = []
for data in cases:
    text = data.decode('utf-8', errors='replace')
    try:
        data.decode('utf-8', errors='strict')
        valid = 1
    except UnicodeDecodeError:
        valid = 0
    expected.append(f'{valid} {len(text.encode())} {len(text)} {text.encode().hex()}\n')

with tempfile.TemporaryDirectory(prefix='native-bytes-boundary-') as temporary:
    root = Path(temporary)
    (root / 'observer.c').write_text(observer)

    def build(name, code):
        ssa, assembly, binary = (root / (name + suffix) for suffix in ('.ssa', '.s', ''))
        ssa.write_text(code)
        subprocess.run([args.qbe.resolve(), '-o', assembly, ssa], check=True, capture_output=True)
        subprocess.run(['/usr/bin/cc', '-std=c11', '-D_DEFAULT_SOURCE', assembly,
                        root / 'observer.c', '-o', binary], check=True, capture_output=True)
        return binary

    binary = build('bounded', il)
    result = subprocess.run([binary], input=b''.join(map(request, cases)), capture_output=True, timeout=60)
    assert result.returncode == 0 and result.stdout == ''.join(expected).encode(), result
    # Each wide-load guard must be necessary, not just present in the source.
    guards = list(re.finditer(r'%(?:measure_)?wide =w csgel %(?:measure_)?remaining, 8', il))
    assert len(guards) == 3
    for i, guard in enumerate(guards):
        changed = il[:guard.end() - 1] + '0' + il[guard.end():]
        broken = build('unbounded-' + str(i), changed)
        failed = subprocess.run([broken], input=request(b'a'), capture_output=True, timeout=60)
        assert failed.returncode != 0, 'Protected-page oracle did not detect the removed bound'
print(f'Bytes guarded reads/writes and independent UTF-8 oracle passed: {len(cases)} cases')
