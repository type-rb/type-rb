import copy
import unittest

from rss_policy import assess


def fixture(pure=True, baseline=True):
    case = {"id": "workload", "frozenBaseline": baseline}
    if pure:
        case["pureGoSource"] = "synthetic.go"
    values = {"native": 1024 * 1024, "previous": 960 * 1024, "typerb-go": 2048 * 1024}
    if baseline:
        values["baseline"] = 960 * 1024
    if pure:
        values["pure-go"] = 2048 * 1024
    rows = [{"case": "workload", "role": role, "status": "pass", "args": [],
             "expectedSha256": "a" * 64, "runtime": {"memoryBytes": value}}
            for role, value in values.items()]
    return rows, [case]


def set_rss(rows, role, value):
    next(row for row in rows if row["role"] == role)["runtime"]["memoryBytes"] = value


class RuntimeRssPolicyTest(unittest.TestCase):
    def test_small_native_increase_preserves_go_competitiveness(self):
        rows, cases = fixture()
        result = assess(rows, cases)
        self.assertEqual(result["status"], "met")
        self.assertGreater(result["rows"][0]["nativeChanges"]["previous"]["ratio"], 1.05)
        self.assertEqual(result["rows"][0]["nativeChanges"]["previous"]["deltaBytes"], 65536)

    def test_equal_or_worse_than_either_go_is_unmet(self):
        for role in ("typerb-go", "pure-go"):
            for value in (1048576, 1048575):
                rows, cases = fixture()
                set_rss(rows, role, value)
                self.assertEqual(assess(rows, cases)["status"], "unmet")

    def test_ten_percent_headroom_boundary(self):
        for current, expected in ((900000, "met"), (900001, "review-required")):
            rows, cases = fixture()
            set_rss(rows, "native", current)
            set_rss(rows, "pure-go", 1000000)
            self.assertEqual(assess(rows, cases)["status"], expected)

    def test_growth_needs_both_relative_and_absolute_warning(self):
        for previous, current, expected in ((1000000, 1262144, "met"),
                                           (1000000, 1262145, "review-required"),
                                           (10000000, 11000000, "met"),
                                           (10000000, 11000001, "review-required")):
            rows, cases = fixture()
            for role in ("previous", "baseline"):
                set_rss(rows, role, previous)
            set_rss(rows, "native", current)
            for role in ("typerb-go", "pure-go"):
                set_rss(rows, role, current * 2)
            self.assertEqual(assess(rows, cases)["status"], expected)

    def test_cumulative_growth_cannot_be_reset_by_previous(self):
        rows, cases = fixture()
        set_rss(rows, "native", 1500000)
        set_rss(rows, "previous", 1490000)
        result = assess(rows, cases)
        self.assertEqual(result["status"], "review-required")
        self.assertTrue(any("baseline" in review for review in result["reviews"]))

    def test_absent_pure_go_is_an_explicit_gap(self):
        rows, cases = fixture(pure=False, baseline=False)
        result = assess(rows, cases)
        self.assertEqual(result["status"], "met")
        self.assertEqual(result["missingPureGo"], ["workload"])
        self.assertFalse(result["rows"][0]["cumulativeAvailable"])

    def test_invalid_or_missing_evidence_cannot_pass(self):
        rows, cases = fixture()
        for invalid in (rows[:-1], rows + [copy.deepcopy(rows[0])]):
            with self.assertRaises(ValueError):
                assess(invalid, cases)
        for value in (0, -1, True, 1.5, float("nan"), float("inf")):
            invalid = copy.deepcopy(rows)
            set_rss(invalid, "native", value)
            with self.assertRaises(ValueError):
                assess(invalid, cases)
        for field, value in (("status", "output-mismatch"), ("args", ["different"]),
                             ("expectedSha256", "b" * 64)):
            invalid = copy.deepcopy(rows)
            invalid[1][field] = value
            with self.assertRaises(ValueError):
                assess(invalid, cases)


if __name__ == "__main__":
    unittest.main()
