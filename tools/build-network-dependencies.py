#!/usr/bin/env python3
"""Build the pinned network libraries; no repository-owned C runtime is used."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import time

from network_artifacts import embed_identity, target_name

PINS = Path(__file__).with_name("network-dependencies.json")


def run(*args):
    subprocess.run([str(a) for a in args], check=True, env={**os.environ, "ZERO_AR_DATE": "1"})


def linked_libraries(path):
    if platform.system() == "Darwin":
        commands = subprocess.check_output(["otool", "-l", str(path)], text=True)
        return re.findall(r"cmd LC_LOAD_DYLIB\n\s+cmdsize \d+\n\s+name (\S+)", commands)
    commands = subprocess.check_output(["readelf", "-d", str(path)], text=True)
    return re.findall(r"\(NEEDED\).*?\[(.*?)\]", commands)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--archives", type=Path, required=True,
                        help="Shared, reusable source archive directory")
    parser.add_argument("--linkage", choices=("static", "dynamic"), default="static",
                        help="Static archives for applications; dynamic is a comparison/development profile")
    options = parser.parse_args()
    pins = json.loads(PINS.read_text())
    if platform.system() not in ("Darwin", "Linux"):
        parser.error("network runtime currently supports Darwin and Linux LP64")
    options.output.mkdir(parents=True, exist_ok=True)
    options.archives.mkdir(parents=True, exist_ok=True)
    output = options.output.resolve()
    cc = os.environ.get("CC", "cc")
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="trbn-network-") as temporary:
        temporary = Path(temporary)
        roots = {}
        for name in ("libuv", "llhttp"):
            pin = pins[name]
            archive = options.archives / f"{name}-{pin['version']}.tar.gz"
            if not archive.exists():
                # Read a bounded response and publish only after hash verification.
                with urllib.request.urlopen(pin["url"], timeout=60) as response:
                    data = response.read(10 * 1024 * 1024 + 1)
                if len(data) > 10 * 1024 * 1024:
                    raise ValueError(f"{name}: oversized archive")
            else:
                if archive.stat().st_size > 10 * 1024 * 1024:
                    raise ValueError(f"{name}: oversized cached archive")
                data = archive.read_bytes()
            if hashlib.sha256(data).hexdigest() != pin["sha256"]:
                raise ValueError(f"{name}: source checksum mismatch")
            if not archive.exists():
                archive.write_bytes(data)
            destination = temporary / name
            destination.mkdir()
            with tarfile.open(archive) as tar:
                for member in tar.getmembers():
                    path = Path(member.name)
                    if path.is_absolute() or ".." in path.parts or not (member.isdir() or member.isfile()):
                        raise ValueError(f"{name}: unsafe archive entry {member.name}")
                tar.extractall(destination, filter="data")
            roots[name] = destination / pin["root"]
        uv = roots["libuv"]
        http = roots["llhttp"]
        # Public opaque libuv sizing/accessor APIs; llhttp's pinned generated ABI
        # is checked independently rather than assuming a platform's layout.
        probe = temporary / "abi.c"
        probe.write_text('''#include <stddef.h>
#include <uv.h>
#include <netdb.h>
#include <llhttp.h>
_Static_assert(offsetof(struct addrinfo, ai_socktype) == 8, "addrinfo socket kind");
_Static_assert(offsetof(struct addrinfo, ai_next) == 40, "addrinfo next");
#ifdef __APPLE__
_Static_assert(offsetof(struct addrinfo, ai_addr) == 32, "Darwin addrinfo address");
#else
_Static_assert(offsetof(struct addrinfo, ai_addr) == 24, "Linux addrinfo address");
#endif
_Static_assert(sizeof(void *) == 8, "LP64 pointers");
_Static_assert(sizeof(long) == 8, "LP64 long");
_Static_assert(sizeof(uv_buf_t) == 16, "uv buffer size");
_Static_assert(offsetof(uv_buf_t, base) == 0, "uv buffer base");
_Static_assert(offsetof(uv_buf_t, len) == 8, "uv buffer length");
_Static_assert(sizeof(llhttp_t) == 96, "parser size");
_Static_assert(offsetof(llhttp_t, data) == 48, "parser user data");
_Static_assert(offsetof(llhttp_t, upgrade) == 80, "upgrade flag");
_Static_assert(sizeof(llhttp_settings_t) == 200, "callback table");
_Static_assert(offsetof(llhttp_settings_t, on_url) == 16, "URL callback");
_Static_assert(offsetof(llhttp_settings_t, on_header_field) == 48, "header callback");
_Static_assert(offsetof(llhttp_settings_t, on_body) == 88, "body callback");
_Static_assert(offsetof(llhttp_settings_t, on_message_complete) == 96, "complete callback");
_Static_assert(UV_TCP == 12 && UV_SIGNAL == 16 && UV_TIMER == 13 && UV_WRITE == 3 && UV_GETADDRINFO == 8, "libuv enum ABI");
_Static_assert(HTTP_REQUEST == 1 && HPE_PAUSED == 21, "llhttp enum ABI");
int main(void) { return 0; }
''')
        run(cc, "-std=c11", "-D_POSIX_C_SOURCE=200809L", "-I" + str(uv / "include"), "-I" + str(http / "include"),
            "-c", probe, "-o", temporary / "abi.o")
        build = temporary / "uv-build"
        run("cmake", "-S", uv, "-B", build, "-DCMAKE_BUILD_TYPE=Release",
            f"-DCMAKE_C_FLAGS=-fPIC -ffile-prefix-map={temporary}=.",
            "-DLIBUV_BUILD_TESTS=OFF", "-DLIBUV_BUILD_BENCH=OFF", "-DBUILD_TESTING=OFF")
        run("cmake", "--build", build, "--target", "uv_a" if options.linkage == "static" else "uv", "--parallel", "2")
        suffix = "dylib" if platform.system() == "Darwin" else "so"
        publication = temporary / "publication"
        publication.mkdir()
        link_flags = ["-dynamiclib", "-Wl,-install_name,@rpath/libllhttp.dylib"] if suffix == "dylib" else ["-shared", "-Wl,-soname,libllhttp.so"]
        if options.linkage == "static":
            suffix = "a"
            link_flags = []
            objects = []
            for source in ("api", "http", "llhttp"):
                obj = temporary / f"{source}.o"
                run(cc, "-O2", "-fPIC", f"-ffile-prefix-map={temporary}=.", "-I" + str(http / "include"),
                    "-c", http / f"src/{source}.c", "-o", obj)
                objects.append(obj)
            run(os.environ.get("AR", "ar"), "rcs", publication / "libllhttp.a", *objects)
        else:
            run(cc, "-O2", "-fPIC", *link_flags, "-I" + str(http / "include"),
                http / "src/api.c", http / "src/http.c", http / "src/llhttp.c",
                "-o", publication / f"libllhttp.{suffix}")
        shutil.copy2(build / f"libuv.{suffix}", publication / f"libuv.{suffix}", follow_symlinks=True)
        for name, root in roots.items():
            shutil.copy2(root / "LICENSE", publication / f"{name}-LICENSE")
        receipt = {"schema": 2, "linkage": options.linkage, "target": target_name(),
                   "pins": pins, "system": platform.system(), "machine": platform.machine(),
                   "cc": subprocess.check_output([cc, "--version"], text=True).splitlines()[0],
                   "cmake": subprocess.check_output(["cmake", "--version"], text=True).splitlines()[0],
                   "builder_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                   "artifact_tool_sha256": hashlib.sha256(Path(__file__).with_name("network_artifacts.py").read_bytes()).hexdigest(),
                   "configuration": {"archive_timestamps": "ZERO_AR_DATE=1; deterministic ar default where available",
                                     "source_prefix": "temporary source/build root mapped to . for static archives",
                                     "libuv": f"Release; target {'uv_a' if options.linkage == 'static' else 'uv'}; tests and benchmarks disabled",
                                     "llhttp": ["-O2", "-fPIC", *link_flags],
                                     "environment": {name: os.environ[name] for name in
                                                     ("CFLAGS", "CPPFLAGS", "LDFLAGS", "MACOSX_DEPLOYMENT_TARGET")
                                                     if name in os.environ}},
                   "linked_libraries": {} if options.linkage == "static" else
                                       {name: linked_libraries(publication / f"{name}.{suffix}")
                                        for name in ("libuv", "libllhttp")},
                   "libraries": {name: hashlib.sha256(path.read_bytes()).hexdigest()
                                 for name in ("libuv", "libllhttp")
                                 for path in [publication / f"{name}.{suffix}"]}}
        if options.linkage == "static":
            embed_identity(receipt, temporary / "identity.s")
            run(cc, "-c", temporary / "identity.s", "-o", publication / "network-identity.o")
        receipt["files"] = {path.name: {"sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                                      "bytes": path.stat().st_size}
                            for path in sorted(publication.iterdir())}
        receipt["build_seconds"] = time.monotonic() - started
        # Replace complete files, never overwrite a library mapped by a running
        # application. Build failure leaves the previous installation intact.
        for path in publication.iterdir():
            with tempfile.NamedTemporaryFile(dir=output, prefix=".network-", delete=False) as file:
                pending = Path(file.name)
            try:
                shutil.copy2(path, pending)
                pending.replace(output / path.name)
            finally:
                pending.unlink(missing_ok=True)
        receipt_path = output / "network-build.json"
        with tempfile.NamedTemporaryFile(mode="w", dir=output, prefix=".receipt-", delete=False) as file:
            json.dump(receipt, file, indent=2)
            file.write("\n")
            pending_receipt = Path(file.name)
        try:
            pending_receipt.replace(receipt_path)
        finally:
            pending_receipt.unlink(missing_ok=True)
    print(output)


if __name__ == "__main__":
    main()
