#!/usr/bin/env python3
"""Observe or verify small, path-specific ordinary language cases.

Observation never rewrites expectations. A changed observation needs review;
known rejected cases are coverage gaps, not successful conformance tests.
"""
import argparse
import hashlib
import html
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


PATHS = ("check", "build", "execute", "repl")


def validate_outcomes(expected):
    if not isinstance(expected, dict) or set(expected) != set(PATHS):
        raise ValueError("missing reviewed path expectations")
    for action, outcome in expected.items():
        if action == "execute" and outcome is None:
            continue
        if not isinstance(outcome, dict):
            raise ValueError("invalid path outcome")
        fields = set(outcome) - {"invalidUtf8"}
        prefix = fields == {"code", "stdout", "stderrFirstLine"}
        if (fields != {"code", "stdout", "stderr"} and not prefix
                or type(outcome.get("code")) is not int
                or not 0 <= outcome["code"] <= 255
                or not all(isinstance(value, str) for key, value in outcome.items() if key not in ("code", "invalidUtf8"))
                or prefix and (action != "execute" or outcome["code"] == 0
                               or not outcome["stderrFirstLine"]
                               or "\n" in outcome["stderrFirstLine"])):
            raise ValueError("invalid path outcome")
        if "invalidUtf8" in outcome:
            raw = outcome["invalidUtf8"]
            if (not isinstance(raw, dict) or not raw or not set(raw).issubset({"stdout", "stderr"})
                    or not all(isinstance(value, str) and re.fullmatch(r"(?:[a-f0-9]{2})+", value)
                               for value in raw.values()) or prefix):
                raise ValueError("invalid UTF-8 evidence")
    if (expected["execute"] is None) != (expected["build"]["code"] != 0):
        raise ValueError("execution must follow a successful build")


def expected_native(case):
    return case["native"]


def outcome_matches(actual, expected):
    if actual is None or expected is None:
        return actual is expected
    if actual.get("timedOut"):
        return False
    if "stderrFirstLine" in expected:
        return (actual["code"] == expected["code"] and actual["stdout"] == expected["stdout"]
                and "invalidUtf8" not in actual
                and actual["stderr"].splitlines()[:1] == [expected["stderrFirstLine"]])
    return actual == expected


def outcomes_match(actual, expected):
    return all(outcome_matches(actual[path], expected[path]) for path in PATHS)


def execution_expectations_match(left, right):
    if left is None or right is None:
        return left is right
    if left.get("timedOut") or right.get("timedOut"):
        return False
    # A reviewed first-line contract omits unstable panic stack frames. Compare
    # its full-stderr counterpart with that contract, not with its JSON keys.
    if "stderrFirstLine" in left and "stderrFirstLine" not in right:
        return outcome_matches(right, left)
    if "stderrFirstLine" in right and "stderrFirstLine" not in left:
        return outcome_matches(left, right)
    return left == right


def parity_gaps(case):
    native, reference = case["native"], case["reference"]
    gaps = []
    for path in ("check", "build"):
        if (native[path]["code"] == 0) != (reference[path]["code"] == 0):
            gaps.append(path)
    # Frontend diagnostic identities differ deliberately between compilers.
    # Runtime and REPL behavior are compared separately from check/build wording.
    if not execution_expectations_match(native["execute"], reference["execute"]):
        gaps.append("execute")
    left, right = native["repl"], reference["repl"]
    rejected_as_reference = (bool(left["stderr"]) and bool(right["stderr"])
                             and left["code"] == right["code"]
                             and left["stdout"] == right["stdout"]
                             and "invalidUtf8" not in left)
    if left != right and not rejected_as_reference:
        gaps.append("repl")
    return gaps


def valid_case_file(name, project_check=False):
    if not isinstance(name, str) or name == "main.trb":
        return False
    if any(part in ("", ".", "..") for part in name.split("/")):
        return False
    if re.fullmatch(r"[a-z][a-z0-9_/-]*\.trb", name):
        return True
    # Bounded text inputs for filesystem scenarios, separate from source.
    if re.fullmatch(r"fixtures/[a-z][a-z0-9_-]*\.txt", name):
        return True
    # File routes include Unicode, bracket parameters and punctuation. They
    # remain relative ordinary files below routes/, never traversal or escapes.
    return (project_check and name.startswith("routes/") and name.endswith(".trb")
            and not re.search(r"[\x00-\x1f\x7f\\]", name))


