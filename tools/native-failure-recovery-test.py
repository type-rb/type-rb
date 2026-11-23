#!/usr/bin/env python3
"""Qualify the private recovery trampoline against the host libc and callback ABI."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
runtime = root / 'compiler/src/backend/qbe/runtime'
def decoded(path):
    return '\n'.join(json.loads(s) for s in re.findall(r'"(?:[^"\\]|\\.)*"', path.read_text()))
# Qualify the deadline-enabled recovery variant and its actual dependencies.
# The ordinary CLI and no-timeout application controls cover the smaller variant.
il = '\n'.join(decoded(runtime / name) for name in (
    'resource_cleanup.trb', 'deadlines.trb', 'failure_recovery.trb'))
system = decoded(runtime / 'system.trb')
fatal = re.findall(r'^function \$trbn_fatal\([^\n]*\) \{.*?^\}', system, re.M | re.S)
bounds = re.findall(r'^data \$trbn_bounds_error = [^\n]+$', system, re.M)
assert len(fatal) == 1 and len(bounds) == 1
il += '\n' + fatal[0] + '\n' + bounds[0]
il = re.sub(r'^(function|data) ', r'export \1 ', il, flags=re.M)
observer = r'''
#include <assert.h>
#include <errno.h>
#include <fcntl.h>
#include <setjmp.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
_Static_assert(sizeof(jmp_buf) <= 1024, "recovery jump environment capacity");
_Static_assert(_Alignof(jmp_buf) <= 16, "recovery jump environment alignment");
_Static_assert(sizeof(void *) == 8, "recovery pointer ABI");
int64_t trbn_gc_temp_count = 7;
extern uintptr_t trbn_recovery_scope;
extern uintptr_t trbn_resource_scope;
extern uintptr_t trbn_deadline_scope;
extern int64_t trbn_runtime_deadline_l(void *, int64_t, int64_t);
extern double trbn_runtime_deadline_d(void *, int64_t, double);
extern void trbn_deadline_checkpoint(void);
extern void trbn_resource_push(void *, int);
extern int trbn_resource_close(void *);
extern int64_t trbn_runtime_recover_l(void *, int64_t);
extern double trbn_runtime_recover_d(void *, double);
extern void trbn_fail(const void *, int64_t);
extern void trbn_fatal(const void *, int64_t);
extern void trbn_array_bounds_fail(void);
extern void trbn_string_bounds_fail(void);
struct closure { uintptr_t descriptor; int64_t (*code)(void *); int64_t captured; };
struct float_closure { uintptr_t descriptor; double (*code)(void *); double captured; };
struct resource_scope { uintptr_t previous; int descriptor; int kind; };
typedef void (*failure_notice)(uintptr_t, const void *, int64_t, int64_t, uintptr_t, int64_t, int64_t);
struct recovery_frame {
    uintptr_t previous;
    int64_t roots;
    uintptr_t context;
    uintptr_t action;
    unsigned char environment[1024];
    uintptr_t resources;
    failure_notice notice;
};
_Static_assert(offsetof(struct closure, code) == 8, "closure code offset");
_Static_assert(sizeof(struct resource_scope) == 16, "file cleanup frame size");
_Static_assert(offsetof(struct resource_scope, descriptor) == 8, "cleanup descriptor offset");
_Static_assert(offsetof(struct resource_scope, kind) == 12, "cleanup kind offset");
_Static_assert(offsetof(struct recovery_frame, environment) == 32, "jump environment offset");
_Static_assert(offsetof(struct recovery_frame, resources) == 1056, "recovery cleanup offset");
_Static_assert(offsetof(struct recovery_frame, notice) == 1064, "optional notice offset");
_Static_assert(sizeof(struct recovery_frame) == 1072, "recovery frame size");
struct deadline_frame {
    uintptr_t previous;
    uintptr_t owner;
    int64_t deadline;
    int64_t roots;
    uintptr_t recovery;
    uintptr_t resources;
    uintptr_t action;
    uint64_t fallback;
    unsigned char environment[1024];
};
_Static_assert(offsetof(struct deadline_frame, environment) == 64, "deadline jump offset");
_Static_assert(offsetof(struct deadline_frame, resources) == 40, "deadline cleanup offset");
_Static_assert(sizeof(struct deadline_frame) == 1088, "deadline frame size");
static int side_effects;
static int failed_descriptor;
static int notices;
static int bounds_notices;
static const char *expected_bounds;
static int push_pipe(struct resource_scope *scope) {
    int descriptors[2];
    assert(pipe(descriptors) == 0);
    assert(close(descriptors[1]) == 0);
    trbn_resource_push(scope, descriptors[0]);
    assert(trbn_resource_scope == (uintptr_t)scope);
    return descriptors[0];
}
static void expect_closed(int descriptor) {
    errno = 0;
    assert(fcntl(descriptor, F_GETFD) == -1 && errno == EBADF);
}
static int64_t failed(void *value) {
    struct closure *closure = value;
    struct resource_scope scope;
    failed_descriptor = push_pipe(&scope);
    side_effects += (int)closure->captured;
    trbn_gc_temp_count = 500;
    trbn_fail("synthetic failure\n", 18);
    assert(0); return -1;
}
static int64_t fatal_failure(void *value) {
    (void)value;
    trbn_fatal("fatal failure\n", 14);
    assert(0); return -1;
}
static int64_t nested(void *value) {
    struct closure *closure = value;
    struct resource_scope scope;
    int outer_descriptor = push_pipe(&scope);
    struct closure child = {0, failed, 1};
    int64_t result = trbn_runtime_recover_l(&child, 41);
    assert(trbn_gc_temp_count == 7 && trbn_recovery_scope != 0);
    expect_closed(failed_descriptor);
    assert(trbn_resource_scope == (uintptr_t)&scope);
    assert(fcntl(outer_descriptor, F_GETFD) >= 0);
    assert(trbn_resource_close(&scope) == 0);
    expect_closed(outer_descriptor);
    return result + closure->captured;
}
static double success_float(void *value) {
    return ((struct float_closure *)value)->captured;
}
static double failed_float(void *value) {
    (void)value;
    struct resource_scope scope;
    failed_descriptor = push_pipe(&scope);
    trbn_fail("float failure\n", 14);
    assert(0); return -1;
}
static void notified(uintptr_t context, const void *message, int64_t size,
                     int64_t raw, uintptr_t path, int64_t line, int64_t column) {
    assert(context == 77 && raw == 1 && path == 0 && line == 0 && column == 0);
    assert(size == 18 && !memcmp(message, "synthetic failure\n", 18));
    /* Notification happens before cleanup and can still inspect the live frame. */
    assert(trbn_resource_scope != 0 && fcntl(failed_descriptor, F_GETFD) >= 0);
    ++notices;
}
static int64_t notifying(void *value) {
    struct recovery_frame *frame = (void *)trbn_recovery_scope;
    assert(frame && frame->notice == NULL);
    frame->notice = notified;
    struct closure child = {0, failed, 0};
    int before = notices;
    assert(trbn_runtime_recover_l(&child, 41) == 41);
    assert(notices == before); /* The nearer ordinary frame suppresses notices. */
    struct float_closure floating = {0, failed_float, 0};
    assert(trbn_runtime_recover_d(&floating, 9.5) == 9.5);
    assert(notices == before);
    return failed(value);
}
static void notified_bounds(uintptr_t context, const void *message, int64_t size,
                            int64_t raw, uintptr_t path, int64_t line, int64_t column) {
    assert(context == 77 && raw == 1 && path == 0 && line == 0 && column == 0);
    assert(size == (int64_t)strlen(expected_bounds));
    assert(!memcmp(message, expected_bounds, (size_t)size));
    assert(trbn_resource_scope != 0 && fcntl(failed_descriptor, F_GETFD) >= 0);
    ++bounds_notices;
}
static int64_t bounds_failure(void *value) {
    struct recovery_frame *frame = (void *)trbn_recovery_scope;
    struct resource_scope scope;
    assert(frame && frame->notice == NULL);
    frame->notice = notified_bounds;
    failed_descriptor = push_pipe(&scope);
    trbn_gc_temp_count = 500;
    if (((struct closure *)value)->captured == 1) {
        trbn_array_bounds_fail();
    } else {
        trbn_string_bounds_fail();
    }
    assert(0); return -1;
}
static int64_t cancelled(void *value) {
    (void)value;
    struct resource_scope scope;
    failed_descriptor = push_pipe(&scope);
    trbn_gc_temp_count = 500;
    /* Expire only after admission and resource acquisition, independently of scheduling. */
    ((struct deadline_frame *)trbn_deadline_scope)->deadline = 0;
    trbn_deadline_checkpoint();
    assert(0); return -1;
}
static int64_t recovering_inside_deadline(void *value) {
    (void)value;
    struct closure child = {0, cancelled, 0};
    trbn_runtime_recover_l(&child, 41);
    assert(0); return -1; /* An inner recovery must not consume cancellation. */
}
static int64_t deadline_inside_recovery(void *value) {
    struct closure *closure = value;
    struct closure child = {0, failed, closure->captured};
    trbn_runtime_deadline_l(&child, 10000, 99);
    assert(0); return -1; /* An ordinary failure is not a timeout response. */
}
int main(int argc, char **argv) {
    if (argc > 1) {
        if (!strcmp(argv[1], "outside")) {
            trbn_fail("outside failure\n", 16);
        } else if (!strcmp(argv[1], "outside-array")) {
            trbn_array_bounds_fail();
        } else if (!strcmp(argv[1], "outside-string")) {
            trbn_string_bounds_fail();
        } else {
            struct closure fatal = {0, fatal_failure, 0};
            trbn_runtime_recover_l(&fatal, 99);
        }
        return 90;
    }
    struct closure outer = {0, nested, 1};
    struct float_closure good = {0, success_float, 2.5};
    struct float_closure bad = {0, failed_float, 0};
    struct closure notice = {0, notifying, 0};
    for (int i = 0; i < 10000; ++i) {
        assert(trbn_runtime_recover_l(&outer, 99) == 42);
        assert(trbn_runtime_recover_d(&good, 9.5) == 2.5);
        assert(trbn_runtime_recover_d(&bad, 9.5) == 9.5);
        expect_closed(failed_descriptor);
        assert(trbn_gc_temp_count == 7 && trbn_recovery_scope == 0);
        assert(trbn_resource_scope == 0);
        assert(trbn_runtime_recover_l(&notice, 77) == 77);
        expect_closed(failed_descriptor);
        assert(notices == i + 1 && trbn_gc_temp_count == 7);
        assert(trbn_recovery_scope == 0 && trbn_resource_scope == 0);
    }
    assert(side_effects == 10000);
    for (int kind = 1; kind <= 2; ++kind) {
        expected_bounds = kind == 1 ? "panic: Array index is out of bounds\n"
                                   : "panic: String index is out of bounds\n";
        struct closure bounds = {0, bounds_failure, kind};
        assert(trbn_runtime_recover_l(&bounds, 77) == 77);
        assert(bounds_notices == kind);
        expect_closed(failed_descriptor);
        assert(trbn_gc_temp_count == 7 && trbn_recovery_scope == 0);
        assert(trbn_resource_scope == 0);
    }
    struct closure cancellation = {0, recovering_inside_deadline, 0};
    assert(trbn_runtime_deadline_l(&cancellation, 10000, 73) == 73);
    expect_closed(failed_descriptor);
    assert(trbn_gc_temp_count == 7 && trbn_resource_scope == 0);
    assert(trbn_recovery_scope == 0 && trbn_deadline_scope == 0);
    struct closure ordinary = {0, deadline_inside_recovery, 0};
    assert(trbn_runtime_recover_l(&ordinary, 61) == 61);
    expect_closed(failed_descriptor);
    assert(trbn_gc_temp_count == 7 && trbn_resource_scope == 0);
    assert(trbn_recovery_scope == 0 && trbn_deadline_scope == 0);
    assert(trbn_runtime_deadline_d(&good, 10000, 9.5) == 2.5);
    assert(trbn_runtime_deadline_d(&good, 0, 9.5) == 9.5);
    assert(trbn_gc_temp_count == 7 && trbn_deadline_scope == 0);
    printf("recovery ABI: jmp_buf=%zu alignment=%zu; 10000 nested/Float/resource/notice cycles\n",
        sizeof(jmp_buf), _Alignof(jmp_buf));
}
'''
with tempfile.TemporaryDirectory(prefix='native-recovery-abi-') as temporary:
    work = Path(temporary)
    (work / 'runtime.ssa').write_text(il + '\n')
    (work / 'observer.c').write_text(observer)
    subprocess.run([str(args.qbe.resolve()), '-o', str(work / 'runtime.s'), str(work / 'runtime.ssa')], check=True)
    subprocess.run(['cc', '-std=c11', '-O2', str(work / 'observer.c'), str(work / 'runtime.s'), '-o', str(work / 'probe')], check=True)
    for mode, code, error in [([], 0, ''), (['outside'], 2, 'outside failure\n'),
                              (['outside-array'], 2, 'panic: index is out of bounds\n'),
                              (['outside-string'], 2, 'panic: index is out of bounds\n'),
                              (['fatal'], 2, 'fatal failure\n')]:
        result = subprocess.run([str(work / 'probe'), *mode], capture_output=True, text=True, timeout=30)
        assert result.returncode == code and result.stderr == error, result
        if not mode:
            print(result.stdout, end='')
        else:
            assert not result.stdout, result
print('Deadline cancellation bypasses inner recovery; ordinary failure leaves deadline scopes')
print('Array/String caught bounds retain their category and clean resources')
print('Out-of-scope, collection bounds and process-fatal failures remain terminal')
