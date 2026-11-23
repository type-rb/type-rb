#!/usr/bin/env python3
"""Map reviewed release declarations and observe one exact compiler candidate.

This produces scenario evidence, never release qualification. Unmapped members,
remaining semantic boundaries and unobserved output/host cells stay explicit.
The canonical manifest and its qualification/deferral decisions are read-only.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
PLAN = ROOT / "tools/release-scenarios.json"
UNSUPPORTED = re.compile(r"unsupported|not implemented|not available for mode|unknown command", re.I)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_json(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key: {key}")
            result[key] = value
        return result
    return json.loads(path.read_text(), object_pairs_hook=unique)


def fields(value, required, optional=()):
    if (not isinstance(value, dict) or not set(required) <= value.keys()
            or value.keys() - set(required) - set(optional)):
        raise ValueError(f"invalid fields; expected {sorted(required)}")


def load_plan(contract, path=PLAN):
    manifest_path = contract / "contracts/v0.5/manifest.json"
    catalog_path = contract / "contracts/v0.5/surfaces.json"
    manifest, catalog, plan = map(read_json, (manifest_path, catalog_path, path))
    if (manifest.get("schemaVersion") != 2 or manifest.get("release") != "0.5.0"
            or manifest.get("catalogSHA256") != digest(catalog_path)):
        raise ValueError("invalid release contract or stale catalog identity")
    fields(plan, ("schemaVersion", "rows", "scenarios"))
    if type(plan["schemaVersion"]) is not int or plan["schemaVersion"] != 1:
        raise ValueError("unsupported scenario plan version")
    if not isinstance(plan["rows"], list) or not plan["rows"] or not isinstance(plan["scenarios"], list) or not plan["scenarios"]:
        raise ValueError("empty scenario plan")
    rows = {row["id"]: row for row in manifest["rows"]}
    surfaces = {surface["id"]: surface for surface in catalog}
    scenarios = {}
    for case in plan["scenarios"]:
        fields(case, ("id", "row", "declarations", "body", "kind", "stdout"), ("diagnostic",))
        if (not all(isinstance(case[key], str) for key in case)
                or not re.fullmatch(r"[a-z][a-z0-9-]+", case["id"])
                or case["id"] in scenarios or case["row"] not in rows
                or case["kind"] not in ("accepted", "rejected") or not case["body"]):
            raise ValueError("invalid or duplicate scenario")
        if case["kind"] == "rejected":
            if not case.get("diagnostic") or case["stdout"]:
                raise ValueError("rejection needs a reviewed diagnostic and no successful output")
            re.compile(case["diagnostic"])
        elif "diagnostic" in case:
            raise ValueError("accepted scenario cannot expect a diagnostic")
        scenarios[case["id"]] = case
    planned, used = {}, set()
    for row in plan["rows"]:
        fields(row, ("id", "bindings", "remaining"))
        if row["id"] not in rows or row["id"] in planned or not isinstance(row["bindings"], list) or not row["bindings"]:
            raise ValueError("unknown, duplicate or empty planned row")
        if not isinstance(row["remaining"], list) or not all(isinstance(s, str) and s.strip() for s in row["remaining"]):
            raise ValueError("remaining boundaries must be explicit strings")
        bindings = set()
        for binding in row["bindings"]:
            fields(binding, ("surface", "member", "scenarios"))
            key = (binding["surface"], binding["member"])
            ids = binding["scenarios"]
            if (key in bindings or key[0] not in rows[row["id"]]["surfaces"]
                    or key[1] not in surfaces[key[0]]["members"]
                    or not isinstance(ids, list) or not ids or len(set(ids)) != len(ids)
                    or any(i not in scenarios or scenarios[i]["row"] != row["id"] for i in ids)):
                raise ValueError("unknown, duplicate or cross-row declaration binding")
            bindings.add(key)
            used.update(ids)
        planned[row["id"]] = row
    if used != scenarios.keys():
        raise ValueError("every scenario must bind an owned declaration")
    inventory = []
    for identity, row in rows.items():
        selected = planned.get(identity)
        mapped = {(b["surface"], b["member"]) for b in selected["bindings"]} if selected else set()
        members = [(surface, member) for surface in row["surfaces"] for member in surfaces[surface].get("members", [])]
        inventory.append({"row": identity, "status": "unqualified", "members": len(members),
                          "mappedMembers": len(mapped),
                          "unmappedSurfaces": [s for s in row["surfaces"] if not any(key[0] == s for key in mapped)],
                          "unmapped": [{"surface": s, "member": m} for s, m in members if (s, m) not in mapped],
                          "remaining": selected["remaining"] if selected else ["No reviewed scenario mapping."],
                          "qualificationReady": row["qualificationReady"], "deferrals": row.get("deferrals", [])})
    return manifest, plan, inventory, {"manifestSHA256": digest(manifest_path),
                                      "catalogSHA256": digest(catalog_path), "planSHA256": digest(path)}


def host():
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}.get(platform.machine())
    if not system or not arch:
        raise ValueError("unsupported observation host")
    return system + "-" + arch


def observe(argv, root, env, source=None, timeout=90):
    child = subprocess.Popen([str(arg) for arg in argv], cwd=root, env=env,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             start_new_session=True)
    timed_out = False
    try:
        stdout, stderr = child.communicate(None if source is None else source.encode(), timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(child.pid, signal.SIGKILL)
        stdout, stderr = child.communicate()
    except BaseException:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
        child.communicate()
        raise
    result = {"argv": [str(a) for a in argv], "exitCode": child.returncode,
              "timedOut": timed_out, "stdout": "", "stderr": ""}
    for name, data in (("stdout", stdout), ("stderr", stderr)):
        data = data.replace(str(root).encode(), b"<case>")
        try:
            result[name] = data.decode("utf-8")
        except UnicodeDecodeError:
            result[name] = data.decode("utf-8", "backslashreplace")
            result[name + "Bytes"] = data.hex()
    return result


def accepted(observation):
    return observation is not None and observation["exitCode"] == 0 and not observation["timedOut"]


def assess(case, observations):
    results = {}
    for path in ("check", "build", "execute", "repl"):
        observed = observations.get(path)
        if observed is None:
            results[path] = "not-observed"
            continue
        if observed["timedOut"]:
            results[path] = "failed"
            continue
        if case["kind"] == "rejected":
            # A missing implementation cannot certify a type/visibility error.
            matches = (not UNSUPPORTED.search(observed["stderr"])
                       and re.search(case["diagnostic"], observed["stderr"]) is not None
                       and observed["stdout"] == ""
                       and ((path == "repl" and observed["exitCode"] == 0)
                            or (path in ("check", "build") and observed["exitCode"] > 0)))
        else:
            matches = accepted(observed) and observed["stderr"] == ""
            if path in ("execute", "repl"):
                matches = matches and observed["stdout"] == case["stdout"] and "stdoutBytes" not in observed
        results[path] = "matched" if matches else "failed"
    return results


def collect(case, compiler, mode, runtime, root, tools, record=lambda observations: None):
    config = {"name": "release-scenario", "mode": "trb" if mode == "native" else mode,
              "sourceDir": "src", "outDir": "out"}
    if mode == "go":
        config["go"] = {"module": "example.com/release-scenario"}
    elif mode == "typescript":
        config["typescript"] = {"runtime": runtime}
    (root / "src").mkdir()
    source = case["declarations"] + "\ndef probe()\n" + case["body"] + "\nend\n"
    (root / "src/main.trb").write_text(source + "\ndef main()\nprobe()\nend\n")
    (root / "trbconfig.jsonc").write_text(json.dumps(config))
    env = dict(os.environ, NO_COLOR="1", TERM="dumb", TRBN_HISTORY=str(root / "history.json"),
               TRB_SCENARIO_PRESENT="value 日本語", TRB_SCENARIO_EMPTY="", TRB_SCENARIO_CWD=str(root),
               TRB_SCENARIO_PYTHON=sys.executable)
    env.pop("TRB_SCENARIO_ABSENT", None)
    observations = {"check": observe([compiler, "check", "--config", "trbconfig.jsonc"], root, env)}
    record(observations)
    executable = root / "program"
    build = [compiler, "build", "--config", "trbconfig.jsonc"]
    if mode in ("go", "native"):
        build += ["--compile", "--outfile", executable]
    observations["build"] = observe(build, root, env)
    observations["execute"] = None
    record(observations)
    if accepted(observations["build"]):
        if mode in ("go", "native"):
            if not executable.is_file():
                raise ValueError("successful build did not publish its executable")
            command = [executable]
        else:
            # Source modules contain declarations; the compiler's generated
            # application entry performs main invocation and initialization.
            entry = root / "out/trb/entry/application" / ("main.rb" if mode == "ruby" else "main.ts")
            if not entry.is_file():
                raise ValueError("successful build did not publish its source entry")
            command = [tools["ruby" if mode == "ruby" else runtime]["path"]]
            if mode == "typescript":
                command += ["--experimental-strip-types"] if runtime == "node" else ["run"]
            command.append(entry)
        outputs = [executable] if mode in ("go", "native") else sorted((root / "out").rglob("*"))
        observations["build"]["artifacts"] = {
            str(path.relative_to(root)): {"sha256": digest(path), "bytes": path.stat().st_size}
            for path in outputs if path.is_file()}
        record(observations)
        observations["execute"] = observe(command, root, env, timeout=30)
        record(observations)
    elif executable.exists():
        raise ValueError("failed build published an executable")
    # Run in a separate scratch directory: loading the project and then
    # redeclaring probe would test duplicate declarations rather than the API.
    scratch = root / "interactive"
    scratch.mkdir()
    repl_config = dict(config, sourceDir=".", outDir="out")
    (scratch / "trbconfig.jsonc").write_text(json.dumps(repl_config))
    repl_env = dict(env, TRB_SCENARIO_CWD=str(scratch))
    observations["repl"] = observe([compiler, "repl", "--config", "trbconfig.jsonc"], scratch,
                                    repl_env, source + "\nprobe()\n:quit\n", timeout=30)
    record(observations)
    return observations


def tool_identities(mode, runtime, compiler):
    names = ["python3"] + ({"go": ["go"], "ruby": ["ruby"], "typescript": [runtime], "native": []}[mode])
    identities = {}
    for name in names:
        path = shutil.which(name)
        if path is None:
            raise ValueError(f"required tool is unavailable: {name}")
        command = [path, "version" if name == "go" else "--version"]
        p = subprocess.run(command, capture_output=True, text=True, timeout=10, check=True)
        identities[name] = {"path": path, "sha256": digest(Path(path)), "version": p.stdout.strip() or p.stderr.strip()}
    if mode == "native":
        qbe = Path(os.environ.get("TRBN_QBE", str(compiler.parent / "qbe"))).resolve(strict=True)
        identities["qbe"] = {"path": str(qbe), "sha256": digest(qbe)}
        cc = shutil.which(os.environ.get("CC", "cc"))
        if cc is None:
            raise ValueError("required native linker is unavailable")
        version = subprocess.run([cc, "--version"], capture_output=True, text=True, timeout=10, check=True)
        identities["cc"] = {"path": cc, "sha256": digest(Path(cc)), "version": version.stdout.strip()}
        for name in ("CRYPTO", "NETWORK", "OIDC", "COMPRESSION"):
            directory = os.environ.get("TRBN_" + name + "_LIB_DIR")
            if directory:
                identities[name.lower() + "Archives"] = {
                    "path": directory,
                    "archives": {p.name: digest(p) for p in sorted(Path(directory).glob("*.a"))}}
    return identities


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--contract-root", type=Path, required=True)
    parser.add_argument("--plan", type=Path, default=PLAN)
    parser.add_argument("--inventory", action="store_true")
    parser.add_argument("--compiler", type=Path)
    parser.add_argument("--compiler-sha256")
    parser.add_argument("--source-revision")
    parser.add_argument("--implementation", choices=("go", "native"))
    parser.add_argument("--mode", choices=("go", "ruby", "typescript", "native"))
    parser.add_argument("--typescript-runtime", choices=("node", "bun"), default="node")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    manifest, plan, inventory, identities = load_plan(args.contract_root, args.plan)
    if args.inventory:
        if any((args.compiler, args.compiler_sha256, args.source_revision, args.implementation, args.mode, args.report)):
            parser.error("inventory does not execute a compiler or write an observation report")
        print(json.dumps({**identities, "rows": inventory}, indent=2))
        return 0
    if (args.compiler is None or args.report is None or not args.implementation or not args.mode
            or not re.fullmatch(r"[a-f0-9]{40}", args.source_revision or "")
            or not re.fullmatch(r"[a-f0-9]{64}", args.compiler_sha256 or "")):
        parser.error("observations require compiler, expected SHA256, source revision, implementation, mode and new report path")
    if args.implementation == "go" and args.mode == "native":
        parser.error("the Go implementation has no Native output obligation")
    compiler = args.compiler.resolve(strict=True)
    if digest(compiler) != args.compiler_sha256:
        raise ValueError("compiler differs from the identified candidate")
    if args.report.exists():
        raise ValueError("refusing to overwrite existing observations")
    current_host = host()
    if current_host not in manifest["hosts"]:
        raise ValueError("host is outside the release contract")
    tools = tool_identities(args.mode, args.typescript_runtime, compiler)
    report = {"schemaVersion": 1, "kind": "release-scenario-observations", **identities,
              "releaseQualified": False, "complete": False, "rows": inventory, "cases": [],
              "candidate": {"implementation": args.implementation, "output": args.mode, "host": current_host,
                            "sourceRevision": args.source_revision, "compilerSHA256": args.compiler_sha256,
                            "toolchains": tools}, "controllerSHA256": digest(Path(__file__))}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    # Reserve the filename before executing any candidate. Every completed case
    # is durable even if a later command, cancellation or preparation fails.
    with args.report.open("x") as stream:
        json.dump(report, stream, indent=2)
    def save():
        args.report.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    try:
        for case in plan["scenarios"]:
            result = {"id": case["id"], "row": case["row"], "complete": False,
                      "observations": {}, "paths": assess(case, {})}
            report["cases"].append(result)
            def record(observations):
                result.update(observations=observations, paths=assess(case, observations))
                save()
            with tempfile.TemporaryDirectory(prefix="trb-release-scenario-") as temporary:
                observations = collect(case, compiler, args.mode, args.typescript_runtime, Path(temporary).resolve(), tools, record)
            record(observations)
            result["complete"] = True
            save()
            print(case["id"], json.dumps(result["paths"]), flush=True)
        if digest(compiler) != args.compiler_sha256:
            raise ValueError("compiler changed during observations")
        report["complete"] = True
    except BaseException as error:
        report["interruption"] = str(error) or type(error).__name__
        save()
        raise
    save()
    return 1 if any("failed" in c["paths"].values() for c in report["cases"]) else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"release-scenarios: {error}", file=sys.stderr)
        sys.exit(2)
