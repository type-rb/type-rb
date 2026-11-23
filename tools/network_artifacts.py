#!/usr/bin/env python3
"""Package, verify and inspect pinned Native dependency artifacts without CMake."""
import argparse
import hashlib
import gzip
import json
from pathlib import Path
import platform
import tarfile
import tempfile

PINS = Path(__file__).with_name("network-dependencies.json")
MARKER = b"TRBN-NETWORK-V1\0"
TARGETS = {"darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"}
FILES = {"libuv.a", "libllhttp.a", "network-identity.o", "libuv-LICENSE", "llhttp-LICENSE"}
MAX_BYTES = 128 * 1024 * 1024


def artifact_spec(family):
    if family == "oidc":
        return (Path(__file__).with_name("oidc-dependencies.json"),
                {"libcurl.a", "libssl.a", "libz.a", "libnghttp2.a", "libicuuc.a", "libicu-uts46.a",
                 "oidc-identity.o", "curl-LICENSE", "zlib-LICENSE", "openssl-LICENSE", "nghttp2-LICENSE", "icu-LICENSE"},
                b"TRBN-OIDC-V1\0", ("libcurl", "libssl", "libz", "libnghttp2", "libicuuc", "libicu-uts46"))
    if family == "crypto":
        return (Path(__file__).with_name("crypto-dependencies.json"),
                {"libcrypto.a", "crypto-identity.o", "openssl-LICENSE"},
                b"TRBN-CRYPTO-V1\0", ("libcrypto",))
    if family == "network":
        return PINS, FILES, MARKER, ("libuv", "libllhttp")
    if family == "compression":
        return (Path(__file__).with_name("compression-dependencies.json"),
                {"libdeflate.a", "compression-identity.o", "libdeflate-LICENSE"},
                b"TRBN-COMPRESSION-V1\0", ("libdeflate",))
    if family == "sqlite":
        return (Path(__file__).with_name("sqlite-dependencies.json"),
                {"libsqlite3.a", "sqlite-identity.o", "sqlite-PUBLIC-DOMAIN"},
                b"TRBN-SQLITE-V1\0", ("libsqlite3",))
    raise ValueError("unknown dependency family")


def target_name():
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system(), "unknown")
    machine = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}.get(platform.machine(), "unknown")
    target = system + "-" + machine
    if target not in TARGETS:
        raise ValueError("unsupported network artifact host")
    return target


def identity(receipt):
    return {key: receipt[key] for key in ("schema", "target", "linkage", "pins", "libraries",
                                        "configuration", "cc", "cmake", "builder_sha256", "artifact_tool_sha256")}


def embed_identity(receipt, assembly, family="network"):
    """Emit data only; all repository-owned runtime behavior remains TypeRB."""
    marker = artifact_spec(family)[2]
    payload = marker + json.dumps(identity(receipt), sort_keys=True, separators=(",", ":")).encode() + b"\0"
    darwin = receipt["target"].startswith("darwin-")
    symbol = ("_" if darwin else "") + "trbn_" + family + "_build_identity"
    lines = ['.section __TEXT,__const' if darwin else '.section .rodata',
             '.globl ' + symbol, symbol + ':']
    for offset in range(0, len(payload), 64):
        lines.append('.byte ' + ','.join(str(byte) for byte in payload[offset:offset + 64]))
    if family in ('network', 'oidc'):
        platform_symbol = ('_' if darwin else '') + 'trbn_' + family + '_darwin'
        lines.extend(['.p2align 3', '.globl ' + platform_symbol, platform_symbol + ':', '.quad ' + ('1' if darwin else '0')])
    if not darwin:
        lines.append('.section .note.GNU-stack,"",@progbits')
    assembly.write_text('\n'.join(lines) + '\n')


def inspect(path, family="network"):
    pins, _, marker, _ = artifact_spec(family)
    if path.stat().st_size > MAX_BYTES:
        raise ValueError("oversized executable")
    data = path.read_bytes()
    if data.count(marker) != 1:
        raise ValueError("expected one embedded dependency identity")
    raw = data.split(marker, 1)[1].split(b"\0", 1)[0]
    if len(raw) > 65536:
        raise ValueError("oversized dependency identity")
    value = json.loads(raw)
    if value.get("pins") != json.loads(pins.read_text()) or value.get("target") not in TARGETS:
        raise ValueError("embedded dependency pins or target differ")
    return value


