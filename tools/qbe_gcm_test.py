#!/usr/bin/env python3
"""Check recursive expression motion and invariant loop guards."""
import argparse
import os
from pathlib import Path
import platform
import re
import subprocess
import tempfile

from qbe_toolchain import ORDERED_CALLS_PROBE, check_ordered_effects


GUARD = """export function l $guard(l %a, l %b, l %c, l %n) {
@start
 jmp @loop
@loop
 %i =l phi @start 0, @join %next
 %sum =l phi @start 0, @join %newsum
 %more =w csltl %i, %n
 jnz %more, @body, @done
@body
 %one =w csltl %a, %b
 %two =w cnel %b, %c
 %both =w and %one, %two
 jnz %both, @yes, @no
@yes
 %left =l add %sum, 7
 jmp @join
@no
 %right =l sub %sum, 3
 jmp @join
@join
 %newsum =l phi @yes %left, @no %right
 %next =l add %i, 1
 jmp @loop
@done
 ret %sum
}
"""

SINGLE_GUARD = GUARD.replace("$guard(", "$single_guard(").replace(
    "%both =w and %one, %two", "%both =w copy %one")

SHARED = """export function l $shared(l %a, l %b, l %c, l %which) {
@start
 %one =l csltl %a, %b
 %two =l cnel %b, %c
 %both =l and %one, %two
 %first =w csltl %which, 0
 jnz %first, @branch, @value
@branch
 jnz %both, @yes, @no
@yes
 ret 7
@no
 ret -3
@value
 ret %both
}
"""

ADDRESS = """export function $address(l %p, l %seed, l %n) {
@start
 %masked =l and %seed, 7
 %offset =l shl %masked, 3
 %address =l add %p, %offset
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %more =w csltl %i, %n
 jnz %more, @body, @done
@body
 %value =l loadl %address
 %updated =l add %value, 1
 storel %updated, %address
 %next =l add %i, 1
 jmp @loop
@done
 ret
}
"""

COPY_CYCLE = """export function l $copy_cycle(l %a, l %b, l %n, l %p) {
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %left =l phi @start %a, @body %right
 %right =l phi @start %b, @body %left
 %more =w csltl %i, %n
 jnz %more, @body, @done
@body
 storel %i, %p
 %unused =l call $observe(l %i)
 %next =l add %i, 1
 jmp @loop
@done
 ret %i
}
"""

LIVE_CYCLE = COPY_CYCLE.replace("$copy_cycle(", "$live_cycle(").replace("ret %i", "ret %left")
OBSERVED_CYCLE = COPY_CYCLE.replace("$copy_cycle(", "$observed_cycle(").replace(
    "storel %i, %p", "storel %left, %p").replace("$observe(l %i)", "$observe(l %right)")
FLOAT_CYCLE = """export function d $float_cycle(d %a, d %b, l %n) {
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %left =d phi @start %a, @body %right
 %right =d phi @start %b, @body %left
 %more =w csltl %i, %n
 jnz %more, @body, @done
@body
 %next =l add %i, 1
 jmp @loop
@done
 ret %left
}
"""

ORDERED_LOADS = """export function l $ordered_loads(l %p, l %other) {
@start
 jmp @init
@init
 storel 7, %p
 %r =l loadl %other
 jnz 1, @first, @second
@first
 %one =l add %r, 8
 ret %one
@second
 %two =l add %r, 8
 ret %two
}
"""

DRIVER = r"""
#include <stdint.h>
#include <stdio.h>
#include <string.h>
extern int64_t guard(int64_t, int64_t, int64_t, int64_t);
extern int64_t single_guard(int64_t, int64_t, int64_t, int64_t);
extern int64_t shared(int64_t, int64_t, int64_t, int64_t);
extern void address(uint64_t *, uint64_t, int64_t);
extern int64_t copy_cycle(int64_t, int64_t, int64_t, int64_t *);
extern int64_t live_cycle(int64_t, int64_t, int64_t, int64_t *);
extern int64_t observed_cycle(int64_t, int64_t, int64_t, int64_t *);
extern double float_cycle(double, double, int64_t);
extern int64_t ordered_calls(int64_t *);
extern int64_t ordered_loads(int64_t *, int64_t *);
static int64_t observed, calls, snapshot[2];
void change_order(int64_t *p) { p[1] = 7; }
int64_t *inspect_order(int64_t *p) {
    snapshot[0] = p[0]; snapshot[1] = p[1]; return snapshot;
}
int64_t observe(int64_t value) { observed = value; ++calls; return 0; }
int main(void) {
    int64_t values[] = {INT64_MIN, -1, 0, 1, 17, INT64_MAX};
    int64_t trips[] = {-3, 0, 1, 2, 31, 65};
    unsigned checks = 0;
    int64_t data[] = {3, 3};
    if (ordered_calls(data) != 7 || data[1] != 7) return 9;
    if (ordered_loads(data, data) != 15 || data[0] != 7) return 10;
    checks += 2;
    for (unsigned a = 0; a < 6; ++a)
    for (unsigned b = 0; b < 6; ++b)
    for (unsigned c = 0; c < 6; ++c) {
        int condition = values[a] < values[b] && values[b] != values[c];
        for (unsigned n = 0; n < 6; ++n) {
            int64_t count = trips[n] < 0 ? 0 : trips[n];
            int64_t expected = count * (condition ? 7 : -3);
            if (guard(values[a], values[b], values[c], trips[n]) != expected)
                return 1;
            if (single_guard(values[a], values[b], values[c], trips[n])
                != count * (values[a] < values[b] ? 7 : -3))
                return 4;
            checks += 2;
        }
        if (shared(values[a], values[b], values[c], -1) != (condition ? 7 : -3)
            || shared(values[a], values[b], values[c], 1) != condition)
            return 2;
        checks += 2;
    }
    for (uint64_t seed = 0; seed < 33; ++seed)
    for (unsigned n = 0; n < 6; ++n) {
        uint64_t data[8];
        for (unsigned i = 0; i < 8; ++i) data[i] = 100 + i;
        address(data, seed, trips[n]);
        for (unsigned i = 0; i < 8; ++i) {
            uint64_t delta = i == (seed & 7) && trips[n] > 0 ? trips[n] : 0;
            if (data[i] != 100 + i + delta) return 3;
            ++checks;
        }
    }
    for (unsigned a = 0; a < 6; ++a)
    for (unsigned b = 0; b < 6; ++b)
    for (unsigned n = 0; n < 6; ++n) {
        int64_t count = trips[n] > 0 ? trips[n] : 0;
        int64_t last = count ? count - 1 : 99;
        int64_t stored = 99;
        observed = 99; calls = 0;
        if (copy_cycle(values[a], values[b], trips[n], &stored) != count
            || stored != last || observed != last || calls != count) return 5;
        ++checks;
        stored = observed = 99; calls = 0;
        int64_t expected = values[count & 1 ? b : a];
        if (live_cycle(values[a], values[b], trips[n], &stored) != expected
            || stored != last || observed != last || calls != count) return 6;
        ++checks;
        stored = observed = 99; calls = 0;
        int64_t left = count ? values[(count - 1) & 1 ? b : a] : 99;
        int64_t right = count ? values[(count - 1) & 1 ? a : b] : 99;
        if (observed_cycle(values[a], values[b], trips[n], &stored) != count
            || stored != left || observed != right || calls != count) return 7;
        ++checks;
        double fa, fb, actual;
        uint64_t bits;
        memcpy(&fa, &values[a], sizeof fa);
        memcpy(&fb, &values[b], sizeof fb);
        actual = float_cycle(fa, fb, trips[n]);
        memcpy(&bits, &actual, sizeof bits);
        if (bits != (uint64_t)expected) return 8;
        ++checks;
    }
    printf("%u GCM semantic checks passed\n", checks);
    return 0;
}
"""


