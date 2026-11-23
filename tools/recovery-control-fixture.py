#!/usr/bin/env python3
"""Synthetic subprocess ledger for the TypeRB recovery scheduler tests."""

import fcntl
import json
import os
from pathlib import Path
import sys
import time


def update(directory, change):
    descriptor = os.open(directory / "ledger.json", os.O_CREAT | os.O_RDWR, 0o600)
    with os.fdopen(descriptor, "r+", encoding="utf-8") as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        stream.seek(0)
        text = stream.read()
        ledger = json.loads(text) if text else {
            "active": [], "started": [], "finished": [], "maximum": 0,
        }
        change(ledger)
        stream.seek(0)
        json.dump(ledger, stream)
        stream.truncate()
        stream.flush()
        return ledger


def main():
    action, directory, *arguments = sys.argv[1:]
    directory = Path(directory)
    if action == "worker":
        label, outcome, peers = arguments

        def start(ledger):
            assert label not in ledger["started"], "duplicate admission"
            ledger["started"].append(label)
            ledger["active"].append(label)
            ledger["maximum"] = max(ledger["maximum"], len(ledger["active"]))

        update(directory, start)
        # A bounded handshake establishes overlap without timing a fast machine.
        deadline = time.monotonic() + 10
        while update(directory, lambda ledger: None)["maximum"] < int(peers):
            if time.monotonic() >= deadline:
                raise RuntimeError("workers did not overlap")
            time.sleep(0.01)
        time.sleep(0.02)

        def finish(ledger):
            ledger["active"].remove(label)
            ledger["finished"].append(label)

        update(directory, finish)
        if outcome == "stderr":
            print(label, file=sys.stderr)
        elif outcome == "exit":
            return 7
        print(label if outcome != "stdout" else "different")
    elif action == "verify":
        limit, *labels = arguments
        ledger = update(directory, lambda ledger: None)
        assert not ledger["active"], "uncollected workers"
        assert sorted(ledger["started"]) == sorted(labels), "missing/duplicate admission"
        assert sorted(ledger["finished"]) == sorted(labels), "missing/duplicate completion"
        assert ledger["maximum"] == int(limit), "concurrency bound or overlap differs"
        print("ok")
    else:
        raise ValueError("unknown fixture command")
    return 0


if __name__ == "__main__":
    sys.exit(main())
