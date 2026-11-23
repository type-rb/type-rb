#!/usr/bin/env python3
"""Fail-closed dependency identity and capability controls."""
import copy
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import qbe_toolchain as qbe


class BackendTest(unittest.TestCase):
    def test_reordered_calls_or_missing_dump_is_rejected(self):
        for dump in ("", "> After GCM:\ncall $inspect_order\ncall $change_order\n"):
            with self.subTest(dump=dump), patch.object(
                    qbe.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", dump)):
                with self.assertRaisesRegex(ValueError, "ordered side effects"):
                    qbe.check_ordered_effects(Path("synthetic-order-qbe"))

    def test_unused_copy_cycle_or_missing_dump_is_rejected(self):
        for dump in ("", "> After GCM:\n@loop\n %left =l phi @start %a, @body %right\nret %i\n"):
            with self.subTest(dump=dump), patch.object(
                    qbe.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", dump)):
                with self.assertRaisesRegex(ValueError, "dead copy cycle"):
                    qbe.check_copy_liveness(Path("synthetic-copy-qbe"))

    def test_missing_or_unconditional_contraction_is_rejected(self):
        for assembly in ("fmul d0, d0, d1\nfadd d0, d0, d2\n", "fmadd d0, d0, d1, d2\n"):
            with self.subTest(assembly=assembly), patch.object(
                    qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                        [], 0, assembly, "")):
                with self.assertRaisesRegex(ValueError, "Float contraction"):
                    qbe.check_contraction(Path("synthetic-contraction-qbe"))

    def test_identity_and_patch_tampering(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            shutil.copytree(qbe.ROOT / "tools/qbe", root / "tools/qbe")
            original = qbe.backend_identity(root)
            self.assertEqual(original["variant"], "typerb-late-product-v1")
            manifest = root / "tools/qbe/backend.json"
            for key, value in (("variant", "stock"), ("version", "1.4"),
                               ("patches", []), ("patches", original["patches"][::-1]),
                               ("patches", [original["patches"][0]] * 2), ("sourceSHA256", "invalid")):
                with self.subTest(key=key):
                    changed = copy.deepcopy(original)
                    changed[key] = value
                    manifest.write_text(json.dumps(changed))
                    with self.assertRaises(ValueError):
                        qbe.backend_identity(root)
            manifest.write_text(json.dumps(original))
            for item in original["patches"]:
                path = root / item["path"]
                text = path.read_text()
                path.write_text(text + "changed\n")
                with self.assertRaisesRegex(ValueError, "patch digest"):
                    qbe.backend_identity(root)
                path.write_text(text)

    def test_corrupt_archive_is_rejected_before_unpacking(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "source.tar.xz"
            archive.write_bytes(b"corrupt source")
            with patch.object(qbe.subprocess, "run") as run:
                with self.assertRaisesRegex(ValueError, "archive digest"):
                    qbe.build(root / "output", archive)
                run.assert_not_called()
            self.assertFalse((root / "output/qbe").exists())

    def test_stock_or_partial_backend_cannot_pass_capability(self):
        for status, assembly in ((1, ""), (0, "call sqrt"), (0, "fsqrt d0, d0")):
            with self.subTest(status=status, assembly=assembly):
                with patch.object(qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                        [], status, assembly, "unsupported")):
                    with self.assertRaisesRegex(ValueError, "required"):
                        qbe.check_capability(Path("synthetic-qbe"))

    def test_verified_cache_requires_matching_binary_and_inputs(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "qbe"
            binary.write_bytes(b"not a verified binary")
            (root / "build-info.json").write_text(json.dumps({"inputs": {}, "binarySHA256": qbe.digest(binary)}))
            archive = root / "source.tar.xz"
            archive.write_bytes(b"corrupt source")
            with self.assertRaisesRegex(ValueError, "archive digest"):
                qbe.build(root, archive)
            self.assertEqual(binary.read_bytes(), b"not a verified binary")

    def test_constant_memory_backend_does_not_claim_indexed_capability(self):
        previous = ("fsqrt d0, d0\nadd x0, x0, #7\nand x0, x0, #65535\nasr x0, x0, #1\n"
                    "ldr x0, [x1, #8]\nstr x0, [x1, #16]\n")
        with patch.object(qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                [], 0, previous, "")):
            with self.assertRaisesRegex(ValueError, "indexed memory"):
                qbe.check_capability(Path("synthetic-previous-qbe"))

    def test_single_use_backend_does_not_claim_shared_memory_capability(self):
        previous = ("fsqrt d0, d0\nadd x0, x0, #7\nand x0, x0, #65535\nasr x0, x0, #1\n"
                    "ldr x0, [x1, #8]\nstr x0, [x1, #16]\n"
                    "ldr x0, [x1, x2, lsl #3]\nstr w0, [x1, x2, lsl #2]\n"
                    "shared_memory:\nadd x3, x1, x2, lsl #3\nldr x0, [x3]\nstr x4, [x3]\n")
        with patch.object(qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                [], 0, previous, "")):
            with self.assertRaisesRegex(ValueError, "shared memory"):
                qbe.check_capability(Path("synthetic-single-use-qbe"))

    def test_previous_motion_does_not_claim_invariant_guard_capability(self):
        assembly = ("fsqrt d0, d0\nsqrtsd %xmm0, %xmm0\nadd x0, x0, #7\n"
                    "and x0, x0, #65535\nasr x0, x0, #1\n"
                    "ldr x0, [x1, #8]\nstr x0, [x1, #16]\n"
                    "ldr x0, [x1, x2, lsl #3]\nstr w0, [x1, x2, lsl #2]\n"
                    "shared_memory:\nldr x0, [x1, x2, lsl #3]\n"
                    "str x4, [x1, x2, lsl #3]\n")
        for dump in ("", "> After GCM:\n@body\n %snk.1 =w csltl %a, %b\n@yes\n"):
            with self.subTest(dump=dump), patch.object(
                    qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                        [], 0, assembly, dump)):
                with self.assertRaisesRegex(ValueError, "invariant expression motion"):
                    qbe.check_capability(Path("synthetic-previous-motion-qbe"))

    def test_guard_only_backend_does_not_claim_address_motion(self):
        assembly = ("fsqrt d0, d0\nsqrtsd %xmm0, %xmm0\nadd x0, x0, #7\n"
                    "and x0, x0, #65535\nasr x0, x0, #1\n"
                    "ldr x0, [x1, #8]\nstr x0, [x1, #16]\n"
                    "ldr x0, [x1, x2, lsl #3]\nstr w0, [x1, x2, lsl #2]\n"
                    "shared_memory:\nldr x0, [x1, x2, lsl #3]\n"
                    "str x4, [x1, x2, lsl #3]\n")
        dump = "> After GCM:\n@body\n %value =l load %address\n@yes\n@done\n"
        with patch.object(qbe.subprocess, "run", return_value=subprocess.CompletedProcess(
                [], 0, assembly, dump)):
            with self.assertRaisesRegex(ValueError, "loop address rematerialization"):
                qbe.check_capability(Path("synthetic-guard-only-qbe"))


if __name__ == "__main__":
    unittest.main()
