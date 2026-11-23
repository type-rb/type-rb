"""Check exported bytes and catalog counts against the recovery decoder bounds."""

from pathlib import Path
import json
import re
import sys


def decoder_limit(decoder: Path, name: str) -> int:
    matches = re.findall(
        rf"^{re.escape(name)} := ([0-9]+)$",
        decoder.read_text(encoding="utf-8"),
        re.MULTILINE,
    )
    if len(matches) != 1 or int(matches[0]) <= 0:
        raise ValueError(f"expected one positive decoder limit for {name}")
    return int(matches[0])


def compiler_input_limit(decoder: Path) -> int:
    return decoder_limit(decoder, "COMPILER_RECOVERY_MAX_INPUT_BYTES")


def check_size(snapshot: Path, decoder: Path) -> tuple[int, int]:
    limit = compiler_input_limit(decoder)
    size = snapshot.stat().st_size
    if size == 0 or size > limit:
        raise ValueError(f"compiler recovery snapshot has {size} bytes; expected 1..{limit}")
    return size, limit


def check_counts(snapshot: dict, decoder: Path) -> dict[str, tuple[int, int]]:
    if not isinstance(snapshot, dict) or snapshot.get("format") != "type-rb-bootstrap-snapshot" or snapshot.get("version") != 4:
        raise ValueError("expected a version 4 compiler recovery snapshot")
    result = {}
    for field, constant in (
        ("sources", "COMPILER_RECOVERY_MAX_SOURCES"),
        ("types", "MANAGED_RECOVERY_SNAPSHOT_MAX_TYPES"),
        ("functions", "COMPILER_RECOVERY_MAX_FUNCTIONS"),
    ):
        values = snapshot.get(field)
        if not isinstance(values, list):
            raise ValueError(f"compiler recovery snapshot {field} must be an array")
        limit = decoder_limit(decoder, constant)
        count = len(values)
        if count > limit:
            raise ValueError(f"compiler recovery snapshot has {count} {field}; limit is {limit}")
        result[field] = (count, limit)
    return result


def catalog_object(pairs):
    # Validate JSON while discarding each decoded member object. The preflight
    # needs catalog lengths, not a second retained compiler graph in Python.
    if any(key == "format" for key, _value in pairs):
        return dict(pairs)
    return None


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: bootstrap_snapshot_size.py SNAPSHOT", file=sys.stderr)
        return 2
    decoder = Path(__file__).resolve().parent.parent / "recovery/src/managed/snapshot.trb"
    try:
        size, limit = check_size(Path(sys.argv[1]), decoder)
        with Path(sys.argv[1]).open(encoding="utf-8") as source:
            counts = check_counts(json.load(source, object_pairs_hook=catalog_object), decoder)
    except (OSError, ValueError) as error:
        print(f"bootstrap snapshot: {error}", file=sys.stderr)
        return 1
    print(f"bootstrap snapshot: {size} bytes / {limit} byte input limit")
    print("bootstrap snapshot catalogs: " + ", ".join(
        f"{count}/{bound} {name}" for name, (count, bound) in counts.items()
    ))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
