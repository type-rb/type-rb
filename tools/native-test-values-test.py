#!/usr/bin/env python3
"""Check test equality semantics and assertion origins against the pinned oracle."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


VALUES = '''import { describe, expect, test } from trb/std/test

record Sample
    value: Float
end

class Link
    @value: Float
    @next: Link?
    def initialize(value: Float)
        @value = value
        @next = nil
    end
    def link(following: Link)
        @next = following
    end
end

describe("Equality") do
    test("scalar NaN is unequal to itself") do
        value := 0.0 / 0.0
        expect(value).to_not_equal(value)
    end
    test("signed zero compares by value") do
        expect(0.0).to_equal(-0.0)
    end
    test("same array identity retains the reference shortcut") do
        values := [0.0 / 0.0]
        expect(values).to_equal(values)
        expect(values).to_not_equal([0.0 / 0.0])
    end
    test("same hash identity retains the reference shortcut") do
        values := {"value" => 0.0 / 0.0}
        expect(values).to_equal(values)
        expect(values).to_not_equal({"value" => 0.0 / 0.0})
    end
    test("record storage identity does not bypass field equality") do
        value := Sample.new(value: 0.0 / 0.0)
        expect(value).to_not_equal(value)
    end
    test("class identity and class fields have distinct roles") do
        value := Link.new(0.0 / 0.0)
        expect(value).to_equal(value)
        expect(value).to_not_equal(Link.new(0.0 / 0.0))
        expect(Link.new(4.0)).to_equal(Link.new(4.0))
    end
    test("cycles compare without recurring forever") do
        mut left := Link.new(1.0)
        mut right := Link.new(1.0)
        left.link(left)
        right.link(right)
        expect(left).to_equal(right)
        mut other := Link.new(2.0)
        other.link(other)
        expect(left).to_not_equal(other)
    end
    test("non-nil functions never compare deeply equal") do
        action := fn(): Integer
            return 1
        end
        expect(action).to_not_equal(action)
    end
    test("nullable absence is different from an empty array") do
        absent: Array<Integer>? := nil
        empty: Array<Integer> := []
        expect(absent).to_be_nil()
        expect(empty).to_equal([])
    end
end
'''

ORIGINS = '''import { describe, expect, expect_err, expect_ok, test } from trb/std/test
import trb/std/result

def helper()
    expect(10).to_equal(11)
end

describe("Origins") do
    test("first same-type origin") do
        expect(1).to_equal(2)
    end
    test("second same-type origin") do
        expect(3).to_equal(4)
    end
    test("helper assertion origin") do
        helper()
    end
    test("quoted unicode byte column") do
        value := "値😀"; expect(value).to_equal("other")
    end
    test("unicode interpolation retains the inner assertion origin") do
        value := "値😀#{expect_ok(Result<String, String>::Err("bad"))}"
        puts(value)
    end
    test("non-Boolean is an assertion failure") do
        expect(1).to_be_true()
    end
    test("wrong Result variant retains the call origin") do
        _value := expect_ok(Result<Integer, String>::Err("bad"))
    end
    test("another Result variant call origin") do
        _value := expect_err(Result<Integer, String>::Ok(1))
    end
    test("runtime trap uses the case origin") do
        values := [1]
        puts(values[3])
    end
    test("later case still runs") do
        expect(true).to_be_true()
    end
end
'''


DYNAMIC = '''import { describe, expect, test } from trb/std/test
import trb/std/string_builder
newtype Label = String
interface Named
    name(): String
end
class Base
end
class Item < Base implements Named
    @text: String
    def initialize(text: String)
        @text = text
    end
    def name(): String
        return @text
    end
end
class Other implements Named
    def name(): String
        return "same"
    end
end
describe("Dynamic equality") do
    test("nominal labels and union alternatives") do
        expect(Label.new("a")).to_equal(Label.new("a"))
        expect(Label.new("a")).to_not_equal(Label.new("b"))
        left: Integer | String := 1
        right: Integer | String := "1"
        expect(left).to_equal(left)
        expect(left).to_not_equal(right)
        text: Integer | String := "held"
        expect(text).to_equal("he" + "ld")
    end
    test("dynamic payloads retain type identity") do
        mut unrelated := StringBuilder.new()
        unrelated.append("kept")
        left: Any := 1
        right: Any := 1.0
        expect(left).to_equal(left)
        expect(left).to_not_equal(right)
        values: Array<Any> := [1, "a", true]
        expect(values).to_equal([1, "a", true])
        expect(values).to_not_equal([1, "b", true])
    end
    test("ranges include endpoint exclusivity") do
        expect(1..3).to_equal(1..3)
        expect(1..3).to_not_equal(1...3)
    end
    test("interfaces retain dynamic class fields") do
        item: Named := Item.new("same")
        expect(item).to_equal(Item.new("same"))
        expect(item).to_not_equal(Item.new("different"))
        expect(item).to_not_equal(Other.new())
    end
    test("nullable values compare their present payload") do
        value: Integer? := 7
        expect(value).to_equal(7)
        expect(value).to_not_equal(8)
    end
end
'''


SCALARS = 'import { describe, expect, test } from trb/std/test\ndef helper()\n expect(10).to_equal(11)\nend\ndescribe("Scalars") do\n test("success") do\n  expect(1).to_equal(1)\n  expect(1).to_not_equal(2)\n  expect(true).to_be_true()\n  expect(false).to_be_false()\n  expect(nil).to_be_nil()\n end\n test("equality") do\n  expect(1).to_equal(2)\n end\n test("inequality") do\n  expect(3).to_not_equal(3)\n end\n test("Boolean") do\n  expect(true).to_be_false()\n end\n test("type") do\n  expect(1).to_be_true()\n end\n test("nil") do\n  expect(5).to_be_nil()\n end\n test("helper") do\n  helper()\n end\n test("continued") do\n  expect(false).to_equal(false)\n end\nend\n'


def replay(binary, root, source, wanted_failures, total, support=None):
    for relative, contents in (support or {}).items():
        path = root / "src" / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(contents)
    (root / "src/sample_test.trb").write_text(source)
    result = subprocess.run([str(binary), "test", "--reporter", "json"], cwd=root,
                            env=dict(os.environ, NO_COLOR="1", TERM="dumb"),
                            text=True, capture_output=True, timeout=180)
    stdout = result.stdout.replace(str(root.resolve()), "<project>").replace(str(root), "<project>")
    stderr = result.stderr.replace(str(root.resolve()), "<project>").replace(str(root), "<project>")
    events = [json.loads(line) for line in stdout.splitlines()]
    assert result.returncode == int(wanted_failures > 0), (result.returncode, stdout, stderr)
    assert len(events) == total * 2 + 1, (stdout, stderr)
    assert events[-1] == {"type": "test_summary", "total": total, "failed": wanted_failures}, events
    assert sum(event["type"] == "test_failed" for event in events) == wanted_failures, events
    for index in range(0, total * 2, 2):
        assert events[index]["type"] == "test_started", events[index]
        assert events[index + 1]["type"] in ("test_passed", "test_failed"), events[index + 1]
        assert events[index]["name"] == events[index + 1]["name"], events[index:index + 2]
    assert stderr in ("", "exit status 1\n"), stderr
    return {"code": result.returncode, "events": events, "stderr": stderr,
            "sourceSha256": hashlib.sha256(source.encode()).hexdigest(),
            **({"supportSha256": {name: hashlib.sha256(text.encode()).hexdigest()
                                   for name, text in support.items()}} if support else {})}


RESULTS = '''import { describe, expect, expect_err, expect_ok, test } from trb/std/test
import trb/std/result

describe("Results") do
    test("managed Ok payload") do
        values := expect_ok(Result<Array<Integer>, Integer>::Ok([7, 8]))
        expect(values.size()).to_equal(2)
        expect(values[1]).to_equal(8)
    end
    test("managed Err payload") do
        values := expect_err(Result<Boolean, Array<Integer>>::Err([3, 4]))
        expect(values.size()).to_equal(2)
        expect(values[0]).to_equal(3)
    end
    test("scalar payloads") do
        amount := expect_ok(Result<Float, Integer>::Ok(2.5))
        expect(amount == 2.5).to_be_true()
        error_amount := expect_err(Result<Boolean, Float>::Err(3.5))
        expect(error_amount == 3.5).to_be_true()
        expect(expect_ok(Result<Boolean, Integer>::Ok(true))).to_be_true()
        expect(expect_err(Result<Boolean, Integer>::Err(7))).to_equal(7)
    end
    test("unexpected Err") do
        _value := expect_ok(Result<Float, Boolean>::Err(false))
    end
    test("unexpected Ok") do
        _value := expect_err(Result<Integer, Float>::Ok(9))
    end
    test("continued") do
        expect(1).to_equal(1)
    end
end
'''


TRAPS = '''import { describe, test } from trb/std/test
describe("Bounds") do
    test("array") do
        values := [1]
        puts(values[2])
    end
    test("string") do
        value := "one"
        puts(value[4])
    end
    test("continued") do
    end
end
'''


ROOT = Path(__file__).resolve().parent.parent
PORTABLE_ROOT = ROOT / "fixtures/projects/test-portable-values/app/src"
PORTABLE = (PORTABLE_ROOT / "sample_test.trb").read_text()
PREDICATES = (PORTABLE_ROOT.parent.parent / "predicates.trb").read_text()
BOUNDARIES = (PORTABLE_ROOT.parent.parent / "boundaries.trb").read_text()
SUPPORT = {"portable": {str(path.relative_to(PORTABLE_ROOT)): path.read_text()
                        for path in sorted(PORTABLE_ROOT.rglob("*.trb"))
                        if path.name != "sample_test.trb"}}

GROUPS = (("values", VALUES, 0, 9), ("origins", ORIGINS, 9, 10),
          ("dynamic", DYNAMIC, 0, 5), ("scalars", SCALARS, 6, 8),
          ("traps", TRAPS, 2, 3), ("results", RESULTS, 2, 6),
          ("portable", PORTABLE, 36, 37), ("predicates", PREDICATES, 4, 11),
          ("boundaries", BOUNDARIES, 7, 8))
ROOT = Path(__file__).resolve().parent.parent
CONTRACT = ROOT / "fixtures/projects/native-test-values.json"


def load_contract(path=CONTRACT):
    contract = json.loads(path.read_text())
    if contract["schemaVersion"] != 1 or contract["typeRBRevision"] != (ROOT / "TYPE_RB_REVISION").read_text().strip():
        raise ValueError("test value contract requires an explicit current-pin replay")
    groups = contract["groups"]
    if set(groups) != {name for name, _, _, _ in GROUPS}:
        raise ValueError("test value contract must retain every required group")
    for name, source, failures, total in GROUPS:
        row = groups[name]
        hashes = {path: hashlib.sha256(text.encode()).hexdigest() for path, text in SUPPORT.get(name, {}).items()}
        if row.get("supportSha256", {}) != hashes:
            raise ValueError("test support source changed without replay: " + name)
        if row["sourceSha256"] != hashlib.sha256(source.encode()).hexdigest():
            raise ValueError("test source changed without a reference replay: " + name)
        if row["code"] != int(failures > 0) or len(row["events"]) != total * 2 + 1:
            raise ValueError("test value contract count or status changed: " + name)
        if row["events"][-1] != {"type": "test_summary", "total": total, "failed": failures}:
            raise ValueError("test value contract summary changed: " + name)
    return groups


def compare_observation(name, observed, expected, native=False):
    for key in ("code", "events", "sourceSha256"):
        if observed[key] != expected[key]:
            raise ValueError(f"{name}: {key} differs from the frozen reference observation: {observed}")
    if observed.get("supportSha256", {}) != expected.get("supportSha256", {}):
        raise ValueError(name + ": supporting source hashes differ")
    if native and observed["stderr"] != "":
        raise ValueError(f"{name}: unexpected Native stderr: {observed['stderr']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reference", type=Path)
    parser.add_argument("--native", type=Path)
    parser.add_argument("--report", type=Path)
    parser.add_argument("--group", choices=tuple(row[0] for row in GROUPS))
    args = parser.parse_args()
    if not args.reference and not args.native:
        parser.error("at least one of --reference or --native is required")
    contract = load_contract()
    rows = []
    try:
        with tempfile.TemporaryDirectory(prefix="native-test-values-") as directory:
            root = Path(directory)
            (root / "src").mkdir()
            for name, source, failures, total in GROUPS:
                if args.group and args.group != name:
                    continue
                row = {"id": name}
                rows.append(row)
                for label, binary, mode in (("reference", args.reference, "go"),
                                             ("native", args.native, "trb")):
                    if not binary:
                        continue
                    (root / "trbconfig.jsonc").write_text(json.dumps({
                        "name": "test-values", "mode": mode, "sourceDir": "src",
                        "go": {"module": "example.com/test-values"},
                    }) + "\n")
                    observed = replay(binary.resolve(), root, source, failures, total, SUPPORT.get(name))
                    row[label] = observed
                    compare_observation(name, observed, contract[name], label == "native")
    finally:
        if args.report:
            args.report.write_text(json.dumps({
                "scope": "additional frozen value/origin controls; historical manifest unchanged",
                "contractSha256": hashlib.sha256(CONTRACT.read_bytes()).hexdigest(),
                "observations": rows,
            }, ensure_ascii=False, indent=2) + "\n")
    modes = "/".join(label for label, binary in (("reference", args.reference), ("native", args.native)) if binary)
    print(f"PASS {len(rows)} frozen test-value groups ({modes})")


if __name__ == "__main__":
    main()
