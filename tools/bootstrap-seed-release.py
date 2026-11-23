#!/usr/bin/env python3
"""Strict packaging/verification for the registered previous-Native seed refresh.

This is a release observer, never a compiler or ordinary build stage. The v1
initial-root verifier remains separate and unchanged.
"""
import hashlib
import csv
import json
import math
from pathlib import Path
import re
import shutil
import statistics
import sys

TAG = "bootstrap-seed-2026-09-29-module-cycles"
MANIFEST = "type-rb-native-bootstrap-manifest-v2.json"
PREDECESSORS = {"bootstrap-seed-2026-09-07": {
    "releaseTag": "bootstrap-seed-2026-08-30",
    "nativeRevision": "0058818314977633c50393796ef9b9f8f1fda50f",
    "manifestSha256": "a46d8c789f661a96aa38b1d4b9fd9ee21e46ccb3f4f2303a4f4d43caae1701b0",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "ef438d13598c534766334b408a39715c56ff1b69db528910ebf7d90ec7720b65"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "b4307c244edc9e4da620f2a7c1b03a733e575da032efefae615f9edf75048a37"},
    ],
}, "bootstrap-seed-2026-09-08": {
    "releaseTag": "bootstrap-seed-2026-09-07",
    "nativeRevision": "1f7e8a110bbb2b13f0609709deb6fc8f09dc8b44",
    "manifestSha256": "330e59e08bb1173b5865e782a7c578a556942ba5174422434cc56ab4fedfeed5",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "b960d8720ad6bb256fb019d04cd6ab80e86870228bed16776bba9fbe78f5769f"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "ff3bc9a2409e91eba0e2ef5109bf72a10aa16d4f360bc32aceafc50baa96a580"},
    ],
}, "bootstrap-seed-2026-09-08-loop-transfers": {
    "releaseTag": "bootstrap-seed-2026-09-08",
    "nativeRevision": "f8c293f6f9683b0a29b7eee614fc4fff261d36b8",
    "manifestSha256": "6b92832b482e8b502045a71f7f267f2e3bb5b221cd2c52ce1c07e9a4405fe083",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "9a815fd3bdcfd24a082111814442ee11380d31532058024cc7d7564e203b0629"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "77e8e9df3b91cbbf7cb823044c0c79c23e63767c5986369f8d9a11e23773abf3"},
    ],
}, "bootstrap-seed-2026-09-08-boolean-arrays": {
    "releaseTag": "bootstrap-seed-2026-09-08-loop-transfers",
    "nativeRevision": "1d53ed0f5b9471335c913dd9d148ff3b9eb1b483",
    "manifestSha256": "1262b05399ff63f3890b126f34bcce2c8c92405149ce902d49a3eb3d979dab68",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "756413fe6daa7b2a286bd4ae2721099807bf2061122f6d84f0414ea851c31d8f"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "13ae8ba588768b1e1936d83d022b85341144a401687e1cea2afa839f0e1a1e48"},
    ],
}, "bootstrap-seed-2026-09-09-record-arrays": {
    "releaseTag": "bootstrap-seed-2026-09-08-boolean-arrays",
    "nativeRevision": "e99df93e81c36c765ead99526fe58b2c2a978ced",
    "manifestSha256": "faf866e10a4b1f3ca45fcf2c85db0efb01fbed9f02f44876ade0b4037f28da92",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "1c8fdd630b3e129b9e121abce34c6d6dbd928b844cbfc1cff5f20a53b6ccaec7"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "af5549d42390d3ac3ec74c646ddd37c5930b411f706e5cf2dfd4ca7258bb09fd"},
    ],
}, "bootstrap-seed-2026-09-10-hash": {
    "releaseTag": "bootstrap-seed-2026-09-09-record-arrays",
    "nativeRevision": "db48f64f6c3f6ce3fb904b2d28c95fa46522ebfc",
    "manifestSha256": "67a6d9eb4a5449404fc513e7865be431d09fd158b14ecaede81e035abdd313f7",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "a95b8758af7d45ccbd40100cf00e15f2783a381b66a253a21fe683c06968e9bd"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "b77758a43f9341880ee2d78602ac131d3e6580808136df36a2de60e6dd633560"},
    ],
}, "bootstrap-seed-2026-09-11-array-iteration": {
    "releaseTag": "bootstrap-seed-2026-09-10-hash",
    "nativeRevision": "79f699e9245f79131646ebf43207f6b7526c4f68",
    "manifestSha256": "0fc38b20b2f5a4e0bdd7f15c33b1bac3cb66406d72b54006fcb8a518e5f80ee0",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "13d2b494b5b4864f0f7c823a5c9e2c38850e865725ed8cef87fd487d1358185d"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "6bae5730fc543c9dc8609a19d4884064b8d363ca038fe75a5b74465eb8b3c7a5"},
    ],
}, "bootstrap-seed-2026-09-12-compiler-names": {
    "releaseTag": "bootstrap-seed-2026-09-11-array-iteration",
    "nativeRevision": "b4a1b383e5678907649334203f534ae62fa42af6",
    "manifestSha256": "3e3c71b9921a81d58224e67833957dcd281840dd80204cb114f54700332b39b0",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "7ba29a5897782366569be9b2be3461f59fcf3e8feccfe41694ae4a29c457d19a"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "8c9d2ea099880ebbce4f4e91230b1cfab40e3fce4c145adacba36718ddbc5852"},
    ],
}, "bootstrap-seed-2026-09-29-module-cycles": {
    "releaseTag": "bootstrap-seed-2026-09-12-compiler-names",
    "nativeRevision": "d7ffb9384229125216696a220c7f370422472178",
    "manifestSha256": "04a0242b06818641e72d65ccbe4ece79380d875768772e60500fa36f05559c83",
    "targets": [
        {"asset": "type-rb-native-bootstrap-darwin-arm64", "sha256": "28d1b5a3aa42013ea8876173dbf760f7e71ee221a69bf16c915e485ccf60bdd9"},
        {"asset": "type-rb-native-bootstrap-linux-arm64", "sha256": "40cf35282750792c60dee1efdbdf331681ee7b5dd7efb5fae7a551824db4221c"},
    ],
}}

