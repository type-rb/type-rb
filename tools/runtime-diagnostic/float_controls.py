"""N-body attribution controls; never substitute them for benchmark results."""
import re
import subprocess

from measure import digest


def control_sources(source):
    start = source.index("func advance(")
    end = source.index("\nfunc nbody(", start)
    body = source[start:end]

    def replace_once(text, before, after):
        if text.count(before) != 1:
            raise ValueError("Float control source shape changed")
        return text.replace(before, after)

    def separately_rounded(text, native_order):
        text = replace_once(text, "dx*dx + dy*dy + dz*dz",
                            "float64(dx*dx) + float64(dy*dy) + float64(dz*dz)")
        for axis, delta in zip("xyz", ("dx", "dy", "dz")):
            for index, mass in (("i", "mi"), ("j", "mj")):
                product = f"{delta} * {mass}"
                rounded = f"float64({product})"
                if native_order:
                    product = f"({delta} * bodies[{index}].mass) * mag"
                    rounded = f"float64(float64({delta} * bodies[{index}].mass) * mag)"
                text = replace_once(text, product, rounded)
            product = f"dt * bodies[i].v{axis}"
            text = replace_once(text, product, f"float64({product})")
        return text

    native = body
    for index, mass in (("i", "mi"), ("j", "mj")):
        native = replace_once(native, f"         {mass} := bodies[{index}].mass * mag\n", "")
        for delta in ("dx", "dy", "dz"):
            native = replace_once(native, f"{delta} * {mass}",
                                  f"({delta} * bodies[{index}].mass) * mag")
    bodies = {
        "control-go-original": body,
        "control-go-separate": separately_rounded(body, False),
        "control-go-native-order-fused": native,
        "control-go-native-order-separate": separately_rounded(native, True),
    }
    return {role: source[:start] + changed + source[end:] for role, changed in bodies.items()}


def prepare_controls(source, directory, original, record, write):
    artifacts = {}
    for role, text in control_sources(source).items():
        target = directory / role
        target.mkdir()
        code, program = target / "pure-go.go", target / "program"
        code.write_text(text)
        record(["go", "build", "-trimpath", "-o", program, code], target / "build", 120,
               kind="attribution-build", case="n-body", role=role)
        if any((target / "build" / stream).stat().st_size for stream in ("stdout", "stderr")):
            raise ValueError("Unexpected Float control build output")
        assembly = subprocess.check_output(["objdump", "-d", "--disassemble=main.advance", program]).decode()
        (target / "advance.s").write_text(assembly)
        fused = len(re.findall(r"\b(?:fmadd|fmsub|fnmadd|fnmsub)\b", assembly))
        expected = 0 if role.endswith("separate") else 11
        if "main.advance" not in assembly or fused != expected:
            raise ValueError("Float control instruction sequence differs")
        identity = {"sha256": digest(program), "bytes": program.stat().st_size}
        if role == "control-go-original" and identity != original:
            raise ValueError("Float control original differs from the retained Pure Go binary")
        artifacts[role] = identity
        write(target / "attribution.json", {
            "sourceSHA256": digest(code), "assemblySHA256": digest(target / "advance.s"),
            "fusedAdvanceOperations": fused, "artifact": identity,
            "scope": "Rounding and multiplication-order attribution only, not qualification",
        })
    return artifacts
