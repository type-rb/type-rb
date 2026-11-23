"""Store evidence files losslessly in one compressed JSON-lines file per CI job."""
import argparse
import base64
import gzip
import hashlib
import json
from pathlib import Path, PurePosixPath
import re


def safe_path(name):
    if (not isinstance(name, str) or not name or '\\' in name or '\0' in name or
            PurePosixPath(name).is_absolute() or
            any(part in ('', '.', '..') for part in name.split('/'))):
        raise ValueError('Unsafe evidence path')
    return name


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def line(record):
    return (json.dumps(record, sort_keys=True, separators=(',', ':')) + '\n').encode()


def pack(root, output, selections):
    root = root.resolve()
    if not selections or len(set(selections)) != len(selections):
        raise ValueError('Select distinct evidence paths')
    paths, missing = {}, []
    for name in selections:
        safe_path(name)
        source = root / name
        if any(p.is_symlink() for p in [source, *source.parents] if p.is_relative_to(root)):
            raise ValueError('Evidence symlinks are unsupported')
        if not source.exists():
            missing.append(name)
            continue
        for path in ([source] if source.is_file() else sorted(source.rglob('*'))):
            if path.is_symlink():
                raise ValueError('Evidence symlinks are unsupported')
            if path.is_dir():
                continue
            if not path.is_file() or path.resolve() == output.resolve():
                raise ValueError('Select regular evidence files, excluding the output')
            relative = path.relative_to(root).as_posix()
            if any(part.startswith('.') for part in PurePosixPath(relative).parts):
                raise ValueError('Hidden files need explicit publication review')
            if relative in paths:
                raise ValueError('Overlapping evidence selections')
            paths[relative] = path
    if not paths:
        raise ValueError('No evidence to retain')
    # Exclusive creation prevents overwriting another cohort. A failed write is
    # removed so that CI can upload the original evidence as its failure fallback.
    with output.open('xb') as target:
        try:
            with gzip.GzipFile(filename='', fileobj=target, mode='wb', mtime=0) as stream:
                stream.write(line(dict(format='typerb-evidence-jsonl', schemaVersion=1,
                                       files=len(paths), missing=sorted(missing))))
                for name, path in sorted(paths.items()):
                    data = path.read_bytes()
                    stream.write(line(dict(path=name, mode=path.stat().st_mode & 0o777,
                                           bytes=len(data), sha256=sha256(data),
                                           base64=base64.b64encode(data).decode('ascii'))))
        except BaseException:
            output.unlink()
            raise
    return verify(output)


def records(bundle):
    with gzip.open(bundle, 'rb') as stream:
        header = json.loads(stream.readline())
        if (set(header) != {'format', 'schemaVersion', 'files', 'missing'} or
                header['format'] != 'typerb-evidence-jsonl' or header['schemaVersion'] != 1 or
                type(header['files']) is not int or header['files'] <= 0 or
                not isinstance(header['missing'], list)):
            raise ValueError('Invalid evidence header')
        missing = [safe_path(name) for name in header['missing']]
        if len(set(missing)) != len(missing):
            raise ValueError('Duplicate missing path')
        seen = set()
        for raw in stream:
            record = json.loads(raw)
            if set(record) != {'path', 'mode', 'bytes', 'sha256', 'base64'}:
                raise ValueError('Invalid evidence record')
            name = safe_path(record['path'])
            if (name in seen or type(record['mode']) is not int or
                    not 0 <= record['mode'] <= 0o777 or type(record['bytes']) is not int or
                    not isinstance(record['sha256'], str) or
                    not re.fullmatch(r'[0-9a-f]{64}', record['sha256'])):
                raise ValueError('Duplicate path or invalid metadata')
            if any(name == p or name.startswith(p + '/') for p in missing):
                raise ValueError('Recorded file conflicts with missing inventory')
            data = base64.b64decode(record['base64'], validate=True)
            if len(data) != record['bytes'] or sha256(data) != record['sha256']:
                raise ValueError('Evidence content differs from its size or SHA-256')
            seen.add(name)
            yield record, data
        if len(seen) != header['files']:
            raise ValueError('Incomplete evidence bundle')
        if any(str(parent) in seen for name in seen for parent in PurePosixPath(name).parents):
            raise ValueError('File path conflicts with a parent directory')


def verify(bundle):
    count = size = 0
    for record, data in records(bundle):
        count += 1
        size += len(data)
    return dict(files=count, sourceBytes=size, bundleBytes=bundle.stat().st_size,
                sha256=sha256(bundle.read_bytes()))


def extract(bundle, output):
    # Validate the complete cohort before creating an extraction directory.
    result = verify(bundle)
    output.mkdir()  # Must not exist, including as a symlink or empty directory.
    for record, data in records(bundle):
        target = output / record['path']
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as stream:
            stream.write(data)
        target.chmod(record['mode'])
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('pack')
    p.add_argument('root', type=Path)
    p.add_argument('output', type=Path)
    p.add_argument('paths', nargs='+')
    p = sub.add_parser('verify')
    p.add_argument('bundle', type=Path)
    p = sub.add_parser('extract')
    p.add_argument('bundle', type=Path)
    p.add_argument('output', type=Path)
    args = parser.parse_args()
    if args.command == 'pack':
        result = pack(args.root, args.output, args.paths)
    elif args.command == 'extract':
        result = extract(args.bundle, args.output)
    else:
        result = verify(args.bundle)
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
