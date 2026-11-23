#!/usr/bin/env python3
"""Check memory operand selection against independent byte-level expectations."""
import argparse
from pathlib import Path
import platform
import subprocess
import tempfile

from qbe_toolchain import check_capability


def offsets(width):
    limit = 4095 * width
    return sorted({-256, -255, -1, 0, 1, 2, 3, 7, 8, 15, 16, 24,
                   255, 256, limit - 1, limit, limit + width})


def byte_at(index):
    return (index * 37 + index // 11) & 255


def loaded_bits(address, width, result_width, signed=False):
    value = int.from_bytes(bytes(byte_at(address + i) for i in range(width)), "little", signed=signed)
    return value % (2 ** result_width)


def indexed_fixture(loads, c_types):
    """Use an independent byte oracle, including wrapping 64-bit indexes."""
    il, declarations, checks = [], [], ["fill();"]
    for number, (operation, cls, width, signed) in enumerate(loads):
        result_width = 64 if cls in ("l", "d") else 32
        for shift in range(5):
            for reverse in (False, True):
                name = f"indexed_load_{number}_{shift}_{int(reverse)}"
                operands = "%offset, %p" if reverse else "%p, %offset"
                il.append(f"export function {cls} ${name}(l %p, l %i) {{\n@start\n"
                          f" %offset =l shl %i, {shift}\n %a =l add {operands}\n"
                          f" %v ={cls} {operation} %a\n ret %v\n}}\n")
                declarations.append(f"extern {c_types[cls]} {name}(void *, uint64_t);")
                indexes = [-7, -1, 0, 3, 19]
                if shift:
                    indexes.append((1 << (64 - shift)) + 3)
                for base in (512, 513):
                    for index in indexes:
                        offset = (index << shift) % (1 << 64)
                        if offset >= 1 << 63:
                            offset -= 1 << 64
                        expected = loaded_bits(base + offset, width, result_width, signed)
                        value = f"{name}(arena + {base}, UINT64_C({index % (1 << 64)}))"
                        if cls in ("s", "d"):
                            value = f"bits{result_width}({value})"
                        checks.append(f"CHECK((uint64_t)({value}) == UINT64_C({expected}), {name});")

    for number, (operation, cls, width) in enumerate((
            ("storeb", "w", 1), ("storeh", "w", 2), ("storew", "w", 4),
            ("storel", "l", 8), ("stores", "s", 4), ("stored", "d", 8))):
        bits = 64 if cls in ("l", "d") else 32
        for shift in range(5):
            name = f"indexed_store_{number}_{shift}"
            il.append(f"export function ${name}(l %p, l %i, {cls} %v) {{\n@start\n"
                      f" %offset =l shl %i, {shift}\n %a =l add %offset, %p\n"
                      f" {operation} %v, %a\n ret\n}}\n")
            declarations.append(f"extern void {name}(void *, uint64_t, {c_types[cls]});")
            for index in (-7, 0, 19):
                for value in (0, (1 << bits) - 1, (1 << (bits - 1)) + 1):
                    argument = f"UINT{bits}_C({value})"
                    if cls in ("s", "d"):
                        argument = f"value{bits}({argument})"
                    checks.append(
                        f"memset(arena, 0xa5, sizeof arena); "
                        f"{name}(arena + 513, UINT64_C({index % (1 << 64)}), {argument}); "
                        f"CHECK(stored({513 + (index << shift)}, {width}, UINT64_C({value})), {name});")

    il.append("export function l $indexed_shared(l %p, l %q, l %i) {\n@start\n"
              " %offset =l shl %i, 3\n %a =l add %p, %offset\n %b =l add %q, %offset\n"
              " %x =l loadl %a\n %y =l loadl %b\n %sum =l add %x, %y\n ret %sum\n}\n")
    declarations.append("extern uint64_t indexed_shared(void *, void *, uint64_t);")
    expected = (loaded_bits(520, 8, 64) + loaded_bits(1032, 8, 64)) % (1 << 64)
    checks += ["fill();", f"CHECK(indexed_shared(arena + 512, arena + 1024, 1) == UINT64_C({expected}), shared_index);"]

    # An address shared by a load, a store and optionally an opaque call must
    # retain every non-memory use while folding both eligible memory operands.
    for number, (store, load, cls, width) in enumerate((
            ("storeb", "loadub", "w", 1), ("storeh", "loaduh", "w", 2),
            ("storew", "loadw", "w", 4), ("storel", "loadl", "l", 8),
            ("stores", "loads", "s", 4), ("stored", "loadd", "d", 8))):
        shift = width.bit_length() - 1
        bits = 64 if cls in ("l", "d") else 32
        expected = int.from_bytes(b"\xa5" * width, "little")
        for escaped in (False, True):
            name = f"indexed_update_{number}_{int(escaped)}"
            escape = " call $opaque_address(l %a)\n" if escaped else ""
            il.append(f"export function {cls} ${name}(l %p, l %i, {cls} %v) {{\n@start\n"
                      f" %offset =l shl %i, {shift}\n %a =l add %offset, %p\n"
                      f" %old ={cls} {load} %a\n{escape} {store} %v, %a\n ret %old\n}}\n")
            declarations.append(f"extern {c_types[cls]} {name}(void *, uint64_t, {c_types[cls]});")
            indexes = [-7, -1, 0, 3, 19] + ([(1 << (64 - shift)) + 3] if shift else [])
            for index in indexes:
                offset = (index << shift) % (1 << 64)
                if offset >= 1 << 63:
                    offset -= 1 << 64
                value = (1 << bits) - 1
                argument = f"UINT{bits}_C({value})"
                if cls in ("s", "d"):
                    argument = f"value{bits}({argument})"
                call = f"{name}(arena + 513, UINT64_C({index % (1 << 64)}), {argument})"
                if cls in ("s", "d"):
                    call = f"bits{bits}({call})"
                checks.append(f"memset(arena, 0xa5, sizeof arena); "
                              f"CHECK((uint64_t)({call}) == UINT64_C({expected}), {name}_old); "
                              f"CHECK(stored({513 + offset}, {width}, UINT64_C({value})), {name}_new);")
                if escaped:
                    checks.append(f"CHECK(observed == arena + {513 + offset}, {name}_escape);")

    # Both the bases and indexes exceed the register set across an opaque call.
    spill = ["export function l $indexed_spills(l %p, l %indexes) {\n@start"]
    for index in range(40):
        spill += [f" %slot{index} =l add %p, {index * 8}",
                  f" %base{index} =l loadl %slot{index}",
                  f" %islot{index} =l add %indexes, {index * 8}",
                  f" %index{index} =l loadl %islot{index}"]
    spill.append(" call $opaque_address(l %p)")
    for index in range(40):
        spill += [f" %offset{index} =l shl %index{index}, 3",
                  f" %address{index} =l add %base{index}, %offset{index}",
                  f" %value{index} =l loadl %address{index}",
                  f" %sum{index} =l add {'0' if index == 0 else '%sum' + str(index - 1)}, %value{index}"]
    spill += [" ret %sum39", "}"]
    il.append("\n".join(spill))
    declarations.append("extern uint64_t indexed_spills(void **, int64_t *);")
    expected = sum(loaded_bits(2000 + i * 16 + (i % 3 - 1) * 8, 8, 64) for i in range(40)) % (1 << 64)
    checks += ["fill();", "void *indexed_bases[40]; int64_t indexes[40];",
               "for (int i = 0; i < 40; ++i) { indexed_bases[i] = arena + 2000 + i * 16; indexes[i] = i % 3 - 1; }",
               f"CHECK(indexed_spills(indexed_bases, indexes) == UINT64_C({expected}), indexed_spills);"]
    return il, declarations, checks


def fixture():
    il, declarations, checks = [], [], ["fill();"]
    count = 0
    loads = []
    for cls in ("w", "l"):
        loads += [(operation, cls, width, signed) for operation, width, signed in (
            ("loadsb", 1, True), ("loadub", 1, False), ("loadsh", 2, True),
            ("loaduh", 2, False), ("loadsw", 4, True), ("loaduw", 4, False))]
    loads += [("load", "w", 4, False), ("load", "l", 8, False),
              ("load", "s", 4, False), ("load", "d", 8, False)]
    c_types = {"w": "uint32_t", "l": "uint64_t", "s": "float", "d": "double"}
    for operation, cls, width, signed in loads:
        result_width = 64 if cls in ("l", "d") else 32
        for offset in offsets(width):
            name = f"memory_load_{count}"
            il.append(f"export function {cls} ${name}(l %p) {{\n@start\n"
                      f" %a =l add %p, {offset}\n %v ={cls} {operation} %a\n ret %v\n}}\n")
            declarations.append(f"extern {c_types[cls]} {name}(void *);")
            for base in (512, 513):
                expected = loaded_bits(base + offset, width, result_width, signed)
                value = f"{name}(arena + {base})"
                if cls in ("s", "d"):
                    value = f"bits{result_width}({value})"
                checks.append(f"CHECK((uint64_t)({value}) == UINT64_C({expected}), {name});")
            count += 1

    for operation, cls, width in (("storeb", "w", 1), ("storeh", "w", 2),
                                   ("storew", "w", 4), ("storel", "l", 8),
                                   ("stores", "s", 4), ("stored", "d", 8)):
        argument_width = 64 if cls in ("l", "d") else 32
        values = [0, 2 ** argument_width - 1, 2 ** (argument_width - 1),
                  0x7ff8000000000001 if argument_width == 64 else 0x7fc00001]
        for offset in offsets(width):
            name = f"memory_store_{count}"
            il.append(f"export function ${name}(l %p, {cls} %v) {{\n@start\n"
                      f" %a =l add %p, {offset}\n {operation} %v, %a\n ret\n}}\n")
            declarations.append(f"extern void {name}(void *, {c_types[cls]});")
            for base in (512, 513):
                for value in values:
                    argument = f"UINT{argument_width}_C({value})"
                    if cls in ("s", "d"):
                        argument = f"value{argument_width}({argument})"
                    checks.append(f"memset(arena, 0xa5, sizeof arena); {name}(arena + {base}, {argument}); "
                                  f"CHECK(stored({base + offset}, {width}, UINT64_C({value})), {name});")
            count += 1

    # Escaped addresses retain their non-memory calculation. Frame addresses
    # and symbols remain on the ordinary path.
    il += ["export function l $memory_variable(l %p, l %offset) {\n@start\n"
           " %a =l add %p, %offset\n %v =l loadl %a\n ret %v\n}\n",
           "export function l $memory_shared(l %p) {\n@start\n %a =l add %p, 8\n"
           " %v =l loadl %a\n call $opaque_address(l %a)\n ret %v\n}\n",
           "export function l $memory_stack() {\n@start\n %p =l alloc8 32\n"
           " call $write_stack(l %p)\n %a =l add %p, 8\n %v =l loadl %a\n ret %v\n}\n",
           "export data $memory_symbol = align 8 { l 123, l 456 }\n",
           "export function l $memory_global() {\n@start\n %a =l add $memory_symbol, 8\n"
           " %v =l loadl %a\n ret %v\n}\n"]
    declarations += ["extern uint64_t memory_variable(void *, uint64_t), memory_shared(void *);",
                     "extern uint64_t memory_stack(void), memory_global(void), memory_spills(void *);"]
    checks.append("fill();")
    for offset in (-256, -1, 0, 1, 8, 32768):
        expected = loaded_bits(512 + offset, 8, 64)
        checks.append(f"CHECK(memory_variable(arena + 512, (uint64_t)({offset})) == UINT64_C({expected}), variable);")
    expected = loaded_bits(520, 8, 64)
    checks += [f"CHECK(memory_shared(arena + 512) == UINT64_C({expected}) && observed == arena + 520, shared);",
               "CHECK(memory_stack() == UINT64_C(0x123456789abcdef0), stack);",
               "CHECK(memory_global() == 456, global);"]

    # Keep more base pointers live across an opaque call than fit in registers.
    # This tests shared spilling/reloading of bases inside memory operands.
    spill = ["export function l $memory_spills(l %p) {\n@start"]
    for index in range(40):
        spill += [f" %slot{index} =l add %p, {index * 8}", f" %base{index} =l loadl %slot{index}"]
    spill.append(" call $opaque_address(l %p)")
    for index in range(40):
        spill += [f" %field{index} =l add %base{index}, 8", f" %value{index} =l loadl %field{index}",
                  f" %sum{index} =l add {'0' if index == 0 else '%sum' + str(index - 1)}, %value{index}"]
    spill += [" ret %sum39", "}"]
    il.append("\n".join(spill))
    expected = sum(loaded_bits(2008 + index * 16, 8, 64) for index in range(40)) % (2 ** 64)
    checks += ["void *pointers[40]; for (int i = 0; i < 40; ++i) pointers[i] = arena + 2000 + i * 16;",
               f"CHECK(memory_spills(pointers) == UINT64_C({expected}), spills);"]
    indexed_il, indexed_declarations, indexed_checks = indexed_fixture(loads, c_types)
    il += indexed_il
    declarations += indexed_declarations
    checks += indexed_checks
    c = """#include <stdint.h>
#include <stdio.h>
#include <string.h>
static unsigned char arena[65536];
static void *observed;
void opaque_address(void *p) { observed = p; }
void write_stack(void *p) { uint64_t x = UINT64_C(0x123456789abcdef0); memcpy((char *)p + 8, &x, 8); }
static void fill(void) { for (unsigned i = 0; i < sizeof arena; ++i) arena[i] = (i * 37 + i / 11) & 255; }
static int stored(unsigned start, unsigned width, uint64_t value) {
    for (unsigned i = 0; i < sizeof arena; ++i) {
        unsigned char expected = i >= start && i < start + width ? value >> ((i - start) * 8) : 0xa5;
        if (arena[i] != expected) return 0;
    }
    return 1;
}
static uint32_t bits32(float value) { uint32_t bits; memcpy(&bits, &value, 4); return bits; }
static uint64_t bits64(double value) { uint64_t bits; memcpy(&bits, &value, 8); return bits; }
static float value32(uint32_t bits) { float value; memcpy(&value, &bits, 4); return value; }
static double value64(uint64_t bits) { double value; memcpy(&value, &bits, 8); return value; }
#define CHECK(condition, name) do { if (!(condition)) { fprintf(stderr, #name " failed\\n"); return 1; } ++checked; } while (0)
"""
    c += "\n".join(declarations) + "\nint main(void) { unsigned checked = 0;\n"
    c += "\n".join(checks) + '\nprintf("%u memory and fallback checks passed\\n", checked); return 0;\n}\n'
    return "\n".join(il), c


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", required=True, type=Path)
    parser.add_argument("--baseline", type=Path)
    args = parser.parse_args()
    qbe = args.qbe.resolve()
    check_capability(qbe)
    with tempfile.TemporaryDirectory(prefix="qbe-memory-") as temporary:
        root = Path(temporary)
        il, c = fixture()
        source = root / "memory.ssa"
        source.write_text(il)
        driver = root / "driver.c"
        driver.write_text(c)
        for target in ("arm64", "arm64_apple", "amd64_sysv", "amd64_apple"):
            assembly = root / (target + ".s")
            subprocess.run([str(qbe), "-t", target, "-o", str(assembly), str(source)], check=True)
            if args.baseline and target.startswith("amd64"):
                previous = subprocess.check_output([str(args.baseline.resolve()), "-t", target, str(source)])
                assert assembly.read_bytes() == previous, target + " emission changed"
        machine = platform.machine()
        assert machine in ("aarch64", "arm64", "x86_64", "AMD64"), machine
        architecture = "arm64" if machine in ("aarch64", "arm64") else "amd64"
        target = architecture + ("_apple" if platform.system() == "Darwin" else "_sysv" if architecture == "amd64" else "")
        binary = root / "memory"
        subprocess.run(["cc", "-std=c11", "-O0", str(driver), str(root / (target + ".s")), "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)


if __name__ == "__main__":
    main()
