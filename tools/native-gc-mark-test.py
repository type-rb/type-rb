#!/usr/bin/env python3
"""Exercise the emitted GC on deep/wide graphs and failing moving worklists."""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--compiler', type=Path, required=True)
args = parser.parse_args()

observer = r'''
#include <assert.h>
#include <setjmp.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern void trbn_gc_mark(void *);
extern void trbn_gc_mark_grow(void *);
struct work { void *data; int64_t count, capacity; };
struct node { uintptr_t descriptor; void *next; int64_t value; };
struct array { uintptr_t descriptor; int64_t length, capacity; void *data; };
static uintptr_t fixed[] = {1, 1, 16, 8};
static uintptr_t scalar[] = {1, 0, 16};
static uintptr_t managed_array[] = {2, 1};
static uintptr_t scalar_array[] = {2, 0};
static uintptr_t string_descriptor[] = {0};
static uintptr_t literal[] = {0};
static size_t allocations, releases, active_bytes, last_bytes;
static unsigned char *active;
static int fail_allocate, expected_failure, failures;
static jmp_buf failure;
static const uint64_t guard = UINT64_C(0x58432a79e09b37ad);

static void check_active(void) {
    assert(active);
    uint64_t first, last;
    memcpy(&first, active, 8);
    memcpy(&last, active + 8 + active_bytes, 8);
    assert(first == guard && last == guard);
}
static void *allocate(size_t bytes) {
    ++allocations;
    last_bytes = bytes;
    if (fail_allocate) return NULL;
    unsigned char *p = malloc(bytes + 16); assert(p);
    memcpy(p, &guard, 8);
    memcpy(p + 8 + bytes, &guard, 8);
    memset(p + 8, 0xa5, bytes);
    return p;
}
void *observe_malloc(size_t bytes) {
    assert(!active);
    active = allocate(bytes);
    if (!active) return NULL;
    active_bytes = bytes;
    return active + 8;
}
void *observe_realloc(void *old, size_t bytes) {
    check_active();
    assert(old == active + 8 && bytes > active_bytes);
    unsigned char *next = allocate(bytes);
    if (!next) return NULL;
    memcpy(next + 8, active + 8, active_bytes);
    /* Always move and poison the old allocation to expose stale data pointers. */
    memset(active + 8, 0xd7, active_bytes);
    free(active);
    active = next;
    active_bytes = bytes;
    return active + 8;
}
void observe_free(void *p) {
    check_active();
    assert(p == active + 8);
    free(active);
    active = NULL; active_bytes = 0; ++releases;
}
void trbn_fatal(const void *message, int64_t bytes) {
    const char text[] = "panic: allocation failed\n";
    assert(expected_failure && bytes == 26 && !memcmp(message, text, sizeof text - 1));
    ++failures;
    longjmp(failure, 1);
}
static void depth(int mixed) {
    const size_t count = 200000;
    struct node *nodes = calloc(count, sizeof *nodes); assert(nodes);
    struct array *arrays = mixed ? calloc(count, sizeof *arrays) : NULL;
    void **edges = mixed ? calloc(count, sizeof *edges) : NULL;
    assert(!mixed || (arrays && edges));
    for (size_t i = 0; i < count; ++i) {
        nodes[i].descriptor = (uintptr_t)fixed; nodes[i].value = (int64_t)i;
        void *next = i + 1 < count ? &nodes[i + 1] : &nodes[0];
        if (mixed) {
            edges[i] = next;
            arrays[i] = (struct array){(uintptr_t)managed_array, 1, 1, &edges[i]};
            next = &arrays[i];
        }
        nodes[i].next = next;
    }
    size_t before = allocations;
    trbn_gc_mark(nodes);
    trbn_gc_mark(nodes); /* Repeated roots do not revisit the cyclic graph. */
    assert(allocations == before && !active);
    for (size_t i = 0; i < count; ++i) {
        assert(nodes[i].descriptor == ((uintptr_t)fixed | 1));
        assert(nodes[i].value == (int64_t)i);
        if (mixed) assert(arrays[i].descriptor == ((uintptr_t)managed_array | 1));
    }
    free(edges); free(arrays); free(nodes);
}
static void breadth(void) {
    const size_t count = 8192;
    struct node *nodes = calloc(count, sizeof *nodes); assert(nodes);
    void **edges = calloc(count * 2 + 2, sizeof *edges); assert(edges);
    struct array root = {(uintptr_t)managed_array, count * 2 + 2, count * 2 + 2, edges};
    for (size_t i = 0; i < count; ++i) {
        nodes[i] = (struct node){(uintptr_t)fixed, &root, (int64_t)i};
        edges[2*i] = edges[2*i+1] = &nodes[i];
    }
    edges[count * 2] = literal; /* Followed by a null element. */
    size_t before = allocations, freed = releases;
    trbn_gc_mark(&root);
    assert(!active && allocations > before + 1 && releases == freed + 1);
    for (size_t i = 0; i < count; ++i) {
        assert(nodes[i].descriptor == ((uintptr_t)fixed | 1));
        assert(nodes[i].next == &root && nodes[i].value == (int64_t)i);
    }
    assert(literal[0] == 0);
    free(edges); free(nodes);
}
static void grow_failure(struct work *work) {
    struct work before = *work;
    int previous = failures;
    expected_failure = fail_allocate = 1;
    if (!setjmp(failure)) { trbn_gc_mark_grow(work); abort(); }
    assert(failures == previous + 1 && !memcmp(work, &before, sizeof before));
    expected_failure = fail_allocate = 0;
}
int main(void) {
    struct node leaf = {(uintptr_t)scalar, (void *)1, 123};
    struct array numbers = {(uintptr_t)scalar_array, 10, 10, (void *)1};
    uintptr_t text[] = {(uintptr_t)string_descriptor, 5, 5};
    trbn_gc_mark(NULL); trbn_gc_mark(literal); trbn_gc_mark(&leaf);
    trbn_gc_mark(&numbers); trbn_gc_mark(text);
    assert(!allocations && literal[0] == 0 && leaf.value == 123);
    assert(leaf.descriptor == ((uintptr_t)scalar | 1));
    assert(numbers.descriptor == ((uintptr_t)scalar_array | 1));
    assert(text[0] == ((uintptr_t)string_descriptor | 1));
    depth(0); depth(1); breadth();
    uintptr_t inline_data[64];
    for (size_t i = 0; i < 64; ++i) inline_data[i] = i + 42;
    struct work work = {inline_data, 64, 64};
    grow_failure(&work); /* First allocation. */
    trbn_gc_mark_grow(&work);
    assert(work.count == 64 && work.capacity == 128);
    assert(!memcmp(work.data, inline_data, sizeof inline_data));
    grow_failure(&work); /* Growing existing heap storage. */
    check_active();
    assert(!memcmp(work.data, inline_data, sizeof inline_data));
    observe_free(work.data);
    work = (struct work){(void *)1, 0, INT64_C(1) << 59};
    size_t before = allocations;
    grow_failure(&work); /* Byte-count overflow fails before touching storage. */
    assert(allocations == before);
    work.data = observe_malloc(1024);
    work.capacity = INT64_C(1) << 58;
    before = allocations;
    grow_failure(&work); /* Largest power-of-two capacity with a safe doubling. */
    assert(allocations == before + 1 && last_bytes == (UINT64_C(1) << 62));
    observe_free(work.data);
    assert(!active && failures == 4);
    puts("Deep and wide graphs, cycles, moving growth and allocation failures passed");
}
'''

