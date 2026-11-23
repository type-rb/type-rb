#!/usr/bin/env python3
"""Acquire pinned QBE with the reviewed IL, selector and code-motion fixes."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
QBE_CFLAGS = "-std=c99 -O2 -g -Wall -Wextra -Wpedantic"

MOTION_PROBE = """export function l $motion(l %a, l %b, l %c, l %n) {
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

ADDRESS_MOTION_PROBE = """export function l $address_motion(l %p, l %i, l %n) {
@start
 %offset =l shl %i, 3
 %address =l add %p, %offset
 jmp @loop
@loop
 %count =l phi @start 0, @body %next
 %sum =l phi @start 0, @body %newsum
 %more =w csltl %count, %n
 jnz %more, @body, @done
@body
 %value =l loadl %address
 %newsum =l add %sum, %value
 %next =l add %count, 1
 jmp @loop
@done
 ret %sum
}
"""

COPY_CYCLE_PROBE = """export function l $copy_cycle(l %a, l %b, l %n) {
@start
 jmp @loop
@loop
 %i =l phi @start 0, @body %next
 %left =l phi @start %a, @body %right
 %right =l phi @start %b, @body %left
 %more =w csltl %i, %n
 jnz %more, @body, @done
@body
 %next =l add %i, 1
 jmp @loop
@done
 ret %i
}
"""


# GVN rescues the second branch address into the entry block before GCM.
ORDERED_CALLS_PROBE = """export function l $ordered_calls(l %p) {
@start
 jmp @init
@init
 call $change_order(l %p)
 %r =l call $inspect_order(l %p)
 jnz 1, @first, @second
@first
 %one =l add %r, 8
 %a =l loadl %one
 ret %a
@second
 %two =l add %r, 8
 %b =l loadl %two
 ret %b
}
"""


def check_ordered_effects(binary, target="arm64"):
    result = subprocess.run([str(binary), "-t", target, "-d", "G"], input=ORDERED_CALLS_PROBE,
                            text=True, capture_output=True, timeout=30)
    moved = result.stderr.partition("> After GCM:")[2]
    calls = re.findall(r"\bcall \$([a-z_]+)", moved)
    if result.returncode or calls != ["change_order", "inspect_order"]:
        raise ValueError("QBE lacks the required ordered side effects")


def check_copy_liveness(binary):
    result = subprocess.run([str(binary), "-t", "arm64", "-d", "G"], input=COPY_CYCLE_PROBE,
                            text=True, capture_output=True, timeout=30)
    moved = result.stderr.partition("> After GCM:")[2]
    if (result.returncode or not re.search(r"\bret[wlsd]? ", moved)
            or re.search(r"%(?:left|right)(?:\.\d+)?\b", moved)):
        raise ValueError("QBE lacks the required dead copy cycle elimination")


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def backend_identity(root=ROOT):
    backend = json.loads((root / "tools/qbe/backend.json").read_text())
    if set(backend) != {"name", "version", "sourceURL", "sourceSHA256", "variant", "patches"}:
        raise ValueError("invalid QBE identity shape")
    if backend["name"] != "QBE" or backend["version"] != "1.3" or backend["variant"] != "typerb-late-product-v1":
        raise ValueError("unsupported QBE identity")
    if not backend["sourceURL"].startswith("https://") or not re.fullmatch(r"[0-9a-f]{64}", backend["sourceSHA256"]):
        raise ValueError("invalid QBE source identity")
    paths = ["tools/qbe/sqrt.patch", "tools/qbe/arm64-immediates.patch", "tools/qbe/arm64-memory.patch", "tools/qbe/gcm.patch", "tools/qbe/fma.patch"]
    if len(backend["patches"]) != len(paths):
        raise ValueError("invalid QBE patch set")
    for patch, path in zip(backend["patches"], paths):
        if set(patch) != {"path", "sha256"} or patch["path"] != path:
            raise ValueError("invalid QBE patch identity")
        if digest(root / patch["path"]) != patch["sha256"]:
            raise ValueError("QBE patch digest differs")
    return backend


def check_contraction(binary):
    source = ("export function d $contract(d %a, d %b, d %c) {\n@start\n"
              " %product =d mulc %a, %b\n %result =d add %product, %c\n ret %result\n}\n")
    for target in ("arm64", "arm64_apple", "amd64_sysv"):
        for permitted in (True, False):
            probe = source if permitted else source.replace(" mulc ", " mul ")
            result = subprocess.run([str(binary), "-t", target], input=probe,
                                    text=True, capture_output=True, timeout=30)
            fused = bool(re.search(r"\bfmadd\b", result.stdout))
            expected = permitted and target.startswith("arm64")
            if result.returncode or fused != expected:
                raise ValueError(f"QBE lacks the required {target} Float contraction behavior")


def check_capability(binary):
    source = ("export function d $probe(d %x) {\n@start\n %y =d sqrt %x\n ret %y\n}\n"
              "export function l $integer(l %x) {\n@start\n %a =l add %x, 7\n"
              " %b =l and %a, 65535\n %c =l sar %b, 65\n ret %c\n}\n"
              "export function l $memory(l %p) {\n@start\n %a =l add %p, 8\n"
              " %x =l loadl %a\n %b =l add %p, 16\n storel %x, %b\n ret %x\n}\n"
              "export function l $indexed(l %p, l %i, l %j) {\n@start\n"
              " %offset =l shl %i, 3\n %a =l add %p, %offset\n %x =l loadl %a\n"
              " %small =l shl %j, 2\n %b =l add %p, %small\n storew %x, %b\n ret %x\n}\n"
              "export function l $shared_memory(l %p, l %i, l %v) {\n@start\n"
              " %offset =l shl %i, 3\n %a =l add %p, %offset\n %old =l loadl %a\n"
              " storel %v, %a\n ret %old\n}\n")
    for target, instruction in (("arm64_apple", "fsqrt"), ("arm64", "fsqrt"), ("amd64_sysv", "sqrtsd")):
        result = subprocess.run([str(binary), "-t", target], input=source, text=True,
                                capture_output=True, timeout=30)
        if result.returncode or instruction not in result.stdout:
            raise ValueError(f"QBE lacks the required {target} sqrt operation: {result.stderr.strip()}")
        if target.startswith("arm64"):
            for operation, constant in (("add", "7"), ("and", "65535"), ("asr", "1")):
                if not re.search(rf"\b{operation}\s+[^\n]*, #{constant}\s*$", result.stdout, re.MULTILINE):
                    raise ValueError(f"QBE lacks the required {target} integer immediate selection")
            for operation, offset in (("ldr", "8"), ("str", "16")):
                if not re.search(rf"\b{operation}\s+[^\n]*\[x\d+, #{offset}\]", result.stdout):
                    raise ValueError(f"QBE lacks the required {target} memory operand selection")
            for operation, shift in (("ldr", "3"), ("str", "2")):
                if not re.search(rf"\b{operation}\s+[^\n]*\[x\d+, x\d+, lsl #{shift}\]", result.stdout):
                    raise ValueError(f"QBE lacks the required {target} indexed memory operand selection")
            shared = re.split(r"(?m)^_?shared_memory:\s*$", result.stdout)
            if len(shared) != 2 or any(not re.search(
                    rf"\b{operation}\s+[^\n]*\[x\d+, x\d+, lsl #3\]", shared[1])
                    for operation in ("ldr", "str")):
                raise ValueError(f"QBE lacks the required {target} shared memory operand selection")
    result = subprocess.run([str(binary), "-t", "arm64", "-d", "G"], input=MOTION_PROBE,
                            text=True, capture_output=True, timeout=30)
    moved = result.stderr.partition("> After GCM:")[2]
    body = moved.partition("@body\n")[2].partition("@yes\n")[0]
    if result.returncode or not body or re.search(r"\b(?:csltl|cnel|and)\b", body):
        raise ValueError("QBE lacks the required invariant expression motion")
    result = subprocess.run([str(binary), "-t", "arm64", "-d", "G"], input=ADDRESS_MOTION_PROBE,
                            text=True, capture_output=True, timeout=30)
    moved = result.stderr.partition("> After GCM:")[2]
    body = moved.partition("@body\n")[2].partition("@done\n")[0]
    if result.returncode or not re.search(r"\bload %snk\.\d+", body):
        raise ValueError("QBE lacks the required loop address rematerialization")
    single = MOTION_PROBE.replace("%both =w and %one, %two", "%both =w copy %one")
    result = subprocess.run([str(binary), "-t", "arm64", "-d", "G"], input=single,
                            text=True, capture_output=True, timeout=30)
    moved = result.stderr.partition("> After GCM:")[2]
    body = moved.partition("@body\n")[2].partition("@yes\n")[0]
    if result.returncode or "csltl" not in body:
        raise ValueError("QBE lacks the required single comparison rematerialization")
    check_copy_liveness(binary)
    check_contraction(binary)
    check_ordered_effects(binary)


def build(output, archive=None):
    backend = backend_identity()
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    cc = Path(os.environ.get("CC", "/usr/bin/cc")).resolve()
    if not cc.is_file():
        raise ValueError("QBE C compiler is not an executable file")
    inputs = {"backend": backend, "builderSHA256": digest(__file__), "ccSHA256": digest(cc),
              "platform": sys.platform, "machine": os.uname().machine, "cflags": QBE_CFLAGS}
    receipt = output / "build-info.json"
    binary = output / "qbe"
    if receipt.is_file() and binary.is_file():
        previous = json.loads(receipt.read_text())
        if previous.get("inputs") == inputs and previous.get("binarySHA256") == digest(binary):
            return binary
    archive = Path(archive).resolve() if archive else output.parent / "qbe-1.3.tar.xz"
    with tempfile.TemporaryDirectory(prefix="qbe-build-", dir=output.parent) as temporary:
        stage = Path(temporary)
        if not archive.is_file():
            download = stage / "source.tar.xz"
            subprocess.run(["curl", "--fail", "--location", "--retry", "3", backend["sourceURL"],
                            "-o", str(download)], check=True)
            if digest(download) != backend["sourceSHA256"]:
                raise ValueError("QBE archive digest differs")
            archive.parent.mkdir(parents=True, exist_ok=True)
            os.replace(download, archive)
        if digest(archive) != backend["sourceSHA256"]:
            raise ValueError("QBE archive digest differs")
        subprocess.run(["tar", "-xf", str(archive), "-C", str(stage)], check=True)
        source = stage / "qbe-1.3"
        for patch in backend["patches"]:
            subprocess.run(["patch", "--batch", "--fuzz=0", "-p1", "-i", str(ROOT / patch["path"])],
                           cwd=source, check=True, stdout=sys.stderr)
        subprocess.run(["make", "-j2", f"CC={cc}", f"CFLAGS={QBE_CFLAGS}"], cwd=source, check=True, stdout=sys.stderr)
        check_capability(source / "qbe")
        output.mkdir(parents=True, exist_ok=True)
        staged_binary = stage / "published-qbe"
        shutil.copy2(source / "qbe", staged_binary)
        info = {"inputs": inputs, "binarySHA256": digest(staged_binary)}
        staged_receipt = stage / "build-info.json"
        staged_receipt.write_text(json.dumps(info, indent=2) + "\n")
        os.replace(staged_binary, binary)
        os.replace(staged_receipt, receipt)
    return binary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path, nargs="?")
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--check", type=Path)
    parser.add_argument("--identity", action="store_true")
    args = parser.parse_args()
    try:
        if args.identity:
            print(json.dumps(backend_identity(), sort_keys=True))
        elif args.check:
            backend_identity()
            check_capability(args.check.resolve())
        elif args.output:
            print(build(args.output, args.archive))
        else:
            parser.error("provide an output directory, --check or --identity")
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"qbe-toolchain: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
