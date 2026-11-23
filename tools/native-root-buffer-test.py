#!/usr/bin/env python3
"""Exercise emitted roots with moving storage, allocation failure and collection."""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--compiler', type=Path, required=True)
parser.add_argument('--qbe', type=Path, required=True)
args = parser.parse_args()

observer = r'''
#include <assert.h>
#include <setjmp.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern void trbn_gc_temp_push(void *);
extern void trbn_gc_temp_reserve(int64_t);
extern void *trbn_gc_alloc(void *, int64_t, int64_t);
extern void *trbn_string_from_bytes(const void *, int64_t);
extern void trbn_gc_collect(int);
void **trbn_gc_temp_data;
int64_t trbn_gc_temp_count, trbn_gc_temp_capacity;
void *trbn_gc_heap, *trbn_gc_global_roots[12];
int64_t trbn_gc_heap_bytes, trbn_gc_allocated_bytes, trbn_gc_reclaimed_bytes;
int64_t trbn_gc_peak_heap_bytes, trbn_gc_heap_target;
int64_t trbn_gc_collection_count, trbn_gc_automatic_collection_count;
uintptr_t trbn_desc_string[] = {0};
static uintptr_t record_descriptor[] = {1, 0, 8};
static uintptr_t values[300][2], literal[] = {0, 0, 0};
static void *expected[600];
static unsigned char *active, *retired[32];
static size_t active_bytes, retired_bytes[32], retired_count;
static int realloc_calls, fail_realloc, expected_failure, failures, collect_before;
static int check_fresh_failure;
static jmp_buf failure;
static const uint64_t guard = UINT64_C(0x738ed15a409b26cf);

static void check_block(unsigned char *block, size_t bytes) {
    uint64_t *head = (uint64_t *)block, *tail = (uint64_t *)(block + 16 + bytes);
    assert(head[0] == guard && head[1] == guard);
    assert(tail[0] == guard && tail[1] == guard);
}
static void check_buffers(void) {
    if (active) check_block(active, active_bytes);
    for (size_t i = 0; i < retired_count; ++i) {
        check_block(retired[i], retired_bytes[i]);
        for (size_t j = 0; j < retired_bytes[i]; ++j)
            assert(retired[i][16 + j] == 0xd7);
    }
}
static void roots(int64_t count) {
    assert(trbn_gc_temp_count == count && count <= trbn_gc_temp_capacity);
    assert(trbn_gc_temp_data == (active ? (void **)(active + 16) : NULL));
    for (int64_t i = 0; i < count; ++i) assert(trbn_gc_temp_data[i] == expected[i]);
    check_buffers();
}
void *observe_realloc(void *old, size_t bytes) {
    ++realloc_calls;
    assert(old == (active ? active + 16 : NULL));
    assert(bytes >= 512 && bytes > active_bytes);
    check_buffers();
    if (check_fresh_failure) {
        assert(trbn_gc_heap && trbn_gc_heap_bytes == 24);
        assert(trbn_gc_allocated_bytes == 24 && trbn_gc_peak_heap_bytes == 24);
        uintptr_t *base = trbn_gc_heap;
        assert(base[0] == 0 && base[1] == (uintptr_t)record_descriptor && base[2] == 0);
        assert(trbn_gc_temp_count == 64 && trbn_gc_temp_capacity == 64);
    }
    if (fail_realloc) return NULL;
    unsigned char *next = malloc(bytes + 32); assert(next);
    memset(next + 16, 0xa5, bytes);
    uint64_t *head = (uint64_t *)next, *tail = (uint64_t *)(next + 16 + bytes);
    head[0] = head[1] = tail[0] = tail[1] = guard;
    if (active) {
        memcpy(next + 16, active + 16, active_bytes);
        memset(active + 16, 0xd7, active_bytes);
        assert(retired_count < 32);
        retired[retired_count] = active;
        retired_bytes[retired_count++] = active_bytes;
    }
    active = next; active_bytes = bytes;
    return next + 16;
}
void trbn_fatal(const void *message, int64_t bytes) {
    const char text[] = "panic: allocation failed\n";
    assert(expected_failure && bytes == 26 && !memcmp(message, text, sizeof text - 1));
    ++failures;
    longjmp(failure, 1);
}
void trbn_gc_mark_constants(void) { /* The synthetic C fixture owns no constants. */ }
void trbn_gc_trace(int automatic, int64_t collection) { (void)automatic; (void)collection; }
void trbn_gc_maybe_collect(int64_t projected) {
    assert(projected >= 24);
    if (collect_before) trbn_gc_collect(0);
}
static void reset(void) {
    assert(!trbn_gc_heap && trbn_gc_heap_bytes == 0);
    check_buffers();
    free(active);
    for (size_t i = 0; i < retired_count; ++i) free(retired[i]);
    active = NULL; active_bytes = retired_count = 0;
    trbn_gc_temp_data = NULL; trbn_gc_temp_count = trbn_gc_temp_capacity = 0;
    trbn_gc_allocated_bytes = trbn_gc_reclaimed_bytes = trbn_gc_peak_heap_bytes = 0;
    realloc_calls = fail_realloc = expected_failure = check_fresh_failure = collect_before = 0;
}
static void fill(int64_t count) {
    for (int64_t i = 0; i < count; ++i) {
        expected[i] = values[i];
        trbn_gc_temp_push(values[i]);
    }
    roots(count);
}
int main(void) {
    for (int i = 0; i < 300; ++i) {
        values[i][0] = (uintptr_t)record_descriptor;
        values[i][1] = (uintptr_t)i;
    }
    /* Nil/static guards precede capacity and allocation, even with no buffer. */
    fail_realloc = 1;
    trbn_gc_temp_push(NULL); trbn_gc_temp_push(literal); roots(0);
    trbn_gc_temp_reserve(0); assert(realloc_calls == 0);
    fail_realloc = 0;
    for (int i = 0; i < 260; ++i) {
        expected[i] = values[i % 17]; /* Repeated pointers must not be deduplicated. */
        trbn_gc_temp_push(expected[i]); roots(i + 1);
        int64_t capacity = i < 64 ? 64 : i < 128 ? 128 : i < 256 ? 256 : 512;
        assert(trbn_gc_temp_capacity == capacity);
    }
    assert(realloc_calls == 4);
    fail_realloc = 1;
    trbn_gc_temp_push(NULL); trbn_gc_temp_push(literal); roots(260);
    expected[260] = values[0]; trbn_gc_temp_push(values[0]); roots(261);
    trbn_gc_temp_reserve(512); roots(261); assert(realloc_calls == 4);
    fail_realloc = 0;
    trbn_gc_temp_reserve(513); roots(261);
    assert(trbn_gc_temp_capacity == 1024 && realloc_calls == 5);
    reset();

    trbn_gc_temp_reserve(129); roots(0);
    assert(trbn_gc_temp_capacity == 256 && realloc_calls == 1);
    fill(128); trbn_gc_temp_reserve(257); roots(128);
    assert(trbn_gc_temp_capacity == 512 && realloc_calls == 2);
    reset();

    /* Full-buffer push and reserve must leave their old buffer/count intact. */
    fill(64); fail_realloc = expected_failure = 1;
    if (!setjmp(failure)) { trbn_gc_temp_push(values[64]); assert(0); }
    roots(64); assert(trbn_gc_temp_capacity == 64 && failures == 1);
    if (!setjmp(failure)) { trbn_gc_temp_reserve(65); assert(0); }
    roots(64); assert(trbn_gc_temp_capacity == 64 && failures == 2);
    reset();

    /* Fresh allocation fails at root growth only after initialization/accounting. */
    fill(64); fail_realloc = expected_failure = check_fresh_failure = 1;
    if (!setjmp(failure)) { trbn_gc_alloc(record_descriptor, 8, 0); assert(0); }
    roots(64); assert(failures == 3 && trbn_gc_temp_capacity == 64);
    assert(trbn_gc_heap_bytes == 24 && trbn_gc_allocated_bytes == 24);
    trbn_gc_temp_count = 0; trbn_gc_collect(0);
    assert(!trbn_gc_heap && trbn_gc_heap_bytes == 0 && trbn_gc_reclaimed_bytes == 24);
    reset();

    /* Spare-capacity fresh publication cannot accidentally call realloc. */
    fill(63); fail_realloc = 1;
    expected[63] = trbn_gc_alloc(record_descriptor, 8, 0);
    roots(64); assert(realloc_calls == 1);
    trbn_gc_temp_count = 0; trbn_gc_collect(0); reset();

    /* Moving roots retain real objects across collection; then release all of them. */
    collect_before = 1;
    for (int i = 0; i < 130; ++i) {
        uintptr_t *object = trbn_gc_alloc(record_descriptor, 8, 0);
        assert(object[0] == (uintptr_t)record_descriptor && object[1] == 0);
        object[1] = (uintptr_t)i;
        expected[2 * i] = object; roots(2 * i + 1);
        expected[2 * i + 1] = trbn_string_from_bytes("ok", 2); roots(2 * i + 2);
        trbn_gc_collect(0);
        for (int j = 0; j <= i; ++j) {
            uintptr_t *record = expected[2 * j], *string = expected[2 * j + 1];
            assert(record[0] == (uintptr_t)record_descriptor && record[1] == (uintptr_t)j);
            assert(string[0] == (uintptr_t)trbn_desc_string && string[1] == 2 && string[2] == 2);
            assert(!memcmp(string + 3, "ok\0", 3));
        }
    }
    assert(trbn_gc_heap_bytes == 130 * (24 + 35));
    assert(trbn_gc_reclaimed_bytes == 0);
    trbn_gc_temp_count = 0; trbn_gc_collect(0);
    assert(!trbn_gc_heap && trbn_gc_heap_bytes == 0);
    assert(trbn_gc_allocated_bytes == trbn_gc_reclaimed_bytes);
    reset();
    puts("Temporary roots, moving growth, failure ordering and collection passed");
    return 0;
}
'''

