#!/usr/bin/env python3
"""Repeat the reviewed compiler extraction and path migration on a rebased tree.

Only leading compiler/CLI import declarations are rewritten. Program strings in
unit tests and immutable historical consumers are not general search targets.
All source assignments and collisions are checked before any file is changed.
"""
import argparse
from collections import OrderedDict
import json
from pathlib import Path
import re
import subprocess
import sys

NAMED_IMPORT = re.compile(r'import \{ ([^}]+) \} from ([a-z][a-z0-9_/]*)(.*)')
FUNCTION = re.compile(r'^def ([a-z][a-z0-9_]*)\(', re.M)
MODULE = re.compile(r'[a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*')


def split_header(source):
    lines = source.splitlines(keepends=True)
    count = 0
    while count < len(lines) and lines[count].startswith('import '):
        count += 1
    if count < len(lines) and lines[count] == '\n':
        count += 1
    return ''.join(lines[:count]), ''.join(lines[count:])


def named_imports(header):
    imports = []
    for line in header.splitlines():
        if not line:
            continue
        match = NAMED_IMPORT.fullmatch(line)
        if not match:
            raise ValueError(f'extraction requires named imports: {line}')
        for member in match[1].split(', '):
            parts = member.split(' as ')
            imports.append((parts[-1], member, match[2]))
    return imports


def extract_checker(sources, owners):
    """Move complete function bodies; keep each implementation in one owner."""
    header, body = split_header(sources['checked_program'])
    matches = list(FUNCTION.finditer(body))
    present = {match[1] for match in matches}
    remaining = {name for name, owner in owners.items() if owner == 'checked_program'}
    if present == remaining:
        for name, owner in owners.items():
            if len(re.findall(r'^def ' + re.escape(name) + r'\(', sources.get(owner, ''), re.M)) != 1:
                raise ValueError(f'partially applied checker extraction: {owner}.{name}')
        return
    if present != set(owners) or len(present) != len(matches):
        raise ValueError('checker declarations changed; review function ownership before migration')
    for owner in set(owners.values()) - {'checked_program', 'lambda_checking'}:
        if owner in sources:
            raise ValueError(f'extracted owner already exists: {owner}')
    if body[:matches[0].start()].strip():
        raise ValueError('checker has declarations before its first function')
    pieces = OrderedDict()
    for index, match in enumerate(matches):
        finish = matches[index + 1].start() if index + 1 < len(matches) else len(body)
        pieces.setdefault(owners[match[1]], []).append(body[match.start():finish])
    pool = named_imports(header)
    # Names formerly declared in one file become ordinary explicit imports.
    pool += [(name, name, owner) for name, owner in owners.items()]
    for owner, fragments in pieces.items():
        old_header, old_body = ('', '') if owner == 'checked_program' else split_header(sources.get(owner, ''))
        local_pool = named_imports(old_header) + pool
        if old_body and not old_body.endswith('\n\n'):
            old_body += '\n'
        new_body = old_body + ''.join(fragments)
        declared = {match[1] for match in FUNCTION.finditer(new_body)}
        imports = OrderedDict()
        imported = set()
        for local_name, member, module in local_pool:
            if local_name in declared or module == owner or local_name in imported:
                continue
            if re.search(r'\b' + re.escape(local_name) + r'\b', new_body):
                imports.setdefault(module, []).append(member)
                imported.add(local_name)
        new_header = ''.join('import { ' + ', '.join(members) + ' } from ' + module + '\n'
                             for module, members in imports.items())
        sources[owner] = new_header + ('\n' if new_header else '') + new_body


def rewrite_imports(source, modules, owners, default_imports=None):
    default_imports = default_imports or {}
    header, body = split_header(source)
    lines = []
    for line in header.splitlines(keepends=True):
        match = NAMED_IMPORT.fullmatch(line.rstrip('\n'))
        if match:
            grouped = OrderedDict()
            for member in match[1].split(', '):
                origin = member.split(' as ')[0]
                owner = owners.get(origin, match[2]) if match[2] == 'checked_program' else match[2]
                grouped.setdefault(modules.get(owner, owner), []).append(member)
            for module, members in grouped.items():
                lines.append('import { ' + ', '.join(members) + ' } from ' + module + match[3] + '\n')
        elif line.startswith('import '):
            parts = line.rstrip('\n').split(' ')
            old = parts[1]
            parts[1] = modules.get(old, old)
            if old in default_imports and parts[1] != old:
                member = default_imports[old]
                suffix = ' '.join(parts[2:])
                if suffix.startswith('as '):
                    alias, *tail = suffix[3:].split(' ', 1)
                    member += ' as ' + alias
                    suffix = tail[0] if tail else ''
                lines.append('import { ' + member + ' } from ' + parts[1] + (' ' + suffix if suffix else '') + '\n')
            else:
                lines.append(' '.join(parts) + '\n')
        else:
            lines.append(line)
    return ''.join(lines) + body


