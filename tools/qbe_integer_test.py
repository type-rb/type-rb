#!/usr/bin/env python3
"""Execute integer edge cases against an independent fixed-width Python oracle."""
import argparse
from pathlib import Path
import platform
import random
import subprocess
import tempfile

from qbe_toolchain import check_capability


OPERATIONS = ("add", "sub", "and", "or", "xor", "shl", "shr", "sar")


def expected(operation, value, operand, width):
    modulus = 2 ** width
    value %= modulus
    operand %= modulus
    if operation == "add":
        result = value + operand
    elif operation == "sub":
        result = value - operand
    elif operation == "and":
        result = value & operand
    elif operation == "or":
        result = value | operand
    elif operation == "xor":
        result = value ^ operand
    elif operation == "shl":
        result = value * 2 ** (operand % width)
    elif operation == "shr":
        result = value // 2 ** (operand % width)
    else:
        signed = value if value < modulus // 2 else value - modulus
        result = signed // 2 ** (operand % width)
    return result % modulus


def fixture(*, wide_shift_immediates=True):
    rng = random.Random(43129)
    il = []
    declarations = []
    checks = []
    count = 0
    for width, cls in ((32, "w"), (64, "l")):
        constants = {0, 1, -1, 4095, 4096, 4097, -4095, -4096, -4097,
                     0xfff000, -0xfff000, 0xffffff, 0x1000000,
                     0x100000fff, 0xffffffff00000fff, 0x123456789abcdef0}
        for bits in (1, 7, 8, 12, 16, 24, 31, 32, 48, 63):
            mask = 2 ** bits - 1
            constants.update((mask, -mask, ~mask, 2 ** bits))
        constants.update(rng.getrandbits(64) for _ in range(12))
        values = [0, 1, -1, 2 ** (width - 1), 2 ** (width - 1) - 1,
                  2 ** width - 1, 0x5555555555555555, 0xaaaaaaaaaaaaaaaa]
        values += [rng.getrandbits(width) for _ in range(8)]
        for operation in OPERATIONS:
            operands = sorted(constants)
            if operation in ("shl", "shr", "sar"):
                operands = [-2 ** 63, -65, -64, -63, -33, -32, -31, -1,
                            0, 1, 31, 32, 33, 63, 64, 65, 127, 128,
                            2 ** 32 + 1, 2 ** 63 - 1]
            for operand in operands:
                name = f"integer_{count}"
                # Upstream AMD64 prints immediate shifts verbatim; its assembler
                # accepts only an unsigned byte. Exercise wider counts through
                # a register there, retaining the full ARM64 immediate corpus.
                register_shift = (operation in ("shl", "shr", "sar")
                                  and not wide_shift_immediates and not 0 <= operand <= 255)
                parameters = f"{cls} %x" + (", w %shift" if register_shift else "")
                shift_operand = "%shift" if register_shift else str(operand)
                il.append(f"export function {cls} ${name}({parameters}) {{\n@start\n"
                          f" %r ={cls} {operation} %x, {shift_operand}\n ret %r\n}}\n")
                c_parameters = f"uint{width}_t" + (", uint32_t" if register_shift else "")
                declarations.append(f"extern uint{width}_t {name}({c_parameters});")
                for value in values:
                    value %= 2 ** width
                    result = expected(operation, value, operand, width)
                    arguments = f"UINT{width}_C({value})"
                    if register_shift:
                        arguments += f", UINT32_C({operand % 2 ** 32})"
                    checks.append(f"if ((uint64_t){name}({arguments}) != UINT64_C({result})) "
                                  f'{{ fprintf(stderr, "{name} input {value} failed\\n"); return 1; }}')
                count += 1
            # A variable operand must retain register selection and the same semantics.
            name = f"integer_{count}"
            arg_cls = "w" if operation in ("shl", "shr", "sar") else cls
            arg_width = 32 if arg_cls == "w" else width
            il.append(f"export function {cls} ${name}({cls} %x, {arg_cls} %y) {{\n@start\n"
                      f" %r ={cls} {operation} %x, %y\n ret %r\n}}\n")
            declarations.append(f"extern uint{width}_t {name}(uint{width}_t, uint{arg_width}_t);")
            for value, operand in zip(values, operands):
                value %= 2 ** width
                operand %= 2 ** arg_width
                result = expected(operation, value, operand, width)
                checks.append(f"if ((uint64_t){name}(UINT{width}_C({value}), UINT{arg_width}_C({operand})) "
                              f'!= UINT64_C({result})) {{ fprintf(stderr, "{name} variable failed\\n"); return 1; }}')
            count += 1
    il += ["export data $anchor = align 8 { l 17 }\n",
           "export function l $address(l %x) {\n@start\n %r =l add %x, $anchor\n ret %r\n}\n",
           "export function d $floating(d %x) {\n@start\n %a =d add %x, d_0.25\n"
           " %r =d sub %a, d_0.5\n ret %r\n}\n"]
    declarations += ["extern uint64_t anchor, address(uint64_t);", "extern double floating(double);"]
    checks += ["if (address(0) != (uintptr_t)&anchor || floating(2.5) != 2.25) return 1;"]
    c = ("#include <stdint.h>\n#include <stdio.h>\n" + "\n".join(declarations)
         + "\nint main(void) {\n" + "\n".join(checks)
         + f'\nprintf("{len(checks)} integer and fallback checks passed\\n"); return 0;\n}}\n')
    return "\n".join(il), c


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", required=True, type=Path)
    parser.add_argument("--baseline", type=Path)
    args = parser.parse_args()
    qbe = args.qbe.resolve()
    check_capability(qbe)
    with tempfile.TemporaryDirectory(prefix="qbe-integer-") as temporary:
        root = Path(temporary)
        for target in ("arm64", "arm64_apple", "amd64_sysv", "amd64_apple"):
            il, c = fixture(wide_shift_immediates=target.startswith("arm64"))
            source = root / (target + ".ssa")
            source.write_text(il)
            (root / (target + ".c")).write_text(c)
            assembly = root / (target + ".s")
            subprocess.run([str(qbe), "-t", target, "-o", str(assembly), str(source)], check=True)
            if args.baseline and target.startswith("amd64"):
                previous = subprocess.check_output([str(args.baseline.resolve()), "-t", target, str(source)])
                assert assembly.read_bytes() == previous, target + " emission changed"
        machine = platform.machine()
        architecture = "arm64" if machine in ("aarch64", "arm64") else "amd64"
        assert machine in ("aarch64", "arm64", "x86_64", "AMD64"), machine
        target = architecture + ("_apple" if platform.system() == "Darwin" else "_sysv" if architecture == "amd64" else "")
        driver = root / (target + ".c")
        binary = root / "integers"
        subprocess.run(["cc", "-std=c11", "-O0", str(driver), str(root / (target + ".s")), "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)


if __name__ == "__main__":
    main()
