#!/usr/bin/env python3
"""Replay the frozen project scenario manifests with the reference or Native.

The reference replay requires the recorded reference outcomes exactly, so a
pin update cannot change them silently. The Native replay follows each
manifest's mode rule: loading and checking steps use the fixture as written,
and execution steps run the same source with only the mode changed to trb.
Native results are compared with the recorded reference outcomes under each
scenario's expectation. Checking steps compare acceptance, execution steps
compare exit status and stdout, and diagnostic wording is not compared. A
Native rejection caused by missing support never matches a reference rejection.
"""
import argparse
import concurrent.futures
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MANIFESTS = ROOT / "fixtures" / "projects"
EXECUTION = {"run", "build", "test"}
EXPECTATIONS = {"match-reference", "match-reference-check", "match-reference-status", "triage"}
GO_EXIT_STATUS = re.compile(r"^exit status \d+$")
TEST_STATUS = re.compile(r"^(PASS|FAIL) |^\d+ test\(s\), \d+ failure\(s\)$")
# A Native rejection caused by missing support is not parity with a reference
# rejection, even though both commands fail.
UNSUPPORTED = re.compile(r"unknown command|is not implemented|unsupported project configuration field|"
                         r"is not available for mode")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(selected, current_reference=False):
    paths = [Path(p) for p in selected] if selected else sorted(MANIFESTS.glob("m*-manifest.json"))
    if current_reference:
        paths = [MANIFESTS / "reference-test-manifest.json"
                 if path.resolve() == (MANIFESTS / "m2-manifest.json").resolve()
                 else path for path in paths]
    manifests = []
    for path in paths:
        manifest = json.loads(path.read_text())
        if path.name == "reference-test-manifest.json":
            if (manifest["typeRBRevision"] != (ROOT / "TYPE_RB_REVISION").read_text().strip()
                    or manifest["historicalManifestSha256"] != digest(MANIFESTS / "m2-manifest.json")):
                raise ValueError("current reference test scenarios require a reviewed pin replay")
        for scenario in manifest["scenarios"]:
            if scenario["expectation"] not in EXPECTATIONS:
                raise ValueError(f"{scenario['id']}: unknown expectation {scenario['expectation']}")
        manifests.append((path, manifest))
    return manifests


def environment(home):
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": home, "TMPDIR": home}
    for name in ("GOCACHE", "GOMODCACHE", "GOFLAGS", "GOTOOLCHAIN", "GOROOT"):
        if name in os.environ:
            env[name] = os.environ[name]
    return env


def run_step(binary, root, cwd, args):
    # Toolchains such as Go may still write telemetry below the isolated home
    # after the command exits, so leftover files never fail a replay.
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as home:
        try:
            proc = subprocess.run([str(binary), *args], cwd=root / cwd, capture_output=True,
                                  timeout=900, env=environment(home))
            code, stdout, stderr = proc.returncode, proc.stdout, proc.stderr
        except subprocess.TimeoutExpired as error:
            code, stdout, stderr = None, error.stdout or b"", error.stderr or b""
    def norm(data):
        text = data.decode("utf-8", "backslashreplace")
        return text.replace(str(root.resolve()), "<fixture>").replace(str(root), "<fixture>")
    return {"args": args, "code": code, "stdout": norm(stdout), "stderr": norm(stderr)}


def with_trb_mode(root, cwd):
    config = root / cwd / "trbconfig.jsonc"
    if config.is_file():
        text = config.read_text()
        config.write_text(re.sub(r'("mode"\s*:\s*)"[^"]*"', r'\1"trb"', text, count=1))


def replay(binary, scenario, native):
    fixture = ROOT / scenario["fixture"]
    results = []
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as tmp:
        written = Path(tmp) / "written" / scenario["id"]
        shutil.copytree(fixture, written)
        substituted = Path(tmp) / "trb" / scenario["id"]
        shutil.copytree(fixture, substituted)
        with_trb_mode(substituted, scenario["cwd"])
        for step in scenario["steps"]:
            root = substituted if native and step[0] in EXECUTION else written
            if native and step[0] == "install":
                # Installation state must be visible to both copies.
                result = run_step(binary, written, scenario["cwd"], step)
                run_step(binary, substituted, scenario["cwd"], step)
            else:
                result = run_step(binary, root, scenario["cwd"], step)
            results.append(result)
            if result["code"] != 0:
                break
    return results


