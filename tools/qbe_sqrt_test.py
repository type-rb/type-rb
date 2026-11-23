#!/usr/bin/env python3
"""Check the external QBE sqrt operation against the host scalar libm ABI."""
import argparse
from pathlib import Path
import platform
import subprocess
import tempfile

from qbe_toolchain import check_capability

ROOT = Path(__file__).resolve().parent.parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--qbe", required=True, type=Path)
    args = parser.parse_args()
    qbe = args.qbe.resolve()
    check_capability(qbe)
    source = ROOT / "tools/qbe/tests/operation.ssa"
    targets = {"arm64": "fsqrt", "arm64_apple": "fsqrt", "amd64_sysv": "sqrtsd",
               "amd64_apple": "sqrtsd", "amd64_win": "sqrtsd", "rv64": "fsqrt.d"}
    with tempfile.TemporaryDirectory(prefix="native-qbe-sqrt-") as temporary:
        root = Path(temporary)
        for target, instruction in targets.items():
            assembly = root / (target + ".s")
            subprocess.run([str(qbe), "-t", target, "-o", str(assembly), str(source)], check=True)
            assert instruction in assembly.read_text(), target
        machine = platform.machine()
        if machine in ("arm64", "aarch64"):
            target = "arm64_apple" if platform.system() == "Darwin" else "arm64"
        elif machine in ("x86_64", "AMD64"):
            target = "amd64_apple" if platform.system() == "Darwin" else "amd64_sysv"
        else:
            raise ValueError(f"unsupported test host: {machine}")
        binary = root / "edges"
        subprocess.run(["cc", "-std=c11", "-O0", "-fno-builtin", "-frounding-math",
                        str(ROOT / "tools/qbe/tests/edge-driver.c"), str(root / (target + ".s")),
                        "-lm", "-o", str(binary)], check=True)
        subprocess.run([str(binary)], check=True)
    for operation in ("%y =w sqrt %x", "%y =l sqrt %x", "%y =d sqrt %x, %x"):
        result = subprocess.run([str(qbe)], input="function d $bad(d %x) {\n@start\n " +
                                operation + "\n ret %x\n}\n", text=True, capture_output=True)
        assert result.returncode != 0, operation
    print("QBE sqrt target emission, scalar edge behavior and invalid forms passed")


if __name__ == "__main__":
    main()