def validate(document):
    if (not isinstance(document, dict)
            or set(document) != {"schemaVersion", "referenceAstSha256", "features", "cases"}
            or type(document["schemaVersion"]) is not int or document["schemaVersion"] != 2
            or not isinstance(document["referenceAstSha256"], str)
            or not re.fullmatch(r"[a-f0-9]{64}", document["referenceAstSha256"])):
        raise ValueError("invalid language case registry")
    cases = document["cases"]
    if not isinstance(cases, list) or not cases:
        raise ValueError("empty language cases")
    identities = set()
    for case in cases:
        if (not isinstance(case, dict)
                or set(case) not in ({"id", "title", "declarations", "body", "files", "reference", "native"},
                                    {"id", "title", "declarations", "body", "files", "reference", "native", "projectCheck"})
                or ("projectCheck" in case and type(case["projectCheck"]) is not bool)
                or not all(isinstance(case[key], str) for key in ("id", "title", "declarations", "body"))
                or not re.fullmatch(r"[a-z][a-z0-9-]*", case["id"])
                or not case["title"] or not case["body"]):
            raise ValueError("invalid language case fields")
        if case["id"] in identities:
            raise ValueError("duplicate language case")
        identities.add(case["id"])
        if not isinstance(case["files"], dict):
            raise ValueError("invalid case files")
        for name, source in case["files"].items():
            if (not valid_case_file(name, case.get("projectCheck", False))
                    or not isinstance(source, str)):
                raise ValueError("invalid case file")
        validate_outcomes(case["reference"])
        validate_outcomes(case["native"])
    features = document["features"]
    if not isinstance(features, list) or not features:
        raise ValueError("empty feature inventory")
    feature_ids, nodes, covered = set(), set(), set()
    for feature in features:
        if (not isinstance(feature, dict)
                or set(feature) != {"id", "title", "scope", "syntax", "cases", "pending", "note"}
                or not all(isinstance(feature[key], str) for key in ("id", "title", "scope", "note"))
                or not re.fullmatch(r"[a-z][a-z0-9-]*", feature["id"])
                or feature["id"] in feature_ids or not feature["title"]
                or feature["scope"] not in ("basic", "toolchain", "packages", "retired")
                or not isinstance(feature["syntax"], list) or not isinstance(feature["cases"], list)
                or not isinstance(feature["pending"], list)
                or not all(isinstance(item, str) and item for item in feature["pending"])
                or not all(isinstance(item, str) for item in feature["cases"])
                or not all(isinstance(node, str) and re.fullmatch(r"[A-Z][A-Za-z]+", node)
                           for node in feature["syntax"])
                or len(set(feature["syntax"])) != len(feature["syntax"])
                or nodes.intersection(feature["syntax"])
                or len(set(feature["cases"])) != len(feature["cases"])
                or not set(feature["cases"]).issubset(identities)
                or feature["scope"] == "basic" and not feature["cases"]
                or feature["scope"] != "basic" and not feature["note"]):
            raise ValueError("invalid or incomplete feature inventory")
        feature_ids.add(feature["id"])
        nodes.update(feature["syntax"])
        covered.update(feature["cases"])
    if covered != identities:
        raise ValueError("unmapped language cases")
    return cases


def verify_reference_ast(document, path):
    if digest(path) != document["referenceAstSha256"]:
        raise ValueError("reference AST changed; review the syntax inventory with the pin")
    actual = set()
    for source in sorted(path.parent.glob("*.go")):
        if not source.name.endswith("_test.go"):
            actual.update(re.findall(r"func\s+\(\s*(?:\w+\s+)?\*(\w+)\s*\)\s+(?:statementNode|expressionNode)\s*\(\s*\)", source.read_text()))
    mapped = {node for feature in document["features"] for node in feature["syntax"]}
    if not actual or actual != mapped:
        raise ValueError(f"unmapped or unknown reference syntax: {sorted(actual ^ mapped)}")


def coverage_states(case):
    expected, reference = case["native"], case["reference"]
    def acceptance(path):
        accepts = expected[path]["code"] == 0
        if accepts:
            return "accepts" if reference[path]["code"] == 0 else "accepts; reference rejects"
        return "rejects valid input" if reference[path]["code"] == 0 else "rejects as reference"
    execution = ("not reached" if expected["execute"] is None else
                 "matches reference" if execution_expectations_match(expected["execute"], reference["execute"]) else "differs")
    repl = expected["repl"]
    repl_state = ("matches reference" if repl == reference["repl"] else
                  "rejects as reference" if "repl" not in parity_gaps(case) else
                  "rejects valid input" if repl["stderr"] and not reference["repl"]["stderr"] else
                  "diagnostic/output differs" if repl["stderr"] else "output differs")
    return dict(check=acceptance("check"), build=acceptance("build"), execute=execution, repl=repl_state)


