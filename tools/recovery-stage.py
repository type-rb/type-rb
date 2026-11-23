#!/usr/bin/env python3
"""Test-only monotonic stage receipts; never part of a measured compiler chain."""
import json
from pathlib import Path
import sys
import time

STAGES = (
    'source-preparation', 'matched-go', 'snapshot', 'recovery-b0',
    'recovery-generations', 'ordinary-fixed-point', 'generation-controls',
    'module-boundaries', 'file-cli', 'build-cli', 'project-controls',
    'normalization', 'conformance',
)


def read_events(path):
    events = [json.loads(line) for line in path.read_text().splitlines()]
    previous = -1
    for index, event in enumerate(events):
        if (index >= len(STAGES) * 2 or set(event) != {'stage', 'event', 'monotonicNs'}
                or event['stage'] != STAGES[index // 2]
                or event['event'] != ('start' if index % 2 == 0 else 'end')
                or type(event['monotonicNs']) is not int
                or event['monotonicNs'] < previous):
            raise ValueError('Invalid, reordered or non-monotonic recovery stage evidence')
        previous = event['monotonicNs']
    return events


def record(path, event, stage):
    events = read_events(path) if path.exists() else []
    index = len(events)
    if (index >= len(STAGES) * 2 or stage != STAGES[index // 2]
            or event != ('start' if index % 2 == 0 else 'end')):
        raise ValueError('Unexpected recovery stage transition')
    now = time.monotonic_ns()
    if events and now < events[-1]['monotonicNs']:
        raise ValueError('Monotonic clock moved backwards')
    # One root-suite writer; each invocation flushes and closes before returning.
    # Initial creation refuses stale evidence. No shared file across suites/runs.
    with path.open('a' if events else 'x') as stream:
        stream.write(json.dumps({'stage': stage, 'event': event, 'monotonicNs': now}) + '\n')


def finalize(path, outcome):
    summary = {'schemaVersion': 1, 'suiteOutcome': outcome, 'stages': [], 'complete': False}
    try:
        events = read_events(path)
        now = time.monotonic_ns()
        for index in range(0, len(events), 2):
            start = events[index]
            end = events[index + 1] if index + 1 < len(events) else None
            stopped = end['monotonicNs'] if end else now
            if stopped < start['monotonicNs']:
                raise ValueError('Monotonic clock moved backwards')
            summary['stages'].append({
                'name': start['stage'], 'state': 'completed' if end else outcome if outcome != 'success' else 'incomplete',
                'elapsedSeconds': (stopped - start['monotonicNs']) / 1e9,
            })
        summary['complete'] = outcome == 'success' and len(events) == len(STAGES) * 2
    except (OSError, ValueError, TypeError, KeyError) as error:
        summary['error'] = str(error)
    path.with_suffix('.summary.json').write_text(json.dumps(summary, indent=2) + '\n')
    return summary['complete']


def main():
    action, filename, value = sys.argv[1:]
    path = Path(filename)
    if action in ('start', 'end'):
        record(path, action, value)
        return 0
    if action == 'finalize' and value in ('success', 'failed', 'cancelled'):
        return 0 if finalize(path, value) else 1
    raise ValueError('Expected start/end stage or finalize suite outcome')


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError, TypeError, KeyError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
