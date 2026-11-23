"""Reuse exact diagnostic artifacts without implying a new bootstrap qualification."""
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import zipfile

from measure import digest

PREFIX = "_temp/hardware-diagnostic/evidence/"
REPOSITORY = "type-rb/type-rb-native"


def validate_reuse(plan):
    if set(plan) != {"runId", "artifactId", "archiveSHA256", "controllerRevision"}:
        raise ValueError("Invalid artifact reuse registration")
    if any(type(plan[key]) is not int or plan[key] <= 0 for key in ("runId", "artifactId")):
        raise ValueError("Artifact reuse requires exact positive identifiers")
    for key, length in (("archiveSHA256", 64), ("controllerRevision", 40)):
        if not re.fullmatch(r"[0-9a-f]{" + str(length) + "}", plan[key]):
            raise ValueError("Artifact reuse requires exact digests")


def unpack_verified(archive, output, expected):
    if digest(archive) != expected:
        raise ValueError("Reused artifact archive digest differs")
    with zipfile.ZipFile(archive) as packed:
        members = [item for item in packed.infolist() if item.filename.startswith(PREFIX)]
        if not members or sum(item.file_size for item in members) > 1024 * 1024 * 1024:
            raise ValueError("Missing or oversized diagnostic evidence")
        for item in members:
            relative = PurePosixPath(item.filename[len(PREFIX):])
            if relative.is_absolute() or ".." in relative.parts:
                raise ValueError("Unsafe diagnostic evidence path")
            if item.is_dir():
                continue
            target = output / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            with packed.open(item) as source, target.open("xb") as destination:
                shutil.copyfileobj(source, destination)


def load_reused(plan, output):
    validate_reuse(plan)
    api = f"repos/{REPOSITORY}/actions"
    artifact = json.loads(subprocess.check_output(["gh", "api", f"{api}/artifacts/{plan['artifactId']}"]))
    run = json.loads(subprocess.check_output(["gh", "api", f"{api}/runs/{plan['runId']}"]))
    if (artifact["expired"] or artifact["workflow_run"]["id"] != plan["runId"]
            or artifact["workflow_run"]["head_sha"] != plan["controllerRevision"]
            or run["head_sha"] != plan["controllerRevision"] or run["run_attempt"] != 1
            or run["conclusion"] != "success"):
        raise ValueError("Reused diagnostic provenance differs")
    archive = output / "reused-artifact.zip"
    with archive.open("xb") as stream:
        subprocess.run(["gh", "api", f"{api}/artifacts/{plan['artifactId']}/zip"],
                       stdout=stream, check=True)
    destination = output / "reused-evidence"
    unpack_verified(archive, destination, plan["archiveSHA256"])
    context = json.loads((destination / "context.json").read_text())
    if context["controllerRevision"] != plan["controllerRevision"]:
        raise ValueError("Reused evidence controller differs")
    records = json.loads((destination / "raw.json").read_text())
    if not records or any(row["status"] != "pass" for row in records):
        raise ValueError("Reused diagnostic did not pass")
    return destination


def verified_program(directory, identity):
    binary = directory / "program"
    if digest(binary) != identity["sha256"] or binary.stat().st_size != identity["bytes"]:
        raise ValueError("Reused application identity differs")
    return binary


def verified_compiler(evidence, role):
    if not re.fullmatch(r"[a-z][a-z0-9-]*", role):
        raise ValueError("Invalid reused compiler role")
    directory = evidence / "compilers" / role
    generations = json.loads((directory / "generations.json").read_text())
    if len(generations) != 4 or generations[-1] != generations[-2]:
        raise ValueError("Reused compiler lacks an ordinary fixed point")
    binary = directory / "3/compiler"
    identity = generations[-1]
    if digest(binary) != identity["sha256"] or binary.stat().st_size != identity["bytes"]:
        raise ValueError("Reused compiler identity differs")
    binary.chmod(0o755)
    return binary