def feature_table(document):
    cases = {case["id"]: case for case in document["cases"]}
    lines = ["<!-- Generated from tools/native-language-cases.json; do not edit by hand. -->", "",
             "Each count describes registered probes, not language coverage percentages.", "",
             "| Family | Phase | Probes with differences | Uncovered contracts | Reference syntax nodes |",
             "| --- | --- | --- | --- | --- |"]
    for feature in document["features"]:
        gaps = [identity for identity in feature["cases"] if parity_gaps(cases[identity])]
        def cell(value):
            return html.escape(value).replace("|", "&#124;").replace("\n", " ")
        pending = "; ".join(feature["pending"]) or "none registered"
        lines.append(f"| {cell(feature['title'])} | {feature['scope']} | {len(gaps)} / {len(feature['cases'])}"
                     f" | {cell(pending)} | {cell(', '.join(feature['syntax']))} |")
    return "\n".join(lines) + "\n"


def pages_data(document, registry):
    data = {"schemaVersion": 1, "registrySha256": digest(registry),
            "referenceRevision": (registry.parent.parent / "TYPE_RB_REVISION").read_text().strip(),
            "features": document["features"],
            "cases": [{"id": case["id"], "title": case["title"],
                       "states": coverage_states(case), "gaps": parity_gaps(case),
                       "referenceReplRejects": bool(case["reference"]["repl"]["stderr"])}
                      for case in document["cases"]]}
    return ("// Generated by tools/native-language-coverage.py; do not edit by hand.\n"
            + "export const ordinaryLanguage = " + json.dumps(data, ensure_ascii=False, indent=2) + ";\n")


def observe(binary, arguments, root, text=None):
    env = dict(os.environ, NO_COLOR="1", TERM="dumb",
               TRBN_HISTORY=str(root / "native-history.json"))
    child = subprocess.Popen([str(binary), *arguments], cwd=root, env=env,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, start_new_session=True)
    timed_out = False
    try:
        stdout, stderr = child.communicate(text.encode("utf-8") if text is not None else None, timeout=30)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(child.pid, signal.SIGKILL)
        stdout, stderr = child.communicate()
    except BaseException:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
        child.communicate()
        raise
    result = {"code": 124 if timed_out else child.returncode}
    invalid = {}
    for stream, data in (("stdout", stdout), ("stderr", stderr)):
        data = data.replace(str(root).encode(), b"<case>")
        try:
            result[stream] = data.decode("utf-8")
        except UnicodeDecodeError:
            result[stream] = data.decode("utf-8", "backslashreplace")
            invalid[stream] = data.hex()
    if invalid:
        result["invalidUtf8"] = invalid
    if timed_out:
        result["timedOut"] = True
    return result