def status_lines(text):
    return [line for line in text.splitlines() if TEST_STATUS.search(line)]


def without_go_exit(text):
    return "\n".join(line for line in text.splitlines() if not GO_EXIT_STATUS.match(line))


def compare_native(scenario, observed):
    expectation = scenario["expectation"]
    if expectation == "triage":
        return "triage", []
    mismatches = []
    reference = scenario["reference"]
    for index, expected in enumerate(reference):
        command = expected["args"][0]
        if expectation == "match-reference-check" and command != "check":
            continue
        if index >= len(observed):
            mismatches.append(f"step {index + 1} ({command}) was not reached")
            break
        actual = observed[index]
        if (expected["code"] == 0) != (actual["code"] == 0):
            mismatches.append(f"step {index + 1} ({command}) exit {actual['code']}, reference {expected['code']}")
            continue
        if actual["code"] != 0 and UNSUPPORTED.search(actual["stderr"]):
            reason = actual["stderr"].strip().splitlines()[0][:120]
            mismatches.append(f"step {index + 1} ({command}) rejected for missing support: {reason}")
            continue
        if command not in EXECUTION:
            continue
        if command == "test":
            if expectation == "match-reference-status":
                same = status_lines(expected["stdout"]) == status_lines(actual["stdout"])
            else:
                same = without_go_exit(expected["stdout"]) == without_go_exit(actual["stdout"])
        else:
            same = expected["code"] == actual["code"] and expected["stdout"] == actual["stdout"]
        if not same:
            mismatches.append(f"step {index + 1} ({command}) output differs")
    return ("pass" if not mismatches else "fail"), mismatches


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    role = parser.add_mutually_exclusive_group(required=True)
    role.add_argument("--reference", type=Path)
    role.add_argument("--native", type=Path)
    parser.add_argument("--manifest", action="append", default=[])
    parser.add_argument("--scenario")
    parser.add_argument("--current-reference", action="store_true",
                        help="use reviewed current-pin reference outcomes without rewriting historical manifests")
    parser.add_argument("--require", action="store_true", help="fail when a Native scenario does not pass")
    parser.add_argument("--jobs", type=int, default=1, choices=range(1, 9))
    args = parser.parse_args()
    if args.current_reference and not args.reference:
        parser.error("--current-reference is only for reference qualification")
    binary = (args.reference or args.native).resolve()
    native = args.native is not None
    report = {"schemaVersion": 1, "role": "native" if native else "reference",
              "compilerSha256": digest(binary), "manifests": [], "failures": []}
    for path, manifest in load(args.manifest, args.current_reference):
        scenarios = [s for s in manifest["scenarios"] if args.scenario in (None, s["id"])]
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as pool:
            observed = list(pool.map(lambda s: replay(binary, s, native), scenarios))
        entries, passed = [], 0
        for scenario, steps in zip(scenarios, observed):
            if native:
                status, mismatches = compare_native(scenario, steps)
            else:
                status = "pass" if steps == scenario["reference"] else "fail"
                mismatches = [] if status == "pass" else ["reference outcomes differ from the manifest"]
            passed += status == "pass"
            if status == "fail":
                report["failures"].append(f"{manifest['milestone']}:{scenario['id']}")
            entries.append({"id": scenario["id"], "expectation": scenario["expectation"],
                            "status": status, "mismatches": mismatches, "observed": steps})
        report["manifests"].append({"milestone": manifest["milestone"], "typeRBRevision": manifest["typeRBRevision"],
                                    "passed": passed, "total": len(scenarios), "scenarios": entries})
    print(json.dumps(report, ensure_ascii=False, indent=2))
    for entry in report["manifests"]:
        print(f"{entry['milestone']}: {entry['passed']} / {entry['total']} scenarios pass", file=sys.stderr)
    if report["failures"] and (not native or args.require):
        print("Project scenario failures: " + ", ".join(report["failures"]), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
