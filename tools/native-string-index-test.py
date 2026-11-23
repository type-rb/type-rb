#!/usr/bin/env python3
"""Verify ordinary String indexing and bounded, allocation-free queries."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--compiler', type=Path, required=True)
parser.add_argument('--output', type=Path)
args = parser.parse_args()
# Inspect actual candidate emission, including shared emitter functions. Joining
# source literals omits calls and substitutions and can silently test stale IL.
with tempfile.TemporaryDirectory(prefix='native-string-index-emission-') as temporary:
    source = Path(temporary) / 'main.trb'
    source.write_text('import trb/std/process\ndef main()\n'
                      'values := Process.argv()\ntext := values[0]\n'
                      'puts(text[0])\nputs(text.include?(values[1]))\nend\n')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    decoded = emitted.stdout.decode()
# The compiler-only source slicer is deliberately omitted from ordinary apps.
# Retain its existing direct-source coverage, with the emitted UTF-8 callees.
runtime_source = Path(__file__).resolve().parent.parent / 'compiler/src/backend/qbe/runtime/system.trb'
runtime_literals = '\n'.join(json.loads(s) for s in re.findall(r'"(?:[^"\\]|\\.)*"', runtime_source.read_text()))
source_slice = re.findall(r'^function l \$trbn_source_slice\([^\n]*\) \{.*?^\}', runtime_literals, re.M | re.S)
assert len(source_slice) == 1
decoded += '\n' + source_slice[0]
names = ['trbn_string_index', 'trbn_utf8_width', 'trbn_utf8_count', 'trbn_utf8_span',
         'trbn_string_offset', 'trbn_string_from_codepoint', 'trbn_utf8_scalar', 'trbn_source_slice',
         'trbn_string_is_utf8', 'trbn_string_query', 'trbn_string_codepoint_query',
         'trbn_string_find_bytes', 'trbn_string_bounds_fail', 'trbn_failure_notice',
         'trbn_unwind', 'trbn_fail', 'trbn_fatal', 'trbn_resource_unwind', 'trbn_resource_close']
bodies = []
for name in names:
    matches = re.findall(r'^function (?:l |w )?\$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
    assert len(matches) == 1, name
    bodies.append('export ' + matches[0])
data_names = ['string_byte_values', 'trbn_bounds_error', 'trbn_string_caught_bounds',
              'trbn_recovery_scope', 'trbn_resource_scope', 'trbn_resource_error']
data = []
for name in data_names:
    matches = re.findall(r'^data \$' + name + r' = [^\n]+', decoded, re.M)
    assert len(matches) == 1, name
    data.append(matches[0])
il = '\n'.join(data + bodies) + '\n'
# The observer supplies inputs and counts allocations; String indexing itself
# is extracted from the candidate compiler's emitted runtime, including its
# actual bounds failure, notification and resource cleanup dependencies.
observer = r'''
#include <assert.h>
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
struct string { uint64_t descriptor; int64_t length, points; unsigned char bytes[]; };
_Static_assert(sizeof(void *) == 8 && offsetof(struct string, bytes) == 24, "String ABI");
extern struct string *trbn_string_index(struct string *, int64_t);
extern struct string *trbn_string_from_codepoint(int64_t);
extern struct string *trbn_source_slice(struct string *, int64_t, int64_t);
extern int64_t trbn_utf8_count(const unsigned char *, int64_t);
extern int64_t trbn_utf8_scalar(const unsigned char *, int64_t);
extern int64_t trbn_string_query(struct string *, struct string *, int64_t);
extern int64_t trbn_string_find_bytes(struct string *, struct string *, int64_t);
static unsigned allocations;
void *trbn_string_alloc(int64_t size) { ++allocations; return calloc(1, (size_t)size + 8); }
static struct string *make(const unsigned char *data, size_t size) {
    struct string *s = calloc(1, 25 + size); assert(s);
    s->length = size; s->points = trbn_utf8_count(data, size);
    memcpy(s->bytes, data, size); return s;
}
struct guarded { void *region; size_t size; struct string *value; };
static struct guarded guard(const unsigned char *data, size_t size) {
    size_t page = (size_t)sysconf(_SC_PAGESIZE);
    assert(size % 8 == 0 && size + 24 < page);
    unsigned char *region = mmap(NULL, page * 2, PROT_READ | PROT_WRITE,
                                MAP_PRIVATE | MAP_ANON, -1, 0);
    assert(region != MAP_FAILED);
    assert(mprotect(region + page, page, PROT_NONE) == 0);
    struct string *s = (struct string *)(region + page - size - 24);
    s->length = size; s->points = trbn_utf8_count(data, size);
    if (size) memcpy(s->bytes, data, size);
    return (struct guarded){region, page * 2, s};
}
static void expect_query(struct string *value, const unsigned char *part, size_t size,
                         int64_t first, int64_t last, int64_t prefix, int64_t suffix, int64_t contains) {
    struct string *pattern = make(part, size);
    const int64_t expected[] = {first, last, prefix, suffix, contains};
    for (int mode = 0; mode < 5; ++mode)
        assert(trbn_string_query(value, pattern, mode) == expected[mode]);
    free(pattern);
}
static int64_t byte_find(const unsigned char *value, size_t length,
                         const unsigned char *part, size_t size, size_t start) {
    for (size_t at = start; size <= length && at <= length - size; ++at)
        if (!memcmp(value + at, part, size)) return (int64_t)at;
    return -1;
}
static void expect_literal_queries(struct string *value, const unsigned char *part, size_t size) {
    struct string *pattern = make(part, size);
    int64_t found = byte_find(value->bytes, value->length, part, size, 0);
    int64_t prefix = size <= (size_t)value->length && !memcmp(value->bytes, part, size) ? 0 : -1;
    int64_t suffix = size <= (size_t)value->length &&
        !memcmp(value->bytes + value->length - size, part, size) ? 0 : -1;
    assert(trbn_string_query(value, pattern, 2) == prefix);
    assert(trbn_string_query(value, pattern, 3) == suffix);
    assert(trbn_string_query(value, pattern, 4) == (found < 0 ? -1 : 0));
    if (size) {
        for (size_t start = 0; start <= (size_t)value->length; ++start)
            assert(trbn_string_find_bytes(value, pattern, start) ==
                   byte_find(value->bytes, value->length, part, size, start));
    }
    free(pattern);
}
/* Independent scalar decoder: malformed input consumes exactly one byte. */
static int64_t reference_count(const unsigned char *data, size_t length) {
    int64_t count = 0;
    for (size_t at = 0; at < length; ++count) {
        unsigned first = data[at];
        size_t width = first >= 0xc2 && first <= 0xdf ? 2 :
                       first >= 0xe0 && first <= 0xef ? 3 :
                       first >= 0xf0 && first <= 0xf4 ? 4 : 1;
        if (width > length - at) width = 1;
        for (size_t tail = 1; tail < width; ++tail)
            if ((data[at + tail] & 0xc0) != 0x80) { width = 1; break; }
        if (width >= 3 && ((first == 0xe0 && data[at + 1] < 0xa0) ||
                          (first == 0xed && data[at + 1] >= 0xa0) ||
                          (first == 0xf0 && data[at + 1] < 0x90) ||
                          (first == 0xf4 && data[at + 1] >= 0x90))) width = 1;
        at += width;
    }
    return count;
}
int main(int argc, char **argv) {
    unsigned char ascii[128];
    for (unsigned i = 0; i < 128; ++i) ascii[i] = (unsigned char)i;
    struct string *source = make(ascii, sizeof ascii);
    if (argc == 3) {
        source->points = strtoll(argv[1], NULL, 10);
        trbn_string_index(source, strtoll(argv[2], NULL, 10));
        return 1;
    }
    assert(argc == 1);
    struct string *retained[128];
    for (int i = 0; i < 128; ++i) {
        retained[i] = trbn_string_index(source, i);
        assert(retained[i] == trbn_string_index(source, i - 128));
    }
    memset(source->bytes, 255, 128); free(source);
    for (int i = 0; i < 128; ++i) {
        assert(retained[i]->length == 1 && retained[i]->points == 1);
        assert(retained[i]->bytes[0] == i && retained[i]->bytes[1] == 0);
        assert(retained[i]->descriptor == 0);
    }
    assert(allocations == 0);
    const unsigned char unicode[] = "A\xc2\xa2\xe3\x81\x82\xf0\x9f\x98\x80";
    const int64_t points[] = {65, 162, 12354, 128512};
    source = make(unicode, sizeof unicode - 1);
    assert(source->length == 10 && source->points == 4);
    struct string *values[4];
    for (int i = 0; i < 4; ++i) {
        values[i] = trbn_string_index(source, i);
        struct string *negative = trbn_string_index(source, i - 4);
        assert(values[i]->points == 1 && values[i]->length == i + 1);
        assert(trbn_utf8_scalar(values[i]->bytes, values[i]->length) == points[i]);
        assert(negative->length == values[i]->length);
        assert(!memcmp(negative->bytes, values[i]->bytes, values[i]->length + 1));
        if (i) free(negative);
    }
    assert(allocations == 6);
    struct string *slice = trbn_source_slice(source, 1, 4);
    assert(slice->length == 9 && slice->points == 3);
    assert(!memcmp(slice->bytes, unicode + 1, 10)); free(slice);
    memset(source->bytes, 0, source->length); free(source);
    for (int i = 0; i < 4; ++i) {
        assert(values[i]->bytes[values[i]->length] == 0);
        assert(trbn_utf8_scalar(values[i]->bytes, values[i]->length) == points[i]);
        if (i) free(values[i]);
    }
    const int64_t boundaries[] = {0, 127, 128, 2047, 2048, 55295, 57344, 65535, 65536, 1114111,
                                  -1, 55296, 57343, 1114112};
    for (unsigned i = 0; i < sizeof boundaries / sizeof boundaries[0]; ++i) {
        int64_t expected = i < 10 ? boundaries[i] : 65533;
        struct string *value = trbn_string_from_codepoint(boundaries[i]);
        assert(value->points == 1 && value->bytes[value->length] == 0);
        assert(trbn_utf8_count(value->bytes, value->length) == 1);
        assert(trbn_utf8_scalar(value->bytes, value->length) == expected);
        free(value);
    }
    const unsigned char *invalid[] = {(const unsigned char *)"\x80", (const unsigned char *)"\xc0\xaf",
        (const unsigned char *)"\xed\xa0\x80", (const unsigned char *)"\xf4\x90\x80\x80",
        (const unsigned char *)"\xe3\x81", (const unsigned char *)"\xf0\x90\x80"};
    for (unsigned i = 0; i < sizeof invalid / sizeof invalid[0]; ++i) {
        size_t length = strlen((const char *)invalid[i]);
        source = make(invalid[i], length);
        assert(source->points == (int64_t)length);
        struct string *value = trbn_string_index(source, 0);
        assert(value->length == 3 && value->points == 1);
        assert(!memcmp(value->bytes, "\xef\xbf\xbd", 4));
        free(value); free(source);
    }
    unsigned before_queries = allocations;
    source = make((const unsigned char *)"ababa", 5);
    expect_query(source, (const unsigned char *)"aba", 3, 0, 2, 0, 0, 0);
    expect_query(source, (const unsigned char *)"", 0, 0, 5, 0, 0, 0);
    expect_query(source, (const unsigned char *)"ababab", 6, -1, -1, -1, -1, -1);
    free(source);
    source = make((const unsigned char *)"a\0ba\0", 5);
    expect_query(source, (const unsigned char *)"a\0", 2, 0, 3, 0, 0, 0);
    free(source);
    source = make((const unsigned char *)"\xc2\xa2", 2);
    expect_query(source, (const unsigned char *)"\xa2", 1, -1, -1, -1, 0, 0);
    free(source);
    /* These Strings have no terminator: the first byte beyond the stored length
       is inaccessible, for both the receiver and the pattern. */
    struct guarded value = guard((const unsigned char *)"ab\xf0\x9f\x98\x80" "cd", 8);
    expect_query(value.value, (const unsigned char *)"\xf0\x9f\x98\x80", 4, 2, 2, -1, -1, 0);
    expect_query(value.value, (const unsigned char *)"cd", 2, 3, 3, -1, 0, 0);
    expect_query(value.value, (const unsigned char *)"ce", 2, -1, -1, -1, -1, -1);
    for (int mode = 0; mode < 5; ++mode) assert(trbn_string_query(value.value, value.value, mode) == 0);
    struct guarded invalid_value = guard((const unsigned char *)"abcdef\xe3\x81", 8);
    expect_query(invalid_value.value, (const unsigned char *)"\xef\xbf\xbd", 3, 6, 7, -1, -1, -1);
    expect_query(invalid_value.value, (const unsigned char *)"\xe3\x81", 2, 6, 6, -1, 0, 0);
    struct guarded empty = guard((const unsigned char *)"", 0);
    for (int mode = 0; mode < 5; ++mode) {
        assert(trbn_string_query(empty.value, empty.value, mode) == 0);
        assert(trbn_string_query(empty.value, value.value, mode) == -1);
        assert(trbn_string_query(value.value, empty.value, mode) == (mode == 1 ? 5 : 0));
    }
    assert(munmap(value.region, value.size) == 0);
    assert(munmap(invalid_value.region, invalid_value.size) == 0);
    assert(munmap(empty.region, empty.size) == 0);
    /* Exercise every short needle, including NUL and malformed bytes, with
       the receiver ending at an inaccessible page. A matching first byte at
       the final position must not admit a two-byte read past that boundary. */
    const unsigned char edge_bytes[] = {'A', 0, 0x80, 0xc2, 0xa2, 'A', 'b', 'A'};
    struct guarded edge = guard(edge_bytes, sizeof edge_bytes);
    unsigned char needle[2];
    for (unsigned first = 0; first < 256; ++first) {
        needle[0] = first;
        expect_literal_queries(edge.value, needle, 1);
        for (unsigned second = 0; second < 256; ++second) {
            needle[1] = second;
            expect_literal_queries(edge.value, needle, 2);
            int64_t expected = first >= 0xc2 && first <= 0xdf &&
                               second >= 0x80 && second <= 0xbf ? 1 : 2;
            assert(trbn_utf8_count(needle, 2) == expected);
        }
    }
    for (size_t start = 0; start < sizeof edge_bytes; ++start)
        expect_literal_queries(edge.value, edge_bytes + start, sizeof edge_bytes - start);
    assert(munmap(edge.region, edge.size) == 0);
    /* A deterministic byte corpus checks the ASCII path against the decoder,
       including unaligned suffixes, truncated tails and a zero-length span at
       the protected page itself. */
    uint32_t random = 1;
    for (unsigned round = 0; round < 4096; ++round) {
        unsigned char bytes[128];
        for (unsigned i = 0; i < sizeof bytes; ++i) {
            random = random * 1664525u + 1013904223u;
            bytes[i] = random >> 24;
            if (round < 128) bytes[i] &= 0x7f;
        }
        struct guarded span = guard(bytes, sizeof bytes);
        for (size_t offset = 0; offset <= sizeof bytes; ++offset)
            assert(trbn_utf8_count(span.value->bytes + offset, sizeof bytes - offset) ==
                   reference_count(bytes + offset, sizeof bytes - offset));
        assert(munmap(span.region, span.size) == 0);
    }
    /* Put valid and malformed sequences across every word boundary. Each
       suffix also checks short tails and every unaligned address up to a
       protected page, including the empty span on that page itself. */
    const unsigned char sequences[][4] = {
        {0xc2, 0xa2, 0, 'a'}, {0xe3, 0x81, 0x82, 0}, {0xf0, 0x9f, 0x98, 0x80},
        {0xc0, 0xaf, 0, 'a'}, {0xed, 0xa0, 0x80, 0}, {0xf4, 0x90, 0x80, 0x80},
        {0xe3, 0x81, 'a', 0}, {0xf0, 0x90, 0x80, 'a'}
    };
    for (size_t at = 0; at <= 124; ++at) {
        for (size_t example = 0; example < sizeof sequences / sizeof *sequences; ++example) {
            unsigned char bytes[128]; memset(bytes, 'x', sizeof bytes);
            memcpy(bytes + at, sequences[example], 4);
            struct guarded span = guard(bytes, sizeof bytes);
            for (size_t offset = 0; offset <= sizeof bytes; ++offset)
                assert(trbn_utf8_count(span.value->bytes + offset, sizeof bytes - offset) ==
                       reference_count(bytes + offset, sizeof bytes - offset));
            assert(munmap(span.region, span.size) == 0);
        }
    }
    assert(allocations == before_queries);
    puts("String indexing, Unicode lifetime, allocation-free queries and guarded bounds passed");
    return 0;
}
'''
observations = []
with tempfile.TemporaryDirectory(prefix='native String index ') as directory:
    root = Path(directory)
    (root / 'index.ssa').write_text(il)
    (root / 'observer.c').write_text(observer)
    subprocess.run([str(args.qbe.resolve()), '-o', str(root / 'index.s'), str(root / 'index.ssa')], check=True, capture_output=True, timeout=30)
    subprocess.run(['/usr/bin/cc', '-O2', str(root / 'index.s'), str(root / 'observer.c'), '-o', str(root / 'probe')], check=True, timeout=30)
    good = subprocess.run([str(root / 'probe')], capture_output=True, timeout=10)
    assert good.returncode == 0 and good.stderr == b'', good
    assert good.stdout == b'String indexing, Unicode lifetime, allocation-free queries and guarded bounds passed\n', good
    observations.append({'case': 'ascii-cache', 'reads': 256, 'allocations': 0})
    observations.append({'case': 'string-queries', 'modes': 5, 'allocations': 0,
                         'controls': ['overlap', 'nul', 'invalid-utf8', 'guarded-spans', 'empty', 'oversized']})
    observations.append({'case': 'short-byte-search', 'singleByteNeedles': 256,
                         'twoByteNeedles': 65536, 'guardedReceiverBytes': 8, 'startPositions': 9})
    observations.append({'case': 'utf8-count-oracle', 'twoByteInputs': 65536,
                         'guardedInputs': 4096, 'suffixesPerInput': 129,
                         'boundarySequenceInputs': 1000, 'maximumBytes': 128})
    for length, index in [(0, 0), (0, -1), (128, -129), (128, 128), (128, -9007199254740991), (128, 9007199254740991)]:
        result = subprocess.run([str(root / 'probe'), str(length), str(index)], capture_output=True, timeout=10)
        # This is the actual runtime fatal path (exit 2), not an observer's
        # replacement exit code.
        assert result.returncode == 2 and result.stdout == b'', result
        assert result.stderr == b'panic: index is out of bounds\n', result
        observations.append({'case': 'required-bounds-failure', 'length': length, 'index': index})
report = {'compilerSha256': hashlib.sha256(args.compiler.read_bytes()).hexdigest(),
          'sourceSliceSha256': hashlib.sha256(source_slice[0].encode()).hexdigest(),
          'runtimeSha256': hashlib.sha256(il.encode()).hexdigest(), 'observations': observations}
if args.output:
    args.output.write_text(json.dumps(report, indent=2) + '\n')
print('String indexing and queries: ASCII cache, UTF-8, zero-allocation queries, guarded spans and 6 bounds failures passed')
