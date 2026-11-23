"""Small-input cross-language observations, independent of daily publication."""
import argparse
import csv
import io
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "daily-performance"))
from measure import observe, correct, summarize, source_case, digest
from state import ROOT, read, write, git, now

UPSTREAM = "40296663ed350d5fe4a6ab5e367bab61cb77c219"
ARCHIVE_SHA = "aabcf6726cdc14f0f45b99e5daba48584f94bbb48883fd3711a1d040474d1cb4"
LANGUAGES = ["native", "pure-go", "c", "cpp", "rust", "java"]


def extract_sources(archive_path, destination):
    if digest(archive_path) != ARCHIVE_SHA:
        raise ValueError("Upstream archive checksum differs")
    archive = zipfile.ZipFile(archive_path)
    records = {}
    with open(ROOT / "benchmarks/benchmarksgame/context-sources.tsv") as stream:
        for row in csv.DictReader(stream, delimiter="\t"):
            role = "pure-go" if row["language"] == "go" else row["language"]
            data = archive.read(row["archive_path"])
            directory = destination / row["case"] / role
            directory.mkdir(parents=True)
            source = directory / (row["entry"] + ".java" if role == "java" else Path(row["archive_path"]).name)
            source.write_bytes(data)
            if digest(source) != row["source_sha256"]:
                raise ValueError("Upstream program checksum differs")
            records[row["case"], role] = {**row, "source": source}
    return records


def context_cases(suite, sources):
    registered = {case for case, _role in sources}
    known = {case["id"] for case in suite["cases"]}
    unknown = registered - known
    if unknown:
        raise ValueError("Unknown weekly context cases: " + ", ".join(sorted(unknown)))
    required = set(LANGUAGES) - {"native"}
    cases = []
    for case in suite["cases"]:
        if case["id"] not in registered:
            continue
        missing = required - {role for name, role in sources if name == case["id"]}
        if missing:
            raise ValueError("Missing weekly context roles for " + case["id"] + ": " + ", ".join(sorted(missing)))
        cases.append(case)
    if not cases:
        raise ValueError("Weekly context has no registered cases")
    return cases


def commands(role, source, program, classes, entry, compiler, qbe, tools, inputs):
    if role == "native":
        return [compiler, "build", source, "--output", program, "--qbe", qbe,
                "--cc", tools["c"], "--target", "linux-arm64-v0"], [program, *inputs]
    if role == "pure-go":
        return [tools[role], "build", "-trimpath", "-o", program, source], [program, *inputs]
    if role == "c":
        return [tools[role], "-O3", "-std=gnu11", "-x", "c", source, "-lm", "-o", program], [program, *inputs]
    if role == "cpp":
        return [tools[role], "-O3", "-std=gnu++17", "-include", "cstdlib", "-x", "c++", source, "-lm", "-o", program], [program, *inputs]
    if role == "rust":
        return [tools[role], "--crate-name", entry, "-C", "opt-level=3", source, "-o", program], [program, *inputs]
    return [tools[role], "-d", classes, source], [tools["jvm"], "-XX:ActiveProcessorCount=1", "-cp", classes, entry, *inputs]