def verify(directory, target, family="network"):
    pins, files, _, libraries = artifact_spec(family)
    receipt_name = family + "-build.json"
    receipt = json.loads((directory / receipt_name).read_text())
    if receipt.get("schema") != 2 or receipt.get("linkage") != "static" or receipt.get("target") != target:
        raise ValueError("dependency schema, linkage or target differs")
    if receipt.get("pins") != json.loads(pins.read_text()):
        raise ValueError("dependency source pins differ")
    if set(receipt.get("files", {})) != files:
        raise ValueError("dependency file inventory differs")
    if {p.name for p in directory.iterdir()} != files | {receipt_name}:
        raise ValueError("unexpected dependency installation files")
    total = 0
    for name, entry in receipt["files"].items():
        path = directory / name
        if path.is_symlink() or not path.is_file():
            raise ValueError("dependency must be an ordinary file: " + name)
        total += path.stat().st_size
        if total > MAX_BYTES or path.stat().st_size != entry["bytes"]:
            raise ValueError("dependency size differs: " + name)
        if hashlib.sha256(path.read_bytes()).hexdigest() != entry["sha256"]:
            raise ValueError("dependency checksum differs: " + name)
    if inspect(directory / (family + "-identity.o"), family) != identity(receipt):
        raise ValueError("embedded dependency build differs")
    for library in libraries:
        if receipt["libraries"][library] != receipt["files"][library + ".a"]["sha256"]:
            raise ValueError("archive identity differs")
    return receipt


def pack(directory, output, family="network"):
    _, files, _, _ = artifact_spec(family)
    receipt_name = family + "-build.json"
    receipt = json.loads((directory / receipt_name).read_text())
    verify(directory, receipt["target"], family)
    # The archive hash is the installation trust input. Publication and signing
    # belong to the release workflow, not to this local packaging operation.
    with tarfile.open(output, "w:gz") as archive:
        for name in sorted(files | {receipt_name}):
            archive.add(directory / name, arcname=name, recursive=False)
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    output.with_name(output.name + ".sha256").write_text(digest + "  " + output.name + "\n")
    return digest


def install(archive, output, digest, target, family="network"):
    _, files, _, _ = artifact_spec(family)
    receipt_name = family + "-build.json"
    if output.exists():
        raise ValueError("installation destination already exists")
    if archive.stat().st_size > MAX_BYTES or hashlib.sha256(archive.read_bytes()).hexdigest() != digest:
        raise ValueError("artifact checksum or size differs")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="." + family + "-install-", dir=output.parent) as temporary:
        staging = Path(temporary) / family
        staging.mkdir()
        unpacked = Path(temporary) / 'contents.tar'
        # Bound decompression before tarfile parses PAX/long-name metadata too.
        total = 0
        with gzip.open(archive, 'rb') as compressed, unpacked.open('wb') as target_file:
            while True:
                chunk = compressed.read(min(1024 * 1024, MAX_BYTES - total + 1))
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_BYTES:
                    raise ValueError('oversized expanded artifact')
                target_file.write(chunk)
        with tarfile.open(unpacked, 'r:') as source:
            names = set()
            total = 0
            for member in source.getmembers():
                if not member.isfile() or member.name not in files | {receipt_name} or member.name in names:
                    raise ValueError("unsafe or duplicate artifact entry")
                names.add(member.name)
                total += member.size
                if total > MAX_BYTES:
                    raise ValueError("oversized artifact contents")
            if names != files | {receipt_name}:
                raise ValueError("incomplete artifact")
            source.extractall(staging, filter="data")
        verify(staging, target, family)
        # Publish a complete new directory; never replace a consumer's files.
        staging.rename(output)


def main(family="network"):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    check = sub.add_parser("verify")
    check.add_argument("directory", type=Path)
    check.add_argument("--target", choices=sorted(TARGETS), default=target_name())
    packing = sub.add_parser("pack")
    packing.add_argument("directory", type=Path)
    packing.add_argument("output", type=Path)
    installing = sub.add_parser("install")
    installing.add_argument("archive", type=Path)
    installing.add_argument("output", type=Path)
    installing.add_argument("--sha256", required=True)
    installing.add_argument("--target", choices=sorted(TARGETS), default=target_name())
    inspecting = sub.add_parser("inspect")
    inspecting.add_argument("executable", type=Path)
    args = parser.parse_args()
    if args.command == "verify":
        print(json.dumps(verify(args.directory, args.target, family), indent=2))
    elif args.command == "pack":
        print(pack(args.directory, args.output, family))
    elif args.command == "install":
        install(args.archive, args.output, args.sha256, args.target, family)
        print(args.output)
    else:
        print(json.dumps(inspect(args.executable, family), indent=2))


if __name__ == "__main__":
    main()
