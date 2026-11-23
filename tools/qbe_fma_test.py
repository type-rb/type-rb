#!/usr/bin/env python3
"""Check explicitly permitted Float contraction against independent libm oracles."""
import argparse
from pathlib import Path
import platform
import re
import subprocess
import tempfile


FORMS = {"add": ("add", "%product, %c"), "reverse_add": ("add", "%c, %product"),
         "subtract": ("sub", "%product, %c"), "reverse_subtract": ("sub", "%c, %product")}


def late_product_operations(width):
    functions = []
    for producer in ("div", "sqrt"):
        expression = "div %value, %b" if producer == "div" else "sqrt %value"
        for name, (operation, operands) in FORMS.items():
            functions.append(f"""export function {width} ${width}_{producer}_{name}(l %values, l %count, {width} %b, {width} %c) {{
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %sum ={width} phi @start %c, @body %result
 %more =w csltl %i, %count
 jnz %more, @body, @done
@body
 %offset =l mul %i, {8 if width == "d" else 4}
 %address =l add %values, %offset
 %value ={width} load{width} %address
 %factor ={width} {expression}
 %product ={width} mulc %factor, %c
 %result ={width} {operation} {operands.replace('%c', '%sum')}
 %next =l add %i, 1
 jmp @loop
@done
 ret %sum
}}
""")
    functions.append(f"""export function {width} ${width}_division_observed({width} %a, {width} %b, {width} %c, l %count, l %out) {{
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %previous ={width} phi @start %a, @body %product
 %more =w csltl %i, %count
 jnz %more, @body, @done
@body
 %factor ={width} div %previous, %b
 %product ={width} mulc %factor, %c
 %result ={width} add %product, %a
 store{width} %result, %out
 %next =l add %i, 1
 jmp @loop
@done
 ret %previous
}}
""")
    return "".join(functions)


def operations(width):
    functions = []
    for name, (operation, operands) in FORMS.items():
        for prefix, multiply in (("", "mulc"), ("strict_", "mul")):
            functions.append(f"""export function {width} ${width}_{prefix}{name}({width} %a, {width} %b, {width} %c) {{
@start
 %product ={width} {multiply} %a, %b
 %result ={width} {operation} {operands}
 ret %result
}}
""")
    functions.append(f"""export function {width} ${width}_reuse({width} %a, {width} %b, {width} %c, l %out) {{
@start
 %product ={width} mulc %a, %b
 %result ={width} add %product, %c
 store{width} %product, %out
 ret %result
}}
export function {width} ${width}_twice({width} %a, {width} %b) {{
@start
 %product ={width} mulc %a, %b
 %result ={width} add %product, %product
 ret %result
}}
export function {width} ${width}_call({width} %a, {width} %b, {width} %c) {{
@start
 %product ={width} mulc %a, %b
 %other ={width} call $echo_{width}({width} %c)
 %result ={width} add %product, %other
 ret %result
}}
export function {width} ${width}_branch({width} %a, {width} %b, {width} %c, w %which) {{
@start
 %product ={width} mulc %a, %b
 jnz %which, @sum, @product
@sum
 %result ={width} add %product, %c
 ret %result
@product
 ret %product
}}
export function {width} ${width}_return_product({width} %a, {width} %b, {width} %c, l %out) {{
@start
 %product ={width} mulc %a, %b
 %result ={width} add %product, %c
 store{width} %result, %out
 ret %product
}}
export function {width} ${width}_successor({width} %a, {width} %b, {width} %c, w %which, l %out) {{
@start
 %product ={width} mulc %a, %b
 %result ={width} add %product, %c
 jnz %which, @observe, @done
@observe
 store{width} %product, %out
 jmp @done
@done
 ret %result
}}
export function {width} ${width}_carried({width} %a, {width} %b, {width} %c, l %count, l %out) {{
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %previous ={width} phi @start %a, @body %product
 %more =w csltl %i, %count
 jnz %more, @body, @done
@body
 %product ={width} mulc %previous, %b
 %result ={width} add %product, %c
 store{width} %result, %out
 %next =l add %i, 1
 jmp @loop
@done
 ret %previous
}}
export function {width} ${width}_accumulate(l %values, l %count, {width} %b, {width} %c) {{
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %sum ={width} phi @start %c, @body %result
 %more =w csltl %i, %count
 jnz %more, @body, @done
@body
 %offset =l mul %i, {8 if width == "d" else 4}
 %address =l add %values, %offset
 %value ={width} load{width} %address
 %product ={width} mulc %value, %b
 %result ={width} add %sum, %product
 %next =l add %i, 1
 jmp @loop
@done
 ret %sum
}}
export function {width} ${width}_update(l %values, l %count, {width} %b) {{
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %more =w csltl %i, %count
 jnz %more, @body, @done
@body
 %offset =l mul %i, {8 if width == "d" else 4}
 %address =l add %values, %offset
 %value ={width} load{width} %address
 %product ={width} mulc %value, %b
 %result ={width} add %value, %product
 store{width} %result, %address
 %next =l add %i, 1
 jmp @loop
@done
 ret %b
}}
export function {width} ${width}_constant() {{
@start
 %product ={width} mulc {width}_2.0, {width}_3.0
 %result ={width} add %product, {width}_4.0
 ret %result
}}
""")
    return "".join(functions) + late_product_operations(width)


