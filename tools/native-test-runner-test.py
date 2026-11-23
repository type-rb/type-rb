#!/usr/bin/env python3
"""Qualify project test execution without counting unrelated failures as parity."""

import argparse
import importlib.util
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "fixtures/projects/native-test-manifest.json"
spec = importlib.util.spec_from_file_location("project_scenarios", ROOT / "tools/project-scenarios.py")
scenarios = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scenarios)


def load_manifest(path=MANIFEST):
    manifest = json.loads(path.read_text())
    historical = json.loads((ROOT / "fixtures/projects/m2-manifest.json").read_text())
    required = {row["id"] for row in historical["scenarios"]} - {"test-name-filter"}
    actual = [row["id"] for row in manifest["scenarios"]]
    if len(actual) != len(required) or set(actual) != required:
        raise ValueError("required M2 scenarios changed")
    if manifest["typeRBRevision"] != (ROOT / "TYPE_RB_REVISION").read_text().strip():
        raise ValueError("test manifest requires an explicit current-pin replay")
    deferrals = manifest.get("deferrals", [])
    if len(deferrals) != 1 or deferrals[0].get("id") != "test-name-filter" or deferrals[0].get("issue") != "https://github.com/type-rb/type-rb-native/issues/643":
        raise ValueError("only the approved test-name-filter deferral is allowed")
    paths = set()
    for row in manifest["scenarios"]:
        paths.update(str(p.relative_to(ROOT)) for p in (ROOT / row["fixture"]).rglob("*") if p.is_file())
    if set(manifest["fixtureHashes"]) != paths:
        raise ValueError("test fixture hash inventory is incomplete")
    for relative, expected in manifest["fixtureHashes"].items():
        if scenarios.digest(ROOT / relative) != expected:
            raise ValueError("test fixture changed: " + relative)
    # Historical outcomes only change for reviewed file selection and the
    # accepted portable display contract; origins, statuses and fixtures stay exact.
    originals = {row["id"]: row for row in historical["scenarios"]}
    for row in manifest["scenarios"]:
        original = originals[row["id"]]
        expected = json.loads(json.dumps(original["reference"]))
        steps = original["steps"]
        if row["id"] == "test-positional-duplicate-names":
            steps = [["test", "src/first_test.trb"]]
            expected[0]["args"] = steps[0]
        if row["id"] == "assert-structural-failure":
            expected[0]["stdout"] = expected[0]["stdout"].replace(
                "application.Point{X:1, Y:2}", "Point(x: 1, y: 2)").replace(
                "application.Point{X:1, Y:3}", "Point(x: 1, y: 3)")
        if row["reference"] != expected or row["steps"] != steps or row["fixture"] != original["fixture"] or row["cwd"] != original["cwd"] or row["expectation"] != original["expectation"]:
            raise ValueError("reviewed scenario changed: " + row["id"])
    return manifest


def canonical_diagnostic(text):
    text = scenarios.without_go_exit(text).strip()
    text = re.sub(r"^trbn?: ", "compiler: ", text)
    return re.sub(r"error\[TRBN?\d+\]", "error", text)


def compare(row, observed):
    expected = row["reference"]
    if len(observed) != len(expected):
        return ["execution did not reach exactly the required steps"]
    errors = []
    for index, (want, got) in enumerate(zip(expected, observed)):
        label = f"step {index + 1}"
        if got["args"] != want["args"] or got["code"] != want["code"]:
            errors.append(label + ": command or exact exit status differs")
        if row["id"] == "test-json-reporter":
            try:
                same = [json.loads(s) for s in got["stdout"].splitlines()] == [json.loads(s) for s in want["stdout"].splitlines()]
            except (ValueError, TypeError):
                same = False
        else:
            same = got["stdout"] == want["stdout"]
        if not same:
            errors.append(label + ": output or assertion origin differs")
        wanted_error = canonical_diagnostic(want["stderr"])
        actual_error = canonical_diagnostic(got["stderr"])
        if row["id"] == "test-selection-invalid":
            # The OS error text is host-specific. Require the selected path and
            # missing-file category; a parser or unavailable-provider error fails.
            same_error = bool(re.fullmatch(r"compiler: test path src/missing_test\.trb: (?:stat <fixture>/app/src/missing_test\.trb: )?(?:no such file or directory|path does not exist)", actual_error, re.IGNORECASE))
        else:
            same_error = actual_error == wanted_error
        if not same_error:
            errors.append(label + ": required diagnostic or original source location differs")
    return errors


def deferred_control(binary, flag):
    row = {"id": "deferred-name-selection", "fixture": "fixtures/projects/test-basic-pass", "cwd": "app", "steps": [["test", flag, "Calculator"]]}
    observations = scenarios.replay(binary, row, True)
    result = observations[0]
    passed = result["code"] == 1 and result["stdout"] == "" and flag in result["stderr"] and "test name selection is deferred" in result["stderr"]
    return {"flag": flag, "status": "pass" if passed else "fail", "observed": observations}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    role = parser.add_mutually_exclusive_group(required=True)
    role.add_argument("--native", type=Path)
    role.add_argument("--reference", type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    manifest = load_manifest()
    binary = (args.native or args.reference).resolve()
    rows = []
    for row in manifest["scenarios"]:
        observed = scenarios.replay(binary, row, args.native is not None)
        errors = compare(row, observed) if args.native else ([] if observed == row["reference"] else ["exact reference observations changed"])
        rows.append({"id": row["id"], "status": "fail" if errors else "pass", "mismatches": errors, "observed": observed})
    controls = [deferred_control(binary, flag) for flag in ("-t", "--test-name-pattern")] if args.native else []
    passed = sum(row["status"] == "pass" for row in rows)
    report = {"schemaVersion": 1, "role": "native" if args.native else "reference", "compilerSha256": scenarios.digest(binary), "manifestSha256": scenarios.digest(MANIFEST), "referenceRevision": manifest["typeRBRevision"], "required": len(rows), "passed": passed, "scenarios": rows, "deferrals": manifest["deferrals"], "companionControls": controls}
    encoded = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.report:
        args.report.write_text(encoded)
    print(encoded, end="")
    print(f"Native test-runner contract: {passed}/{len(rows)}; approved deferrals are excluded", file=sys.stderr)
    return int(passed != len(rows) or any(row["status"] != "pass" for row in controls))


if __name__ == "__main__":
    raise SystemExit(main())
