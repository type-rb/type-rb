"""One registered hardware diagnostic, separate from acceptance and publication."""
import argparse
import json
import os
from pathlib import Path
import platform
import re
import shutil
import statistics
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "daily-performance"))
from measure import correct, digest, observe, source_case
from state import ROOT, git, read, write
sys.path.insert(0, str(Path(__file__).resolve().parent))
from reuse import load_reused, validate_reuse, verified_compiler, verified_program
from float_controls import prepare_controls


def validate_registration(plan):
    if plan["cohortBudget"] != 1 or plan["firstSamples"] != 1 or plan["steadySamples"] != 5:
        raise ValueError("Expected one first execution and five steady executions")
    variants = plan["variants"]
    if not 2 <= len(variants) <= 4 or list(variants)[0] != "accepted":
        raise ValueError("Expected accepted source followed by bounded candidates")
    if plan["suiteSHA256"] != digest(ROOT / "tools/daily-performance/suite.json"):
        raise ValueError("Registered workload suite changed")
    if str(os.environ.get("GITHUB_RUN_ATTEMPT", "1")) != "1":
        raise ValueError("Do not retry a completed diagnostic cohort")
    suite = read(ROOT / "tools/daily-performance/suite.json")
    if "reuse" in plan:
        validate_reuse(plan["reuse"])
    cases = {case["id"] for case in suite["cases"]}
    selected = plan.get("cases", sorted(cases))
    if not selected or len(set(selected)) != len(selected) or set(selected) - cases:
        raise ValueError("Diagnostic cases must be a nonempty unique subset of the suite")
    if "floatControls" in plan:
        control = plan["floatControls"]
        case = next(case for case in suite["cases"] if case["id"] == "n-body")
        if ("n-body" not in selected or set(control) != {"sourceSHA256", "goVersion"}
                or control["sourceSHA256"] != digest(ROOT / case["pureGoSource"])
                or not re.fullmatch(r"go version go[0-9]+\.[0-9]+\.[0-9]+ linux/arm64", control["goVersion"])):
            raise ValueError("Float attribution controls require exact N-body source and Go identities")
    for role, variant in variants.items():
        if not re.fullmatch(r"[a-z][a-z0-9-]*", role):
            raise ValueError("Invalid role name")
        if "reuse" in plan:
            keys = {"reusedRole", "parentCompilerRole"} & set(variant)
            if len(keys) != 1 or any(not re.fullmatch(r"[a-z][a-z0-9-]*", variant[key]) for key in keys):
                raise ValueError("Each reused-cohort variant needs one exact preparation mode")
        revision = variant["revision"]
        if not re.fullmatch(r"[0-9a-f]{40}", revision):
            raise ValueError("Each source revision must be exact")
        git("merge-base", "--is-ancestor", revision, "HEAD")
        if set(variant["appSHA256"]) != cases or any(
                not re.fullmatch(r"[0-9a-f]{64}", value)
                for value in variant["appSHA256"].values()):
            raise ValueError("Every Native application must have a fixed identity")
        # Changes to workload or reference inputs cannot masquerade as a
        # compiler comparison. Verify all public inputs before preparation.
        files = {"TYPE_RB_REVISION", "tools/daily-performance/suite.json"}
        for case in suite["cases"]:
            files.add(case["source"])
            files.update(case[key] for key in ("expectedFile", "pureGoSource") if key in case)
        for name in files:
            saved = subprocess.check_output(["git", "show", f"{revision}:{name}"], cwd=ROOT)
            if saved != (ROOT / name).read_bytes():
                raise ValueError(f"{role}: workload or reference differs: {name}")
    return suite


def summary(records):
    if len(records) != 6 or any(record["status"] != "pass" for record in records):
        return None
    steady = [record for record in records if record["phase"] == "steady"]
    if len(steady) != 5 or len([r for r in records if r["phase"] == "first"]) != 1:
        raise ValueError("Incomplete first/steady observation set")
    result = {key: statistics.median(row[key] for row in steady)
              for key in ("wallSeconds", "cpuSeconds", "memoryBytes")}
    result["steadyWallSpread"] = max(r["wallSeconds"] for r in steady) / min(r["wallSeconds"] for r in steady)
    result["allWallSpread"] = max(r["wallSeconds"] for r in records) / min(r["wallSeconds"] for r in records)
    result["cpuResolved"] = all(row["cpuSeconds"] > 0 for row in steady)
    return result


def runtime_schedule(cases, iteration):
    # Keep the original case index when rotating cases. Its position in the
    # rotated list would cancel the independent rotation of implementations.
    indexed = list(enumerate(cases))
    offset = iteration % len(indexed)
    for index, entry in indexed[offset:] + indexed[:offset]:
        roles = entry[3]
        start = (iteration + index) % len(roles)
        for role in roles[start:] + roles[:start]:
            yield entry, role


