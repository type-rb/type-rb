#!/usr/bin/env python3
import copy
import importlib.util
import json
import hashlib
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch, Mock

spec = importlib.util.spec_from_file_location("coverage", Path(__file__).with_name("native-language-coverage.py"))
coverage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(coverage)


class LanguageCoverageTests(unittest.TestCase):
    def setUp(self):
        self.document = json.loads(Path(__file__).with_name("native-language-cases.json").read_text())

    def test_registry_and_distinct_execution_paths(self):
        cases = coverage.validate(self.document)
        self.assertGreaterEqual(len(cases), 80)
        self.assertTrue({"named-arguments", "utf8-values", "unused-binding", "record-equality",
                         "generic-function", "result-try"}.issubset({case["id"] for case in cases}))
        utf8 = next(case for case in cases if case["id"] == "utf8-string")
        expected = coverage.expected_native(utf8)
        self.assertEqual(expected["check"]["code"], 0)
        self.assertEqual(expected["build"]["code"], 0)
        self.assertEqual(expected["execute"]["stdout"], "日本語\n")
        self.assertEqual(expected["repl"]["code"], 0)
        self.assertEqual(expected["repl"]["stderr"], "")

    def test_empty_duplicate_unknown_or_unreviewed_registry_is_rejected(self):
        mutations = [lambda d: d.update(schemaVersion=True), lambda d: d.update(extra=1),
                     lambda d: d.update(referenceAstSha256=None),
                     lambda d: d.update(cases=[]), lambda d: d["cases"].append(copy.deepcopy(d["cases"][0])),
                     lambda d: d["cases"][0].update(native=None), lambda d: d["cases"][0].update(extra=1),
                     lambda d: d["cases"][0].update(id="../outside"),
                     lambda d: d["cases"][0]["native"].update(check=""),
                     lambda d: d["cases"][0]["native"]["repl"].update(code=True),
                     lambda d: d["cases"][0]["native"].update(execute=None),
                     lambda d: d["cases"][0]["files"].update({"../outside.trb": ""}),
                     lambda d: d["features"][0]["cases"].append("unknown"),
                     lambda d: d["features"][0].update(cases=[]),
                     lambda d: d["features"][0]["syntax"].append(d["features"][1]["syntax"][0])]
        for mutation in mutations:
            document = copy.deepcopy(self.document)
            mutation(document)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                coverage.validate(document)

    def test_project_route_paths_stay_inside_the_fixture(self):
        for name in ("routes/items/[id].trb", "routes/files/[...rest].trb", "routes/café.trb"):
            self.assertTrue(coverage.valid_case_file(name, True))
            self.assertFalse(coverage.valid_case_file(name, False))
        for name in ("/routes/x.trb", "routes/../outside.trb", "routes//x.trb",
                     "routes/./x.trb", "routes/x\\escape.trb", "routes/evil\n.trb",
                     "routes/evil\0.trb", "main.trb"):
            self.assertFalse(coverage.valid_case_file(name, True), name)
        document = copy.deepcopy(self.document)
        document["cases"][0]["projectCheck"] = "true"
        with self.assertRaises(ValueError):
            coverage.validate(document)

    def test_text_inputs_stay_under_the_fixture_directory(self):
        self.assertTrue(coverage.valid_case_file("fixtures/resource.txt"))
        for name in ("resource.txt", "/fixtures/resource.txt", "fixtures/../resource.txt",
                     "fixtures//resource.txt", "fixtures/./resource.txt", "fixtures/a/b.txt",
                     "fixtures/resource.py", "fixtures/resource\\escape.txt"):
            self.assertFalse(coverage.valid_case_file(name), name)

    def test_project_check_explicitly_checks_the_same_route_graph_as_build(self):
        case = copy.deepcopy(self.document["cases"][0])
        case.update(projectCheck=True, files={"routes/[id].trb": "def get()\nend\n"})
        calls = []
        def observe(binary, arguments, root, *input):
            calls.append(arguments)
            self.assertTrue((root / "routes/[id].trb").is_file())
            self.assertTrue((root / "trbconfig.jsonc").is_file())
            return {"code": 1, "stdout": "", "stderr": "fixture rejected"}
        with tempfile.TemporaryDirectory() as directory, patch.object(coverage, "observe", side_effect=observe):
            coverage.collect(case, Path("native"), None, Path(directory))
        self.assertEqual(calls[0], ["check", "--config", "trbconfig.jsonc"])
        self.assertNotIn("main.trb", calls[1])

    def test_states_do_not_count_session_exit_zero_as_feature_support(self):
        states = [(case["title"], coverage.coverage_states(case)) for case in coverage.validate(self.document)]
        for title, repl in [("UTF-8 String literal", "matches reference"), ("while", "matches reference"),
                            ("Union inferred hash", "matches reference"), ("Union retained mutation", "output differs"),
                            ("elsif", "matches reference")]:
            with self.subTest(case=title):
                self.assertIn((title, dict(check="accepts", build="accepts", execute="matches reference", repl=repl)),
                              states)

    def test_exact_output_and_diagnostics_are_not_normalized_away(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            observed = coverage.observe(Path(sys.executable),
                                        ["-c", "import os,sys; print(os.getcwd()); print('diagnostic',file=sys.stderr); sys.exit(7)"], root)
        self.assertEqual(observed, {"code": 7, "stdout": "<case>\n", "stderr": "diagnostic\n"})

    def test_timeout_kills_the_owned_process_group_and_remains_failure(self):
        child = Mock(pid=123, returncode=-9)
        child.communicate.side_effect = [subprocess.TimeoutExpired("synthetic", 30), (b"partial", b"error")]
        with patch.object(coverage.subprocess, "Popen", return_value=child) as spawn, patch.object(coverage.os, "killpg") as kill:
            observed = coverage.observe(Path("/synthetic"), [], Path("/case"))
        self.assertTrue(spawn.call_args.kwargs["start_new_session"])
        kill.assert_called_once_with(123, coverage.signal.SIGKILL)
        self.assertEqual(observed, {"code": 124, "stdout": "partial", "stderr": "error", "timedOut": True})

    def test_invalid_reference_input_cannot_be_counted_as_native_gap(self):
        changed = copy.deepcopy(self.document["cases"][0]["reference"])
        changed["check"] = {"code": 1, "stdout": "", "stderr": "invalid source"}
        with patch.object(sys, "argv", ["coverage", "--reference", sys.executable, "--case", "integer", "--observe"]), \
                patch.object(coverage, "collect", return_value={"reference": changed}), patch("builtins.print") as output:
            self.assertEqual(coverage.main(), 1)
            output.assert_any_call("Language case mismatches (1): integer", file=sys.stderr)

    def test_finished_case_files_are_released_before_the_next_case_even_after_failure(self):
        case = next(case for case in self.document["cases"] if case["id"] == "integer")
        cases = [{**case, "id": f"integer-{index}"} for index in range(4)]
        roots = []

        def observations(case, native, reference, root):
            self.assertTrue(all(not previous.exists() for previous in roots))
            roots.append(root)
            (root / "generated-program").write_bytes(b"temporary executable")
            if len(roots) == 1:
                raise ValueError("synthetic missing executable")
            return {"reference": case["reference"]}

        argv = ["coverage", "--reference", sys.executable, "--jobs", "1"]
        with patch.object(sys, "argv", argv), patch.object(coverage, "validate", return_value=cases), \
                patch.object(coverage, "collect", side_effect=observations), patch("builtins.print") as output:
            self.assertEqual(coverage.main(), 1)
        self.assertTrue(all(not root.exists() for root in roots))
        report = json.loads(output.call_args_list[0].args[0])
        self.assertEqual(len(report["cases"]), 4)
        self.assertEqual(report["failures"], ["integer-0"])
        self.assertEqual(report["cases"][0]["error"], "synthetic missing executable")

    def test_rejection_and_runtime_failure_cases_retain_each_oracles_outcome(self):
        cases = {case["id"]: case for case in coverage.validate(self.document)}
        self.assertNotEqual(cases["unused-binding"]["reference"]["check"]["code"], 0)
        self.assertNotEqual(cases["unused-binding"]["native"]["check"]["code"], 0)
        self.assertNotIn("check", coverage.parity_gaps(cases["unused-binding"]))
        failure = cases["array-runtime-bounds"]["reference"]["execute"]
        self.assertNotEqual(failure["code"], 0)
        good = {"code": failure["code"], "stdout": failure["stdout"],
                "stderr": failure["stderrFirstLine"] + "\n\nsynthetic stack location\n"}
        self.assertTrue(coverage.outcome_matches(good, failure))
        self.assertFalse(coverage.outcome_matches({**good, "stderr": "panic: wrong failure\n"}, failure))
        self.assertFalse(coverage.outcome_matches({**good, "stdout": "unexpected effect\n"}, failure))
        self.assertFalse(coverage.outcome_matches({**good, "invalidUtf8": {"stderr": "e6"}}, failure))

    def test_invalid_utf8_output_is_preserved_as_a_gap_instead_of_crashing_the_runner(self):
        with tempfile.TemporaryDirectory() as temporary:
            observed = coverage.observe(Path(sys.executable),
                                        ["-c", "import sys; sys.stderr.buffer.write(bytes([230]))"], Path(temporary))
        self.assertEqual(observed["stderr"], "\\xe6")
        self.assertEqual(observed["invalidUtf8"], {"stderr": "e6"})
        self.assertFalse(coverage.outcome_matches(observed, {"code": 0, "stdout": "", "stderr": "\\xe6"}))

    def test_equivalent_runtime_failure_contracts_have_matching_parity_and_display(self):
        prefix = {"code": 2, "stdout": "effect\n", "stderrFirstLine": "panic: failure"}
        full = {"code": 2, "stdout": "effect\n", "stderr": "panic: failure\n"}
        stack = {**full, "stderr": "panic: failure\n\nunstable stack frame\n"}
        for left, right in ((prefix, full), (prefix, stack), (prefix, prefix), (full, full)):
            for native, reference in ((left, right), (right, left)):
                case = copy.deepcopy(self.document["cases"][0])
                case["native"]["execute"] = native
                case["reference"]["execute"] = reference
                with self.subTest(native=native, reference=reference):
                    self.assertNotIn("execute", coverage.parity_gaps(case))
                    self.assertEqual(coverage.coverage_states(case)["execute"], "matches reference")

    def test_execution_parity_preserves_failure_differences_and_missing_execution(self):
        prefix = {"code": 2, "stdout": "effect\n", "stderrFirstLine": "panic: failure"}
        full = {"code": 2, "stdout": "effect\n", "stderr": "panic: failure\n"}
        different = [
            (prefix, {**full, "code": 1}),
            (prefix, {**full, "stdout": ""}),
            (prefix, {**full, "stderr": "panic: another failure\n"}),
            (prefix, {**full, "invalidUtf8": {"stderr": "e6"}}),
            (prefix, {**full, "invalidUtf8": {"stdout": "e6"}}),
            (prefix, {**prefix, "stderrFirstLine": "panic: another failure"}),
            (prefix, {**prefix, "code": 1}),
            (prefix, {**prefix, "stdout": ""}),
            (full, {**full, "stderr": "panic: failure\nextra output\n"}),
            (full, {**full, "invalidUtf8": {"stderr": "e6"}}),
            (None, full),
            (None, prefix),
            (prefix, {**full, "timedOut": True}),
            ({**full, "timedOut": True}, {**full, "timedOut": True}),
        ]
        for left, right in different:
            for native, reference in ((left, right), (right, left)):
                case = copy.deepcopy(self.document["cases"][0])
                case["native"]["execute"] = native
                case["reference"]["execute"] = reference
                with self.subTest(native=native, reference=reference):
                    self.assertIn("execute", coverage.parity_gaps(case))
        self.assertTrue(coverage.execution_expectations_match(None, None))

    def test_runtime_parity_does_not_weaken_independent_regression_expectations(self):
        cases = {case["id"]: case for case in self.document["cases"]}
        case = cases["array-copy-first-empty"]
        native, reference = case["native"]["execute"], case["reference"]["execute"]
        self.assertNotIn("execute", coverage.parity_gaps(case))
        self.assertEqual(coverage.coverage_states(case)["execute"], "matches reference")
        self.assertIn("execute", coverage.parity_gaps(cases["array-runtime-bounds"]))
        self.assertTrue(coverage.outcome_matches(native, reference))
        changed = {**native, "stderr": native["stderr"] + "unexpected Native output\n"}
        self.assertTrue(coverage.outcome_matches(changed, reference))
        self.assertFalse(coverage.outcome_matches(changed, native))

    def test_reference_syntax_cannot_disappear_or_arrive_in_another_file_unnoticed(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "ast.go"
            nodes = [node for feature in self.document["features"] for node in feature["syntax"]]
            source.write_text("\n".join(f"func (*{node}) expressionNode() {{}}" for node in nodes))
            document = copy.deepcopy(self.document)
            document["referenceAstSha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
            coverage.verify_reference_ast(document, source)
            extra = root / "extra.go"
            extra.write_text("func (node *UnmappedSyntax) statementNode() {}")
            with self.assertRaisesRegex(ValueError, "unmapped or unknown"):
                coverage.verify_reference_ast(document, source)
            extra.unlink()
            source.write_text(source.read_text() + "\n// new reference pin\n")
            with self.assertRaisesRegex(ValueError, "reference AST changed"):
                coverage.verify_reference_ast(document, source)

    def test_known_rejections_and_uncovered_contracts_cannot_pass_strict_parity(self):
        def observations(case, *_args):
            result = copy.deepcopy({role: case[role] for role in ("native", "reference")})
            for role in result.values():
                runtime = role["execute"]
                if runtime and "stderrFirstLine" in runtime:
                    runtime["stderr"] = runtime.pop("stderrFirstLine") + "\n"
            return result
        argv = ["coverage", "--native", sys.executable, "--reference", sys.executable,
                "--reference-ast", "synthetic-ast.go", "--require-parity"]
        with patch.object(sys, "argv", argv), patch.object(coverage, "verify_reference_ast"), \
                patch.object(coverage, "collect", side_effect=observations), patch("builtins.print"):
            self.assertEqual(coverage.main(), 1)
        self.assertTrue(any(feature["pending"] for feature in self.document["features"] if feature["scope"] == "basic"))

    def test_feature_table_does_not_present_ast_inventory_as_language_completion(self):
        table = coverage.feature_table(self.document)
        self.assertIn("not language coverage percentages", table)
        self.assertIn("Uncovered contracts", table)
        self.assertIn("Target source interop | toolchain", table)

    def test_changed_native_observation_fails_without_rewriting_expectations(self):
        case = self.document["cases"][0]
        wrong = copy.deepcopy(coverage.expected_native(case))
        wrong["execute"]["stdout"] = "wrong\n"
        registry = Path(__file__).with_name("native-language-cases.json")
        before = registry.read_bytes()
        for observe, status in ((False, 1), (True, 0)):
            argv = ["coverage", "--native", sys.executable, "--case", case["id"]]
            if observe:
                argv.append("--observe")
            with patch.object(sys, "argv", argv), patch.object(coverage, "collect", return_value={"native": wrong}), patch("builtins.print"):
                self.assertEqual(coverage.main(), status)
            self.assertEqual(registry.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