with tempfile.TemporaryDirectory(prefix='native-gc-mark-') as temporary:
    root = Path(temporary)
    source = root / 'main.trb'
    source.write_text('def main()\nputs("mark")\nend\n')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    decoded = emitted.stdout.decode()
    bodies = []
    for name in ('trbn_gc_mark', 'trbn_gc_enqueue', 'trbn_gc_mark_grow',
                 'trbn_gc_scan_fixed', 'trbn_gc_scan_array'):
        matches = re.findall(r'^function \$' + name + r'\([^\n]*\) \{.*?^\}', decoded, re.M | re.S)
        assert len(matches) == 1, name
        body = matches[0]
        for allocation in ('malloc', 'realloc', 'free'):
            body = body.replace('call $' + allocation + '(', 'call $observe_' + allocation + '(')
        bodies.append('export ' + body)
    data = re.findall(r'^data \$trbn_allocation_error = [^\n]+', decoded, re.M)
    assert len(data) == 1
    (root / 'runtime.ssa').write_text('\n'.join(bodies + data))
    (root / 'observer.c').write_text(observer)
    for command in ([str(args.qbe.resolve()), '-o', str(root / 'runtime.s'), str(root / 'runtime.ssa')],
                    ['/usr/bin/cc', '-O2', str(root / 'runtime.s'), str(root / 'observer.c'),
                     '-o', str(root / 'probe')]):
        subprocess.run(command, check=True, capture_output=True, timeout=30)
    result = subprocess.run([str(root / 'probe')], capture_output=True, timeout=30)
    assert result.returncode == 0 and not result.stderr, result
    print(result.stdout.decode(), end='')

    # Exercise compiler-authored descriptors, roots and pacing as well as the
    # synthetic observer. Every link owns a managed Array and a dynamic String;
    # a cycle and a shared edge must survive automatic collections unchanged.
    source.write_text('''class Node
  @next: Node?
  @children: Array<Node>
  @label: String
  def initialize(label: String)
    @next = nil
    @children = []
    @label = label
  end
end
def graph(): Integer
  mut first := Node.new("start" + "-node")
  mut head := first
  mut count := 1
  while count < 120000
    mut made := Node.new(count.to_s())
    made.next = head
    made.children.push(head)
    head = made
    count += 1
  end
  first.next = head
  mut current: Node? := head
  mut seen := 0
  while current != nil && seen < 120000
    if seen == 0
      puts(current.label)
      puts(current.children[0].label)
    end
    current = current.next
    seen += 1
  end
  return seen
end
def main()
  puts(graph())
  puts(graph())
end
''')
    emitted = subprocess.run([str(args.compiler.resolve()), 'emit-qbe', str(source)],
                             check=True, capture_output=True, timeout=30)
    assert not emitted.stderr, emitted
    (root / 'application.ssa').write_bytes(emitted.stdout)
    for command in ([str(args.qbe.resolve()), '-o', str(root / 'application.s'), str(root / 'application.ssa')],
                    ['/usr/bin/cc', str(root / 'application.s'), '-lm', '-o', str(root / 'application')]):
        subprocess.run(command, check=True, capture_output=True, timeout=30)
    result = subprocess.run([str(root / 'application')], capture_output=True, timeout=30,
                            env=dict(os.environ, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
    assert result.returncode == 0, result
    assert result.stdout == b'119999\n119998\n120000\n' * 2, result
    stats = {}
    for line in result.stderr.decode().splitlines():
        version, metric, value = line.split(',')
        assert version == 'type-rb-native-gc-stat-v1', line
        stats[metric] = int(value)
    assert stats['automatic-collections'] > 0, stats
    assert stats['live-bytes'] == 0, stats
    assert stats['allocated-bytes'] == stats['reclaimed-bytes'], stats
    print('Ordinary deep class/Array/String graphs and complete cyclic reclamation passed')
