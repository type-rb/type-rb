from pathlib import Path
import json
import tempfile
import unittest

from bootstrap_snapshot_size import catalog_object, check_counts, check_size, compiler_input_limit, decoder_limit


class SnapshotSizeTest(unittest.TestCase):
    def test_discarded_members_preserve_catalog_lengths(self):
        source = dict(format="type-rb-bootstrap-snapshot", version=4,
                      sources=[{"id": "main"}], types=[],
                      functions=[{"blocks": [{"instructions": [{"op": "literal"}]}]}, {}])
        decoded = json.loads(json.dumps(source), object_pairs_hook=catalog_object)
        self.assertEqual(decoded["functions"], [None, None])
        self.assertEqual(decoded["sources"], [None])
        decoder = Path(__file__).resolve().parent.parent / "recovery/src/managed/snapshot.trb"
        self.assertEqual(check_counts(decoded, decoder)["functions"][0], 2)

    def test_catalog_limits_are_read_from_the_actual_decoder(self):
        decoder = Path(__file__).resolve().parent.parent / "recovery/src/managed/snapshot.trb"
        for field, constant in (("sources", "COMPILER_RECOVERY_MAX_SOURCES"),
                                ("types", "MANAGED_RECOVERY_SNAPSHOT_MAX_TYPES"),
                                ("functions", "COMPILER_RECOVERY_MAX_FUNCTIONS")):
            limit = decoder_limit(decoder, constant)
            for count in (0, limit, limit + 1):
                with self.subTest(field=field, count=count):
                    snapshot = dict(format="type-rb-bootstrap-snapshot", version=4,
                                    sources=[], types=[], functions=[])
                    snapshot[field] = [None] * count
                    if count <= limit:
                        self.assertEqual(check_counts(snapshot, decoder)[field], (count, limit))
                    else:
                        with self.assertRaisesRegex(ValueError, f"has {count} {field}"):
                            check_counts(snapshot, decoder)

    def test_missing_catalogs_or_wrong_schema_do_not_pass(self):
        decoder = Path(__file__).resolve().parent.parent / "recovery/src/managed/snapshot.trb"
        for snapshot in ({}, [], {"format": "type-rb-bootstrap-snapshot", "version": 4},
                         dict(format="type-rb-bootstrap-snapshot", version=3, sources=[], types=[], functions=[]),
                         dict(format="type-rb-bootstrap-snapshot", version=4, sources=[], types=[], functions=None)):
            with self.subTest(snapshot=snapshot), self.assertRaises(ValueError):
                check_counts(snapshot, decoder)

    def test_actual_decoder_boundary_without_allocating_large_input(self):
        decoder = Path(__file__).resolve().parent.parent / "recovery/src/managed/snapshot.trb"
        limit = compiler_input_limit(decoder)
        with tempfile.TemporaryDirectory() as directory:
            snapshot = Path(directory) / "snapshot.json"
            for size in (1, limit, limit + 1, 0):
                with self.subTest(size=size):
                    with snapshot.open("wb") as output:
                        output.truncate(size)
                    if 0 < size <= limit:
                        self.assertEqual(check_size(snapshot, decoder), (size, limit))
                    else:
                        with self.assertRaisesRegex(ValueError, f"has {size} bytes"):
                            check_size(snapshot, decoder)

    def test_missing_or_ambiguous_bound_cannot_silently_disable_preflight(self):
        declaration = "COMPILER_RECOVERY_MAX_INPUT_BYTES := 12\n"
        with tempfile.TemporaryDirectory() as directory:
            decoder = Path(directory) / "snapshot.trb"
            for source in ("", declaration * 2, declaration.replace("12", "0")):
                decoder.write_text(source, encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "one positive"):
                    compiler_input_limit(decoder)


if __name__ == "__main__":
    unittest.main()
