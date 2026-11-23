#!/usr/bin/env python3
"""Check mixed-object sweep accounting, survivor links and backing-store release."""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--compiler', type=Path, required=True)
args = parser.parse_args()
# Test actual emitted code so named bounds and emitter helpers cannot diverge
# from the production runtime through a test-only string-literal parser.
with tempfile.TemporaryDirectory(prefix='native-storage-emission-') as temporary:
    source = Path(temporary) / 'main.trb'
    source.write_text('def main()\nputs("storage")\nend\n')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    decoded = emitted.stdout.decode()

bodies = []
for name in ['trbn_storage_alloc', 'trbn_storage_free', 'trbn_storage_insert',
             'trbn_storage_unlink', 'trbn_array_backing_new', 'trbn_array_backing_release', 'trbn_array_backing_flush',
             'trbn_gc_sweep', 'trbn_gc_mark', 'trbn_gc_enqueue', 'trbn_gc_mark_grow', 'trbn_gc_scan_fixed', 'trbn_gc_scan_array']:
    matches = re.findall(r'^function (?:l )?\$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    body = matches[0]
    if name == 'trbn_gc_sweep':
        body = body.replace('call $trbn_array_backing_release(', 'call $observe_backing_release(')
        body = body.replace('call $trbn_storage_free(', 'call $observe_object_free(')
    elif not name.startswith('trbn_array_backing_') and name != 'trbn_gc_mark_grow' and name != 'trbn_gc_mark':
        body = body.replace('call $malloc(', 'call $observe_malloc(')
        body = body.replace('call $free(', 'call $observe_storage_free(')
    bodies.append('export ' + body)
storage = re.findall(r'^data \$trbn_storage_available = [^\n]+', decoded, re.M)
assert len(storage) == 1
bodies.append(storage[0])
cache = re.findall(r'^data \$trbn_array_backing_cache = [^\n]+', decoded, re.M)
assert len(cache) == 1
bodies.append('export ' + cache[0])
allocation_error = re.findall(r'^data \$trbn_allocation_error = [^\n]+', decoded, re.M)
assert len(allocation_error) == 1
bodies.append(allocation_error[0])
observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern void *trbn_storage_alloc(int64_t);
extern void trbn_storage_free(void *, int64_t);
extern void trbn_gc_sweep(void);
extern void trbn_gc_mark(void *);
extern void *trbn_array_backing_new(void);
extern void trbn_array_backing_release(void *, int64_t);
extern void trbn_array_backing_flush(void);
extern uintptr_t trbn_array_backing_cache[2];
void trbn_fatal(const void *message, int64_t bytes) { (void)message; (void)bytes; abort(); }
void *trbn_gc_heap;
int64_t trbn_gc_heap_bytes, trbn_gc_reclaimed_bytes;
static uintptr_t string_descriptor[] = {0};
static uintptr_t fixed_descriptor[] = {1, 2, 24, 8, 16};
static uintptr_t scalar_array_descriptor[] = {2, 0};
static uintptr_t managed_array_descriptor[] = {2, 1};
static uintptr_t literal[] = {0, 0, 0, 0};
struct allocation {
    uintptr_t *base, *descriptor, *backing;
    size_t bytes, backing_bytes;
    unsigned char payload[128];
};
static struct allocation objects[8];
static void *release_order[16];
static size_t expected_releases, released, storage_blocks;
void *observe_malloc(size_t size) {
    void *p = malloc(size); assert(p); ++storage_blocks; return p;
}
void observe_storage_free(void *p) { assert(storage_blocks); --storage_blocks; free(p); }
void observe_object_free(void *pointer, int64_t size) {
    assert(released < expected_releases && pointer == release_order[released++]);
    trbn_storage_free(pointer, size);
}
static void *zero_object(size_t size) {
    void *p = trbn_storage_alloc(size); assert(p); memset(p, 0, size); return p;
}
void observe_backing_release(void *pointer, int64_t capacity) {
    assert(released < expected_releases);
    assert(pointer == release_order[released++]);
    trbn_array_backing_release(pointer, capacity);
}
static void snapshot(struct allocation *a) {
    assert(a->bytes - 16 <= sizeof a->payload);
    memcpy(a->payload, a->base + 2, a->bytes - 16);
}
static size_t prepare(int reverse, int references) {
    size_t total = 0;
    trbn_gc_heap = NULL;
    for (unsigned i = 0; i < 8; ++i) {
        struct allocation *a = &objects[i];
        memset(a, 0, sizeof *a);
        if (i < 3) {
            size_t length = i * 23;
            a->descriptor = string_descriptor; a->bytes = 33 + length;
            a->base = zero_object(a->bytes);
            a->base[2] = length; a->base[3] = length;
            memset(a->base + 4, 'a' + i, length);
        } else if (i < 5) {
            a->descriptor = fixed_descriptor; a->bytes = 40;
            a->base = zero_object(a->bytes);
            a->base[4] = 0x1234;
        } else {
            size_t capacity = i == 5 ? 0 : 4;
            a->descriptor = i == 7 ? managed_array_descriptor : scalar_array_descriptor;
            a->bytes = 40; a->backing_bytes = capacity * sizeof(uintptr_t);
            a->base = zero_object(a->bytes);
            if (capacity) {
                a->backing = capacity == 4 ? trbn_array_backing_new() : calloc(capacity, sizeof(uintptr_t));
                assert(a->backing);
                for (size_t j = 0; j < capacity; ++j) assert(a->backing[j] == 0);
            }
            a->base[2] = capacity; a->base[3] = capacity;
            a->base[4] = (uintptr_t)a->backing;
        }
        a->base[1] = (uintptr_t)a->descriptor;
        total += a->bytes + a->backing_bytes;
    }
    if (references) {
        /* A fixed record and managed Array retain a String, a static literal,
           and each other through a cycle. Scalar elements must not be traced. */
        objects[3].base[2] = (uintptr_t)(objects[0].base + 1);
        objects[3].base[3] = (uintptr_t)(objects[7].base + 1);
        objects[7].backing[0] = (uintptr_t)(objects[3].base + 1);
        objects[7].backing[1] = (uintptr_t)literal;
        objects[6].backing[0] = (uintptr_t)(objects[1].base + 1);
    }
    for (unsigned i = 0; i < 8; ++i) {
        unsigned index = reverse ? 7 - i : i;
        objects[index].base[0] = (uintptr_t)trbn_gc_heap;
        trbn_gc_heap = objects[index].base;
        snapshot(&objects[index]);
    }
    trbn_gc_heap_bytes = (int64_t)total;
    trbn_gc_reclaimed_bytes = 8192; /* Preserve earlier collections' accounting. */
    return total;
}
static size_t sweep(unsigned present, unsigned keep, int reverse) {
    expected_releases = released = 0;
    size_t reclaimed = 0;
    for (unsigned i = 0; i < 8; ++i) {
        unsigned index = reverse ? i : 7 - i;
        struct allocation *a = &objects[index];
        if (!(present & (1u << index))) continue;
        if (!(keep & (1u << index))) {
            if (a->backing) release_order[expected_releases++] = a->backing;
            release_order[expected_releases++] = a->base;
            reclaimed += a->bytes + a->backing_bytes;
        }
    }
    int64_t before_heap = trbn_gc_heap_bytes, before_reclaimed = trbn_gc_reclaimed_bytes;
    trbn_gc_sweep();
    assert(released == expected_releases);
    assert(trbn_gc_heap_bytes == before_heap - (int64_t)reclaimed);
    assert(trbn_gc_reclaimed_bytes == before_reclaimed + (int64_t)reclaimed);
    uintptr_t *cursor = trbn_gc_heap;
    for (unsigned i = 0; i < 8; ++i) {
        unsigned index = reverse ? i : 7 - i;
        struct allocation *a = &objects[index];
        if (!(keep & (1u << index))) continue;
        assert(cursor == a->base);
        assert(cursor[1] == (uintptr_t)a->descriptor); /* Mark cleared. */
        assert(!memcmp(cursor + 2, a->payload, a->bytes - 16));
        cursor = (uintptr_t *)cursor[0];
    }
    assert(!cursor);
    return reclaimed;
}
int main(void) {
    trbn_gc_reclaimed_bytes = 8192;
    sweep(0, 0, 0);
    for (int reverse = 0; reverse < 2; ++reverse) {
        for (unsigned keep = 0; keep < 256; ++keep) {
            size_t total = prepare(reverse, 0);
            trbn_gc_mark(NULL); trbn_gc_mark(literal);
            assert(literal[0] == 0);
            for (unsigned i = 0; i < 8; ++i)
                if (keep & (1u << i)) trbn_gc_mark(objects[i].base + 1);
            sweep(255, keep, reverse);
            for (unsigned i = 0; i < 8; ++i)
                if (keep & (1u << i)) trbn_gc_mark(objects[i].base + 1);
            assert(sweep(keep, keep, reverse) == 0);
            sweep(keep, 0, reverse);
            assert(!trbn_gc_heap && trbn_gc_heap_bytes == 0 && storage_blocks == 0);
            assert(trbn_gc_reclaimed_bytes == 8192 + (int64_t)total);
            sweep(0, 0, reverse);
        }
        size_t total = prepare(reverse, 1);
        trbn_gc_mark(objects[3].base + 1);
        trbn_gc_mark(objects[6].base + 1);
        unsigned keep = (1u << 0) | (1u << 3) | (1u << 6) | (1u << 7);
        sweep(255, keep, reverse);
        sweep(keep, 0, reverse);
        assert(trbn_gc_heap_bytes == 0 && trbn_gc_reclaimed_bytes == 8192 + (int64_t)total);
        assert(literal[0] == 0 && storage_blocks == 0);
    }
    assert(trbn_array_backing_cache[1] == 2);
    trbn_array_backing_flush();
    assert(!trbn_array_backing_cache[0] && !trbn_array_backing_cache[1]);
    puts("Mixed-object sweep, backing reuse, cyclic roots and accounting passed");
}
'''
with tempfile.TemporaryDirectory(prefix='native-gc-sweep-') as temporary:
    root = Path(temporary)
    (root / 'runtime.ssa').write_text('\n'.join(bodies))
    (root / 'observer.c').write_text(observer)
    subprocess.run([str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                   check=True, capture_output=True, timeout=30)
    subprocess.run(['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'),
                    '-o', str(root / 'probe')], check=True, capture_output=True, timeout=30)
    result = subprocess.run([str(root / 'probe')], capture_output=True, timeout=30)
    assert result.returncode == 0 and result.stderr == b'', result
print('GC sweep accounting and retained mixed-object checks passed')