with tempfile.TemporaryDirectory(prefix='native-root-buffer-') as temporary:
    root = Path(temporary)
    source = root / 'main.trb'
    source.write_text('def main()\nputs("roots")\nend\n')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    text = emitted.stdout.decode()
    names = ['trbn_gc_temp_push', 'trbn_gc_temp_grow', 'trbn_gc_temp_reserve',
             'trbn_gc_alloc', 'trbn_gc_update_peak', 'trbn_gc_collect', 'trbn_gc_mark_roots',
             'trbn_gc_mark', 'trbn_gc_enqueue', 'trbn_gc_mark_grow', 'trbn_gc_scan_fixed', 'trbn_gc_scan_array', 'trbn_gc_sweep',
             'trbn_storage_alloc', 'trbn_storage_free', 'trbn_storage_insert', 'trbn_storage_unlink',
             'trbn_array_backing_release',
             'trbn_string_alloc', 'trbn_string_from_bytes', 'trbn_utf8_count', 'trbn_utf8_width']
    bodies = []
    for name in names:
        matches = re.findall(r'^function (?:l )?\$' + name + r'\([^\n]*\) \{.*?^\}', text, re.M | re.S)
        assert len(matches) == 1, name
        body = matches[0]
        if name != 'trbn_gc_mark_grow':
            body = body.replace('call $realloc(', 'call $observe_realloc(')
        bodies.append('export ' + body)
    for name in ('trbn_allocation_error', 'trbn_storage_available', 'trbn_array_backing_cache'):
        matches = re.findall(r'^data \$' + name + r' = [^\n]+', text, re.M)
        assert len(matches) == 1, name
        bodies.append(matches[0])
    (root / 'runtime.ssa').write_text('\n'.join(bodies))
    (root / 'observer.c').write_text(observer)
    for command in ([str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                    ['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'), '-o', str(root / 'probe')]):
        subprocess.run(command, check=True, capture_output=True, timeout=30)
    result = subprocess.run([str(root / 'probe')], capture_output=True, timeout=30)
    assert result.returncode == 0 and result.stderr == b'', result
    assert result.stdout == b'Temporary roots, moving growth, failure ordering and collection passed\n', result
print('Actual emitted root publication, moving realloc, OOM and collection passed')
