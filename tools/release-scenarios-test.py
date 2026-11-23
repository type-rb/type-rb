#!/usr/bin/env python3
import copy
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("scenarios", Path(__file__).with_name("release-scenarios.py"))
scenarios = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scenarios)


class ScenarioTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.plan = scenarios.read_json(scenarios.PLAN)
        # This small fixture exercises catalog ownership without copying a
        # moving canonical release inventory into this repository.
        members = {}
        for row in self.plan["rows"]:
            for binding in row["bindings"]:
                members.setdefault(binding["surface"], []).append(binding["member"])
        self.catalog = [{"id": key, "members": value} for key, value in members.items()]
        self.catalog.append({"id": "protocol:unmapped"})
        self.manifest = {"schemaVersion": 2, "release": "0.5.0", "hosts": [scenarios.host()], "rows": [
            {"id": "package.std.process", "surfaces": list(members), "qualificationReady": False},
            {"id": "other", "surfaces": ["protocol:unmapped"], "qualificationReady": False,
             "deferrals": [{"id": "fixture-approved-deferral", "scope": "retained verbatim"}]}]}
        self.contract = self.root / "contracts/v0.5"
        self.contract.mkdir(parents=True)
        self.plan_path = self.root / "plan.json"
        self.write()

    def write(self):
        catalog = self.contract / "surfaces.json"
        catalog.write_text(json.dumps(self.catalog))
        self.manifest["catalogSHA256"] = scenarios.digest(catalog)
        (self.contract / "manifest.json").write_text(json.dumps(self.manifest))
        self.plan_path.write_text(json.dumps(self.plan))

    def load(self):
        self.write()
        return scenarios.load_plan(self.root, self.plan_path)

    def test_declared_members_do_not_erase_other_rows_or_claim_qualification(self):
        _, _, inventory, hashes = self.load()
        process, other = inventory
        self.assertEqual(process["mappedMembers"], 16)
        self.assertEqual(process["unmapped"], [])
        self.assertTrue(process["remaining"])
        self.assertEqual(other["unmappedSurfaces"], ["protocol:unmapped"])
        self.assertEqual(other["deferrals"], self.manifest["rows"][1]["deferrals"])
        self.assertTrue(all(row["status"] == "unqualified" for row in inventory))
        self.assertFalse(process["qualificationReady"])
        self.assertEqual(hashes["manifestSHA256"], scenarios.digest(self.contract / "manifest.json"))

    def test_new_declarations_remain_unmapped(self):
        self.catalog[1]["members"].append("def future_api(): String")
        inventory = self.load()[2]
        self.assertEqual(inventory[0]["unmapped"], [{"surface": self.catalog[1]["id"], "member": "def future_api(): String"}])

    def test_unknown_stale_duplicate_and_cross_row_bindings_are_rejected(self):
        original = copy.deepcopy(self.plan)
        mutations = [lambda p: p.update(schemaVersion=True), lambda p: p.update(scenarios=[]),
                     lambda p: p["scenarios"].append(copy.deepcopy(p["scenarios"][0])),
                     lambda p: p["rows"][0]["bindings"][0].update(member="removed declaration"),
                     lambda p: p["rows"][0]["bindings"][0].update(surface="foreign:surface"),
                     lambda p: p["rows"][0]["bindings"][0].update(scenarios=["missing"]),
                     lambda p: p["rows"][0]["bindings"].append(copy.deepcopy(p["rows"][0]["bindings"][0])),
                     lambda p: p["scenarios"][0].update(row="other"),
                     lambda p: p["scenarios"][-1].update(diagnostic=""),
                     lambda p: p.update(qualificationReady=True)]
        for mutation in mutations:
            self.plan = copy.deepcopy(original)
            mutation(self.plan)
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                self.load()

    def test_catalog_identity_and_duplicate_json_keys_fail_closed(self):
        (self.contract / "surfaces.json").write_text("[]")
        with self.assertRaisesRegex(ValueError, "catalog"):
            scenarios.load_plan(self.root, self.plan_path)
        self.plan_path.write_text('{"schemaVersion":1,"schemaVersion":1}')
        with self.assertRaisesRegex(ValueError, "duplicate JSON key"):
            scenarios.read_json(self.plan_path)

    @staticmethod
    def observation(code=0, stdout="", stderr="", timeout=False):
        return {"exitCode": code, "stdout": stdout, "stderr": stderr, "timedOut": timeout}

    def test_repl_zero_exit_and_unsupported_negative_input_never_pass(self):
        accepted = self.plan["scenarios"][0]
        rejected = next(c for c in self.plan["scenarios"] if c["kind"] == "rejected")
        stderr = "unsupported standard package member; argument expected String"
        observed = {p: self.observation(0 if p == "repl" else 1, stderr=stderr) for p in ("check", "build", "repl")}
        for case in (accepted, rejected):
            result = scenarios.assess(case, observed)
            self.assertEqual(result, {"check": "failed", "build": "failed", "execute": "not-observed", "repl": "failed"})
        wrong = self.observation(1, stderr="syntax error")
        self.assertEqual(scenarios.assess(rejected, {"check": wrong})["check"], "failed")
        valid = self.observation(1, stderr="argument 1 has type Integer, expected String")
        self.assertEqual(scenarios.assess(rejected, {"check": valid})["check"], "matched")

    def test_timeout_invalid_utf8_wrong_output_and_missing_execution_stay_visible(self):
        case = self.plan["scenarios"][0]
        for observed in (self.observation(stdout="wrong"), self.observation(stdout=case["stdout"], timeout=True),
                         dict(self.observation(stdout=case["stdout"]), stdoutBytes="ff")):
            self.assertEqual(scenarios.assess(case, {"execute": observed})["execute"], "failed")
        self.assertEqual(scenarios.assess(case, {})["execute"], "not-observed")

    def test_real_process_observation_preserves_exit_bytes_and_cleans_timeout_group(self):
        observed = scenarios.observe([sys.executable, "-c", "import os; os.write(1, b'\\xff'); raise SystemExit(7)"], self.root, os.environ)
        self.assertEqual(observed["exitCode"], 7)
        self.assertEqual(observed["stdoutBytes"], "ff")
        observed = scenarios.observe([sys.executable, "-c", "import os,time; print(os.getpid(), flush=True); time.sleep(20)"], self.root, os.environ, timeout=0.2)
        self.assertTrue(observed["timedOut"])
        with self.assertRaises(ProcessLookupError):
            os.kill(int(observed["stdout"].strip()), 0)

    def test_selected_compiler_is_used_for_all_language_paths(self):
        calls = []
        compiler = self.root / "identified-compiler"
        def observe(argv, root, env, source=None, timeout=90):
            calls.append((argv, root, source))
            if len(argv) > 1 and argv[1] == "build":
                (root / "program").write_text("fixture executable")
            return self.observation()
        with patch.object(scenarios, "observe", side_effect=observe):
            scenarios.collect(self.plan["scenarios"][0], compiler, "native", "node", self.root, {})
        self.assertEqual([call[0][0] for call in calls], [compiler, compiler, self.root / "program", compiler])
        self.assertIn("probe()", calls[-1][2])
        self.assertFalse((calls[-1][1] / "src/main.trb").exists())

    def test_successful_build_without_artifact_retains_actual_observations(self):
        recorded = []
        with patch.object(scenarios, "observe", return_value=self.observation()), \
                self.assertRaisesRegex(ValueError, "publish its executable"):
            scenarios.collect(self.plan["scenarios"][0], self.root / "compiler", "native", "node",
                              self.root, {}, lambda result: recorded.append(copy.deepcopy(result)))
        self.assertEqual(set(recorded[-1]), {"check", "build", "execute"})
        self.assertEqual(recorded[-1]["build"]["exitCode"], 0)
        self.assertIsNone(recorded[-1]["execute"])

    def test_preparation_failure_and_interruption_preserve_prior_observations(self):
        compiler = self.root / "candidate"
        compiler.write_text("candidate identity")
        report = self.root / "report.json"
        argv = ["release-scenarios", "--contract-root", str(self.root), "--plan", str(self.plan_path),
                "--compiler", str(compiler), "--compiler-sha256", scenarios.digest(compiler),
                "--source-revision", "a" * 40, "--implementation", "native", "--mode", "native", "--report", str(report)]
        first = {p: self.observation() for p in ("check", "build", "execute", "repl")}
        with patch.object(sys, "argv", argv), patch.object(scenarios, "tool_identities", return_value={}), \
                patch.object(scenarios, "collect", side_effect=[first, ValueError("preparation failed")]):
            with self.assertRaisesRegex(ValueError, "preparation failed"):
                scenarios.main()
        saved = scenarios.read_json(report)
        self.assertEqual(len(saved["cases"]), 2)
        self.assertTrue(saved["cases"][0]["complete"])
        self.assertFalse(saved["cases"][1]["complete"])
        self.assertFalse(saved["complete"])
        self.assertFalse(saved["releaseQualified"])
        self.assertEqual(saved["interruption"], "preparation failed")
        with patch.object(sys, "argv", argv), self.assertRaisesRegex(ValueError, "overwrite"):
            scenarios.main()
        compiler.write_text("changed")
        with patch.object(sys, "argv", argv), self.assertRaisesRegex(ValueError, "candidate"):
            scenarios.main()


if __name__ == "__main__":
    unittest.main()