def check_motion(qbe, target):
    for name, source in (("guard", GUARD), ("single", SINGLE_GUARD),
                         ("shared", SHARED), ("address", ADDRESS)):
        result = subprocess.run([str(qbe), "-t", target, "-d", "G"], input=source,
                                text=True, capture_output=True, check=True)
        moved = result.stderr.split("> After GCM:", 1)[1]
        for value in re.findall(r"(%snk\.\d+)\s*=", moved):
            if len(re.findall(re.escape(value) + r"(?!\d)", moved)) == 1:
                raise AssertionError(f"{target} {name}: unused sunk operand {value}")
        if name == "guard":
            # The induction comparison stays in the loop, while the compound
            # predicate depends only on immutable function arguments.
            body = moved.split("@body\n", 1)[1].split("@yes\n", 1)[0]
            if re.search(r"\b(?:csltl|cnel|and)\b", body):
                raise AssertionError(f"{target}: invariant predicate was sunk into the loop")
        elif name == "address":
            body = moved.split("@body\n", 1)[1].split("@done\n", 1)[0]
            if not re.search(r"\bload %snk\.\d+", body):
                raise AssertionError(f"{target}: cheap address rematerialization was disabled")
        elif name == "single":
            body = moved.split("@body\n", 1)[1].split("@yes\n", 1)[0]
            if "csltl" not in body:
                raise AssertionError(f"{target}: single comparison rematerialization was disabled")
    result = subprocess.run([str(qbe), "-t", target, "-d", "G"], input=COPY_CYCLE,
                            text=True, capture_output=True, check=True)
    moved = result.stderr.split("> After GCM:", 1)[1]
    if re.search(r"%(?:left|right)(?:\.\d+)?\b", moved):
        raise AssertionError(f"{target}: unused copy cycle survived")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", type=Path, required=True)
    args = parser.parse_args()
    qbe = args.qbe.resolve()
    targets = ("arm64", "arm64_apple", "amd64_sysv", "amd64_apple", "amd64_win", "rv64")
    with tempfile.TemporaryDirectory(prefix="qbe-gcm-") as temporary:
        root = Path(temporary)
        source = root / "input.ssa"
        source.write_text(GUARD + SINGLE_GUARD + SHARED + ADDRESS
                          + COPY_CYCLE + LIVE_CYCLE + OBSERVED_CYCLE + FLOAT_CYCLE
                          + ORDERED_CALLS_PROBE + ORDERED_LOADS)
        for target in targets:
            check_motion(qbe, target)
            check_ordered_effects(qbe, target)
            subprocess.run([str(qbe), "-t", target, "-o", str(root / (target + ".s")),
                            str(source)], check=True)
        machine = platform.machine()
        native = "arm64" if machine in ("arm64", "aarch64") else "amd64"
        native += "_apple" if platform.system() == "Darwin" else ("_sysv" if native == "amd64" else "")
        driver = root / "driver.c"
        driver.write_text(DRIVER)
        program = root / "program"
        subprocess.run([os.environ.get("CC", "cc"), str(driver), str(root / (native + ".s")),
                        "-o", str(program)], check=True)
        result = subprocess.run([str(program)], capture_output=True, text=True, check=True)
        assert result.stdout == "5474 GCM semantic checks passed\n", result.stdout
        assert not result.stderr, result.stderr
        print(result.stdout, end="")
        print("Recursive operands and invariant guards verified for six targets")


if __name__ == "__main__":
    main()
