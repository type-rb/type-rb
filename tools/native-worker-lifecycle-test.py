#!/usr/bin/env python3
"""Exercise SQLite Jobs through the public Native CLI and real child processes."""

import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import signal
import sqlite3
import subprocess
import tempfile
import time


MAIN = '''import { SampleJob, OtherJob } from jobs/sample
import trb/std/process

def main()
    release := Process.environment("JOBS_PROBE_RELEASE")
    if release == nil
        return
    end
    receipt := SampleJob.perform_later(release, 42, 2.75, true) catch |error|
        puts(error.message)
        return
    end
    puts(receipt.id)
    if Process.environment("JOBS_PROBE_OTHER") != nil
        other := OtherJob.perform_later() catch |error|
            puts(error.message)
            return
        end
        puts(other.id)
    end
end
'''
JOBS = '''import { Job, queue, priority } from trb/jobs
import { File, FileMode } from trb/std/file
import trb/std/path
import trb/std/result

def release_exists(path: String): Boolean
    value := File.open(Path.new(path)) do |file|
        try file.read_text(max_bytes: 16)
    end
    case value
    when Result::Ok(text)
        return text == "release"
    when Result::Err(_error)
        return false
    end
end

class SampleJob < Job
    queue("mail")
    priority(10)

    def perform(path: String, order: Integer, cost: Float, enabled: Boolean)
        marked := File.open(Path.new(path + ".started"), mode: FileMode::Write) do |file|
            try file.write_text(order.to_s() + ":" + cost.to_s() + ":" + enabled.to_s())
        end
        case marked
        when Result::Ok(_unit)
        when Result::Err(error)
            puts(error.message)
            return
        end
        while !release_exists(path)
        end
    end
end

class OtherJob < Job
    def perform()
        puts("other")
    end
end
'''
CONFIG = '''import { JobAdapter } from trb/jobs
import { SQLAdapter } from trb/jobs/sql
import { Duration } from trb/std/time

JOBS_ADAPTER: JobAdapter := SQLAdapter.new(
    source_environment: "JOBS_PROBE_DB",
    poll_interval: Duration.milliseconds(80),
    lease_timeout: Duration.seconds(6),
)
'''
JOBS_WITH_RESULTS = '''
import { Job, JobError, JobResult } from trb/jobs
import trb/std/unit

class SampleJob < Job
    def perform(path: String, order: Integer, cost: Float, enabled: Boolean): JobResult
        if enabled && path != ""
            return JobResult::Err(JobError.new(message: "fixture rejected " + order.to_s() + ":" + cost.to_s()))
        end
        return JobResult::Ok(Unit.new())
    end
end

class OtherJob < Job
    def perform(): JobResult
        return JobResult::Ok(Unit.new())
    end
end
'''


def checked(command, root, env, timeout=60):
    result = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True, timeout=timeout)
    assert result.returncode == 0 and result.stderr == "", (command, result)
    return result.stdout


def poll(observe, seconds, description):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = observe()
        if value:
            return value
        time.sleep(0.04)
    raise AssertionError(description)


def process_table():
    result = subprocess.check_output(["ps", "-ax", "-o", "pid=,ppid="], text=True)
    return {int(pid): int(parent) for pid, parent in map(str.split, result.splitlines())}


def ensure_reaped(*pids):
    for pid in pids:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            continue
        raise AssertionError(("owned process remains", pid))