def plan(root, manifest):
    root = Path(root)
    modules = manifest['modules']
    owners = manifest['checkerFunctions']
    previous = manifest.get('previousModules', {})
    import_paths = dict(modules)
    default_imports = dict(manifest.get('defaultImports', {}))
    for old, owner in previous.items():
        if owner not in modules:
            raise ValueError(f'previous module has no current owner: {old}')
        import_paths[old] = modules[owner]
        if owner in default_imports:
            default_imports[old] = default_imports[owner]
    if (any(not MODULE.fullmatch(name) or not MODULE.fullmatch(target) for name, target in modules.items())
            or len(set(modules.values())) != len(modules)):
        raise ValueError('invalid or colliding module assignments')
    directory = root / 'compiler/src'
    sources, originals = {}, {}
    reverse = {target: name for name, target in modules.items()}
    for path in sorted(directory.rglob('*.trb')):
        name = path.relative_to(directory).with_suffix('').as_posix()
        original = name if name in modules else reverse.get(name, previous.get(name))
        if original is None:
            raise ValueError(f'unassigned compiler module: {name}')
        if original in sources:
            raise ValueError(f'flat/nested source collision: {original}')
        if path.is_symlink() or not path.resolve().is_relative_to(directory.resolve()):
            raise ValueError(f'compiler source escapes its root: {name}')
        sources[original] = path.read_text()
        originals[original] = path
    # Extracted owners may not exist yet; every pre-existing owner must exist.
    new_owners = set(owners.values()) - {'checked_program', 'lambda_checking'}
    missing = set(modules) - set(sources) - new_owners
    if missing:
        raise ValueError('missing assigned compiler modules: ' + ', '.join(sorted(missing)))
    extract_checker(sources, owners)
    writes = {}
    removals = []
    for name, source in sources.items():
        destination = directory / (modules[name] + '.trb')
        original = originals.get(name)
        if destination.exists() and destination != original:
            raise ValueError(f'destination already exists: {destination.relative_to(root)}')
        parent = destination.parent
        while parent != directory:
            if parent.is_symlink() or (parent.exists() and not parent.is_dir()):
                raise ValueError(f'invalid destination parent: {parent.relative_to(root)}')
            parent = parent.parent
        content = rewrite_imports(source, import_paths, owners, default_imports)
        if destination != original or content != original.read_text():
            writes[destination] = content
        if original and original != destination:
            removals.append(original)
    for path in sorted((root / 'compiler/cli').rglob('*.trb')):
        if path.is_symlink():
            raise ValueError('CLI source symlinks are not migration inputs')
        content = rewrite_imports(path.read_text(), import_paths, owners, default_imports)
        if content != path.read_text():
            writes[path] = content
    # These reviewed current consumers contain canonical paths or generated
    # compiler imports. Historical tools and arbitrary fixture strings stay out.
    consumers = set(manifest.get('pathConsumers', [])) | set(manifest.get('moduleConsumers', [])) | set(manifest.get('lookupConsumers', {}))
    for name in sorted(consumers):
        path = root / name
        original = path.read_text()
        content = writes.get(path, original)
        if name in manifest.get('pathConsumers', []):
            for old, new in import_paths.items():
                content = content.replace('compiler/src/' + old + '.trb', 'compiler/src/' + new + '.trb')
        if name in manifest.get('moduleConsumers', []):
            for old, new in import_paths.items():
                content = re.sub(r'(?<=from )' + re.escape(old) + r'(?=[\n\'\"]|\\n)', new, content)
        for module in manifest.get('lookupConsumers', {}).get(name, []):
            content = content.replace('find_module(state, "' + module + '")',
                                      'find_module(state, "' + modules[module] + '")')
        if content != original:
            writes[path] = content
    return writes, removals


def apply(root, manifest):
    writes, removals = plan(root, manifest)
    for path, source in writes.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    for path in removals:
        path.unlink()
    return len(writes), len(removals)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--manifest', type=Path, default=Path(__file__).with_name('compiler-layout-migration.json'))
    parser.add_argument('--write', action='store_true')
    args = parser.parse_args()
    args.root = args.root.resolve()
    manifest = json.loads(args.manifest.read_text())
    try:
        if args.write:
            written, removed = apply(args.root, manifest)
            if manifest.get('syncRecovery'):
                subprocess.run([sys.executable, str(args.root / 'tools/recovery_layout_sync.py'), '--write', '--module-map', str(args.manifest.resolve())], cwd=args.root, check=True)
            print(f'Compiler layout: wrote {written} files, removed {removed} old paths')
        else:
            writes, removals = plan(args.root, manifest)
            if writes or removals:
                print(f'Compiler layout needs migration: {len(writes)} writes, {len(removals)} old paths', file=sys.stderr)
                return 1
            print('Compiler layout migration is already applied')
    except (ValueError, OSError) as error:
        print(f'Compiler layout: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
