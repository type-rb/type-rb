#!/usr/bin/env python3
"""Check bounded libc backing reuse, zeroing, growth and allocation failures."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
args = parser.parse_args()
source = Path(__file__).resolve().parent.parent / 'compiler/src/backend/qbe/runtime/managed_storage.trb'
decoded = '\n'.join(json.loads(s) for s in re.findall(r'"(?:[^"\\]|\\.)*"', source.read_text()))
data = re.findall(r'^data \$trbn_array_backing_cache = [^\n]+', decoded, re.M)
assert len(data) == 1
bodies = ['export ' + data[0]]
for name in ['trbn_array_backing_new', 'trbn_array_backing_release', 'trbn_array_backing_flush']:
    matches = re.findall(r'^function (?:l )?\$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    bodies.append('export ' + matches[0].replace('call $calloc(', 'call $observe_calloc(')
                  .replace('call $free(', 'call $observe_free('))

observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern void *trbn_array_backing_new(void);
extern void trbn_array_backing_release(void *, int64_t);
extern void trbn_array_backing_flush(void);
extern uintptr_t trbn_array_backing_cache[2];
enum { CAP = 8192, EXTRA = 37, TOTAL = CAP + EXTRA };
static void *blocks[TOTAL];
static size_t allocated, released;
static int fail;
void *observe_calloc(size_t count, size_t width) {
    assert(count == 4 && width == 8);
    if (fail) return NULL;
    void *p = calloc(count, width); assert(p); ++allocated; return p;
}
void observe_free(void *p) { assert(p); ++released; free(p); }
static void zero(void *p) {
    assert(p && (uintptr_t)p % 8 == 0);
    for (unsigned i = 0; i < 32; ++i) assert(((unsigned char *)p)[i] == 0);
}
static void empty(void) {
    size_t count = trbn_array_backing_cache[1];
    assert(count <= CAP);
    for (size_t i = 0; i < count; ++i) {
        void *p = trbn_array_backing_new(); zero(p); observe_free(p);
    }
    assert(!trbn_array_backing_cache[0] && !trbn_array_backing_cache[1]);
    assert(allocated == released);
}
int main(void) {
    fail = 1;
    assert(!trbn_array_backing_new());
    assert(!trbn_array_backing_cache[0] && !trbn_array_backing_cache[1]);
    fail = 0;
    for (unsigned i = 0; i < TOTAL; ++i) {
        blocks[i] = trbn_array_backing_new(); zero(blocks[i]);
        for (unsigned j = 0; j < i; ++j) assert(blocks[j] != blocks[i]);
        memset(blocks[i], 0xa5, 32);
    }
    for (unsigned i = 0; i < TOTAL; ++i) {
        trbn_array_backing_release(blocks[i], 4);
        assert(trbn_array_backing_cache[1] == (i < CAP ? i + 1 : CAP));
    }
    assert(released == EXTRA && allocated == TOTAL);
    size_t before = allocated;
    fail = 1; /* Cached reuse cannot require a successful system allocation. */
    for (unsigned i = 0; i < CAP; ++i) {
        void *p = trbn_array_backing_new(); zero(p);
        assert(p == blocks[CAP - i - 1]);
        memset(p, 0x37, 32);
        blocks[CAP - i - 1] = NULL;
        observe_free(p);
    }
    assert(before == allocated && allocated == released);
    assert(!trbn_array_backing_cache[0] && !trbn_array_backing_cache[1]);
    assert(!trbn_array_backing_new());
    fail = 0;
    /* Reused buffers retain libc provenance and preserve elements on growth. */
    uint64_t *p = trbn_array_backing_new(); zero(p);
    trbn_array_backing_release(p, 4);
    uint64_t *reused = trbn_array_backing_new(); assert(reused == p); zero(reused);
    for (unsigned i = 0; i < 4; ++i) reused[i] = UINT64_C(0x1234567800000000) + i;
    uint64_t *grown = realloc(reused, 128); assert(grown);
    for (unsigned i = 0; i < 4; ++i) assert(grown[i] == UINT64_C(0x1234567800000000) + i);
    trbn_array_backing_release(grown, 16);
    assert(!trbn_array_backing_cache[0] && allocated == released);
    /* All uncached capacities take the normal release path. */
    int64_t capacities[] = {0, 1, 2, 3, 5, 8, 16, 256};
    for (unsigned i = 0; i < sizeof capacities / sizeof *capacities; ++i) {
        void *data = calloc((size_t)capacities[i] + 1, 8); assert(data); ++allocated;
        trbn_array_backing_release(data, capacities[i]);
        assert(!trbn_array_backing_cache[1] && allocated == released);
    }
    /* Alternating live buffers and cached buffers must remain disjoint. */
    for (unsigned i = 0; i < 100; ++i) { blocks[i] = trbn_array_backing_new(); zero(blocks[i]); }
    for (unsigned i = 0; i < 100; i += 2) trbn_array_backing_release(blocks[i], 4);
    for (unsigned i = 0; i < 100; i += 2) {
        blocks[i] = trbn_array_backing_new(); zero(blocks[i]);
        for (unsigned j = 1; j < 100; j += 2) assert(blocks[i] != blocks[j]);
    }
    for (unsigned i = 0; i < 100; ++i) trbn_array_backing_release(blocks[i], 4);
    empty();
    for (unsigned i = 0; i < TOTAL; ++i) blocks[i] = trbn_array_backing_new();
    for (unsigned i = 0; i < TOTAL; ++i) trbn_array_backing_release(blocks[i], 4);
    assert(trbn_array_backing_cache[1] == CAP);
    trbn_array_backing_flush();
    assert(!trbn_array_backing_cache[0] && !trbn_array_backing_cache[1]);
    assert(allocated == released);
    trbn_array_backing_flush(); /* Empty final reclamation is also safe. */
    assert(allocated == released);
    puts("Bounded Array backing reuse, zeroing, realloc and failures passed");
}
'''
with tempfile.TemporaryDirectory(prefix='native-array-backing-') as temporary:
    root = Path(temporary)
    (root / 'runtime.ssa').write_text('\n'.join(bodies))
    (root / 'observer.c').write_text(observer)
    for command in [[str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                    ['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'), '-o', str(root / 'probe')],
                    [str(root / 'probe')]]:
        result = subprocess.run(command, capture_output=True, timeout=30)
        assert result.returncode == 0 and result.stderr == b'', result
print('Array backing allocation and bounded cache checks passed')