def run(args):
    if platform.system() != "Linux" or platform.machine() not in ("aarch64", "arm64"):
        raise ValueError("Weekly measurement requires Linux arm64")
    evidence = Path(args.evidence).resolve(); evidence.mkdir(parents=True, exist_ok=False)
    sources = extract_sources(args.archive, evidence / "sources")
    suite = read(ROOT / "tools/daily-performance/suite.json")
    cases = context_cases(suite, sources)
    shutil.copytree(ROOT / "benchmarks/benchmarksgame/licenses", evidence / "licenses")
    tools = {role: shutil.which(name) for role, name in {"pure-go": "go", "c": "cc", "cpp": "c++", "rust": "rustc", "java": "javac", "jvm": "java"}.items()}
    if not all(tools.values()): raise ValueError("Missing registered language toolchain")
    native = read(args.compilers)["native"]
    roles = {"native": {"revision": native["revision"], "sha256": digest(native["path"])}}
    for role, path in tools.items():
        version = subprocess.check_output([path, "version" if role == "pure-go" else "--version"], stderr=subprocess.STDOUT).decode().strip()
        if role != "jvm": roles[role] = {"revision": UPSTREAM, "sha256": digest(path), "version": version}
    core = min(os.sched_getaffinity(0))
    env = {k: v for k, v in os.environ.items() if not k.startswith("TYPE_RB_NATIVE_RUNTIME_")}
    env.update(LC_ALL="C", GOMAXPROCS="1")
    rows, raw = [], []
    for case in cases:
        case_dir = evidence / case["id"]
        expected = source_case(case, case_dir)
        (case_dir / "expected").write_bytes(expected)
        observations, build_records, runtime_commands, case_rows = {}, {}, {}, {}
        for role in LANGUAGES:
            directory = case_dir / role; directory.mkdir()
            program, classes = directory / "program", directory / "classes"
            source = case_dir / "src/main.trb" if role == "native" else sources[case["id"], role]["source"]
            entry = "native" if role == "native" else sources[case["id"], role]["entry"]
            build, runtime = commands(role, source, program, classes, entry, native["path"], args.qbe, tools, case["args"])
            runtime_commands[role] = runtime
            observations[role], build_records[role] = [], []
            row = {"case": case["id"], "area": case["area"], "role": role, "status": "pass", "args": case["args"],
                   "sourceSha256": digest(source), "expectedSha256": digest(case_dir / "expected")}
            # Weekly build timings are retained as evidence; the page compares runtime/RSS only.
            for index in range(4):
                program.unlink(missing_ok=True); shutil.rmtree(classes, ignore_errors=True); classes.mkdir()
                print(f"Build {case['id']} {role} {index + 1}/4", flush=True)
                record = observe(build, directory / f"build-{index}", 90, cwd=case_dir, env=env, core=core)
                # Compiler warnings remain in evidence; a missing artifact cannot pass.
                if record["status"] == "pass" and not (any(classes.rglob("*.class")) if role == "java" else program.is_file()):
                    record["status"] = "build-failure"
                record.update(phase="warmup" if index == 0 else "retained", kind="build", case=case["id"], role=role)
                build_records[role].append(record); raw.append(record)
                if record["status"] != "pass": row["status"] = record["status"]; break
            if row["status"] == "pass":
                row["artifacts"] = [{"name": p.name, "bytes": p.stat().st_size, "sha256": digest(p)} for p in sorted(classes.rglob("*.class"))] if role == "java" else [{"name": "program", "bytes": program.stat().st_size, "sha256": digest(program)}]
            case_rows[role] = row
        for index in range(suite["warmups"] + suite["samples"]):
            names = LANGUAGES[index % len(LANGUAGES):] + LANGUAGES[:index % len(LANGUAGES)]
            for role in names:
                if case_rows[role]["status"] != "pass": continue
                directory = case_dir / role / f"runtime-{index}"
                print(f"Run {case['id']} {role} {index + 1}/6", flush=True)
                record = observe(runtime_commands[role], directory, suite["timeoutSeconds"], cwd=case_dir, env=env, core=core)
                correct(record, directory, expected)
                record.update(phase="warmup" if index < suite["warmups"] else "retained", kind="runtime", case=case["id"], role=role)
                observations[role].append(record); raw.append(record)
                if record["status"] != "pass": case_rows[role]["status"] = record["status"]
        for role, row in case_rows.items():
            row.update(runtime=summarize(observations[role], suite["samples"]), build=summarize(build_records[role], 3))
            rows.append(row)
        write(evidence / "raw.json", raw)
    write(evidence / "context.json", {"cpu": subprocess.check_output(["lscpu"]).decode(), "platform": platform.platform(), "core": core,
        "qbeSha256": digest(args.qbe), "tools": tools, "roles": roles, "archiveSha256": ARCHIVE_SHA,
        "javaRuntime": subprocess.check_output([tools["jvm"], "--version"]).decode(), "javaRuntimeSha256": digest(tools["jvm"]),
        "GOMAXPROCS": "1", "cachePolicy": "warm filesystem and build caches; fresh process for every runtime sample",
        "memoryMetric": "GNU time maximum RSS; not a process-tree sum", "javaPolicy": "JVM startup and JIT included in each fresh process"})
    write(args.output, {"profile": "weekly", "revision": git("rev-parse", "HEAD"), "at": now(), "rows": rows,
        "status": "measured" if all(r["status"] == "pass" for r in rows) else "measured-with-failures",
        "platform": "Linux arm64 / ubuntu-24.04-arm", "core": core, "roles": roles, "samples": suite["samples"], "buildSamples": 3})


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("compilers", "qbe", "archive", "evidence", "output"): parser.add_argument("--" + name, required=True)
    run(parser.parse_args())
