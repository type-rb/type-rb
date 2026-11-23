"""Prospective runtime RSS trade-off policy; never rewrites historical results.

Call only after the owning cohort has validated source, tool, raw-observation,
output and median identities. This policy assesses RSS, not overall adoption.
"""
POLICY = "go-competitive-runtime-rss-v1"
ABSOLUTE_WARNING_BYTES = 256 * 1024


def rss(row):
    if row.get("status") != "pass":
        raise ValueError("RSS policy requires a passing case/role row")
    value = row["runtime"]["memoryBytes"]
    if type(value) is not int or value <= 0:
        raise ValueError("Runtime RSS must be positive integer bytes")
    return value


def increase(current, earlier):
    delta = current - earlier
    return {"baselineBytes": earlier, "deltaBytes": delta,
            "ratio": current / earlier,
            "reviewRequired": current * 10 > earlier * 11 and delta > ABSOLUTE_WARNING_BYTES}


def assess(rows, cases):
    """Return met, review-required or unmet for a complete registered RSS set.

    Missing declared Go counterparts, failed rows and duplicate/unexpected rows
    are invalid inputs. A case without a declared Pure Go counterpart records
    that gap; it does not receive an inferred Pure Go victory.
    """
    if not cases or len({case["id"] for case in cases}) != len(cases):
        raise ValueError("Expected unique nonempty registered cases")
    expected = set()
    for case in cases:
        roles = {"native", "previous", "typerb-go"}
        if case.get("frozenBaseline", True):
            roles.add("baseline")
        if "pureGoSource" in case:
            roles.add("pure-go")
        expected.update((case["id"], role) for role in roles)
    indexed = {(row["case"], row["role"]): row for row in rows}
    if len(indexed) != len(rows) or indexed.keys() != expected:
        raise ValueError("Missing, duplicated or unexpected RSS case/role row")
    output, failures, reviews, missing_pure = [], [], [], []
    for case in cases:
        name = case["id"]
        selected = {role: row for (case_name, role), row in indexed.items() if case_name == name}
        values = {role: rss(row) for role, row in selected.items()}
        # Different implementations may have different source hashes, but they
        # must agree on invocation arguments and the expected output identity.
        for row in selected.values():
            if (row["args"] != selected["native"]["args"] or
                    row["expectedSha256"] != selected["native"]["expectedSha256"]):
                raise ValueError("RSS comparisons use different inputs or outputs")
        current = values["native"]
        comparisons = {}
        for role in ("typerb-go", "pure-go"):
            if role not in values:
                missing_pure.append(name)
                continue
            other = values[role]
            win = current < other
            near = win and current * 10 > other * 9
            comparisons[role] = {"bytes": other, "ratio": current / other,
                                 "advantageBytes": other - current,
                                 "strictlyLower": win, "reviewRequired": near}
            if not win:
                failures.append(f"{name}: RSS does not beat {role}")
            elif near:
                reviews.append(f"{name}: less than 10% RSS advantage over {role}")
        changes = {role: increase(current, values[role])
                   for role in ("previous", "baseline") if role in values}
        for role, change in changes.items():
            if change["reviewRequired"]:
                reviews.append(f"{name}: RSS growth exceeds 10% and 256 KiB versus {role}")
        output.append({"case": name, "nativeBytes": current, "go": comparisons,
                       "nativeChanges": changes,
                       "cumulativeAvailable": "baseline" in values})
    return {"policy": POLICY,
            "status": "unmet" if failures else "review-required" if reviews else "met",
            "failures": failures, "reviews": reviews, "missingPureGo": missing_pure,
            "rows": output,
            "scope": "Runtime RSS only; correctness, runtime benefit and other acceptance authorities remain separate"}