class Fixture:
    def __init__(self, root, binary):
        self.root = root
        self.binary = binary
        (root / "src/config").mkdir(parents=True)
        (root / "src/jobs").mkdir()
        (root / "src/main.trb").write_text(MAIN)
        (root / "src/jobs/sample.trb").write_text(JOBS)
        (root / "src/config/jobs.trb").write_text(CONFIG)
        (root / "trbconfig.jsonc").write_text(json.dumps({
            "name": "jobs-lifecycle", "mode": "trb", "sourceDir": "src",
            "jobs": {"configuration": "config/jobs"},
        }))
        self.program = root / "enqueue"
        env = dict(os.environ, TERM="dumb", NO_COLOR="1")
        env.pop("JOBS_PROBE_RELEASE", None)
        checked([binary, "check"], root, env)
        built_at = time.monotonic()
        checked([binary, "build", "--compile", "--outfile", self.program], root, env)
        build_seconds = time.monotonic() - built_at
        started_at = time.monotonic()
        checked([self.program], root, env)
        startup_seconds = time.monotonic() - started_at
        self.observations = {
            "enqueue": {"bytes": self.program.stat().st_size, "buildSeconds": build_seconds,
                        "startupWithoutEnqueueSeconds": startup_seconds},
            "workers": [],
            "cases": [],
        }
        assert b"TRBN-SQLITE-V1\0" in self.program.read_bytes()
        standalone_root = root / "control"
        (standalone_root / "src").mkdir(parents=True)
        (standalone_root / "trbconfig.jsonc").write_text(
            '{"name":"control","mode":"trb","sourceDir":"src"}'
        )
        source = standalone_root / "src/main.trb"
        source.write_text('import trb/std/process\ndef main()\nvalue := Process.environment("JOBS_PROBE_RELEASE")\nif value != nil\nputs(value)\nend\nend\n')
        plain = standalone_root / "plain"
        built_at = time.monotonic()
        checked([binary, "build", "--compile", "--outfile", plain], standalone_root, env)
        build_seconds = time.monotonic() - built_at
        started_at = time.monotonic()
        checked([plain], standalone_root, env)
        self.observations["processControl"] = {
            "bytes": plain.stat().st_size, "buildSeconds": build_seconds,
            "startupSeconds": time.monotonic() - started_at,
        }
        assert b"TRBN-SQLITE-V1\0" not in plain.read_bytes()

    def case(self, name, other=False):
        directory = self.root / name
        directory.mkdir()
        self.database = directory / "queue.sqlite"
        self.release = directory / "release"
        self.marker = directory / "release.started"
        self.scratch = directory / "temp"
        self.scratch.mkdir()
        self.env = dict(os.environ, TMPDIR=str(self.scratch), NO_COLOR="1",
                        JOBS_PROBE_DB=str(self.database), JOBS_PROBE_RELEASE=str(self.release))
        self.env.pop("JOBS_PROBE_OTHER", None)
        if other:
            self.env["JOBS_PROBE_OTHER"] = "1"
        identifiers = checked([self.program], self.root, self.env).splitlines()
        assert len(identifiers) == (2 if other else 1) and all(re.fullmatch("[a-f0-9]{32}", i) for i in identifiers), identifiers
        self.job = identifiers[0]
        with sqlite3.connect(self.database) as connection:
            assert connection.execute("PRAGMA journal_mode=WAL").fetchone() == ("wal",)
            row = connection.execute("SELECT payload, queue_name, priority, payload_version FROM trb_jobs WHERE id=?", (self.job,)).fetchone()
            assert (json.loads(row[0]), *row[1:]) == ([str(self.release), 42, 2.75, True], "mail", 10, 1), row

    def row(self):
        with sqlite3.connect(self.database) as connection:
            return connection.execute("SELECT state, claimed_by, claimed_at, attempts FROM trb_jobs WHERE id=?", (self.job,)).fetchone()

    def release_handler(self):
        self.release.write_text("release")

    def once(self):
        checked([self.binary, "jobs", "start", "--once", "--queue", "mail"], self.root, self.env, timeout=45)
        assert not list(self.scratch.iterdir()), list(self.scratch.iterdir())

    @contextmanager
    def start(self):
        with tempfile.TemporaryFile() as log:
            started_at = time.monotonic()
            cli = subprocess.Popen([self.binary, "jobs", "start", "--queue=mail"], cwd=self.root,
                                   stdin=subprocess.DEVNULL, stdout=log, stderr=log, env=self.env, start_new_session=True)
            try:
                def ready():
                    if cli.poll() is not None:
                        log.seek(0)
                        raise AssertionError(("worker exited before handler start", cli.returncode, log.read().decode()))
                    row = self.row()
                    if not row or row[0] != "running" or not self.marker.exists():
                        return None
                    worker = int(row[1].split("-", 1)[1])
                    tree = process_table()
                    handler = next((pid for pid, parent in tree.items() if parent == worker), None)
                    if tree.get(worker) == cli.pid and handler is not None and self.marker.read_text() == "42:2.75:true":
                        return worker, handler, row
                    return None

                worker, handler, row = poll(ready, 35, "handler never entered the held state")
                program, = self.scratch.glob("trbn-jobs-*/worker")
                self.observations["workers"].append({
                    "case": self.scratch.parent.name, "bytes": program.stat().st_size,
                    "buildAndStartSeconds": time.monotonic() - started_at,
                    "cliPid": cli.pid, "workerPid": worker, "handlerPid": handler,
                })
                yield cli, worker, handler, row, log
            finally:
                if cli.poll() is None:
                    self.release_handler()
                    cli.send_signal(signal.SIGTERM)
                    try:
                        cli.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        # Kill only still-linked descendants of this fixture's CLI.
                        tree = process_table()
                        for child, parent in tree.items():
                            if tree.get(parent) == cli.pid:
                                os.kill(child, signal.SIGKILL)
                        for child, parent in tree.items():
                            if parent == cli.pid:
                                os.kill(child, signal.SIGKILL)
                        cli.kill()
                        cli.wait(timeout=5)

    def finish(self, cli, worker, handler, log, diagnostic="", timeout=8):
        status = cli.wait(timeout=timeout)
        log.seek(0)
        output = log.read().decode()
        assert (status != 0) == bool(diagnostic) and diagnostic in output, (status, output, diagnostic)
        ensure_reaped(worker, handler)
        self.observations["workers"][-1]["exitStatus"] = status
        self.observations["workers"][-1]["childrenReaped"] = True
        assert not list(self.scratch.iterdir()), list(self.scratch.iterdir())

    def passed(self, observation):
        self.observations["cases"].append(self.scratch.parent.name)
        print(observation, flush=True)


