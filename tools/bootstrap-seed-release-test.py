#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("seed_release", Path(__file__).with_name("bootstrap-seed-release.py"))
seed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(seed)


class SeedReleaseTests(unittest.TestCase):
    def test_invalid_corpus_has_complete_diagnostic_expectations(self):
        directory = Path(__file__).resolve().parent.parent / "compiler/conformance/invalid"
        sources = {path.stem for path in directory.glob("*.source")}
        diagnostics = {path.stem for path in directory.glob("*.diag")}
        self.assertTrue(sources)
        self.assertEqual(sources, diagnostics, "every rejection fixture needs its exact diagnostic oracle")
        for path in directory.glob("*.diag"):
            with self.subTest(fixture=path.stem):
                self.assertTrue(path.read_text().startswith("TRBN"))

    def test_active_ci_consumers_authenticate_the_current_seed(self):
        root = Path(__file__).resolve().parent.parent
        for name in ("runtime-worker-memory",
                     "benchmarksgame-formal", "benchmarksgame-build-formal",
                     "linux-amd64-targets", "daily-performance", "weekly-performance"):
            with self.subTest(workflow=name):
                workflow = (root / ".github/workflows" / (name + ".yml")).read_text()
                self.assertIn("bootstrap-seed-2026-09-29-module-cycles", workflow)
                self.assertIn("04993b6bf598356a51143edd9dbde1a024c403cc", workflow)
                self.assertIn("tools/bootstrap-seed-download.sh", workflow)
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn("ROOT_RELEASE_TAG: bootstrap-seed-2026-08-30", workflow)
        self.assertIn("path: .native-target-seed-source", workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-seed-source"', workflow)
        self.assertIn("AMD64_LOOP_SETUP_REVISION: 4e1d0b4aee97b9a5bd73a98f918b31d47985da25", workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-loop-source"', workflow)

    def test_amd64_bridge_does_not_replace_ordinary_candidate_input(self):
        observer = Path(__file__).with_name("linux-amd64-targets.sh").read_text()
        self.assertIn('"$root_compiler" emit-qbe "$seed_entry"', observer)
        self.assertIn('runtime_seed=$first_transition', observer)
        self.assertIn('"$first_transition" emit-qbe "$loop_entry"', observer)
        self.assertIn('runtime_seed=$loop_transition', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$compiler_entry"', observer)
        self.assertIn('4e1d0b4aee97b9a5bd73a98f918b31d47985da25 || fail "loop source revision differs"', observer)
        self.assertIn('require_clean_revision "$loop_source_root"', observer)
        self.assertIn('21f507e7ee7de2577f4137f6dfb9f732c14c1640 || fail "seed source revision differs"', observer)
        self.assertIn('require_clean_revision "$seed_source_root"', observer)
        self.assertIn('compiler/conformance/valid/logical-short-circuit.trb', observer)
        self.assertIn("printf 'ok\\n' > \"$evidence/setup/logical-condition.expected\"", observer)
        self.assertIn('cmp "$evidence/setup/logical-condition.expected" "$evidence/setup/logical-condition.stdout"', observer)
        self.assertNotIn('require_empty_file "$evidence/setup/logical-condition.stdout"', observer)

    def test_boolean_bridge_requires_the_exact_accepted_source(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn('test -n "$loop_source_root" || fail "Boolean source requires the accepted loop source"', observer)
        self.assertIn('require_clean_revision "$boolean_source_root"', observer)
        self.assertIn('57cb41ad6be91716e31fa555ed8ea8c8ce7a5f51 || fail "boolean source revision differs"', observer)
        self.assertIn('"$loop_transition" emit-qbe "$boolean_entry"', observer)
        self.assertIn('runtime_seed=$boolean_transition', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$compiler_entry"', observer)
        self.assertIn('"$boolean_transition" check "$candidate_root/compiler/conformance/valid/boolean-array-values.trb"', observer)
        self.assertIn('AMD64_BOOLEAN_SETUP_REVISION: 57cb41ad6be91716e31fa555ed8ea8c8ce7a5f51', workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-boolean-source"', workflow)

    def test_hash_bridge_requires_the_exact_accepted_source(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn('test -n "$record_source_root" || fail "Hash source requires the accepted record Array source"', observer)
        self.assertIn('require_clean_revision "$hash_source_root"', observer)
        self.assertIn('8a6d9ff73b14a97bca1b010ddaad6a38972b5373 || fail "Hash source revision differs"', observer)
        self.assertIn('"$record_transition" emit-qbe "$hash_entry"', observer)
        self.assertIn('runtime_seed=$hash_transition', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$compiler_entry"', observer)
        self.assertIn('"$hash_transition" check "$candidate_root/compiler/conformance/valid/hash-values.trb"', observer)
        self.assertIn('AMD64_HASH_SETUP_REVISION: 8a6d9ff73b14a97bca1b010ddaad6a38972b5373', workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-hash-source"', workflow)

    def test_cycle_bridge_preserves_historical_argument_shapes(self):
        script = Path(__file__).with_name("linux-amd64-targets.sh").resolve()
        missing = str(Path(self.temporary.name) / "missing-source")
        for count in range(8, 19):
            with self.subTest(arguments=count):
                result = subprocess.run(["/bin/sh", str(script), *([missing] * count)],
                                        capture_output=True, text=True, timeout=10)
                if 9 <= count <= 17:
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertTrue(result.stderr.startswith("linux-amd64-targets:"), result.stderr)
                    self.assertNotIn("usage:", result.stderr)
                else:
                    self.assertEqual(result.returncode, 64, result.stderr)
                    self.assertIn("usage: linux-amd64-targets.sh", result.stderr)
                self.assertEqual(result.stdout, "")
                self.assertFalse(Path(missing).exists())

    def test_iteration_bridge_retains_source_and_candidate_boundaries(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn('test -n "$hash_source_root" || fail "Array iteration source requires the accepted Hash source"', observer)
        self.assertIn('require_clean_revision "$iteration_source_root"', observer)
        self.assertIn('508f721f8964d67a5893e547d2e2fb3de5b20a63 || fail "Array iteration source revision differs"', observer)
        self.assertIn('"$hash_transition" emit-qbe "$iteration_entry"', observer)
        self.assertIn('runtime_seed=$iteration_transition', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$compiler_entry"', observer)
        self.assertIn('"$iteration_transition" check "$candidate_root/compiler/conformance/valid/array-iteration-$iteration_case.trb"', observer)
        self.assertIn('AMD64_ITERATION_SETUP_REVISION: 508f721f8964d67a5893e547d2e2fb3de5b20a63', workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-iteration-source"', workflow)

    def test_compiler_name_bridge_retains_accepted_source_and_process_boundary(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn('test -n "$iteration_source_root" || fail "compiler-name source requires the accepted Array iteration source"', observer)
        self.assertIn('require_clean_revision "$names_source_root"', observer)
        self.assertIn('6ca79d22cde2ddba5fe836c66899b6e08a6511dc || fail "compiler-name source revision differs"', observer)
        self.assertIn('9d3fc404ea55f459ef4bf5417112f0bb47054077ea94f3f6a5d57fbc23958ad0 || fail "compiler-name entry digest differs"', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$names_entry"', observer)
        self.assertIn('runtime_seed=$names_transition', observer)
        self.assertIn('"$names_transition" check "$names_check_entry"', observer)
        self.assertIn('require_forbidden_processes_absent "$evidence/setup/names-check-process.trace"', observer)
        self.assertIn('AMD64_NAMES_SETUP_REVISION: 6ca79d22cde2ddba5fe836c66899b6e08a6511dc', workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-names-source"', workflow)

    def test_cycle_bridge_retains_accepted_source_and_prior_handoff(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        workflow = (root / ".github/workflows/linux-amd64-targets.yml").read_text()
        self.assertIn('require_clean_revision "$cycles_source_root"', observer)
        self.assertIn('a0f9147d29d73ad8a0237c7c51a0a5993bbe3a36 || fail "module-cycle source revision differs"', observer)
        self.assertIn('19cd0afbd30f2113c3cfc5bf096ea9108ac9de8a2a835e63d68f8b49f535c116 || fail "module-cycle entry digest differs"', observer)
        self.assertIn('names_check_entry=$cycles_entry', observer)
        self.assertIn('"$runtime_seed" emit-qbe "$cycles_entry"', observer)
        self.assertIn('runtime_seed=$cycles_transition', observer)
        self.assertIn('"$cycles_transition" check "$compiler_entry"', observer)
        self.assertIn('require_forbidden_processes_absent "$trace" "module-cycle capability proof"', observer)
        self.assertIn('AMD64_CYCLES_SETUP_REVISION: a0f9147d29d73ad8a0237c7c51a0a5993bbe3a36', workflow)
        self.assertIn('"$GITHUB_WORKSPACE/.native-target-cycles-source"', workflow)

    def test_target_fixture_identity_matches_current_sources(self):
        root = Path(__file__).resolve().parent.parent
        observer = (root / "tools/linux-amd64-targets.sh").read_text()
        for relative in ("portable-entry/trbconfig.jsonc", "portable-entry/src/main.trb",
                         "runtime-failures/trbconfig.jsonc", "runtime-failures/src/main.trb"):
            with self.subTest(source=relative):
                self.assertIn(seed.digest(root / "corpus/portable-entry" / relative), observer)

    def test_refresh_roles_match_the_existing_compatibility_boundary(self):
        observer = Path(__file__).with_name("bootstrap-seed-refresh.sh").read_text()
        self.assertIn('build_step "$seed" setup-first setup', observer)
        self.assertIn('build_step "$workspace/setup-first/compiler" setup-runtime setup', observer)
        self.assertIn('for step in b2 b3 b4; do\n    build_step "$seed" "$step" ordinary', observer)
        self.assertIn('if test "$mode" = verify; then\n    cmp "$input" "$workspace/b4/compiler"', observer)

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.inputs = self.root / "inputs"
        self.inputs.mkdir()
        self.revision = "a" * 40
        for name, os_name, runner, target, limit in seed.TARGETS:
            asset = "type-rb-native-bootstrap-" + name
            binary = self.inputs / asset
            binary.write_bytes(("synthetic " + name).encode())
            checksum = seed.digest(binary)
            metadata = {"asset": asset, "profile": name + "-v0", "os": os_name,
                        "architecture": "arm64", "runnerImage": runner, "qbeTarget": target,
                        "ccBoundary": "system-cc", "mode": "0755", "size": binary.stat().st_size,
                        "sha256": checksum, "attestationSubjectSha256": checksum,
                        "qbeBinarySize": 100, "qbeBinarySha256": "b" * 64}
            (self.inputs / (name + ".json")).write_text(json.dumps(metadata))
        self.package = self.root / "package"
        seed.create(self.revision, self.inputs, self.package)
        self.manifest_path = self.package / seed.MANIFEST
        self.manifest = seed.read_json(self.manifest_path)
        self.asset = self.manifest["targets"][0]["asset"]
        self.release = {"tag_name": seed.TAG, "target_commitish": self.revision,
                        "draft": False, "prerelease": True, "immutable": True,
                        "assets": [{"name": p.name, "size": p.stat().st_size, "digest": "sha256:" + seed.digest(p)}
                                   for p in sorted(self.package.iterdir())]}
        self.release_path = self.root / "release.json"
        self.release_path.write_text(json.dumps(self.release))

    def verify(self):
        seed.verify(seed.TAG, self.revision, self.asset, self.package, self.release_path)

    def test_both_targets_and_repeatable_package(self):
        for target in self.manifest["targets"]:
            seed.verify(seed.TAG, self.revision, target["asset"], self.package, self.release_path)
        repeated = self.root / "repeated"
        seed.create(self.revision, self.inputs, repeated)
        for path in self.package.iterdir():
            self.assertEqual(path.read_bytes(), (repeated / path.name).read_bytes())
        with self.assertRaises(FileExistsError):
            seed.create(self.revision, self.inputs, self.package)

    def test_manifest_rejects_changed_identity_or_unknown_fields(self):
        mutations = [
            lambda m: m.update(unexpected=True), lambda m: m.update(schemaVersion=True),
            lambda m: m.update(schemaVersion=1), lambda m: m.update(releaseTag="latest"),
            lambda m: m.update(nativeRevision="c" * 40), lambda m: m.update(status="stable"),
            lambda m: m["predecessor"].update(releaseTag="other"),
            lambda m: m["predecessor"]["targets"][0].update(sha256="c" * 64),
            lambda m: m["backend"].update(version="other"),
            lambda m: m["targets"].reverse(), lambda m: m["targets"].pop(),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                manifest = copy.deepcopy(self.manifest)
                mutate(manifest)
                with self.assertRaises(ValueError):
                    seed.validate_manifest(manifest, self.revision)

    def test_target_shape_types_limits_and_provenance(self):
        mutations = {"unexpected": 1, "size": True, "sha256": "X" * 64,
                     "attestationSubjectSha256": "c" * 64, "qbeBinarySize": -1,
                     "qbeBinarySha256": "c", "ccBoundary": "wrapper", "mode": "0777",
                     "runnerImage": "self-hosted", "architecture": "amd64"}
        for index, target in enumerate(self.manifest["targets"]):
            for field, value in mutations.items():
                with self.subTest(index=index, field=field):
                    changed = copy.deepcopy(target)
                    changed[field] = value
                    with self.assertRaises(ValueError):
                        seed.validate_target(changed, seed.TARGETS[index])
            target["size"] = seed.TARGETS[index][4] + 1
            with self.assertRaises(ValueError):
                seed.validate_target(target, seed.TARGETS[index])

    def test_registered_predecessors_remain_tag_bound(self):
        for tag in seed.PREDECESSORS:
            manifest = copy.deepcopy(self.manifest)
            manifest["releaseTag"] = tag
            manifest["predecessor"] = seed.PREDECESSORS[tag]
            seed.validate_manifest(manifest, self.revision, tag)
            for other in seed.PREDECESSORS:
                if other != tag:
                    with self.assertRaises(ValueError):
                        seed.validate_manifest(manifest, self.revision, other)
        for tag in ("latest", "bootstrap-seed-2099-01-01", None, []):
            with self.assertRaises(ValueError):
                seed.validate_manifest(self.manifest, self.revision, tag)

    def test_manifest_size_limits_remain_tag_bound(self):
        for tag in seed.PREDECESSORS:
            manifest = copy.deepcopy(self.manifest)
            manifest["releaseTag"] = tag
            manifest["predecessor"] = seed.PREDECESSORS[tag]
            if seed.LIMITS[tag][2] is None:
                continue
            for target, limit in zip(manifest["targets"], seed.LIMITS[tag][:2]):
                target["size"] = limit
            seed.validate_manifest(manifest, self.revision, tag)
            for index in range(2):
                changed = copy.deepcopy(manifest)
                changed["targets"][index]["size"] += 1
                with self.assertRaises(ValueError):
                    seed.validate_manifest(changed, self.revision, tag)
            manifest["targets"][0]["size"] = 349296
            manifest["targets"][1]["size"] = 325864
            if tag in (seed.TAG, "bootstrap-seed-2026-09-12-compiler-names") or tag in ("bootstrap-seed-2026-09-08-boolean-arrays", "bootstrap-seed-2026-09-09-record-arrays", "bootstrap-seed-2026-09-10-hash", "bootstrap-seed-2026-09-11-array-iteration"):
                seed.validate_manifest(manifest, self.revision, tag)
            else:
                with self.assertRaises(ValueError):
                    seed.validate_manifest(manifest, self.revision, tag)

    def test_record_array_budget_does_not_relax_historical_manifests(self):
        for tag in seed.PREDECESSORS:
            manifest = copy.deepcopy(self.manifest)
            manifest["releaseTag"] = tag
            manifest["predecessor"] = seed.PREDECESSORS[tag]
            manifest["targets"][0]["size"] = 365808
            manifest["targets"][1]["size"] = 327736
            if tag in (seed.TAG, "bootstrap-seed-2026-09-12-compiler-names") or tag in ("bootstrap-seed-2026-09-09-record-arrays", "bootstrap-seed-2026-09-10-hash", "bootstrap-seed-2026-09-11-array-iteration"):
                seed.validate_manifest(manifest, self.revision, tag)
            else:
                with self.assertRaises(ValueError):
                    seed.validate_manifest(manifest, self.revision, tag)

    def test_hash_budget_does_not_relax_historical_manifests(self):
        for tag in seed.PREDECESSORS:
            manifest = copy.deepcopy(self.manifest)
            manifest["releaseTag"] = tag
            manifest["predecessor"] = seed.PREDECESSORS[tag]
            manifest["targets"][0]["size"] = 398888
            manifest["targets"][1]["size"] = 368936
            if tag in (seed.TAG, "bootstrap-seed-2026-09-12-compiler-names") or tag in ("bootstrap-seed-2026-09-10-hash", "bootstrap-seed-2026-09-11-array-iteration"):
                seed.validate_manifest(manifest, self.revision, tag)
            else:
                with self.assertRaises(ValueError):
                    seed.validate_manifest(manifest, self.revision, tag)

    def test_iteration_budget_does_not_relax_historical_manifests(self):
        for tag in seed.PREDECESSORS:
            manifest = copy.deepcopy(self.manifest)
            manifest["releaseTag"] = tag
            manifest["predecessor"] = seed.PREDECESSORS[tag]
            manifest["targets"][0]["size"] = 415432
            manifest["targets"][1]["size"] = 387192
            if tag in (seed.TAG, "bootstrap-seed-2026-09-12-compiler-names", "bootstrap-seed-2026-09-11-array-iteration"):
                seed.validate_manifest(manifest, self.revision, tag)
            else:
                with self.assertRaises(ValueError):
                    seed.validate_manifest(manifest, self.revision, tag)

    def test_cycle_refresh_authenticates_predecessor_and_checks_basic_features(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/bootstrap-seed-refresh.yml").read_text()
        self.assertEqual(seed.TAG, "bootstrap-seed-2026-09-29-module-cycles")
        self.assertIn("tag=bootstrap-seed-2026-09-12-compiler-names", workflow)
        self.assertIn("revision=d7ffb9384229125216696a220c7f370422472178", workflow)
        self.assertIn("'bootstrap-seed-2026-09-29-module-cycles' || github.sha", workflow)
        self.assertIn("tag=bootstrap-seed-2026-09-12-compiler-names", workflow)
        self.assertIn("REGISTERED_COMPILER_SOURCE_TREE: d811b0d7868f5e3701586fd563ef5b496eaba058", workflow)
        observer = (root / "tools/bootstrap-seed-refresh.sh").read_text()
        self.assertIn("tools/bootstrap-module-cycles.py", observer)
        for fixture in ("array-iteration-live", "array-iteration-control", "array-iteration-managed", "range-values", "range-streaming", "range-extrema", "range-managed"):
            self.assertIn(fixture, observer)
            self.assertTrue((root / "compiler/conformance/valid" / (fixture + ".trb")).is_file())

    def test_migration_size_observations_keep_identity_and_types_strict(self):
        manifest = copy.deepcopy(self.manifest)
        for target in manifest["targets"]:
            target["size"] = 900000
        seed.validate_manifest(manifest, self.revision)
        for value in (True, 0, -1, 1.5, None, "900000"):
            changed = copy.deepcopy(manifest)
            changed["targets"][0]["size"] = value
            with self.subTest(value=value), self.assertRaises(ValueError):
                seed.validate_manifest(changed, self.revision)
        # Both migration-era tags retain observations; strict predecessors keep caps.
        for tag in seed.PREDECESSORS:
            changed = copy.deepcopy(manifest)
            changed["releaseTag"] = tag
            changed["predecessor"] = seed.PREDECESSORS[tag]
            if tag in ("bootstrap-seed-2026-09-12-compiler-names", "bootstrap-seed-2026-09-29-module-cycles"):
                self.assertEqual(seed.LIMITS[tag], (None, None, None))
                seed.validate_manifest(changed, self.revision, tag)
            else:
                with self.subTest(tag=tag), self.assertRaises(ValueError):
                    seed.validate_manifest(changed, self.revision, tag)

    def test_download_verification_preserves_the_previous_tag(self):
        for tag in seed.PREDECESSORS:
            if tag == seed.TAG:
                continue
            self.manifest["releaseTag"] = tag
            self.manifest["predecessor"] = seed.PREDECESSORS[tag]
            self.manifest_path.write_text(json.dumps(self.manifest))
            names = [target["asset"] for target in self.manifest["targets"]] + [seed.MANIFEST]
            (self.package / "SHA256SUMS").write_text("".join(
                seed.digest(self.package / name) + "  " + name + "\n" for name in names))
            self.release["tag_name"] = tag
            self.release["assets"] = [{"name": p.name, "size": p.stat().st_size,
                                       "digest": "sha256:" + seed.digest(p)}
                                      for p in sorted(self.package.iterdir())]
            self.release_path.write_text(json.dumps(self.release))
            for target in self.manifest["targets"]:
                seed.verify(tag, self.revision, target["asset"], self.package, self.release_path)
        with self.assertRaises(ValueError):
            self.verify()

    def test_corruption_and_checksum_reordering_fail(self):
        (self.package / self.asset).write_bytes(b"changed")
        with self.assertRaises(ValueError):
            self.verify()
        (self.package / self.asset).write_bytes((self.inputs / self.asset).read_bytes())
        sums = self.package / "SHA256SUMS"
        sums.write_text("\n".join(reversed(sums.read_text().splitlines())) + "\n")
        with self.assertRaises(ValueError):
            self.verify()

    def test_release_state_and_complete_asset_set(self):
        mutations = [lambda r: r.update(draft=True), lambda r: r.update(immutable=False),
                     lambda r: r.update(prerelease=False), lambda r: r.update(tag_name="other"),
                     lambda r: r.update(target_commitish="main"), lambda r: r["assets"].pop(),
                     lambda r: r["assets"].append(r["assets"][0]),
                     lambda r: r["assets"][0].update(digest="sha256:" + "c" * 64)]
        for mutate in mutations:
            release = copy.deepcopy(self.release)
            mutate(release)
            self.release_path.write_text(json.dumps(release))
            with self.assertRaises(ValueError):
                self.verify()

    def test_duplicate_json_keys_and_nonfinite_numbers_fail(self):
        for text in ('{"key":1,"key":2}', '{"value":NaN}', '{"value":Infinity}'):
            self.release_path.write_text(text)
            with self.assertRaises(ValueError):
                seed.read_json(self.release_path)

    def test_bad_input_does_not_create_package(self):
        (self.inputs / self.asset).write_bytes(b"corrupt")
        output = self.root / "not-created"
        with self.assertRaises(ValueError):
            seed.create(self.revision, self.inputs, output)
        self.assertFalse(output.exists())

    def test_all_retained_observations_are_checked(self):
        path = self.root / "observations.csv"
        header = "stage,iteration,elapsed_seconds,peak_rss_bytes,status,cpu_seconds\n"
        for role, stages in (("ordinary", ("b1-b2", "b2-b3", "b3-b4")),
                             ("transition", ("b2-b3", "b3-b4"))):
            rows = [f"{stage},{iteration},1,100,0,1\n" for stage in stages for iteration in range(1, 8)]
            path.write_text(header + "".join(rows))
            seed.observations(path, role)
            for index in range(len(rows)):
                for old, new in ((",1,100,0,1", ",2.1,100,0,1"),
                                 (",1,100,0,1", ",1,201,0,1"),
                                 (",1,100,0,1", ",1,100,0,2.1"),
                                 (",1,100,0,1", ",NaN,100,0,1"),
                                 (",1,100,0,1", ",1,100,1,1")):
                    changed = rows.copy()
                    changed[index] = changed[index].replace(old, new)
                    path.write_text(header + "".join(changed))
                    with self.assertRaises(ValueError):
                        seed.observations(path, role)
            path.write_text(header + "".join(rows[:-1]))
            with self.assertRaises(ValueError):
                seed.observations(path, role)


if __name__ == "__main__":
    unittest.main()
