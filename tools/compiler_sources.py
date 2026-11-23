#!/usr/bin/env python3
"""Compose compiler source roots without changing their logical module paths."""
import argparse
import hashlib
from pathlib import Path
import shutil


def source_files(root):
    root = Path(root)
    if not root.is_dir() or root.is_symlink():
        raise ValueError(f"expected a compiler source directory: {root}")
    sources = []
    for source in sorted(root.rglob('*')):
        if source.is_symlink():
            raise ValueError(f"compiler source symlink is not supported: {source}")
        relative = source.relative_to(root)
        if (source.is_file() and source.suffix == '.trb'
                and not source.name.endswith('_test.trb')
                and relative.parts[0] not in ('testing', 'tests')):
            sources.append((source.relative_to(root), source))
    return sources


def source_hashes(root):
    return [f'{hashlib.sha256(source.read_bytes()).hexdigest()}  {source.as_posix()}'
            for _, source in source_files(root)]


def stage_sources(roots, destination):
    destination = Path(destination)
    selected = {}
    directories = set()
    for root in roots:
        for relative, source in source_files(root):
            target = destination / relative
            if (relative in selected or relative in directories or
                    any(parent in selected for parent in relative.parents) or
                    target.exists() or target.is_symlink()):
                raise ValueError(f"compiler source collision: {relative}")
            for parent in target.parents:
                if parent.is_symlink() or parent.exists() and not parent.is_dir():
                    raise ValueError(f"invalid compiler source destination: {parent}")
                if parent == destination:
                    break
            selected[relative] = source
            directories.update(relative.parents)
    # Validate every input and destination before publishing any staged file.
    for relative, source in selected.items():
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
    return list(selected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hash', dest='hash_root', type=Path,
                        help='print production content keys using the staging selection')
    parser.add_argument('destination', type=Path, nargs='?')
    parser.add_argument('sources', type=Path, nargs='*')
    args = parser.parse_args()
    try:
        if args.hash_root is not None:
            if args.destination is not None or args.sources:
                parser.error('--hash accepts one source root and no staging arguments')
            print('\n'.join(source_hashes(args.hash_root)))
        else:
            if args.destination is None or not args.sources:
                parser.error('staging requires a destination and at least one source root')
            stage_sources(args.sources, args.destination)
    except (OSError, ValueError) as error:
        parser.exit(1, f"compiler source staging: {error}\n")


if __name__ == '__main__':
    main()
