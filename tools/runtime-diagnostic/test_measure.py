"""Controls for diagnostic identities, failed observations and first executions."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("hardware_diagnostic", ROOT / "measure.py")
diagnostic = importlib.util.module_from_spec(spec)
spec.loader.exec_module(diagnostic)
from reuse import PREFIX, unpack_verified, validate_reuse, verified_compiler, verified_program
from float_controls import control_sources


class DiagnosticTest(unittest.TestCase):
    def setUp(self):
        self.plan = json.loads((ROOT / "registration.json").read_text())

    def test_registered_public_inputs_are_unchanged(self):
        diagnostic.validate_registration(self.plan)

    def test_selected_cases_reject_missing_unknown_and_duplicate_inputs(self):
        for cases in ([], ["unknown"], ["n-body", "n-body"]):
            with self.subTest(cases=cases), self.assertRaisesRegex(ValueError, "subset"):
                diagnostic.validate_registration(dict(self.plan, cases=cases))

    def test_float_controls_preserve_the_original_and_surrounding_program(self):
        source = (diagnostic.ROOT / "benchmarks/benchmarksgame/pure-go/n-body/main.go").read_text()
        controls = control_sources(source)
        self.assertEqual(controls["control-go-original"], source)
        prefix = source[:source.index("func advance(")]
        suffix = source[source.index("\nfunc nbody("):]
        for changed in controls.values():
            self.assertTrue(changed.startswith(prefix))
            self.assertTrue(changed.endswith(suffix))
        self.assertEqual(controls["control-go-separate"].count("float64("), 12)
        self.assertEqual(controls["control-go-native-order-separate"].count("float64("), 18)
        with self.assertRaisesRegex(ValueError, "source shape changed"):
            control_sources(source.replace("dx*dx + dy*dy + dz*dz", "dx*dx"))

    def test_float_controls_require_exact_source_and_selected_nbody(self):
        plan = copy.deepcopy(self.plan)
        source = diagnostic.ROOT / "benchmarks/benchmarksgame/pure-go/n-body/main.go"
        plan["floatControls"] = {"sourceSHA256": diagnostic.digest(source),
                                 "goVersion": "go version go1.27.3 linux/arm64"}
        diagnostic.validate_registration(plan)
        for field, value in (("sourceSHA256", "0" * 64), ("goVersion", "latest")):
            changed = copy.deepcopy(plan)
            changed["floatControls"][field] = value
            with self.assertRaisesRegex(ValueError, "exact N-body"):
                diagnostic.validate_registration(changed)
        plan["cases"] = ["closures"]
        with self.assertRaisesRegex(ValueError, "exact N-body"):
            diagnostic.validate_registration(plan)

    def test_floating_sources_and_changed_workloads_are_rejected(self):
        for field, value in (("revision", "main"), ("appSHA256", {})):
            plan = copy.deepcopy(self.plan)
            plan["variants"]["accepted"][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                diagnostic.validate_registration(plan)
        self.plan["suiteSHA256"] = "0" * 64
        with self.assertRaisesRegex(ValueError, "workload suite changed"):
            diagnostic.validate_registration(self.plan)

    def test_reruns_do_not_replace_a_completed_cohort(self):
        with patch.dict(os.environ, {"GITHUB_RUN_ATTEMPT": "2"}):
            with self.assertRaisesRegex(ValueError, "retry"):
                diagnostic.validate_registration(self.plan)

    def test_first_execution_and_unresolved_cpu_remain_visible(self):
        records = [{"phase": "first" if n == 0 else "steady", "status": "pass",
                    "wallSeconds": 10 if n == 0 else 1, "cpuSeconds": 0,
                    "memoryBytes": 1024} for n in range(6)]
        result = diagnostic.summary(records)
        self.assertEqual(result["wallSeconds"], 1)
        self.assertEqual(result["allWallSpread"], 10)
        self.assertEqual(result["steadyWallSpread"], 1)
        self.assertFalse(result["cpuResolved"])
        records[0]["status"] = "output-mismatch"
        self.assertIsNone(diagnostic.summary(records))
        self.assertIsNone(diagnostic.summary(records[1:]))

    def test_each_implementation_rotates_independently_of_case_order(self):
        for count in (1, 4, 16):
            for width in (4, 5, 7):
                roles = [f"role-{n}" for n in range(width)]
                cases = [({"id": n}, None, None, roles, {}) for n in range(count)]
                first_roles = {n: [] for n in range(count)}
                for iteration in range(width):
                    schedule = list(diagnostic.runtime_schedule(cases, iteration))
                    self.assertEqual(len(schedule), count * width)
                    grouped = [schedule[n:n + width] for n in range(0, len(schedule), width)]
                    self.assertEqual([group[0][0][0]["id"] for group in grouped],
                                     list(range(iteration % count, count))
                                     + list(range(iteration % count)))
                    for group in grouped:
                        case_id = group[0][0][0]["id"]
                        self.assertEqual({entry[0]["id"] for entry, _role in group}, {case_id})
                        self.assertCountEqual([role for _entry, role in group], roles)
                        first_roles[case_id].append(group[0][1])
                for first in first_roles.values():
                    self.assertCountEqual(first, roles)

    def test_reuse_requires_exact_archive_and_preparation_identifiers(self):
        valid = {"runId": 1, "artifactId": 2, "archiveSHA256": "a" * 64,
                 "controllerRevision": "b" * 40}
        validate_reuse(valid)
        for key, value in (("runId", True), ("artifactId", -1),
                           ("archiveSHA256", "latest"), ("controllerRevision", "main")):
            changed = dict(valid, **{key: value})
            with self.subTest(key=key), self.assertRaises(ValueError):
                validate_reuse(changed)
        plan = copy.deepcopy(self.plan)
        plan["reuse"] = valid
        plan["variants"]["accepted"]["reusedRole"] = "../other"
        with self.assertRaisesRegex(ValueError, "preparation mode"):
            diagnostic.validate_registration(plan)

    def test_changed_archive_and_unsafe_members_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "evidence.zip"
            with zipfile.ZipFile(archive, "w") as packed:
                packed.writestr(PREFIX + "../outside", b"untrusted")
            with self.assertRaisesRegex(ValueError, "digest differs"):
                unpack_verified(archive, root / "output", "0" * 64)
            with self.assertRaisesRegex(ValueError, "Unsafe"):
                unpack_verified(archive, root / "output", diagnostic.digest(archive))
            self.assertFalse((root / "outside").exists())

    def test_archive_reuse_extracts_only_registered_evidence(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "evidence.zip"
            with zipfile.ZipFile(archive, "w") as packed:
                packed.writestr(PREFIX + "context.json", b"{}")
                packed.writestr("unrelated/file", b"not evidence")
            output = root / "output"
            unpack_verified(archive, output, diagnostic.digest(archive))
            self.assertEqual([p.name for p in output.iterdir()], ["context.json"])

    def test_reused_binary_tampering_and_unclosed_compilers_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = root / "compilers/accepted"
            (directory / "3").mkdir(parents=True)
            compiler = directory / "3/compiler"
            compiler.write_bytes(b"verified compiler")
            identity = {"sha256": diagnostic.digest(compiler), "bytes": compiler.stat().st_size}
            generations = directory / "generations.json"
            generations.write_text(json.dumps([identity] * 4))
            self.assertEqual(verified_compiler(root, "accepted"), compiler)
            generations.write_text(json.dumps([identity]))
            with self.assertRaisesRegex(ValueError, "fixed point"):
                verified_compiler(root, "accepted")
            generations.write_text(json.dumps([identity] * 4))
            compiler.write_bytes(b"changed compiler")
            with self.assertRaisesRegex(ValueError, "identity differs"):
                verified_compiler(root, "accepted")
            (root / "program").write_bytes(b"changed app")
            with self.assertRaisesRegex(ValueError, "identity differs"):
                verified_program(root, identity)


if __name__ == "__main__":
    unittest.main()
