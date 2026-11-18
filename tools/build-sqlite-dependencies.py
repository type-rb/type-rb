#!/usr/bin/env python3
"""Build a static SQLite bundle from the hash-verified amalgamation."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
from urllib.request import urlopen
from zipfile import ZipFile

from network_artifacts import embed_identity, inspect, target_name, verify


PINS = Path(__file__).with_name("sqlite-dependencies.json")
OPTIONS = (
    "HAVE_USLEEP",
    "LONGDOUBLE_TYPE=double",
    "NDEBUG",
    "SQLITE_DEFAULT_MEMSTATUS=0",
    "SQLITE_DISABLE_INTRINSIC",
    "SQLITE_ENABLE_COLUMN_METADATA",
    "SQLITE_ENABLE_DBPAGE_VTAB",
    "SQLITE_ENABLE_DBSTAT_VTAB",
    "SQLITE_ENABLE_FTS5",
    "SQLITE_ENABLE_GEOPOLY",
    "SQLITE_ENABLE_MATH_FUNCTIONS",
    "SQLITE_ENABLE_MEMORY_MANAGEMENT",
    "SQLITE_ENABLE_OFFSET_SQL_FUNC",
    "SQLITE_ENABLE_PREUPDATE_HOOK",
    "SQLITE_ENABLE_RBU",
    "SQLITE_ENABLE_RTREE",
    "SQLITE_ENABLE_SESSION",
    "SQLITE_ENABLE_SNAPSHOT",
    "SQLITE_ENABLE_STAT4",
    "SQLITE_ENABLE_UNLOCK_NOTIFY",
    "SQLITE_HAVE_ZLIB=1",
    "SQLITE_LIKE_DOESNT_MATCH_BLOBS",
    "SQLITE_SOUNDEX",
    "SQLITE_THREADSAFE=1",
    "SQLITE_WITHOUT_ZONEMALLOC",
)

PROBE = r"""
#include <sqlite3.h>
#include <stdint.h>
#include <stdio.h>

_Static_assert(sizeof(void *) == 8, "LP64 pointer");
_Static_assert(sizeof(int) == 4, "C int width");
_Static_assert(sizeof(sqlite3_int64) == 8, "SQLite integer width");
_Static_assert(sizeof(double) == 8, "C double width");

extern const unsigned char trbn_sqlite_build_identity[];

int main(void) {
    sqlite3 *database = NULL;
    sqlite3_stmt *statement = NULL;
    if (trbn_sqlite_build_identity[0] != 'T' ||
        sqlite3_open(":memory:", &database) != SQLITE_OK ||
        sqlite3_prepare_v2(database, "SELECT ?1", -1, &statement, NULL) != SQLITE_OK ||
        sqlite3_bind_int64(statement, 1, INT64_C(4294967297)) != SQLITE_OK ||
        sqlite3_step(statement) != SQLITE_ROW ||
        sqlite3_column_int64(statement, 0) != INT64_C(4294967297) ||
        sqlite3_finalize(statement) != SQLITE_OK ||
        sqlite3_close(database) != SQLITE_OK) {
        return 1;
    }

    puts(sqlite3_libversion());
    puts(sqlite3_sourceid());
    for (int index = 0; sqlite3_compileoption_get(index) != NULL; ++index) {
        puts(sqlite3_compileoption_get(index));
    }
    return 0;
}
"""


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def unpack_source(pin, archive, root):
    maximum = pin["archiveBytes"] + 1
    if archive is None:
        with urlopen(pin["url"], timeout=30) as stream:
            data = stream.read(maximum)
    else:
        with archive.open("rb") as stream:
            data = stream.read(maximum)
    if len(data) != pin["archiveBytes"] or hashlib.sha256(data).hexdigest() != pin["sha256"]:
        raise ValueError("SQLite source archive size or SHA-256 differs")
    with ZipFile(io.BytesIO(data)) as package:
        for name in ("sqlite3.c", "sqlite3.h"):
            info = package.getinfo(pin["root"] + "/" + name)
            if info.file_size > 16 * 1024 * 1024:
                raise ValueError("oversized SQLite source member")
            (root / name).write_bytes(package.read(info))
    if hashlib.sha3_256((root / "sqlite3.c").read_bytes()).hexdigest() != pin["sourceSHA3_256"]:
        raise ValueError("SQLite amalgamation SHA3-256 differs")


def run(*args):
    subprocess.run(list(map(str, args)), check=True, timeout=180,
                   env=dict(os.environ, ZERO_AR_DATE="1"))


def build(output, archive):
    output = output.resolve()
    if output.exists():
        raise ValueError("choose a new SQLite bundle destination")
    output.parent.mkdir(parents=True, exist_ok=True)
    target = target_name()
    pins = json.loads(PINS.read_text())
    pin = pins["sqlite"]
    cc = os.environ.get("CC", "cc")
    ar = os.environ.get("AR", "ar")
    system_flags = ["-lm"] if target.startswith("darwin-") else ["-lm", "-pthread", "-ldl"]
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix=".sqlite-build-", dir=output.parent) as temporary:
        root = Path(temporary)
        unpack_source(pin, archive, root)
        publication = root / "publication"
        publication.mkdir()
        library = publication / "libsqlite3.a"
        run(cc, "-O2", "-fPIC", "-ffile-prefix-map=" + str(root) + "=.",
            *("-D" + option for option in OPTIONS), "-c", root / "sqlite3.c", "-o", root / "sqlite3.o")
        run(ar, "rcs", library, root / "sqlite3.o")
        notice = publication / "sqlite-PUBLIC-DOMAIN"
        notice.write_text("SQLite is in the public domain.\n" + pin["licenseURL"] + "\n")
        receipt = {
            "schema": 2, "target": target, "linkage": "static", "pins": pins,
            "libraries": {"libsqlite3": digest(library)},
            "configuration": {"defines": list(OPTIONS), "flags": ["-O2", "-fPIC", "-ffile-prefix-map"],
                              "systemLinkFlags": system_flags},
            "cc": subprocess.check_output([cc, "--version"], text=True, timeout=10).splitlines()[0],
            "cmake": None,
            "builder_sha256": digest(Path(__file__)),
            "artifact_tool_sha256": digest(Path(__file__).with_name("network_artifacts.py")),
        }
        assembly = root / "identity.s"
        identity = publication / "sqlite-identity.o"
        embed_identity(receipt, assembly, "sqlite")
        run(cc, "-c", assembly, "-o", identity)
        probe = root / "probe.c"
        probe.write_text(PROBE)
        executable = root / "probe"
        run(cc, "-I", root, probe, identity, library, *system_flags, "-o", executable)
        version, source_id, *options = subprocess.check_output([str(executable)], text=True, timeout=10).splitlines()
        if version != pin["version"] or source_id != pin["sourceID"]:
            raise ValueError("linked SQLite identity differs")
        inspect(executable, "sqlite")
        receipt["observedCompileOptions"] = sorted(options)
        receipt["files"] = {
            path.name: {"bytes": path.stat().st_size, "sha256": digest(path)}
            for path in sorted(publication.iterdir())
        }
        receipt["build_seconds"] = time.monotonic() - started
        (publication / "sqlite-build.json").write_text(json.dumps(receipt, indent=2) + "\n")
        verify(publication, target, "sqlite")
        publication.rename(output)
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=Path, help="Reuse an existing pinned source ZIP")
    parser.add_argument("--output", type=Path, required=True)
    arguments = parser.parse_args()
    print(build(arguments.output, arguments.archive))


if __name__ == "__main__":
    main()
