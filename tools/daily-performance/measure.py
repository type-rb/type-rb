"""Daily diagnostic measurements; independent of formal merge authorities."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import statistics
import subprocess
import threading
import time

from state import ROOT, git, now, read, write, SHA


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def retain_artifact(program, evidence):
    """Preserve complete build output after timing, before the next overwrite."""
    data = program.read_bytes()
    sha256 = hashlib.sha256(data).hexdigest()
    relative = Path("artifacts") / "sha256" / sha256
    saved = evidence / relative
    saved.parent.mkdir(parents=True, exist_ok=True)
    if saved.exists():
        if saved.read_bytes() != data:
            raise ValueError(f"Retained artifact differs from {sha256}")
    else:
        saved.write_bytes(data)
    return {"sha256": sha256, "bytes": len(data), "path": relative.as_posix()}


def observe(command, directory, timeout, cwd=None, env=None, core=None):
    directory.mkdir(parents=True, exist_ok=False)
    metrics = directory / "time.txt"
    invocation = ["/usr/bin/time", "-f", "%U %S %M", "-o", str(metrics), *map(str, command)]
    if core is not None:
        invocation = ["taskset", "-c", str(core), *invocation]
    record = {"command": list(map(str, command)), "timeoutSeconds": timeout}
    with (directory / "stdout").open("wb") as out, (directory / "stderr").open("wb") as err:
        start = time.perf_counter()
        process = subprocess.Popen(invocation, cwd=cwd, env=env, stdout=out, stderr=err, start_new_session=True)
        expired = threading.Event()
        def stop():
            expired.set()
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        timer = threading.Timer(timeout, stop)
        timer.start()
        try:
            process.wait()
            record["status"] = "timeout" if expired.is_set() else "pass" if process.returncode == 0 else "nonzero-exit"
        finally:
            timer.cancel()
            timer.join()
            # Kill the owned group even if a wrapper exited before a descendant.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
        record["wallSeconds"] = time.perf_counter() - start
        record["exitCode"] = process.returncode
    if record["status"] == "pass":
        user, system, rss = map(float, metrics.read_text().strip().split())
        record.update(cpuSeconds=user + system, memoryBytes=int(rss * 1024))
    write(directory / "observation.json", record)
    return record


def correct(record, directory, expected):
    if record["status"] == "pass":
        if (directory / "stdout").read_bytes() != expected:
            record["status"] = "output-mismatch"
        elif (directory / "stderr").stat().st_size:
            record["status"] = "unexpected-stderr"
    write(directory / "observation.json", record)
    return record


def summarize(records, count):
    retained = [record for record in records if record["phase"] == "retained"]
    if len(retained) != count or any(record["status"] != "pass" for record in records):
        return None
    result = {key: statistics.median(record[key] for record in retained)
              for key in ("wallSeconds", "cpuSeconds", "memoryBytes")}
    result["wallMin"] = min(record["wallSeconds"] for record in retained)
    result["wallMax"] = max(record["wallSeconds"] for record in retained)
    return result


def source_case(case, directory):
    source = (ROOT / case["source"]).read_text()
    if "replace" in case:
        before, after = case["replace"]
        if source.count(before) != 1:
            raise ValueError(f"{case['id']}: workload invocation changed")
        source = source.replace(before, after)
    (directory / "src").mkdir(parents=True)
    (directory / "src/main.trb").write_text(source)
    write(directory / "trbconfig.jsonc", {
        "name": "daily-" + case["id"], "version": "0.1.0", "mode": "go",
        "sourceDir": "src", "outDir": "build", "copyFiles": False, "packageManagement": "managed",
        "go": {"module": "example.org/daily/" + case["id"], "version": "1.27"}})
    return (ROOT / case["expectedFile"]).read_bytes() if "expectedFile" in case else case["expected"].encode()


def measure_self_compilation(args, roles, evidence, environment, core):
    """Measure the current compiler building its exact-revision source."""
    source = Path(args.compilers).parent / roles["native"]["revision"] / "source/compiler/src/compiler.trb"
    if not source.is_file():
        raise ValueError("Current compiler source is absent from the prepared worktree")
    directory = evidence / "compiler-self"
    directory.mkdir()
    result = {"status": "pass", "sourceSha256": digest(source), "ir": None,
              "build": None, "binaryBytes": None, "binarySha256": None}
    records = []
    ir_dir = directory / "emit-qbe"
    ir_record = observe([roles["native"]["path"], "emit-qbe", source], ir_dir, 90,
                        env=environment, core=core)
    ir_record.update(kind="compiler-ir", case="compiler-self", role="native", phase="retained")
    ir_output = ir_dir / "stdout"
    if ir_record["status"] == "pass":
        if not ir_output.stat().st_size or (ir_dir / "stderr").stat().st_size:
            ir_record["status"] = "unexpected-stderr" if (ir_dir / "stderr").stat().st_size else "output-mismatch"
        else:
            result["ir"] = {"bytes": ir_output.stat().st_size, "sha256": digest(ir_output),
                            "wallSeconds": ir_record["wallSeconds"],
                            "cpuSeconds": ir_record["cpuSeconds"],
                            "memoryBytes": ir_record["memoryBytes"]}
    write(ir_dir / "observation.json", ir_record)
    # Keep the digest and size, not a 20+ MB IR blob in every daily artifact.
    ir_output.unlink(missing_ok=True)
    records.append(ir_record)
    if ir_record["status"] != "pass":
        result["status"] = ir_record["status"]
        return result, records

    builds = []
    for round_index in range(3):
        output = directory / f"compiler-{round_index}"
        build_dir = directory / f"build-{round_index}"
        record = observe([roles["native"]["path"], "build", source, "--output", output,
                          "--qbe", args.qbe, "--cc", "/usr/bin/cc", "--target", "linux-arm64-v0"],
                         build_dir, 120, env=environment, core=core)
        record.update(kind="compiler-self", case="compiler-self", role="native",
                      phase="warmup" if round_index == 0 else "retained")
        if record["status"] == "pass":
            if not output.is_file() or (build_dir / "stdout").stat().st_size or (build_dir / "stderr").stat().st_size:
                record["status"] = "build-failure"
            else:
                size, sha = output.stat().st_size, digest(output)
                if result["binarySha256"] is not None and (size, sha) != (result["binaryBytes"], result["binarySha256"]):
                    record["status"] = "output-mismatch"
                else:
                    result["binaryBytes"], result["binarySha256"] = size, sha
        output.unlink(missing_ok=True)
        write(build_dir / "observation.json", record)
        records.append(record)
        builds.append(record)
        if record["status"] != "pass":
            result["status"] = record["status"]
            break
    result["build"] = summarize(builds, 2)
    return result, records


def run(args):
    if platform.system() != "Linux" or platform.machine() not in ("aarch64", "arm64"):
        raise ValueError("Daily measurement requires Linux arm64")
    suite = read(ROOT / "tools/daily-performance/suite.json")
    core = min(os.sched_getaffinity(0))
    roles = read(args.compilers)
    if set(roles) != {"native", "previous", "baseline", "typerb-go"}:
        raise ValueError("Expected current, previous, frozen Native and TypeRB Go")
    go_path = shutil.which("go")
    roles["pure-go"] = {"revision": suite["pureGoRevision"], "path": go_path,
                        "version": subprocess.check_output([go_path, "version"]).decode().strip()}
    for role in roles.values():
        if not SHA.fullmatch(role["revision"]):
            raise ValueError("Compiler revision is not exact")
        role["sha256"] = digest(role["path"])
        role["bytes"] = Path(role["path"]).stat().st_size
    evidence = Path(args.evidence).resolve()
    evidence.mkdir(parents=True, exist_ok=False)
    shutil.copytree(ROOT / "benchmarks/benchmarksgame/licenses", evidence / "licenses")
    rows = []
    raw = []
    # Unset instrumentation: allocation accounting gets its own untimed probe.
    environment = {key: value for key, value in os.environ.items()
                   if not key.startswith("TYPE_RB_NATIVE_RUNTIME_")}
    environment["LC_ALL"] = "C"
    environment["GOMAXPROCS"] = "1"
    for case in suite["cases"]:
        case_dir = evidence / case["id"]
        expected = source_case(case, case_dir)
        (case_dir / "expected").write_bytes(expected)
        # Language-area kernels can use features newer than the frozen baseline.
        case_roles = {key: role for key, role in roles.items()
                      if (key != "pure-go" or "pureGoSource" in case) and
                      (key != "baseline" or case.get("frozenBaseline", True))}
        if "pureGoSource" in case:
            shutil.copyfile(ROOT / case["pureGoSource"], case_dir / "pure-go.go")
        row_roles = {}
        # Build each role alternately to avoid favoring one compiler systematically.
        for role in case_roles:
            (case_dir / role).mkdir()
            row_roles[role] = {"case": case["id"], "area": case["area"], "role": role,
                               "sourceSha256": digest(case_dir / ("pure-go.go" if role == "pure-go" else "src/main.trb")),
                               "expectedSha256": hashlib.sha256(expected).hexdigest(),
                               "args": case["args"], "status": "pass", "coverage": None}
            if "family" in case:
                row_roles[role]["family"] = case["family"]
        build_records = {role: [] for role in case_roles}
        runtime_records = {role: [] for role in case_roles}
        for round_index in range(4):
            names = list(case_roles)
            names = names[round_index % len(names):] + names[:round_index % len(names)]
            for role in names:
                row = row_roles[role]
                if row["status"] != "pass":
                    continue
                program = case_dir / role / "program"
                program.unlink(missing_ok=True)
                shutil.rmtree(case_dir / "build", ignore_errors=True)
                if role == "pure-go":
                    command = [go_path, "build", "-trimpath", "-o", program, case_dir / "pure-go.go"]
                elif role == "typerb-go":
                    command = [roles[role]["path"], "build", "--compile", "--config", "trbconfig.jsonc",
                               "--outfile", program]
                else:
                    command = [roles[role]["path"], "build", case_dir / "src/main.trb", "--output", program,
                               "--qbe", args.qbe, "--cc", "/usr/bin/cc", "--target", "linux-arm64-v0"]
                directory = case_dir / role / f"build-{round_index}"
                print(f"Build {case['id']} {role} {round_index + 1}/4", flush=True)
                record = observe(command, directory, 90, cwd=case_dir, env=environment, core=core)
                record.update(phase="warmup" if round_index == 0 else "retained", kind="build", case=case["id"], role=role)
                if record["status"] == "pass" and (not program.exists() or (directory / "stderr").stat().st_size):
                    record["status"] = "build-failure"
                # Persist metadata even if evidence storage fails. The exception
                # aborts the run as an infrastructure failure, without a snapshot.
                write(directory / "observation.json", record)
                if record["status"] == "pass":
                    record["artifact"] = retain_artifact(program, evidence)
                    write(directory / "observation.json", record)
                build_records[role].append(record)
                raw.append(record)
                if record["status"] != "pass":
                    row["status"] = record["status"]
        for role, row in row_roles.items():
            if row["status"] != "pass":
                continue
            program = case_dir / role / "program"
            artifact = build_records[role][-1]["artifact"]
            row.update(artifactBytes=artifact["bytes"], artifactSha256=artifact["sha256"],
                       runtimeArtifact=artifact)
            stripped = case_dir / role / "program.stripped"
            shutil.copyfile(program, stripped)
            subprocess.run(["strip", "--strip-all", stripped], check=True, timeout=30)
            row["strippedBytes"] = stripped.stat().st_size
            if case.get("gcProbe") and role in ("native", "previous", "baseline"):
                directory = case_dir / role / "gc-probe"
                record = observe([program, *case["args"]], directory, suite["timeoutSeconds"],
                                 env={**environment, "TYPE_RB_NATIVE_RUNTIME_STATS": "1"}, core=core)
                record["artifact"] = artifact
                write(directory / "observation.json", record)
                lines = (directory / "stderr").read_text().splitlines()
                prefix = "type-rb-native-gc-stat-v1,automatic-collections,"
                collections = [int(line.removeprefix(prefix)) for line in lines if line.startswith(prefix)]
                row["coverage"] = "GC observed" if record["status"] == "pass" and len(collections) == 1 and collections[0] > 0 else "GC coverage unconfirmed"
        for round_index in range(suite["warmups"] + suite["samples"]):
            names = list(case_roles)
            names = names[round_index % len(names):] + names[:round_index % len(names)]
            for role in names:
                row = row_roles[role]
                if row["status"] != "pass":
                    continue
                directory = case_dir / role / f"runtime-{round_index}"
                print(f"Run {case['id']} {role} {round_index + 1}/6", flush=True)
                record = observe([case_dir / role / "program", *case["args"]], directory,
                                 suite["timeoutSeconds"], env=environment, core=core)
                correct(record, directory, expected)
                record.update(phase="warmup" if round_index < suite["warmups"] else "retained",
                              kind="runtime", case=case["id"], role=role, artifact=row["runtimeArtifact"])
                write(directory / "observation.json", record)
                runtime_records[role].append(record)
                raw.append(record)
                if record["status"] != "pass":
                    row["status"] = record["status"]
        for role, row in row_roles.items():
            row["runtime"] = summarize(runtime_records[role], suite["samples"])
            row["build"] = summarize(build_records[role], 3)
            rows.append(row)
        write(evidence / "raw.json", raw)
    compiler_self, self_records = measure_self_compilation(args, roles, evidence, environment, core)
    raw.extend(self_records)
    write(evidence / "raw.json", raw)
    # Preserve exact commands and host context separately from the compact page data.
    context = {"cpu": subprocess.check_output(["lscpu"]).decode(), "platform": platform.platform(),
               "core": core, "qbeSha256": digest(args.qbe), "roles": roles,
               "go": subprocess.check_output(["go", "version"]).decode().strip(),
               "cc": subprocess.check_output(["/usr/bin/cc", "--version"]).decode(),
               "GOMAXPROCS": "1",
               "cachePolicy": "warm filesystem and Go build cache; no cache drop",
               "memoryMetric": "GNU time maximum resident set size; not a sampled process-tree sum"}
    write(evidence / "context.json", context)
    write(args.output, {"revision": git("rev-parse", "HEAD"), "at": now(), "rows": rows,
                        "compilerSelf": compiler_self,
                        "status": "measured" if all(row["status"] == "pass" for row in rows) and compiler_self["status"] == "pass" else "measured-with-failures",
                        "platform": "Linux arm64 / ubuntu-24.04-arm", "core": core,
                        "roles": {key: {field: value[field] for field in ("revision", "sha256", "bytes", "version") if field in value} for key, value in roles.items()},
                        "pureGoCases": [case["id"] for case in suite["cases"] if "pureGoSource" in case],
                        "baselineCases": [case["id"] for case in suite["cases"] if case.get("frozenBaseline", True)],
                        "samples": suite["samples"], "buildSamples": 3})


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--compilers", required=True)
    parser.add_argument("--qbe", required=True)
    parser.add_argument("--evidence", required=True)
    parser.add_argument("--output", required=True)
    run(parser.parse_args())
