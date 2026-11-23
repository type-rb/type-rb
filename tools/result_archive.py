"""Archive committed public evidence; keep compact, reviewable result indexes."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import tarfile

REPOSITORY = "https://github.com/type-rb/type-rb-native"
KEEP_SUFFIXES = {".md", ".csv", ".tsv", ".json", ".sha256"}
FORBIDDEN_SUFFIXES = {".raw", ".stripped", ".ssa", ".s", ".o", ".a", ".so",
                      ".dylib", ".exe", ".zip", ".gz", ".xz", ".zst",
                      ".stdout", ".stderr", ".status", ".rss-kib"}
ACTIVE_SLOTS = {"published-runtime", "published-build", "compatibility", "bootstrap-seed",
                "target-amd64", "persistent-memory", "accepted-compiler",
                "candidate", "comparison-baseline", "runtime-investigation"}


def lifecycle_errors(root, revision, entries):
    errors = []
    if len(entries) > 512 or sum(e[2] for e in entries.values()) > 4 * 1024**2:
        errors.append("Active results exceed the global 512-file / 4 MiB budget")
    try:
        record = entries["results/active.json"]
        manifest = json.loads(git(root, "cat-file", "blob", record[1]))
        if set(manifest) != {"schemaVersion", "entries"} or manifest["schemaVersion"] != 1:
            raise ValueError("Invalid active result registry")
        slots, directories = set(), set()
        for item in manifest["entries"]:
            if (set(item) != {"slot", "directory", "reason", "retireWhen"} or
                    item["slot"] not in ACTIVE_SLOTS or item["slot"] in slots or
                    not re.fullmatch(r"[a-z0-9][a-z0-9-]+", item["directory"]) or
                    item["directory"] in directories or
                    any(not isinstance(item[k], str) or not item[k].strip() for k in ("reason", "retireWhen"))):
                raise ValueError("Invalid, duplicated or unexplained active result")
            slots.add(item["slot"])
            directories.add(item["directory"])
            if f'results/{item["directory"]}/README.md' not in entries:
                raise ValueError("Active result needs a retained README")
        actual = {p.split('/')[1] for p in entries if len(p.split('/')) >= 3}
        if actual != directories:
            errors.append("Every result directory must occupy one active slot; remove superseded directories")
        if any(len(p.split('/')) == 2 and p not in ("results/README.md", "results/active.json") for p in entries):
            errors.append("Unregistered top-level result file")
    except (KeyError, TypeError, ValueError):
        errors.append("Missing or invalid results/active.json lifecycle registry")
    return errors


def result_reference_errors(root, revision, entries):
    """Reject dangling dated result references; pinned history is checked in Git."""
    sources = [p for p in git(root, 'ls-tree', '-rz', '--name-only', revision).decode().split('\0')
               if PurePosixPath(p).suffix in {'.md', '.js', '.mjs', '.json', '.jsonc', '.py',
                                            '.trb', '.sh', '.yml', '.yaml', '.html', '.toml'}]
    if not sources:
        return []
    try:
        lines = git(root, 'grep', '-I', '-h', '-E', 'results/[0-9]{4}-', revision, '--',
                    *sources).decode()
    except subprocess.CalledProcessError as error:
        if error.returncode == 1:
            return []
        raise
    pattern = (r'(?:https://github\.com/type-rb/type-rb-native/(?:blob|tree)/([a-f0-9]{7,40})/)?'
               r'results/([0-9]{4}-[A-Za-z0-9_-]+)')
    errors = []
    directories = {p.split('/')[1] for p in entries if len(p.split('/')) >= 3}
    for pinned, directory in sorted(set(re.findall(pattern, lines))):
        if pinned:
            try:
                git(root, 'cat-file', '-e', f'{pinned}:results/{directory}')
            except subprocess.CalledProcessError:
                errors.append(f'Historical result reference is unavailable: {pinned}/{directory}')
        elif directory not in directories:
            errors.append(f'Retired result reference must use an exact historical revision: {directory}')
    return errors


def git(root, *args, data=None):
    return subprocess.check_output(["git", "-C", str(root), "--literal-pathspecs", *args], input=data)


def tree(root, revision):
    result = {}
    for row in git(root, "ls-tree", "-rlz", revision, "--", "results").split(b"\0"):
        if row:
            fields, name = row.split(b"\t", 1)
            mode, kind, oid, size = fields.decode().split()
            if kind != "blob" or mode not in ("100644", "100755"):
                raise ValueError("Evidence must contain only regular Git files")
            result[name.decode()] = (mode, oid, int(size))
    return result


def safe_path(name):
    path = PurePosixPath(name)
    if (not name.startswith("results/") or len(path.parts) < 3 or
            any(part in ("", ".", "..") for part in name.split("/")) or
            "\\" in name or "\0" in name or path.is_absolute()):
        raise ValueError("Unsafe evidence path")
    return path


def digest_file(path):
    with open(path, "rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def add_file(archive, name, content, mode=0o644):
    info = tarfile.TarInfo(name)
    info.size, info.mode, info.mtime = len(content), mode, 0
    archive.addfile(info, io.BytesIO(content))


def committed_bytes(root, name, entry):
    root = root.resolve()
    path = root / name
    if (path.is_symlink() or not path.is_file() or
            any(parent.is_symlink() for parent in path.parents if parent.is_relative_to(root)) or
            not path.resolve().is_relative_to(root)):
        raise ValueError(f"Not a regular evidence file: {name}")
    content = path.read_bytes()
    blob = hashlib.sha1(b"blob " + str(len(content)).encode() + b"\0" + content).hexdigest()
    if len(content) != entry[2] or blob != entry[1]:
        raise ValueError(f"Evidence differs from the recorded Git revision: {name}")
    return content


def pack(root, output, names):
    if output.resolve().is_relative_to(root.resolve()):
        raise ValueError("Write archives outside the repository")
    if len(set(names)) != len(names) or not names:
        raise ValueError("Choose distinct result directory names")
    for name in names:
        if not re.fullmatch(r"[a-z0-9][a-z0-9-]+", name):
            raise ValueError("Choose an exact result directory name")
    revision = git(root, "rev-parse", "HEAD").decode().strip()
    entries = {name: entry for name, entry in tree(root, revision).items()
               if name.split("/")[1] in names}
    if set(name.split("/")[1] for name in entries) != set(names):
        raise ValueError("A selected result is absent from Git")
    records = []
    for name, entry in sorted(entries.items()):
        safe_path(name)
        content = committed_bytes(root, name, entry)
        records.append(dict(path=name, bytes=len(content), sha256=hashlib.sha256(content).hexdigest(),
                            mode=int(entry[0], 8) & 0o777))
    manifest = dict(schemaVersion=1, sourceRevision=revision, results=sorted(names), files=records)
    with output.open("xb") as target:
        with gzip.GzipFile(filename="", mode="wb", fileobj=target, mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w|") as archive:
                add_file(archive, "MANIFEST.json", json.dumps(manifest, sort_keys=True).encode() + b"\n")
                for record in records:
                    content = committed_bytes(root, record["path"], entries[record["path"]])
                    if hashlib.sha256(content).hexdigest() != record["sha256"]:
                        raise ValueError("Evidence changed during archival")
                    add_file(archive, record["path"], content, record["mode"])
    return dict(sha256=digest_file(output), archiveBytes=output.stat().st_size,
                files=len(records), sourceBytes=sum(r["bytes"] for r in records),
                sourceRevision=revision, results=sorted(names))


def verify(archive_path, expected):
    if not re.fullmatch(r"[a-f0-9]{64}", expected) or digest_file(archive_path) != expected:
        raise ValueError("Archive SHA-256 differs")
    with tarfile.open(archive_path, "r|gz") as archive:
        first = archive.next()
        if first is None or first.name != "MANIFEST.json" or not first.isfile() or first.size > 32 * 1024**2:
            raise ValueError("Missing or excessive archive manifest")
        manifest = json.load(archive.extractfile(first))
        if (set(manifest) != {"schemaVersion", "sourceRevision", "results", "files"} or
                manifest["schemaVersion"] != 1 or
                not re.fullmatch(r"[a-f0-9]{40}", manifest["sourceRevision"])):
            raise ValueError("Invalid archive manifest")
        records = {}
        for record in manifest["files"]:
            safe_path(record["path"])
            if record["path"] in records or record["mode"] not in (0o644, 0o755):
                raise ValueError("Duplicate path or invalid mode")
            records[record["path"]] = record
        if sorted(set(p.split("/")[1] for p in records)) != manifest["results"]:
            raise ValueError("Result inventory differs")
        seen = set()
        while (member := archive.next()) is not None:
            if member.name not in records or member.name in seen or not member.isfile():
                raise ValueError("Unexpected, duplicate or non-regular archive member")
            record = records[member.name]
            if member.size != record["bytes"] or member.mode != record["mode"]:
                raise ValueError("Archive size or mode differs")
            if hashlib.file_digest(archive.extractfile(member), "sha256").hexdigest() != record["sha256"]:
                raise ValueError("Archive member SHA-256 differs")
            seen.add(member.name)
        if seen != set(records):
            raise ValueError("Incomplete archive")
    return manifest


def keep(name):
    return PurePosixPath(name).suffix in KEEP_SUFFIXES or name.endswith("SHA256SUMS")


def compact(root, archive, checksum, url):
    if not re.fullmatch(re.escape(REPOSITORY) + r"/releases/download/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+\.tar\.gz", url):
        raise ValueError("Use a public evidence release asset URL in this repository")
    manifest = verify(archive, checksum)
    current = tree(root, "HEAD")
    selected = {p for p in current if p.split("/")[1] in manifest["results"]}
    if selected != {r["path"] for r in manifest["files"]}:
        raise ValueError("Current result inventory differs; do not compact it")
    for record in manifest["files"]:
        data = committed_bytes(root, record["path"], current[record["path"]])
        if hashlib.sha256(data).hexdigest() != record["sha256"]:
            raise ValueError("Current evidence differs from archive")
    for name in manifest["results"]:
        if (root / "results" / name / "ARCHIVE.json").exists():
            raise ValueError("An archive record already exists")
        if f"results/{name}/README.md" not in selected:
            raise ValueError("Each compacted result needs an existing README")
    removed = [r for r in manifest["files"] if not keep(r["path"])]
    if removed:
        git(root, "rm", "--quiet", "--pathspec-from-file=-", "--pathspec-file-nul",
            data=b"".join(r["path"].encode() + b"\0" for r in removed))
    for name in manifest["results"]:
        directory = root / "results" / name
        record = dict(schemaVersion=1, url=url, sha256=checksum, archiveBytes=archive.stat().st_size,
                      sourceRevision=manifest["sourceRevision"], memberPrefix=f"results/{name}/",
                      files=sum(r["path"].startswith(f"results/{name}/") for r in manifest["files"]))
        (directory / "ARCHIVE.json").write_text(json.dumps(record, indent=2) + "\n")
        readme = directory / "README.md"
        notice = ("> Storage update: detailed files are preserved in the verified public archive\n"
                  "> identified by [ARCHIVE.json](ARCHIVE.json). Tables and measurements remain in Git.\n"
                  "> Original reports and every archived file are included unchanged in that archive.\n\n")
        readme.write_bytes(notice.encode() + readme.read_bytes())
    return dict(removedFiles=len(removed), removedBytes=sum(r["bytes"] for r in removed))


def check(root, base, head):
    old, new = tree(root, base), tree(root, head)
    errors = lifecycle_errors(root, head, new) + result_reference_errors(root, head, new)
    for name, entry in new.items():
        if old.get(name) == entry:
            continue
        if entry[2] == 0 or entry[2] > 256 * 1024 or PurePosixPath(name).suffix in FORBIDDEN_SUFFIXES:
            errors.append(f"Archive detailed/empty/large evidence instead: {name}")
        elif b"\0" in git(root, "cat-file", "blob", entry[1]):
            errors.append(f"Archive binary evidence instead: {name}")
    for result in {p.split('/')[1] for p in new if len(p.split('/')) >= 3}:
        prefix = f"results/{result}/"
        before = [e[2] for p, e in old.items() if p.startswith(prefix)]
        after = [e[2] for p, e in new.items() if p.startswith(prefix)]
        if len(after) > max(100, len(before)) or sum(after) > max(2 * 1024**2, sum(before)):
            errors.append(f"Result exceeds 100 files / 2 MiB or grows an existing oversized result: {result}")
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    sub = parser.add_subparsers(dest="command", required=True)
    p = sub.add_parser("pack")
    p.add_argument("output", type=Path)
    p.add_argument("results", nargs="+")
    p = sub.add_parser("verify")
    p.add_argument("archive", type=Path)
    p.add_argument("sha256")
    p = sub.add_parser("compact", help="Use only a freshly downloaded, verified published copy")
    p.add_argument("archive", type=Path)
    p.add_argument("sha256")
    p.add_argument("url")
    p = sub.add_parser("check")
    p.add_argument("base")
    p.add_argument("head", nargs="?", default="HEAD")
    args = parser.parse_args()
    if args.command == "pack":
        result = pack(args.root, args.output, args.results)
    elif args.command == "verify":
        manifest = verify(args.archive, args.sha256)
        result = dict(verified=True, files=len(manifest["files"]), sourceRevision=manifest["sourceRevision"])
    elif args.command == "compact":
        result = compact(args.root, args.archive, args.sha256, args.url)
    else:
        result = check(args.root, args.base, args.head)
    print(json.dumps(result, indent=2))
    if args.command == "check" and result:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