def collect(case, native, reference, root):
    for name, text in case["files"].items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    source = case["declarations"] + "\ndef main()\n" + case["body"] + "\nend\n"
    (root / "main.trb").write_text(source)
    repl = case["declarations"] + "\n" + case["body"] + "\n:quit\n"
    observations = {}
    for role, binary in (("reference", reference), ("native", native)):
        if binary is None:
            continue
        if case["files"] or case.get("projectCheck", False):
            # Same source graph; select the implementation's execution mode.
            (root / "trbconfig.jsonc").write_text(json.dumps({
                "name": "language-case", "mode": "go" if role == "reference" else "trb",
                "sourceDir": ".", "outDir": "out", "go": {"module": "example.com/language-case"}}))
        executable = root / (role + "-program")
        check_arguments = ["check", "--config", "trbconfig.jsonc"] if case.get("projectCheck", False) else ["check", "main.trb"]
        result = {"check": observe(binary, check_arguments, root)}
        build_arguments = ["build", "--compile", "--outfile", str(executable)]
        if not case["files"] and not case.get("projectCheck", False):
            build_arguments.append("main.trb")
        result["build"] = observe(binary, build_arguments, root)
        if result["build"]["code"] == 0:
            if not executable.is_file():
                raise ValueError(f"{case['id']}: {role} build published no executable")
            result["execute"] = observe(executable, [], root)
        else:
            if executable.exists():
                raise ValueError(f"{case['id']}: failed {role} build published output")
            result["execute"] = None
        result["repl"] = observe(binary, ["repl"], root, repl)
        observations[role] = result
    return observations


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", type=Path)
    parser.add_argument("--reference", type=Path)
    parser.add_argument("--observe", action="store_true")
    parser.add_argument("--case")
    parser.add_argument("--feature-table", action="store_true")
    parser.add_argument("--check-feature-table", type=Path)
    parser.add_argument("--pages-data", action="store_true")
    parser.add_argument("--check-pages-data", type=Path)
    parser.add_argument("--reference-ast", type=Path)
    parser.add_argument("--require-parity", action="store_true")
    parser.add_argument("--jobs", type=int, default=1, choices=range(1, 5))
    args = parser.parse_args()
    native = args.native.resolve() if args.native else None
    reference = args.reference.resolve() if args.reference else None
    registry = Path(__file__).with_name("native-language-cases.json")
    document = json.loads(registry.read_text())
    cases = validate(document)
    if args.reference_ast:
        verify_reference_ast(document, args.reference_ast)
    if args.feature_table or args.check_feature_table or args.pages_data or args.check_pages_data:
        if args.case or native or reference or args.observe or args.require_parity:
            parser.error("table commands do not execute compilers or select cases")
        if (args.feature_table or args.check_feature_table) and (args.pages_data or args.check_pages_data):
            parser.error("select one generated view")
        table = feature_table(document) if args.feature_table or args.check_feature_table else pages_data(document, registry)
        check_table = args.check_feature_table or args.check_pages_data
        if check_table:
            if check_table.read_text() != table:
                raise ValueError("coverage table differs from reviewed expectations")
        else:
            print(table, end="")
        return 0
    if native is None and reference is None:
        parser.error("select --native, --reference, or both; no compiler is implicit")
    if args.case:
        cases = [case for case in cases if case["id"] == args.case]
        if not cases:
            raise ValueError("unknown language case")
    if args.require_parity and (native is None or reference is None or args.observe
                               or args.case or args.reference_ast is None):
        parser.error("strict parity requires both compilers, the reference AST and the complete reviewed registry")
    report = {"schemaVersion": 2, "nativeSha256": digest(native) if native else None,
              "referenceSha256": digest(reference) if reference else None,
              "registrySha256": digest(registry), "observationOnly": args.observe,
              "typeRBRevision": (registry.parent.parent / "TYPE_RB_REVISION").read_text().strip(),
              "referenceAstVerified": args.reference_ast is not None,
              "cases": [], "failures": [], "parityGaps": [],
              "uncoveredContracts": [{"feature": feature["id"], "contracts": feature["pending"]}
                                     for feature in document["features"]
                                     if feature["scope"] == "basic" and feature["pending"]]}
    with tempfile.TemporaryDirectory(prefix="native-language-") as temporary:
        def run_case(item):
            index, case = item
            # Retain observations, but bound generated files by the worker count.
            with tempfile.TemporaryDirectory(prefix=f"{index}-", dir=temporary) as case_directory:
                root = Path(case_directory).resolve()
                try:
                    observations = collect(case, native, reference, root)
                except ValueError as error:
                    return case, {"error": str(error)}
            return case, observations
        with ThreadPoolExecutor(max_workers=args.jobs) as pool:
            results = list(pool.map(run_case, enumerate(cases)))
        for case, observations in results:
            failed = "error" in observations
            report["cases"].append({"id": case["id"], **observations})
            for role in ("reference", "native"):
                if role in observations:
                    failed |= any(value is not None and value.get("timedOut")
                                  for value in observations[role].values())
                if role in observations and (role == "reference" or not args.observe):
                    failed |= not outcomes_match(observations[role], case[role])
            gaps = parity_gaps(case)
            if gaps:
                report["parityGaps"].append({"id": case["id"], "paths": gaps})
            if failed or args.require_parity and gaps:
                report["failures"].append(case["id"])
    print(json.dumps(report, ensure_ascii=False, indent=2))
    if report["failures"]:
        print("Language case mismatches (" + str(len(report["failures"])) + "): " +
              ", ".join(report["failures"]), file=sys.stderr)
    return 1 if report["failures"] or args.require_parity and report["uncoveredContracts"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