def run(args):
    plan = read(args.registration)
    suite = validate_registration(plan)
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    evidence = output / "evidence"
    evidence.mkdir()
    write(evidence / "registration.json", plan)
    cpu = subprocess.check_output(["lscpu"]).decode()
    core = min(os.sched_getaffinity(0))
    write(evidence / "context.json", {
        "cpu": cpu, "platform": platform.platform(), "core": core,
        "controllerRevision": git("rev-parse", "HEAD"),
        "controllerSHA256": digest(__file__), "runId": os.environ.get("GITHUB_RUN_ID"),
        "scope": "Hardware diagnosis only; no acceptance or publication state changes",
        "firstExecution": "Retained independently; all-sample spread is also reported",
        "buildTiming": "One preparation observation, not a build performance claim",
        "seedSHA256": digest(args.seed), "referenceSHA256": digest(args.reference),
        "goVersion": subprocess.check_output(["go", "version"]).decode().strip(),
        "referenceRevision": (ROOT / "TYPE_RB_REVISION").read_text().strip(),
    })
    if platform.system() != "Linux" or platform.machine() != "aarch64" or "Neoverse-N2" not in cpu:
        raise ValueError("The registered comparison requires a Neoverse N2 runner")
    if digest(args.seed) != "3627fd8c574a668173ac6bb25eb41371c0f6cf0fa9da1016eebfc08b078d4060":
        raise ValueError("Published Native input seed differs")
    env = {key: value for key, value in os.environ.items()
           if not key.startswith("TYPE_RB_NATIVE_RUNTIME_")}
    env.update(LC_ALL="C", GOMAXPROCS="1")
    if ("floatControls" in plan and subprocess.check_output(["go", "version"]).decode().strip()
            != plan["floatControls"]["goVersion"]):
        raise ValueError("Float attribution Go version differs")
    raw, prepared = [], {}
    reused = load_reused(plan["reuse"], output) if "reuse" in plan else None
    reused_plan = read(reused / "registration.json") if reused else None
    if reused:
        if (reused_plan["suiteSHA256"] != plan["suiteSHA256"]
                or read(reused / "context.json")["referenceRevision"] != (ROOT / "TYPE_RB_REVISION").read_text().strip()):
            raise ValueError("Reused workload or reference inputs differ")
        write(evidence / "reuse.json", {
            "source": plan["reuse"], "context": read(reused / "context.json"),
            "preparation": "Exact accepted applications and one new compiler generation per candidate; not a new bootstrap qualification",
        })

    def record(command, directory, timeout, **metadata):
        item = observe(command, directory, timeout, env=env, core=core)
        item.update(metadata)
        raw.append(item)
        write(directory / "observation.json", item)
        write(evidence / "raw.json", raw)
        if item["status"] != "pass":
            raise ValueError(f"Command failed: {directory}")
        return item

    for role, variant in plan["variants"].items():
        if reused and "reusedRole" in variant:
            previous = reused_plan["variants"][variant["reusedRole"]]
            if previous["revision"] != variant["revision"] or previous["appSHA256"] != variant["appSHA256"]:
                raise ValueError("Reused source or registered applications differ")
            prepared[role] = {"reusedRole": variant["reusedRole"]}
            continue
        print(f"Prepare {role} at {variant['revision']}", flush=True)
        source = output / "sources" / role
        source.parent.mkdir(exist_ok=True)
        subprocess.run(["git", "worktree", "add", "--detach", source, variant["revision"]],
                       cwd=ROOT, check=True)
        directory = evidence / "compilers" / role
        directory.mkdir(parents=True)
        qbe_root = output / "backends" / role
        record(["python3", source / "tools/qbe_toolchain.py", qbe_root],
               directory / "qbe-build", 120, kind="qbe-build", role=role)
        qbe = qbe_root / "qbe"
        shutil.copyfile(qbe_root / "build-info.json", directory / "qbe-build-info.json")
        compiler = verified_compiler(reused, variant["parentCompilerRole"]) if reused else args.seed
        generations = []
        for generation in range(1 if reused else 4):
            generation_dir = directory / str(generation)
            generation_dir.mkdir()
            produced = generation_dir / "compiler"
            trace = generation_dir / "process.trace"
            record(["strace", "-f", "-e", "trace=process", "-o", trace,
                    compiler, "build", source / "compiler/src/compiler.trb",
                    "--output", produced, "--qbe", qbe, "--cc", "/usr/bin/cc",
                    "--target", "linux-arm64-v0"],
                   generation_dir / "build", 240, kind="ordinary-generation",
                   role=role, generation=generation)
            if re.search(r'execve\("[^"]*/(?:go|trb|sh|bash|zsh)"', trace.read_text()):
                raise ValueError("Unexpected host compiler or shell in ordinary build")
            if (generation_dir / "build/stdout").stat().st_size or (generation_dir / "build/stderr").stat().st_size:
                raise ValueError("Unexpected ordinary compiler output")
            generations.append({"sha256": digest(produced), "bytes": produced.stat().st_size})
            compiler = produced
        write(directory / "generations.json", generations)
        if not reused and generations[-1] != generations[-2]:
            raise ValueError("Ordinary compiler fixed point differs")
        prepared[role] = {"compiler": compiler, "qbe": qbe}

    cases = []
    for case in suite["cases"]:
        if "cases" in plan and case["id"] not in plan["cases"]:
            continue
        case_dir = evidence / "apps" / case["id"]
        case_dir.mkdir(parents=True)
        expected = source_case(case, case_dir)
        (case_dir / "expected").write_bytes(expected)
        roles = [*prepared, "typerb-go"]
        if "pureGoSource" in case:
            roles.append("pure-go")
            shutil.copyfile(ROOT / case["pureGoSource"], case_dir / "pure-go.go")
        artifacts = {}
        for role in roles:
            role_dir = case_dir / role
            role_dir.mkdir()
            program = role_dir / "program"
            reused_role = prepared.get(role, {}).get("reusedRole")
            if reused and (reused_role or role in ("pure-go", "typerb-go")):
                old_case = reused / "apps" / case["id"]
                if (old_case / "expected").read_bytes() != expected:
                    raise ValueError("Reused expected output differs")
                old_role = reused_role or role
                identity = read(old_case / "identities.json")[old_role]
                shutil.copyfile(verified_program(old_case / old_role, identity), program)
                program.chmod(0o755)
                if reused_role:
                    for extension in ("ssa", "s"):
                        shutil.copyfile(old_case / old_role / f"program.{extension}", role_dir / f"program.{extension}")
                artifacts[role] = identity
                if reused_role and identity["sha256"] != plan["variants"][role]["appSHA256"][case["id"]]:
                    raise ValueError("Reused program differs from registration")
                write(case_dir / "identities.json", artifacts)
                continue
            elif role == "pure-go":
                command = ["go", "build", "-trimpath", "-o", program, case_dir / "pure-go.go"]
            elif role == "typerb-go":
                command = [args.reference, "build", "--compile", "--config",
                           case_dir / "trbconfig.jsonc", "--outfile", program]
            else:
                compiler = prepared[role]["compiler"]
                qbe = prepared[role]["qbe"]
                command = [compiler, "build", case_dir / "src/main.trb", "--output",
                           program, "--qbe", qbe, "--cc", "/usr/bin/cc", "--target", "linux-arm64-v0"]
            record(command, role_dir / "build", 120, kind="application-build",
                   case=case["id"], role=role)
            if not program.is_file() or (role_dir / "build/stderr").stat().st_size:
                raise ValueError("Application build failed")
            artifacts[role] = {"sha256": digest(program), "bytes": program.stat().st_size}
            write(case_dir / "identities.json", artifacts)
            if role in prepared:
                if artifacts[role]["sha256"] != plan["variants"][role]["appSHA256"][case["id"]]:
                    raise ValueError(f"{case['id']} {role}: application differs from registered diagnostic")
                with (role_dir / "program.ssa").open("wb") as emitted:
                    subprocess.run([prepared[role]["compiler"], "emit-qbe", case_dir / "src/main.trb"],
                                   stdout=emitted, check=True, env=env, timeout=90)
                subprocess.run([prepared[role]["qbe"], "-t", "arm64", "-o",
                                role_dir / "program.s", role_dir / "program.ssa"], check=True)
        if case["id"] == "n-body" and "floatControls" in plan:
            controls = prepare_controls((case_dir / "pure-go.go").read_text(), case_dir,
                                        artifacts["pure-go"], record, write)
            roles.extend(controls)
            artifacts.update(controls)
            write(case_dir / "identities.json", artifacts)
        cases.append((case, case_dir, expected, roles, artifacts))

    for iteration in range(6):
        for entry, role in runtime_schedule(cases, iteration):
            case, directory, expected, roles, artifacts = entry
            program = directory / role / "program"
            if digest(program) != artifacts[role]["sha256"]:
                raise ValueError("Application changed after preparation")
            observation_dir = directory / role / f"runtime-{iteration}"
            item = record([program, *case["args"]], observation_dir, 30,
                          kind="runtime", case=case["id"], role=role,
                          phase="first" if iteration == 0 else "steady",
                          iteration=iteration, artifact=artifacts[role])
            correct(item, observation_dir, expected)
            write(observation_dir / "observation.json", item)
            write(evidence / "raw.json", raw)
            if item["status"] != "pass":
                raise ValueError("Runtime output differs")
        print(f"Completed round {iteration}; {len(raw)} observations retained", flush=True)
    rows = []
    for case, _directory, _expected, roles, artifacts in cases:
        for role in roles:
            rows.append({"case": case["id"], "role": role, "artifact": artifacts[role],
                         "runtime": summary([r for r in raw if r["kind"] == "runtime"
                                             and r["case"] == case["id"] and r["role"] == role])})
    write(evidence / "summary.json", {"scope": "Diagnostic, not final qualification", "rows": rows})
    print(json.dumps(rows, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registration", type=Path, required=True)
    parser.add_argument("--seed", type=Path, required=True)
    parser.add_argument("--reference", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    run(parser.parse_args())
