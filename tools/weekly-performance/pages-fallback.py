"""Preserve public weekly data when optional weekly artifact retrieval fails."""
from pathlib import Path
import json
import sys
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "daily-performance"))
from state import empty, validate, write

state = empty()
try:
    with urllib.request.urlopen("https://type-rb.github.io/type-rb-native/benchmarks/weekly-state.json", timeout=20) as response:
        data = response.read(8_000_001)
    if len(data) > 8_000_000:
        raise ValueError("Public weekly snapshot exceeds the limit")
    candidate = validate(json.loads(data))
    if candidate.get("latest") and candidate["latest"].get("profile") != "weekly":
        raise ValueError("Public snapshot is not weekly")
    state = candidate
except (OSError, ValueError, KeyError, TypeError):
    pass
state["publicationWarning"] = "Weekly artifact retrieval failed. Showing the last retrievable public snapshot, if available."
write(sys.argv[1], state)