# Published manifests retain their registered source-era size contracts.
LIMITS = {
    "bootstrap-seed-2026-09-07": (350000, 317000, 667000),
    "bootstrap-seed-2026-09-08": (350000, 317000, 667000),
    "bootstrap-seed-2026-09-08-loop-transfers": (350000, 317000, 667000),
    "bootstrap-seed-2026-09-08-boolean-arrays": (350000, 328000, 678000),
    "bootstrap-seed-2026-09-09-record-arrays": (366000, 328000, 694000),
    "bootstrap-seed-2026-09-10-hash": (400000, 370000, 770000),
    "bootstrap-seed-2026-09-11-array-iteration": (417000, 388000, 805000),
    # MIR migration records exact byte sizes without an integration ceiling.
    # Historical tags above keep their strict source-era contracts.
    "bootstrap-seed-2026-09-12-compiler-names": (None, None, None),
    "bootstrap-seed-2026-09-29-module-cycles": (None, None, None),
}

BACKEND = {"name": "QBE", "version": "1.3", "sourceSha256": "d587905d620dc5e1d2bfa7c2cc642b9b837aa89a3188c6e37b53d756cf66e320"}
TARGETS = [
    ("darwin-arm64", "darwin", "macos-15", "arm64_apple", 350000),
    ("linux-arm64", "linux", "ubuntu-24.04-arm", "arm64", 317000),
]
TARGET_KEYS = {"asset", "profile", "os", "architecture", "runnerImage", "qbeTarget", "ccBoundary", "mode", "size", "sha256", "attestationSubjectSha256", "qbeBinarySize", "qbeBinarySha256"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key")
        result[key] = value
    return result


def invalid_constant(_value):
    raise ValueError("non-finite JSON")


def read_json(path):
    return json.loads(Path(path).read_text(), object_pairs_hook=unique_object,
                      parse_constant=invalid_constant)


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def sha(value, width=64):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{%d}" % width, value) is not None


def size(value, maximum=None):
    return type(value) is int and value > 0 and (maximum is None or value <= maximum)


def validate_target(target, expected):
    name, os_name, runner, qbe_target, limit = expected
    require(isinstance(target, dict) and set(target) == TARGET_KEYS, "target keys differ")
    for key, value in {"asset": "type-rb-native-bootstrap-" + name,
                       "profile": name + "-v0", "os": os_name, "architecture": "arm64",
                       "runnerImage": runner, "qbeTarget": qbe_target,
                       "ccBoundary": "system-cc", "mode": "0755"}.items():
        require(target[key] == value, "target identity differs: " + key)
    require(size(target["size"], limit), "compiler size exceeds accepted target limit")
    require(sha(target["sha256"]) and target["attestationSubjectSha256"] == target["sha256"], "target digest differs")
    require(size(target["qbeBinarySize"]) and sha(target["qbeBinarySha256"]), "QBE binary identity invalid")


def validate_manifest(manifest, revision, tag=TAG):
    require(isinstance(tag, str) and tag in PREDECESSORS and tag in LIMITS, "unknown release tag")
    require(sha(revision, 40), "invalid source revision")
    require(isinstance(manifest, dict) and set(manifest) == {
        "schemaVersion", "status", "releaseTag", "nativeRevision", "predecessor", "backend", "targets"
    }, "manifest keys differ")
    require(type(manifest["schemaVersion"]) is int and manifest["schemaVersion"] == 2, "manifest version differs")
    require(manifest["status"] == "experimental" and manifest["releaseTag"] == tag, "release identity differs")
    require(manifest["nativeRevision"] == revision, "source revision differs")
    require(manifest["predecessor"] == PREDECESSORS[tag], "predecessor provenance differs")
    require(manifest["backend"] == BACKEND, "backend identity differs")
    require(isinstance(manifest["targets"], list) and len(manifest["targets"]) == 2, "target count differs")
    limits = LIMITS[tag]
    for index, (target, expected) in enumerate(zip(manifest["targets"], TARGETS)):
        validate_target(target, expected[:-1] + (limits[index],))
    if limits[2] is not None:
        require(sum(target["size"] for target in manifest["targets"]) <= limits[2], "combined size exceeds accepted limit")


def create(revision, inputs, output):
    inputs, output = Path(inputs), Path(output)
    manifest = {"schemaVersion": 2, "status": "experimental", "releaseTag": TAG,
                "nativeRevision": revision, "predecessor": PREDECESSORS[TAG],
                "backend": BACKEND, "targets": [read_json(inputs / (t[0] + ".json")) for t in TARGETS]}
    validate_manifest(manifest, revision)
    for target in manifest["targets"]:
        binary = inputs / target["asset"]
        require(binary.is_file() and binary.stat().st_size == target["size"] and digest(binary) == target["sha256"], "input compiler differs")
    output.mkdir()  # Never overwrite a prior package.
    for target in manifest["targets"]:
        binary = output / target["asset"]
        shutil.copyfile(inputs / target["asset"], binary)
        binary.chmod(0o755)
    (output / MANIFEST).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    names = [target["asset"] for target in manifest["targets"]] + [MANIFEST]
    (output / "SHA256SUMS").write_text("".join(digest(output / name) + "  " + name + "\n" for name in names))


def verify(tag, revision, asset, directory, release_path):
    directory = Path(directory)
    manifest = read_json(directory / MANIFEST)
    validate_manifest(manifest, revision, tag)
    targets = {target["asset"]: target for target in manifest["targets"]}
    require(asset in targets, "unknown target")
    target = targets[asset]
    binary = directory / asset
    require(binary.is_file() and binary.stat().st_size == target["size"] and digest(binary) == target["sha256"], "downloaded compiler differs")
    expected_sums = "".join(t["sha256"] + "  " + t["asset"] + "\n" for t in manifest["targets"])
    expected_sums += digest(directory / MANIFEST) + "  " + MANIFEST + "\n"
    require((directory / "SHA256SUMS").read_text() == expected_sums, "checksum index differs")
    release = read_json(release_path)
    require(release.get("tag_name") == tag and release.get("target_commitish") == revision,
            "published release revision differs")
    require(release.get("draft") is False and release.get("prerelease") is True and release.get("immutable") is True,
            "release is not an immutable prerelease")
    expected = {t["asset"]: (t["size"], t["sha256"]) for t in manifest["targets"]}
    for name in (MANIFEST, "SHA256SUMS"):
        expected[name] = ((directory / name).stat().st_size, digest(directory / name))
    assets = release.get("assets")
    require(isinstance(assets, list) and len(assets) == len(expected), "release asset count differs")
    require({a.get("name") for a in assets} == set(expected), "release asset names differ")
    for entry in assets:
        count, checksum = expected[entry["name"]]
        require(type(entry.get("size")) is int and entry["size"] == count and entry.get("digest") == "sha256:" + checksum,
                "release asset metadata differs")


def observations(path, role):
    require(role in ("ordinary", "transition"), "unknown measurement role")
    stages = ["b2-b3", "b3-b4"]
    if role == "ordinary":
        stages.insert(0, "b1-b2")
    with Path(path).open(newline="") as source:
        reader = csv.DictReader(source)
        require(reader.fieldnames == ["stage", "iteration", "elapsed_seconds", "peak_rss_bytes", "status", "cpu_seconds"], "measurement fields differ")
        rows = list(reader)
    require(len(rows) == 7 * len(stages), "measurement count differs")
    require({row["stage"] for row in rows} == set(stages), "measurement stages differ")
    for stage in stages:
        iterations = [row["iteration"] for row in rows if row["stage"] == stage]
        require(sorted(iterations) == list("1234567"), "missing or repeated observation")
    require(all(row["status"] == "0" for row in rows), "failed observation")
    for metric in ("elapsed_seconds", "peak_rss_bytes", "cpu_seconds"):
        values = [float(row[metric]) for row in rows]
        require(all(math.isfinite(value) and value > 0 for value in values), "invalid observation")
        medians = [statistics.median(float(row[metric]) for row in rows if row["stage"] == stage) for stage in stages]
        require(max(medians) <= min(medians) * 1.25, "adjacent median limit exceeded")
        require(max(values) <= min(medians) * 2, "retained catastrophic limit exceeded")


if __name__ == "__main__":
    try:
        command, *arguments = sys.argv[1:]
        if command == "create" and len(arguments) == 3:
            create(*arguments)
        elif command == "verify" and len(arguments) == 5:
            verify(*arguments)
        elif command == "observations" and len(arguments) == 2:
            observations(*arguments)
        else:
            raise ValueError("usage: bootstrap-seed-release.py create REV INPUTS OUTPUT | verify TAG REV ASSET DIRECTORY RELEASE_JSON | observations CSV ordinary|transition")
        print("bootstrap seed package " + command + " passed")
    except (ValueError, OSError, KeyError, TypeError) as error:
        sys.exit("bootstrap seed package: " + str(error))
