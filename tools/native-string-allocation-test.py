#!/usr/bin/env python3
"""Check managed initialization with poisoned allocation and real String helpers."""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--compiler', type=Path, required=True)
args = parser.parse_args()
with tempfile.TemporaryDirectory(prefix='native-string-emission-') as temporary:
    source = Path(temporary) / 'main.trb'
    source.write_text('def main()\nnumber := 123\nputs(number.to_s())\n'
                      'text := number.to_s()\nputs("[" + text + "]")\n'
                      'puts("<" + text + "|" + text + ">")\nputs("raw".to_bytes().to_s())\nend\n')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    decoded = emitted.stdout.decode()
names = ['trbn_storage_alloc', 'trbn_storage_free', 'trbn_storage_insert',
         'trbn_storage_unlink', 'trbn_gc_alloc', 'trbn_string_alloc', 'trbn_string_from_bytes',
         'trbn_string_concat', 'trbn_string_concat_three', 'trbn_string_concat_many', 'trbn_integer_to_string', 'trbn_string_from_codepoint',
         'trbn_utf8_width', 'trbn_utf8_count', 'trbn_bytes_utf8_span', 'trbn_bytes_to_string',
         'trbn_gc_temp_push', 'trbn_gc_temp_grow']
bodies = []
for name in names:
    matches = re.findall(r'^function (?:l )?\$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    body = matches[0].replace('call $malloc(', 'call $poison_malloc(')
    if name == 'trbn_gc_alloc':
        # Observe the real append's completed state, without replacing it.
        assert body.count('ret %object') == 1
        body = body.replace('ret %object', 'call $observe_publication(l %object)\n\tret %object')
    bodies.append('export ' + body)
error = re.findall(r'^data \$trbn_allocation_error = [^\n]+', decoded, re.M)
assert len(error) == 1
storage = re.findall(r'^data \$trbn_storage_available = [^\n]+', decoded, re.M)
assert len(storage) == 1
integer_strings = re.findall(r'^data \$trbn_small_integer_strings = [^\n]+\n.*?^\}', decoded, re.M | re.S)
assert len(integer_strings) == 1
decimal_pairs = re.findall(r'^data \$trbn_decimal_pairs = [^\n]+', decoded, re.M)
assert len(decimal_pairs) == 1
il = ('export ' + error[0] + '\n' + storage[0] + '\n' + integer_strings[0] +
      '\n' + decimal_pairs[0] + '\n' + '\n'.join(bodies))
observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
struct string { uintptr_t descriptor; int64_t length, points; unsigned char bytes[]; };
extern void *trbn_storage_alloc(int64_t);
extern void trbn_storage_free(void *, int64_t);
extern void *trbn_gc_alloc(void *, int64_t, int64_t);
extern struct string *trbn_string_from_bytes(const void *, int64_t);
extern struct string *trbn_bytes_to_string(struct string *);
extern struct string *trbn_string_concat(struct string *, struct string *);
extern struct string *trbn_string_concat_three(struct string *, struct string *, struct string *);
extern struct string *trbn_string_concat_many(struct string **, int64_t);
extern int64_t trbn_utf8_count(const void *, int64_t);
extern struct string *trbn_integer_to_string(int64_t);
extern struct string *trbn_string_from_codepoint(int64_t);
int64_t trbn_desc_string[3], other_descriptor[3];
void *trbn_gc_heap;
int64_t trbn_gc_heap_bytes, trbn_gc_allocated_bytes, trbn_gc_temp_count, trbn_gc_temp_capacity;
void **trbn_gc_temp_data;

static int64_t projected, peak, published;
static int fail_allocation;
static void *bases[32];
static int64_t sizes[32], previous_bytes;
void *poison_malloc(size_t size) {
    if (fail_allocation) return NULL;
    void *p = malloc(size); assert(p); memset(p, 0xa5, size); return p;
}
void trbn_gc_maybe_collect(int64_t size) { projected = size; }
void trbn_gc_update_peak(int64_t size) { if (peak < size) peak = size; }
void observe_publication(struct string *s) {
    /* Publication must see the valid initialized header, even before the
       constructor fills the immutable bytes. It must also see accounting. */
    assert((unsigned char *)s == (unsigned char *)trbn_gc_heap + 8);
    assert(trbn_gc_temp_count > 0 && trbn_gc_temp_data[trbn_gc_temp_count - 1] == s);
    assert(peak == trbn_gc_heap_bytes);
    if (s->descriptor == (uintptr_t)trbn_desc_string)
        assert(s->length == 0 && s->points == 0);
    assert(published < 32);
    bases[published] = trbn_gc_heap;
    sizes[published] = trbn_gc_heap_bytes - previous_bytes;
    previous_bytes = trbn_gc_heap_bytes;
    ++published;
}
void trbn_fatal(const void *message, int64_t size) {
    const char expected[] = "panic: allocation failed\n";
    assert(size == 26 && !memcmp(message, expected, sizeof expected - 1));
    fputs(expected, stderr); exit(70);
}
static void clear(void) {
    for (int64_t i = published; i > 0; --i)
        trbn_storage_free(bases[i - 1], sizes[i - 1]);
    trbn_gc_heap = NULL; previous_bytes = 0;
    trbn_gc_heap_bytes = trbn_gc_allocated_bytes = peak = published = 0;
    trbn_gc_temp_count = 0;
}
static void expect_bytes(struct string *s, const void *bytes, size_t size, int64_t points) {
    assert(s->length == (int64_t)size && s->points == points);
    assert(!memcmp(s->bytes, bytes, size) && s->bytes[size] == 0);
}
static void expect(struct string *s, const void *bytes, size_t size, int64_t points) {
    assert(s->descriptor == (uintptr_t)trbn_desc_string);
    expect_bytes(s, bytes, size, points);
}
int main(int argc, char **argv) {
    if (argc > 1) {
        fail_allocation = 1;
        trbn_gc_alloc(argv[1][0] == 's' ? trbn_desc_string : other_descriptor, atoll(argv[2]), 0);
        return 1;
    }
    for (int64_t size = 0; size <= 4096; ++size) {
        unsigned char *object = trbn_gc_alloc(other_descriptor, size, 31);
        assert(*(uintptr_t *)object == (uintptr_t)other_descriptor);
        for (int64_t i = 0; i < size; ++i) assert(object[8 + i] == 0);
        assert(projected == size + 16 + 31);
        assert(trbn_gc_heap_bytes == size + 16);
        assert(trbn_gc_allocated_bytes == size + 16 && published == 1);
        clear();
    }
    /* A managed String keeps a zero header but its bytes remain owned by its
       constructor. This distinguishes the optimized path from blanket clearing. */
    for (int64_t size = 17; size <= 4096; ++size) {
        unsigned char *object = trbn_gc_alloc(trbn_desc_string, size, 0);
        for (int64_t i = 0; i < 16; ++i) assert(object[8 + i] == 0);
        for (int64_t i = 16; i < size; ++i) assert(object[8 + i] == 0xa5);
        assert(projected == size + 16 && trbn_gc_allocated_bytes == size + 16);
        clear();
    }
    /* Keep one cell alive so the next allocation actually reuses a freed cell. */
    for (int64_t total = 16; total <= 256; ++total) {
        void *keeper = trbn_storage_alloc(total);
        void *old = trbn_storage_alloc(total);
        memset(old, 0xa5, total);
        trbn_storage_free(old, total);
        unsigned char *object = trbn_gc_alloc(other_descriptor, total - 16, 0);
        assert(object == (unsigned char *)old + 8);
        for (int64_t i = 0; i < total - 16; ++i) assert(object[8 + i] == 0);
        clear(); trbn_storage_free(keeper, total);
    }
    for (int64_t length = 0; length <= 223; ++length) {
        int64_t total = length + 33;
        void *keeper = trbn_storage_alloc(total);
        void *old = trbn_storage_alloc(total);
        memset(old, 0xa5, total);
        trbn_storage_free(old, total);
        unsigned char input[224]; memset(input, 'x', sizeof input);
        struct string *s = trbn_string_from_bytes(input, length);
        assert((void *)s == (unsigned char *)old + 8);
        expect(s, input, length, length);
        clear(); trbn_storage_free(keeper, total);
    }
    /* Exhaustive fragment triples compare the fused constructor with the
       existing binary contract and an independently decoded whole byte span.
       Empty and NUL fragments, malformed bytes, and split UTF-8 sequences
       exercise joins that cannot blindly sum cached point counts. */
    static const unsigned char atoms[][4] = {
        {0}, {'a'}, {'b'}, {0}, {0xc3, 0xa9}, {0xc3}, {0xa9},
        {0xf0}, {0x9f, 0x99}, {0x82}, {0xff}, {0xe0, 0xa0}, {0x80}
    };
    static const int64_t lengths[] = {0, 1, 1, 1, 2, 1, 1, 1, 2, 1, 1, 2, 1};
    for (unsigned a = 0; a < 13; ++a)
    for (unsigned b = 0; b < 13; ++b)
    for (unsigned c = 0; c < 13; ++c) {
        struct string *fragments[] = {
            trbn_string_from_bytes(atoms[a], lengths[a]),
            trbn_string_from_bytes(atoms[b], lengths[b]),
            trbn_string_from_bytes(atoms[c], lengths[c])
        };
        unsigned char expected[12]; int64_t length = 0;
        unsigned indexes[] = {a, b, c}; int nonempty = 0;
        for (int i = 0; i < 3; ++i) {
            memcpy(expected + length, atoms[indexes[i]], lengths[indexes[i]]);
            length += lengths[indexes[i]];
            nonempty += lengths[indexes[i]] != 0;
        }
        struct string *binary = trbn_string_concat(
            trbn_string_concat(fragments[0], fragments[1]), fragments[2]);
        int64_t before = trbn_gc_allocated_bytes;
        struct string *fused = trbn_string_concat_many(fragments, 3);
        int64_t points = trbn_utf8_count(expected, length);
        expect(binary, expected, length, points);
        expect(fused, expected, length, points);
        assert(trbn_gc_allocated_bytes - before == (nonempty > 1 ? length + 33 : 0));
        before = trbn_gc_allocated_bytes;
        struct string *direct = trbn_string_concat_three(fragments[0], fragments[1], fragments[2]);
        expect(direct, expected, length, points);
        assert(trbn_gc_allocated_bytes - before == (nonempty > 1 ? length + 33 : 0));
        clear();
    }
    unsigned char bytes[256];
    for (unsigned i = 0; i < 256; ++i) bytes[i] = i;
    for (int64_t size = 0; size <= 256; ++size) {
        /* Ascending byte values contain no valid multi-byte sequence. */
        struct string *s = trbn_string_from_bytes(bytes, size);
        expect(s, bytes, size, size);
        assert(trbn_gc_allocated_bytes == size + 33);
        clear();
    }
    /* Bytes decoding must allocate exactly its final length, including sizes
       on both sides of the small-object boundary and maximal invalid subparts. */
    for (int64_t length = 0; length <= 96; ++length) {
        unsigned char ascii[96]; memset(ascii, 'x', sizeof ascii);
        struct string *input = trbn_string_from_bytes(ascii, length);
        struct string *output = trbn_bytes_to_string(input);
        expect(output, ascii, length, length);
        assert(projected == length + 33);
        assert(trbn_gc_allocated_bytes == 2 * (length + 33)); clear();
    }
    for (unsigned byte = 0; byte < 256; ++byte) {
        unsigned char raw = (unsigned char)byte;
        struct string *input = trbn_string_from_bytes(&raw, 1);
        struct string *output = trbn_bytes_to_string(input);
        if (byte < 128) expect(output, &raw, 1, 1);
        else expect(output, "\xef\xbf\xbd", 3, 1);
        assert(projected == output->length + 33);
        assert(trbn_gc_allocated_bytes == 34 + output->length + 33); clear();
    }
    const unsigned char mixed[] = {'a', 0, 0xe3, 0x81, 0x82, 0xe3, 0x81, 'z', 0xf0, 0x90, 0x80};
    struct string *input = trbn_string_from_bytes(mixed, sizeof mixed);
    struct string *decoded = trbn_bytes_to_string(input);
    expect(decoded, "a\0\xe3\x81\x82\xef\xbf\xbdz\xef\xbf\xbd", 12, 6);
    assert(projected == 45 && trbn_gc_allocated_bytes == sizeof mixed + 33 + 45);
    clear();
    const unsigned char left[] = {0xe3, 0x81}, right[] = {0x82, 0, 'a'};
    struct string *a = trbn_string_from_bytes(left, sizeof left);
    struct string *b = trbn_string_from_bytes(right, sizeof right);
    struct string *empty = trbn_string_from_bytes("", 0);
    expect(a, left, sizeof left, 2); expect(b, right, sizeof right, 3);
    expect(trbn_string_concat(a, b), "\xe3\x81\x82\0a", 5, 3);
    assert(trbn_string_concat(empty, a) == a && trbn_string_concat(b, empty) == b);
    clear();
    /* Static strings need neither heap publication nor an allocation. Keep the
       managed-descriptor assertion intact for every constructor and fallback. */
    fail_allocation = 1;
    for (int64_t value = 0; value < 100; ++value) {
        char buffer[32]; int size = snprintf(buffer, sizeof buffer, "%lld", (long long)value);
        struct string *cached = trbn_integer_to_string(value);
        assert(cached->descriptor == 0);
        expect_bytes(cached, buffer, size, size);
        assert(trbn_integer_to_string(value) == cached);
        assert(published == 0 && trbn_gc_heap_bytes == 0 && trbn_gc_allocated_bytes == 0);
        assert(trbn_gc_temp_count == 0);
    }
    fail_allocation = 0;
    int64_t values[] = {-9007199254740991LL, -10001, -10000, -1000, -123, -1,
                        100, 101, 123, 1000, 10000, 10001, 9007199254740991LL};
    for (unsigned i = 0; i < sizeof values / sizeof *values; ++i) {
        char buffer[32]; int size = snprintf(buffer, sizeof buffer, "%lld", (long long)values[i]);
        expect(trbn_integer_to_string(values[i]), buffer, size, size); clear();
    }
    expect(trbn_string_from_codepoint(0), "\0", 1, 1);
    expect(trbn_string_from_codepoint(0x7ff), "\xdf\xbf", 2, 1);
    expect(trbn_string_from_codepoint(0x3042), "\xe3\x81\x82", 3, 1);
    expect(trbn_string_from_codepoint(0x10ffff), "\xf4\x8f\xbf\xbf", 4, 1);
    expect(trbn_string_from_codepoint(0xd800), "\xef\xbf\xbd", 3, 1);
    clear();
    free(trbn_gc_temp_data);
    puts("Managed initialization and poisoned String construction passed");
    return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='native-string-allocation-') as temporary:
    root = Path(temporary)
    (root / 'runtime.ssa').write_text(il)
    (root / 'observer.c').write_text(observer)
    subprocess.run([str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                   check=True, timeout=30)
    subprocess.run(['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'),
                    '-o', str(root / 'probe')], check=True, timeout=30)
    result = subprocess.run([str(root / 'probe')], capture_output=True, timeout=30)
    assert result.returncode == 0 and result.stderr == b'', result
    for descriptor in ['string', 'other']:
        for size in ['17', '4096']:
            failure = subprocess.run([str(root / 'probe'), descriptor, size], capture_output=True, timeout=10)
            assert failure.returncode == 70 and failure.stdout == b'', failure
            assert failure.stderr == b'panic: allocation failed\n', failure
print('Managed initialization, poisoned String constructors and allocation failures passed')
