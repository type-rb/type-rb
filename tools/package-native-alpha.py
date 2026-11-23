#!/usr/bin/env python3
"""Repackage both verified Native CLI artifacts under their release command."""
import argparse
from hashlib import file_digest
from pathlib import Path
import re
import tarfile
import tempfile


PLATFORMS = ("darwin-arm64", "linux-arm64")


def sha256(path):
    with path.open("rb") as payload:
        return file_digest(payload, "sha256").hexdigest()


def package(artifact, platform, version, revision, output):
    archive = f"trbn-{platform}.tar.gz"
    checksum = (artifact / f"{archive}.sha256").read_text().split()
    if checksum != [sha256(artifact / archive), archive]:
        raise ValueError(f"archive checksum does not match for {platform}")
    with tempfile.TemporaryDirectory() as stage:
        stage = Path(stage)
        with tarfile.open(artifact / archive, "r:gz") as payload:
            payload.extractall(stage, filter="data")
        identity = dict(line.split("=", 1) for line in (stage / "build-info.txt").read_text().splitlines())
        for field, expected in {
            "revision": revision,
            "version": version,
            "profile": platform + "-v0",
            "compiler_sha256": sha256(stage / "trbn"),
            "qbe_sha256": sha256(stage / "qbe"),
        }.items():
            if identity.get(field) != expected:
                raise ValueError(f"{platform} has unexpected {field}")
        (stage / "trbn").rename(stage / "trb")
        destination = output / f"trb_{version}_{platform.replace('-', '_')}.tar.gz"
        with tarfile.open(destination, "w:gz") as payload:
            for entry in sorted(stage.iterdir()):
                payload.add(entry, arcname=entry.name)
        return destination


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("revision")
    parser.add_argument("artifacts", type=Path)
    parser.add_argument("output", type=Path)
    arguments = parser.parse_args()
    if not re.fullmatch(r"0\.5\.0-alpha\.[1-9][0-9]*", arguments.version):
        parser.error("expected a v0.5.0 alpha version without the leading v")
    if not re.fullmatch(r"[a-f0-9]{40}", arguments.revision):
        parser.error("expected the full source revision")
    if arguments.output.exists():
        parser.error("the output directory already exists")
    arguments.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=arguments.output.parent) as stage:
        output = Path(stage) / "release"
        output.mkdir()
        paths = [
            package(
                arguments.artifacts / f"trbn-{platform}", platform, arguments.version,
                arguments.revision, output,
            )
            for platform in PLATFORMS
        ]
        (output / "checksums.txt").write_text(
            "".join(f"{sha256(path)}  {path.name}\n" for path in paths)
        )
        output.rename(arguments.output)


if __name__ == "__main__":
    main()