def verify(fixture, quick):
    fixture.case("filtered-once", other=True)
    fixture.release_handler()
    fixture.once()
    assert fixture.row() is None
    with sqlite3.connect(fixture.database) as connection:
        assert connection.execute("SELECT queue_name, state, attempts FROM trb_jobs").fetchall() == [("default", "ready", 0)]
    fixture.passed("queue-filtered one-shot executed and acknowledged")
    if quick:
        return

    for group in (False, True):
        fixture.case("foreground-stop" if group else "targeted-stop")
        with fixture.start() as (cli, worker, handler, row, log):
            if group:
                os.killpg(cli.pid, signal.SIGINT)
            else:
                cli.send_signal(signal.SIGTERM)
            time.sleep(0.25)
            assert cli.poll() is None and process_table().get(handler) == worker
            fixture.release_handler()
            fixture.finish(cli, worker, handler, log)
            assert fixture.row() is None
        fixture.passed(f"graceful {'foreground group' if group else 'targeted'} stop joined its worker and handler")

    fixture.case("repeated-stop")
    with fixture.start() as (cli, worker, handler, row, log):
        cli.send_signal(signal.SIGTERM)
        time.sleep(0.2)
        cli.send_signal(signal.SIGTERM)
        fixture.finish(cli, worker, handler, log, "forced to stop (repeated stop request)")
        assert fixture.row()[:2] == row[:2]
    fixture.passed("repeated stop reaped the active handler and retained the lease")

    fixture.case("idle-snapshot")
    with sqlite3.connect(fixture.database) as observer:
        observer.execute("BEGIN")
        assert observer.execute("SELECT state FROM trb_jobs").fetchone() == ("ready",)
        with fixture.start() as (cli, worker, handler, row, log):
            fixture.release_handler()
            poll(lambda: fixture.row() is None, 6, "worker did not acknowledge")
            cli.send_signal(signal.SIGINT)
            fixture.finish(cli, worker, handler, log)
        assert observer.execute("SELECT state FROM trb_jobs").fetchone() == ("ready",)
    fixture.passed("idle worker joined with another connection retaining the original snapshot")

    fixture.case("heartbeat")
    with fixture.start() as (cli, worker, handler, row, log):
        def heartbeat():
            current = fixture.row()
            return current and current[:2] == row[:2] and current[2] > row[2]
        poll(heartbeat, 5, "lease did not advance during the held handler")
        assert process_table().get(handler) == worker and not fixture.release.exists()
        fixture.release_handler()
        cli.send_signal(signal.SIGTERM)
        fixture.finish(cli, worker, handler, log)
    fixture.passed("lease advanced while the independently observed handler stayed held")

    fixture.case("grace-expiry")
    with fixture.start() as (cli, worker, handler, row, log):
        started = time.monotonic()
        cli.send_signal(signal.SIGTERM)
        fixture.finish(cli, worker, handler, log, "grace period expired; active claim retained", timeout=38)
        assert time.monotonic() - started >= 29.5 and fixture.row()[:2] == row[:2]
    with sqlite3.connect(fixture.database) as connection:
        connection.execute("UPDATE trb_jobs SET claimed_at='2000-01-01 00:00:00' WHERE id=?", (fixture.job,))
    fixture.release_handler()
    fixture.once()
    assert fixture.row() is None
    fixture.passed("grace expiry joined the child and a later worker recovered its lease")

    fixture.case("typed-failure")
    (fixture.root / "src/jobs/sample.trb").write_text(JOBS_WITH_RESULTS)
    with sqlite3.connect(fixture.database) as connection:
        connection.execute("UPDATE trb_jobs SET maximum_attempts=1 WHERE id=?", (fixture.job,))
    fixture.once()
    with sqlite3.connect(fixture.database) as connection:
        failed = connection.execute(
            "SELECT state, attempts, claimed_by, last_error FROM trb_jobs WHERE id=?", (fixture.job,)
        ).fetchone()
    assert failed == ("failed", 1, None, "fixture rejected 42:2.75"), failed
    fixture.passed("typed JobResult failure was recorded after handler exit")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", type=Path, required=True)
    parser.add_argument("--quick", action="store_true")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="native-jobs-lifecycle-") as directory:
        fixture = Fixture(Path(directory), args.native.resolve())
        try:
            verify(fixture, args.quick)
        finally:
            if args.report:
                args.report.write_text(json.dumps(fixture.observations, indent=2) + "\n")


if __name__ == "__main__":
    main()