# Compile at -O0 with contraction disabled. Volatile products are independent
# rounding barriers; the fused oracle calls libm rather than a generated QBE op.
DRIVER = r"""
#include <float.h>
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
typedef @TYPE@ Scalar;
typedef @BITS@ Bits;
extern Scalar @W@_add(Scalar, Scalar, Scalar);
extern Scalar @W@_reverse_add(Scalar, Scalar, Scalar);
extern Scalar @W@_subtract(Scalar, Scalar, Scalar);
extern Scalar @W@_reverse_subtract(Scalar, Scalar, Scalar);
extern Scalar @W@_strict_add(Scalar, Scalar, Scalar);
extern Scalar @W@_strict_reverse_add(Scalar, Scalar, Scalar);
extern Scalar @W@_strict_subtract(Scalar, Scalar, Scalar);
extern Scalar @W@_strict_reverse_subtract(Scalar, Scalar, Scalar);
extern Scalar @W@_reuse(Scalar, Scalar, Scalar, Scalar *);
extern Scalar @W@_twice(Scalar, Scalar);
extern Scalar @W@_call(Scalar, Scalar, Scalar);
extern Scalar @W@_branch(Scalar, Scalar, Scalar, int);
extern Scalar @W@_return_product(Scalar, Scalar, Scalar, Scalar *);
extern Scalar @W@_successor(Scalar, Scalar, Scalar, int, Scalar *);
extern Scalar @W@_carried(Scalar, Scalar, Scalar, long long, Scalar *);
extern Scalar @W@_accumulate(Scalar *, long long, Scalar, Scalar);
extern Scalar @W@_update(Scalar *, long long, Scalar);
extern Scalar @W@_constant(void);
extern Scalar @W@_division_observed(Scalar, Scalar, Scalar, long long, Scalar *);
@LATE_DECLARATIONS@
Scalar echo_@W@(Scalar value) { return value; }
static int same(Scalar a, Scalar b) {
    Bits left, right;
    if (isnan(a) && isnan(b)) return 1;
    memcpy(&left, &a, sizeof left);
    memcpy(&right, &b, sizeof right);
    return left == right;
}
static int check(Scalar a, Scalar b, Scalar c) {
    volatile Scalar product = a * b;
    Scalar separate[] = {product + c, c + product, product - c, c - product};
    Scalar fused[] = {@FMA@(a, b, c), @FMA@(a, b, c),
                      @FMA@(a, b, -c), @FMA@(-a, b, c)};
    Scalar (*permitted[])(Scalar, Scalar, Scalar) = {
        @W@_add, @W@_reverse_add, @W@_subtract, @W@_reverse_subtract};
    Scalar (*strict[])(Scalar, Scalar, Scalar) = {
        @W@_strict_add, @W@_strict_reverse_add, @W@_strict_subtract,
        @W@_strict_reverse_subtract};
    for (unsigned i = 0; i < 4; ++i) {
        Scalar expected = HOST_FUSES ? fused[i] : separate[i];
        if (!same(permitted[i](a, b, c), expected)) return 10 + i;
        if (!same(strict[i](a, b, c), separate[i])) return 20 + i;
    }
    Scalar observed = 0;
    Scalar result = @W@_reuse(a, b, c, &observed);
    if (!same(observed, product)) return 30;
    if (!same(result, separate[0]) && !same(result, fused[0])) return 31;
    if (!same(@W@_twice(a, b), product + product)) return 32;
    result = @W@_call(a, b, c);
    if (!same(result, separate[0]) && !same(result, fused[0])) return 33;
    if (!same(@W@_branch(a, b, c, 0), product)) return 34;
    result = @W@_branch(a, b, c, 1);
    if (!same(result, separate[0]) && !same(result, fused[0])) return 35;
    result = @W@_return_product(a, b, c, &observed);
    if (!same(result, product)) return 40;
    if (!same(observed, separate[0]) && !same(observed, fused[0])) return 41;
    result = @W@_successor(a, b, c, 1, &observed);
    if (!same(observed, product)) return 42;
    if (!same(result, separate[0]) && !same(result, fused[0])) return 43;
    volatile Scalar carried = a;
    Scalar previous = a;
    for (int i = 0; i < 3; ++i) {
        previous = carried;
        carried = previous * b;
    }
    result = @W@_carried(a, b, c, 3, &observed);
    if (!same(result, carried)) return 44;
    if (!same(observed, carried + c) && !same(observed, @FMA@(previous, b, c))) return 45;
    Scalar values[] = {a, b, c}, sum = c;
    for (int i = 0; i < 3; ++i) {
        volatile Scalar rounded = values[i] * b;
        sum = sum + rounded;
    }
    if (!same(@W@_accumulate(values, 3, b, c), sum)) return 46;
    observed = 7.0;
    if (!same(@W@_carried(a, b, c, 0, &observed), a)) return 47;
    if (observed != 7.0) return 48;
    if (!same(@W@_accumulate(values, 0, b, c), c)) return 49;
    volatile Scalar divided = a, factor = a;
    for (unsigned i = 0; i < 3; ++i) {
        factor = divided / b;
        divided = factor * c;
    }
    result = @W@_division_observed(a, b, c, 3, &observed);
    if (!same(result, divided)) return 51;
    if (!same(observed, divided + a) && !same(observed, @FMA@(factor, c, a))) return 52;
    observed = 7.0;
    if (!same(@W@_division_observed(a, b, c, 0, &observed), a)) return 53;
    if (observed != 7.0) return 54;
    Scalar (*late_updates[2][4])(Scalar *, long long, Scalar, Scalar) = {
        {@W@_div_add, @W@_div_reverse_add, @W@_div_subtract, @W@_div_reverse_subtract},
        {@W@_sqrt_add, @W@_sqrt_reverse_add, @W@_sqrt_subtract, @W@_sqrt_reverse_subtract}};
    for (unsigned producer = 0; producer < 2; ++producer) {
        Scalar expected[] = {c, c, c, c};
        for (unsigned i = 0; i < 3; ++i) {
            volatile Scalar factor = producer ? @SQRT@(values[i]) : values[i] / b;
            volatile Scalar rounded = factor * c;
            Scalar fused[] = {@FMA@(factor, c, expected[0]), @FMA@(factor, c, expected[1]),
                              @FMA@(factor, c, -expected[2]), @FMA@(-factor, c, expected[3])};
            Scalar separate[] = {rounded + expected[0], expected[1] + rounded,
                                 rounded - expected[2], expected[3] - rounded};
            for (unsigned form = 0; form < 4; ++form)
                expected[form] = HOST_FUSES ? fused[form] : separate[form];
        }
        for (unsigned form = 0; form < 4; ++form) {
            if (!same(late_updates[producer][form](values, 3, b, c), expected[form]))
                return 60 + producer * 4 + form;
            if (!same(late_updates[producer][form](values, 0, b, c), c))
                return 70 + producer * 4 + form;
        }
    }
    @W@_update(values, 3, b);
    Scalar original[] = {a, b, c};
    for (int i = 0; i < 3; ++i) {
        volatile Scalar rounded = original[i] * b;
        if (!same(values[i], original[i] + rounded) && !same(values[i], @FMA@(original[i], b, original[i]))) return 50;
    }
    return 0;
}
int check_@W@(void) {
    if (@W@_constant() != 10.0) return 36;
    Scalar values[] = {0.0, -0.0, 1.0, -1.0, 2.0, -2.0, INFINITY,
        -INFINITY, NAN, @MAX@, -@MAX@, @MIN@, -@MIN@, @TRUE_MIN@,
        @NEXT_ONE@, -@NEXT_TWO@};
    unsigned count = sizeof values / sizeof *values, checks = 1;
    for (unsigned a = 0; a < count; ++a)
    for (unsigned b = 0; b < count; ++b)
    for (unsigned c = 0; c < count; ++c) {
        int status = check(values[a], values[b], values[c]);
        if (status) {
            fprintf(stderr, "@W@ edge %u/%u/%u failed check %d\n", a, b, c, status);
            return status;
        }
        checks += 47;
    }
    uint64_t state = UINT64_C(0x384028ccab71e107);
    for (unsigned n = 0; n < 4096; ++n) {
        Scalar inputs[3];
        for (unsigned i = 0; i < 3; ++i) {
            state ^= state << 13; state ^= state >> 7; state ^= state << 17;
            Bits bits = state;
            memcpy(&inputs[i], &bits, sizeof bits);
        }
        int status = check(inputs[0], inputs[1], inputs[2]);
        if (status) {
            fprintf(stderr, "@W@ random %u failed check %d\n", n, status);
            return status;
        }
        checks += 47;
    }
    printf("@W@ contraction: %u checks passed\n", checks);
    return 0;
}
"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", required=True, type=Path)
    parser.add_argument("--baseline", type=Path)
    args = parser.parse_args()
    qbe = args.qbe.resolve()
    source = operations("d") + operations("s")
    with tempfile.TemporaryDirectory(prefix="native-qbe-fma-") as temporary:
        root = Path(temporary)
        targets = ("arm64", "arm64_apple", "amd64_sysv", "amd64_apple", "amd64_win", "rv64")
        for target in targets:
            result = subprocess.run([str(qbe), "-t", target], input=source,
                                    text=True, capture_output=True, check=True)
            (root / (target + ".s")).write_text(result.stdout)
            for width in ("d", "s"):
                for name in FORMS:
                    for prefix in ("", "strict_"):
                        symbol = width + "_" + prefix + name
                        body = re.split(rf"(?m)^_?{symbol}:\s*$", result.stdout)[1]
                        body = body.split("/* end function")[0]
                        fused = bool(re.search(r"\bf(?:madd|msub|nmsub)\b", body))
                        assert fused == (target.startswith("arm64") and not prefix), (target, symbol)
                body = re.split(rf"(?m)^_?{width}_accumulate:\s*$", result.stdout)[1]
                body = body.split("/* end function")[0]
                assert not re.search(r"\bfmadd\b", body), (target, width, "accumulator")
                for producer in ("div", "sqrt"):
                    for name in FORMS:
                        symbol = f"{width}_{producer}_{name}"
                        body = re.split(rf"(?m)^_?{symbol}:\s*$", result.stdout)[1]
                        body = body.split("/* end function")[0]
                        fused = bool(re.search(r"\bf(?:madd|msub|nmsub)\b", body))
                        assert fused == target.startswith("arm64"), (target, symbol)
        machine = platform.machine()
        arm64 = machine in ("arm64", "aarch64")
        if arm64:
            target = "arm64_apple" if platform.system() == "Darwin" else "arm64"
        elif machine in ("x86_64", "AMD64"):
            target = "amd64_apple" if platform.system() == "Darwin" else "amd64_sysv"
        else:
            raise ValueError(f"unsupported test host: {machine}")
        for width, kind, bits, fma, limits, one, two in (
                ("d", "double", "uint64_t", "fma", "DBL", "0x1.0000000000001p0", "0x1.0000000000002p0"),
                ("s", "float", "uint32_t", "fmaf", "FLT", "0x1.000002p0f", "0x1.000004p0f")):
            declarations = "\n".join(
                f"extern Scalar @W@_{producer}_{name}(Scalar *, long long, Scalar, Scalar);"
                for producer in ("div", "sqrt") for name in FORMS)
            text = DRIVER.replace("@LATE_DECLARATIONS@", declarations)
            replacements = {"W": width, "TYPE": kind, "BITS": bits, "FMA": fma,
                            "SQRT": "sqrt" if width == "d" else "sqrtf",
                            "MAX": limits + "_MAX", "MIN": limits + "_MIN",
                            "TRUE_MIN": limits + "_TRUE_MIN", "NEXT_ONE": one, "NEXT_TWO": two}
            for key, value in replacements.items():
                text = text.replace("@" + key + "@", value)
            (root / (width + ".c")).write_text(text)
        (root / "main.c").write_text("int check_d(void); int check_s(void);\n"
                                    "int main(void) { int result = check_d(); return result ? result : check_s(); }\n")
        binary = root / "edges"
        subprocess.run(["cc", "-std=c11", "-O0", "-ffp-contract=off", "-fno-builtin",
                        f"-DHOST_FUSES={int(arm64)}", str(root / "d.c"), str(root / "s.c"),
                        str(root / "main.c"), str(root / (target + ".s")), "-lm",
                        "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)
    for operation in ("%y =w mulc %x, %x", "%y =l mulc %x, %x", "%y =d mulc %x"):
        result = subprocess.run([str(qbe)], input="function d $bad(d %x) {\n@start\n " +
                                operation + "\n ret %x\n}\n", text=True, capture_output=True)
        assert result.returncode != 0, operation
    if args.baseline:
        result = subprocess.run([str(args.baseline.resolve())], input=source, text=True, capture_output=True)
        assert result.returncode != 0 and "mulc" in result.stderr, "previous backend accepted the new opcode"
    print("QBE Float contraction, strict multiplication, reuse, calls, returns, successors and loop-carried products passed on six emission targets")


if __name__ == "__main__":
    main()
