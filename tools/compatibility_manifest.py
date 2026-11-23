#!/usr/bin/env python3
"""Validate the tracked TypeRB Native compatibility declaration."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

from qbe_toolchain import backend_identity


class ValidationError(Exception):
    pass


def _reject_duplicate_members(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValidationError(f"duplicate JSON object member: {key}")
        result[key] = value
    return result


def load_json_text_strict(source: str, label: str) -> Any:
    try:
        return json.loads(source, object_pairs_hook=_reject_duplicate_members)
    except json.JSONDecodeError as error:
        raise ValidationError(
            f"{label}: invalid JSON at line {error.lineno}, column {error.colno}: {error.msg}"
        ) from error


def load_json_strict(path: Path) -> Any:
    try:
        source = path.read_text(encoding="utf-8")
    except OSError as error:
        raise ValidationError(f"cannot read {path}: {error}") from error
    return load_json_text_strict(source, str(path))


def _json_type_matches(value: Any, expected: str) -> bool:
    if expected == "object":
        return isinstance(value, dict)
    if expected == "array":
        return isinstance(value, list)
    if expected == "string":
        return isinstance(value, str)
    if expected == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if expected == "null":
        return value is None
    raise ValidationError(f"schema uses unsupported type {expected!r}")


def validate_schema(instance: Any, schema: dict[str, Any], location: str = "$") -> None:
    expected_type = schema.get("type")
    if expected_type is not None and not _json_type_matches(instance, expected_type):
        raise ValidationError(f"{location}: expected {expected_type}")

    if "const" in schema and instance != schema["const"]:
        raise ValidationError(f"{location}: expected constant {schema['const']!r}")
    if "enum" in schema and instance not in schema["enum"]:
        raise ValidationError(f"{location}: value is not in the allowed set")

    if isinstance(instance, str) and "pattern" in schema:
        try:
            matched = re.search(schema["pattern"], instance)
        except re.error as error:
            raise ValidationError(f"{location}: invalid schema pattern: {error}") from error
        if matched is None:
            raise ValidationError(f"{location}: value does not match the required pattern")

    if isinstance(instance, dict):
        required = schema.get("required", [])
        missing = [key for key in required if key not in instance]
        if missing:
            raise ValidationError(f"{location}: missing required member {missing[0]!r}")

        properties = schema.get("properties", {})
        if schema.get("additionalProperties") is False:
            unknown = [key for key in instance if key not in properties]
            if unknown:
                raise ValidationError(f"{location}: unknown member {unknown[0]!r}")
        for key, value in instance.items():
            if key in properties:
                validate_schema(value, properties[key], f"{location}.{key}")

    if isinstance(instance, list):
        if len(instance) < schema.get("minItems", 0):
            raise ValidationError(f"{location}: array has too few items")
        if "maxItems" in schema and len(instance) > schema["maxItems"]:
            raise ValidationError(f"{location}: array has too many items")
        if schema.get("uniqueItems"):
            encoded = [json.dumps(value, sort_keys=True, separators=(",", ":")) for value in instance]
            if len(encoded) != len(set(encoded)):
                raise ValidationError(f"{location}: array items must be unique")
        item_schema = schema.get("items")
        if item_schema is not None:
            for index, value in enumerate(instance):
                validate_schema(value, item_schema, f"{location}[{index}]")


def _canonical_line(path: Path) -> str:
    try:
        source = path.read_text(encoding="utf-8")
    except OSError as error:
        raise ValidationError(f"cannot read canonical input {path}: {error}") from error
    if not source.endswith("\n") or source.count("\n") != 1:
        raise ValidationError(f"{path}: expected exactly one newline-terminated value")
    value = source[:-1]
    if not value:
        raise ValidationError(f"{path}: canonical value is empty")
    return value


def _repository_path(root: Path, relative: str) -> Path:
    if Path(relative).is_absolute():
        raise ValidationError(f"evidence path must be relative: {relative}")
    candidate = (root / relative).resolve()
    try:
        candidate.relative_to(root.resolve())
    except ValueError as error:
        raise ValidationError(f"evidence path escapes the repository: {relative}") from error
    if not candidate.is_file():
        raise ValidationError(f"evidence file does not exist: {relative}")
    return candidate


def _require_text(path: Path, values: list[str], label: str) -> str:
    try:
        source = path.read_text(encoding="utf-8")
    except OSError as error:
        raise ValidationError(f"cannot read {path}: {error}") from error
    for value in values:
        if value not in source:
            raise ValidationError(f"{label}: canonical value {value!r} is absent from {path}")
    return source


def _expected_targets(seed_manifest: dict[str, Any]) -> list[dict[str, str]]:
    targets: list[dict[str, str]] = []
    for target in seed_manifest["targets"]:
        targets.append(
            {
                "profile": target["profile"],
                "status": "experimental",
                "os": target["os"],
                "architecture": target["architecture"],
                "qbeTarget": target["qbeTarget"],
                "ccBoundary": target["ccBoundary"],
            }
        )
    return targets


# Exact inventory of reference consumers. Historical experiments retain their
# source-era oracle; they must not follow a development pin update.
REFERENCE_WORKFLOWS = {
    "native-validation.yml": ("direct", 1),
    "pull-request.yml": ("direct", 2),
    "runtime-worker-memory.yml": ("environment", 1),
    "benchmarksgame-formal.yml": ("environment", 1),
    "benchmarksgame-build-formal.yml": ("environment", 1),
    "linux-amd64-targets.yml": ("environment", 1),
    "daily-performance.yml": ("derived", 1),
    "cumulative-performance.yml": ("derived", 1),
    "weekly-performance.yml": ("derived", 1),
    "representative-performance.yml": ("derived", 1),
}


def _reference_refs(source: str, name: str) -> list[str]:
    """Read the supported checkout-with mapping without a YAML dependency.

    These maintained workflows use block mappings. Unknown/ambiguous spellings
    fail closed rather than guessing at an equivalent checkout expression.
    """
    lines = source.splitlines()
    refs: list[str] = []
    for index, line in enumerate(lines):
        match = re.fullmatch(r"( +)repository: *['\"]?type-rb/type-rb['\"]? *(?:#.*)?", line)
        if match is None:
            continue
        indent = len(match[1])
        start, finish = index, index + 1
        while start > 0 and (not lines[start - 1].strip() or len(lines[start - 1]) - len(lines[start - 1].lstrip()) >= indent):
            start -= 1
        while finish < len(lines) and (not lines[finish].strip() or len(lines[finish]) - len(lines[finish].lstrip()) >= indent):
            finish += 1
        values = [m[1] for text in lines[start:finish]
                  if (m := re.fullmatch(r" {" + str(indent) + r"}ref: *(.*?) *", text))]
        if len(values) != 1:
            raise ValidationError(f"reference checkout {name}: expected one ref per checkout")
        refs.append(values[0])
    return refs


def validate_reference_checkouts(root: Path, revision: str) -> dict[str, str]:
    directory = root / ".github/workflows"
    sources = {path.name: _require_text(path, [], "reference checkout")
               for pattern in ("*.yml", "*.yaml") for path in sorted(directory.glob(pattern))}
    sources = {name: _without_comment_lines(source) for name, source in sources.items()}
    # Catch an added literal consumer, including an unrecognized mapping shape.
    observed = {name for name, source in sources.items()
                if re.search(r"type-rb/type-rb(?![-\w/])", source)}
    expected = set(REFERENCE_WORKFLOWS)
    if observed != expected:
        raise ValidationError("reference checkout inventory differs: missing=" +
                              ",".join(sorted(expected - observed)) + " added=" +
                              ",".join(sorted(observed - expected)))
    for name, (mode, count) in REFERENCE_WORKFLOWS.items():
        source = sources[name]
        refs = _reference_refs(source, name)
        if mode == "direct":
            expected_ref = revision
            identities = ['test "$(cat TYPE_RB_REVISION)" = "$(git -C .type-rb rev-parse HEAD)"']
        elif mode == "derived":
            expected_ref = "${{ steps.reference.outputs.revision }}"
            producer = 'run: echo "revision=$(cat TYPE_RB_REVISION)" >> "$GITHUB_OUTPUT"'
            checkout = source.find("repository: type-rb/type-rb")
            if (len(re.findall(r"(?m)^ +id: reference$", source)) != 1 or
                    _command_count(source, producer) != 1 or
                    checkout < 0 or source.index(producer) > checkout):
                raise ValidationError(f"reference checkout {name}: canonical producer differs")
            identities = ['test "$(git -C .type-rb rev-parse HEAD)" = "$(cat TYPE_RB_REVISION)"']
        else:
            expected_ref = "${{ env.TYPE_RB_REVISION }}"
            pin = revision if mode == "environment" else mode
            pins = re.findall(r"(?m)^ *TYPE_RB_REVISION: *(\S+) *$", source)
            if pins != [pin]:
                raise ValidationError(f"reference checkout {name}: environment pin differs")
            identities = ['test "$(git -C .type-rb rev-parse HEAD)" = "$TYPE_RB_REVISION"']
            canonical = ".native-target-candidate/TYPE_RB_REVISION" if name == "linux-amd64-targets.yml" else "TYPE_RB_REVISION"
            identities.append(f'test "$(cat {canonical})" = "$TYPE_RB_REVISION"')
        if refs != [expected_ref] * count:
            raise ValidationError(f"reference checkout {name}: exact checkout refs differ")
        for identity in identities:
            if _command_count(source, identity) != count:
                raise ValidationError(f"reference checkout {name}: post-checkout identity differs")
        if mode in ("direct", "derived", "environment"):
            prefix = ".native-target-candidate/" if name == "linux-amd64-targets.yml" else ""
            command = f'python3 {prefix}tools/build-reference.py .type-rb "$RUNNER_TEMP/trb"'
            if name in ("benchmarksgame-formal.yml", "benchmarksgame-build-formal.yml",
                        "daily-performance.yml", "weekly-performance.yml", "cumulative-performance.yml",
                        "representative-performance.yml"):
                command += " --trimpath"
            if _command_count(source, command) != count:
                raise ValidationError(f"reference checkout {name}: canonical versioned build differs")
    controller = root / "tools/linux-amd64-targets.sh"
    source = _without_comment_lines(_require_text(controller, [], "reference controller"))
    if re.findall(r"(?m)^TYPE_RB_REVISION=(\S+)$", source) != [revision]:
        raise ValidationError("reference checkout linux-amd64-targets.sh: controller pin differs")
    identity = 'test "$(tr -d \'\\n\' < "$candidate_root/TYPE_RB_REVISION")" = "$TYPE_RB_REVISION" ||'
    if _command_count(source, identity) != 1:
        raise ValidationError("reference checkout linux-amd64-targets.sh: candidate identity differs")
    return sources


def _without_comment_lines(source: str) -> str:
    return "\n".join(line for line in source.split("\n")
                     if not line.lstrip().startswith("#"))


def _command_count(source: str, command: str) -> int:
    return len(re.findall(r"(?m)^[ \t]*(?:run: )?" + re.escape(command) + r"[ \t]*$", source))


def validate_repository_values(
    root: Path, manifest: dict[str, Any], reference_trb: Path | None = None
) -> None:
    native_version = _canonical_line(root / "NATIVE_VERSION")
    type_rb_revision = _canonical_line(root / "TYPE_RB_REVISION")
    if manifest["nativeVersion"] != native_version:
        raise ValidationError("nativeVersion disagrees with NATIVE_VERSION")
    if manifest["typeRB"]["revision"] != type_rb_revision:
        raise ValidationError("typeRB.revision disagrees with TYPE_RB_REVISION")

    workflows = validate_reference_checkouts(root, type_rb_revision)
    workflow = workflows["native-validation.yml"]
    for name in ("linux-amd64-targets.yml", "runtime-worker-memory.yml"):
        versions = re.findall(r"(?m)^ *TYPE_RB_VERSION: *(\S+) *$", workflows[name])
        if versions != [manifest["typeRB"]["version"]]:
            raise ValidationError(f"reference checkout {name}: environment version differs")
    controller = (root / "tools/linux-amd64-targets.sh").read_text()
    if re.findall(r"(?m)^TYPE_RB_VERSION=(\S+)$", controller) != [manifest["typeRB"]["version"]]:
        raise ValidationError("reference checkout linux-amd64-targets.sh: controller version differs")

    if reference_trb is not None:
        try:
            completed = subprocess.run(
                [str(reference_trb), "version"],
                check=False,
                capture_output=True,
                text=True,
            )
        except OSError as error:
            raise ValidationError(f"cannot execute reference compiler {reference_trb}: {error}") from error
        reported = completed.stdout.rstrip("\n")
        if completed.returncode != 0 or completed.stderr or "\n" in reported:
            raise ValidationError("reference compiler did not return one clean version line")
        if reported != manifest["typeRB"]["version"]:
            raise ValidationError("typeRB.version disagrees with the pinned reference compiler")

    snapshot_version = manifest["bootstrap"]["snapshotSchemaVersion"]
    snapshot_source = _require_text(
        root / "recovery/src/managed/snapshot.trb",
        [f"if version != {snapshot_version}"],
        "bootstrap snapshot",
    )
    if not re.search(rf"if version != {snapshot_version}\b", snapshot_source):
        raise ValidationError("snapshotSchemaVersion disagrees with the current decoder")
    if 'tools/check-bootstrap-snapshot.sh "$RUNNER_TEMP/trb"' not in workflow:
        raise ValidationError("snapshotSchemaVersion disagrees with the bootstrap workflow")
    snapshot_check = (root / "tools/check-bootstrap-snapshot.sh").read_text(encoding="utf-8")
    if f"--snapshot-version {snapshot_version}" not in snapshot_check:
        raise ValidationError("snapshotSchemaVersion disagrees with the bootstrap snapshot check")

    seed_manifest_path = (
        root
        / "results/2026-08-30-gate6l-bootstrap-seed-darwin-linux-arm64/release/"
        "type-rb-native-bootstrap-manifest-v1.json"
    )
    seed_manifest = load_json_strict(seed_manifest_path)
    seed = manifest["bootstrap"]["seed"]
    if seed["releaseTag"] != seed_manifest["releaseTag"]:
        raise ValidationError("bootstrap seed releaseTag disagrees with the retained release manifest")
    if seed["manifestSchemaVersion"] != seed_manifest["schemaVersion"]:
        raise ValidationError("bootstrap seed manifestSchemaVersion disagrees with the retained release manifest")
    if seed["status"] != seed_manifest["status"]:
        raise ValidationError("bootstrap seed status disagrees with the retained release manifest")

    expected_backend = {
        "name": seed_manifest["backend"]["name"],
        "version": seed_manifest["backend"]["version"],
        "sourceURL": seed_manifest["backend"]["sourceUrl"],
        "sourceSHA256": seed_manifest["backend"]["sourceSha256"],
    }
    if {key: manifest["backend"][key] for key in expected_backend} != expected_backend:
        raise ValidationError("backend identity disagrees with the retained release manifest")

    try:
        current_backend = backend_identity(root)
    except (ValueError, OSError) as error:
        raise ValidationError(f"backend identity: {error}") from error
    if manifest["backend"] != current_backend:
        raise ValidationError("backend identity disagrees with the current patched backend")

    seed_targets = _expected_targets(seed_manifest)
    current_targets = manifest["targets"]
    current_profiles = [target["profile"] for target in current_targets]
    if len(current_profiles) != len(set(current_profiles)):
        raise ValidationError("target profiles must be unique")
    if current_targets[: len(seed_targets)] != seed_targets:
        raise ValidationError("target profiles disagree with the retained release manifest")
    recovered_targets = current_targets[len(seed_targets) :]

    runtime_source = _require_text(
        root / "compiler/src/backend/qbe/runtime/system.trb",
        [
            *[f'b \\"{target["profile"]}\\"' for target in current_targets],
            *[f'b \\"{target["qbeTarget"]}\\"' for target in current_targets],
        ],
        "self-hosted target profile",
    )
    if not runtime_source:
        raise ValidationError("self-hosted runtime source is empty")

    bootstrap_tool = (root / "tools/bootstrap-seed.sh").read_text(encoding="utf-8")
    for target in current_targets:
        mapping = re.compile(
            rf"(?m)^{re.escape(target['profile'])}\)\n"
            rf"\s*os={re.escape(target['os'])}\n"
            rf"\s*architecture={re.escape(target['architecture'])}\n"
            rf"\s*qbe_target={re.escape(target['qbeTarget'])}\n"
        )
        if mapping.search(bootstrap_tool) is None:
            raise ValidationError(f"target profile {target['profile']} disagrees with bootstrap tooling")

    seed_tool = _require_text(
        root / "tools/bootstrap-seed-manifest.sh",
        [
            f"RELEASE_TAG={seed['releaseTag']}",
            f"QBE_SOURCE_URL={expected_backend['sourceURL']}",
            f"QBE_SOURCE_SHA256={expected_backend['sourceSHA256']}",
            f'version: "{expected_backend["version"]}"',
        ],
        "bootstrap and backend identity",
    )
    if not seed_tool:
        raise ValidationError("bootstrap seed manifest tool is empty")

    compatibility_evidence = manifest["evidence"]["compatibility"]
    compatibility_path = _repository_path(root, compatibility_evidence["result"])
    _require_text(
        compatibility_path,
        [
            manifest["typeRB"]["version"],
            manifest["typeRB"]["revision"],
            compatibility_evidence["nativeRevision"],
            compatibility_evidence["workflow"],
        ],
        "compatibility evidence",
    )

    bootstrap_evidence = manifest["evidence"]["bootstrap"]
    bootstrap_path = _repository_path(root, bootstrap_evidence["result"])
    _require_text(
        bootstrap_path,
        [
            bootstrap_evidence["nativeRevision"],
            seed["releaseTag"],
            bootstrap_evidence["release"],
        ],
        "bootstrap evidence",
    )
    if not bootstrap_evidence["release"].endswith("/" + seed["releaseTag"]):
        raise ValidationError("bootstrap evidence release disagrees with bootstrap.seed.releaseTag")

    target_chain_evidence = manifest["evidence"]["targetChains"]
    evidence_profiles = [entry["profile"] for entry in target_chain_evidence]
    recovered_profiles = [target["profile"] for target in recovered_targets]
    if evidence_profiles != recovered_profiles:
        raise ValidationError(
            "target-chain evidence must exactly cover targets recovered outside the seed manifest"
        )
    if len(evidence_profiles) != len(set(evidence_profiles)):
        raise ValidationError("target-chain evidence profiles must be unique")
    for entry in target_chain_evidence:
        evidence_path = _repository_path(root, entry["result"])
        _require_text(
            evidence_path,
            [
                entry["profile"],
                entry["nativeRevision"],
                entry["workflow"],
            ],
            f"target-chain evidence for {entry['profile']}",
        )


def validate_manifest_data(
    root: Path,
    manifest: dict[str, Any],
    schema: dict[str, Any],
    reference_trb: Path | None = None,
) -> None:
    validate_schema(manifest, schema)
    validate_repository_values(root, manifest, reference_trb)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    default_root = Path(__file__).resolve().parent.parent
    parser.add_argument("--root", type=Path, default=default_root)
    parser.add_argument("--manifest", type=Path)
    parser.add_argument("--schema", type=Path)
    parser.add_argument("--reference-trb", type=Path)
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    root = arguments.root.resolve()
    manifest_path = arguments.manifest or root / "compatibility/current.json"
    schema_path = arguments.schema or root / "compatibility/schema-v3.json"
    try:
        manifest = load_json_strict(manifest_path)
        schema = load_json_strict(schema_path)
        if not isinstance(manifest, dict) or not isinstance(schema, dict):
            raise ValidationError("manifest and schema roots must be JSON objects")
        validate_manifest_data(root, manifest, schema, arguments.reference_trb)
    except ValidationError as error:
        print(f"compatibility-manifest: {error}", file=sys.stderr)
        return 1
    print(
        "compatibility-manifest: valid "
        f"Native {manifest['nativeVersion']} -> "
        f"TypeRB {manifest['typeRB']['version']}@{manifest['typeRB']['revision']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
