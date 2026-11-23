#!/usr/bin/env python3
"""Probe the real raw storage runtime with guarded backing allocations.

QBE loads/stores are not sanitizer-instrumented. These checks independently
validate live bytes, owner prefixes, backing bounds and immediate page release.
"""
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

data = re.findall(r'^data \$trbn_storage_available = [^\n]+', decoded, re.M)
assert len(data) == 1
bodies = ['export ' + data[0]]
for name in ['trbn_storage_alloc', 'trbn_storage_free', 'trbn_storage_insert', 'trbn_storage_unlink']:
    matches = re.findall(r'^function (?:l )?\$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    bodies.append('export ' + matches[0].replace('call $malloc(', 'call $guard_malloc(')
                  .replace('call $free(', 'call $guard_free('))
observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern void *trbn_storage_alloc(int64_t);
extern void trbn_storage_free(void *, int64_t);
extern uintptr_t trbn_storage_available[31];
#define LIMIT 2048
#define PAGE_BYTES 16384
#define HEADER_BYTES 64
struct block { unsigned char *base; size_t size; };
struct object { unsigned char *base; size_t size; unsigned char pattern; };
static struct block blocks[LIMIT];
static struct object objects[LIMIT];
static size_t allocations, releases, last_request;
static int fail_allocation;
static int small(size_t n) { return n >= 16 && n <= 256; }
static void guard(struct block *b) {
    for (unsigned i = 0; i < 32; ++i) {
        assert(b->base[-32 + (int)i] == 0xd7);
        assert(b->base[b->size + i] == 0xd7);
    }
}
void *guard_malloc(size_t size) {
    last_request = size;
    if (fail_allocation) return NULL;
    assert(size <= PAGE_BYTES);
    unsigned char *raw = malloc(size + 64); assert(raw);
    memset(raw, 0xd7, size + 64); memset(raw + 32, 0xa5, size);
    for (unsigned i = 0; i < LIMIT; ++i) if (!blocks[i].base) {
        blocks[i] = (struct block){raw + 32, size}; ++allocations;
        return raw + 32;
    }
    abort();
}
void guard_free(void *pointer) {
    for (unsigned i = 0; i < LIMIT; ++i) if (blocks[i].base == pointer) {
        guard(&blocks[i]);
        memset(blocks[i].base, 0xdd, blocks[i].size);
        free(blocks[i].base - 32); blocks[i].base = NULL; ++releases;
        return;
    }
    abort(); /* Reject an interior, stale or duplicate system free. */
}
static void check_object(struct object *o) {
    assert((uintptr_t)o->base % 8 == 0);
    for (size_t i = 0; i < o->size; ++i) assert(o->base[i] == o->pattern);
    if (small(o->size))
        for (size_t i = o->size; i < ((o->size + 7) & ~(size_t)7); ++i)
            assert(o->base[i] == 0xe9);
}
static void put(unsigned index, size_t size) {
    assert(!objects[index].base);
    unsigned char *base = trbn_storage_alloc(size); assert(base);
    for (unsigned i = 0; i < LIMIT; ++i) if (objects[i].base) {
        uintptr_t a = (uintptr_t)objects[i].base, b = (uintptr_t)base;
        assert(a + objects[i].size <= b || b + size <= a);
    }
    unsigned char pattern = (unsigned char)(index % 251 + 1);
    objects[index] = (struct object){base, size, pattern};
    memset(base, pattern, size);
    if (small(size)) memset(base + size, 0xe9, ((size + 7) & ~(size_t)7) - size);
    check_object(&objects[index]);
}
static void drop(unsigned index) {
    struct object *o = &objects[index]; assert(o->base); check_object(o);
    trbn_storage_free(o->base, o->size); o->base = NULL;
}
static void check(void) {
    unsigned live_blocks = 0;
    for (unsigned i = 0; i < LIMIT; ++i) if (objects[i].base) check_object(&objects[i]);
    for (unsigned b = 0; b < LIMIT; ++b) if (blocks[b].base) {
        struct block *block = &blocks[b]; guard(block); ++live_blocks;
        unsigned live = 0, direct = 0;
        size_t stride = 0;
        for (unsigned i = 0; i < LIMIT; ++i) if (objects[i].base) {
            struct object *o = &objects[i];
            if (!small(o->size)) {
                if (o->base == block->base) { assert(o->size == block->size); ++direct; }
                continue;
            }
            uintptr_t owner = ((uintptr_t *)o->base)[-1];
            if (owner != (uintptr_t)block->base) continue;
            size_t expected_stride = ((o->size + 7) & ~(size_t)7) + 8;
            assert(!stride || stride == expected_stride); stride = expected_stride;
            uintptr_t cell = (uintptr_t)o->base - 8;
            assert(cell >= owner + HEADER_BYTES && cell + stride <= owner + PAGE_BYTES);
            assert((cell - owner - HEADER_BYTES) % stride == 0);
            ++live;
        }
        assert((live != 0) != (direct != 0)); /* No retained empty pages. */
        if (direct) { assert(direct == 1); continue; }
        assert(block->size == PAGE_BYTES);
        uintptr_t *page = (uintptr_t *)block->base;
        size_t capacity = (PAGE_BYTES - HEADER_BYTES) / stride;
        assert(page[4] == capacity && page[5] == live && page[6] == stride);
        assert(page[3] >= (uintptr_t)page + HEADER_BYTES);
        assert(page[3] <= (uintptr_t)page + HEADER_BYTES + capacity * stride);
        assert((page[3] - (uintptr_t)page - HEADER_BYTES) % stride == 0);
        unsigned class_index = (unsigned)(stride / 8 - 3);
        assert(class_index < 31 && page[7] == (uintptr_t)&trbn_storage_available[class_index]);
        unsigned found = 0, traversed = 0;
        uintptr_t previous = 0;
        for (uintptr_t cursor = trbn_storage_available[class_index]; cursor;) {
            assert(++traversed < LIMIT);
            uintptr_t *entry = (uintptr_t *)cursor;
            assert(entry[1] == previous && entry[5] > 0 && entry[5] < entry[4]);
            if (cursor == (uintptr_t)page) ++found;
            previous = cursor; cursor = entry[0];
        }
        assert(found == (live < capacity));
    }
    assert(allocations - releases == live_blocks);
}
static void empty(void) {
    for (unsigned i = 0; i < LIMIT; ++i) if (objects[i].base) drop(i);
    check(); assert(allocations == releases);
    for (unsigned i = 0; i < 31; ++i) assert(!trbn_storage_available[i]);
}
int main(void) {
    /* Every byte-size boundary, three full pages plus a partial page, then
       full-to-available reuse and head/interior/last page removal. */
    for (size_t size = 16; size <= 256; ++size) {
        unsigned capacity = (unsigned)((PAGE_BYTES - HEADER_BYTES) / (((size + 7) & ~(size_t)7) + 8));
        unsigned count = capacity * 3 + 1;
        assert(count <= LIMIT);
        size_t before = allocations;
        for (unsigned i = 0; i < count; ++i) put(i, size);
        check(); assert(allocations == before + 4);
        for (unsigned i = 0; i < count; i += 2) drop(i);
        check();
        before = allocations;
        for (unsigned i = 0; i < count; i += 2) put(i, size);
        check(); assert(allocations == before + ((count - 1) % 2 == 0)); /* Empty pages are freed. */
        for (unsigned i = capacity; i < capacity * 2; ++i) drop(i);
        check();
        for (unsigned i = 0; i < capacity; ++i) drop(i);
        check(); empty();
    }
    /* Sparse retained pages, mixed classes and large direct fallbacks. */
    for (unsigned wave = 0; wave < 8; ++wave) {
        for (unsigned i = 0; i < 1200; ++i) if (!objects[i].base)
            put(i, i % 11 == 0 ? 257 + i % 4097 : 16 + i % 241);
        check();
        for (unsigned i = 0; i < 1200; ++i) if (objects[i].base && (i + wave) % 17) drop(i);
        check();
    }
    empty();
    /* Small invalid sizes retain the direct allocation contract. Large signed
       and wrapping inputs must reach malloc unchanged, never a class index. */
    for (size_t size = 0; size < 16; ++size) { put(0, size); check(); empty(); }
    int64_t invalid[] = {-1, INT64_MIN, INT64_MAX, 257, 8192};
    fail_allocation = 1;
    for (unsigned i = 0; i < sizeof invalid / sizeof *invalid; ++i) {
        assert(!trbn_storage_alloc(invalid[i])); assert(last_request == (size_t)invalid[i]);
    }
    for (int64_t size = 16; size <= 256; ++size) {
        assert(!trbn_storage_alloc(size)); assert(last_request == PAGE_BYTES);
        check();
    }
    fail_allocation = 0;
    for (unsigned i = 0; i < (PAGE_BYTES - HEADER_BYTES) / 24; ++i) put(i, 16);
    check(); fail_allocation = 1;
    assert(!trbn_storage_alloc(16)); check();
    drop(73); put(73, 16); check(); /* Reuse needs no system allocation. */
    fail_allocation = 0; empty();
    puts("Managed storage boundaries, guarded reuse, sparse pages and failures passed");
}
'''
with tempfile.TemporaryDirectory(prefix='native-managed-storage-') as temporary:
    root = Path(temporary)
    (root / 'runtime.ssa').write_text('\n'.join(bodies))
    (root / 'observer.c').write_text(observer)
    for command in [[str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                    ['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'), '-o', str(root / 'probe')],
                    [str(root / 'probe')]]:
        result = subprocess.run(command, capture_output=True, timeout=30)
        assert result.returncode == 0 and result.stderr == b'', result
print('Managed storage guarded allocation and lifecycle checks passed')
