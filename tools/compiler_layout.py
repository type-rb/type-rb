#!/usr/bin/env python3
"""Check compiler ownership boundaries, including tests and CLI composition.

These are responsibility rules, not a directory DAG. Reciprocal frontend/MIR
imports and mutually recursive checking modules are intentional.
"""
import argparse
from pathlib import Path
import re
import sys

GROUPS = {
    'support': {'support'},
    'state': {'state', 'support', 'frontend/types', 'mir'},
    'frontend/types': {'frontend/types', 'frontend/resolution', 'state', 'support'},
    'frontend/syntax': {'frontend/syntax', 'frontend/types', 'frontend/resolution', 'state', 'support'},
    'frontend/resolution': {'frontend/resolution', 'frontend/types', 'frontend/syntax', 'frontend/providers', 'state', 'support', 'mir', 'project'},
    'frontend/checking': {'frontend/checking', 'frontend/types', 'frontend/resolution', 'frontend/syntax', 'frontend/providers', 'state', 'support', 'mir', 'project'},
    'frontend/providers': {'frontend/providers', 'frontend/types', 'frontend/resolution', 'frontend/syntax', 'state', 'support', 'project'},
    'mir': {'mir', 'frontend/checking', 'frontend/types', 'frontend/syntax', 'frontend/resolution', 'state', 'support'},
    'backend/qbe': {'backend/qbe', 'mir', 'state', 'support', 'frontend/types'},
    'project': {'project', 'support', 'frontend/syntax', 'frontend/providers', 'state'},
}
DIRECTORY_OWNERS = {name: name for name in GROUPS}
for owner, children in {
    'frontend/checking': ('body', 'program', 'builtins', 'nominal', 'collections'),
    'mir': ('model', 'build', 'lowering', 'analysis', 'passes', 'verify'),
    'backend/qbe': ('emit', 'runtime'),
}.items():
    DIRECTORY_OWNERS.update({owner + '/' + child: owner for child in children})
# Existing declaration-bound intrinsic identity and the shared parsed/checked
# iteration projection have narrow exceptions, not permission for whole layers.
EXCEPTIONS = {
    ('backend/qbe/emit/constants', 'frontend/resolution/entry_resolution'),
    ('frontend/types/transform_model', 'frontend/syntax/iteration_syntax'),
    ('frontend/types/transform_model', 'mir/model/iteration'),
    ('state/source_state', 'frontend/syntax/iteration_syntax'),
}
IMPORT = re.compile(r'import (?:\{[^}]*\} from )?([a-z][a-z0-9_/]*)(?: as [A-Za-z_][A-Za-z0-9_]*)?(?:\s*#.*)?')


def import_header(source):
    """Return leading imports without interpreting embedded program fixtures."""
    result = []
    for number, line in enumerate(source.splitlines(), 1):
        if not line.strip() or line.startswith('#'):
            continue
        if not line.startswith('import '):
            break
        match = IMPORT.fullmatch(line)
        if not match:
            raise ValueError(f'unsupported import header on line {number}: {line}')
        result.append((number, match.group(1)))
    return result


def unit_test_owner(path):
    """Concern-specific suites must still name a colocated production owner."""
    stem = path.stem.removesuffix('_test')
    return any(not source.stem.endswith('_test') and
               (stem == source.stem or stem.startswith(source.stem + '_'))
               for source in path.parent.glob('*.trb'))


def check(root):
    root = Path(root)
    modules = {}
    errors = []
    for area in ('src', 'cli'):
        directory = root / 'compiler' / area
        for path in sorted(directory.rglob('*.trb')):
            relative = path.relative_to(directory)
            name = relative.with_suffix('').as_posix()
            label = path.relative_to(root).as_posix()
            if path.is_symlink() or not path.resolve().is_relative_to(directory.resolve()):
                errors.append(f'{label}: source must stay inside its declared root')
                continue
            if name in modules:
                errors.append(f'{label}: duplicate composed module {name}')
                continue
            directory_owner = relative.parent.as_posix()
            group = 'cli' if area == 'cli' else DIRECTORY_OWNERS.get(directory_owner, directory_owner)
            if relative.parts[0] in ('testing', 'tests'):
                group = relative.parts[0]
            if group == '.' and (name == 'compiler' or name.endswith('_test')):
                group = 'entry'
            if group not in GROUPS and group not in ('entry', 'cli', 'testing', 'tests'):
                errors.append(f'{label}: missing responsibility owner')
            if group == 'tests' and not name.endswith('_test'):
                errors.append(f'{label}: test cases require the _test suffix; helpers belong in testing')
            modules[name] = (path, group, name.endswith('_test'), area)
    if 'compiler' not in modules:
        errors.append('compiler/src/compiler.trb: missing compiler entry')
    edge_count = 0
    for name, (path, group, test, area) in modules.items():
        label = path.relative_to(root).as_posix()
        if test and group not in ('tests', 'testing') and not unit_test_owner(path):
            errors.append(f'{label}: unit test must name a colocated production owner; system/feature suites belong in tests')
        try:
            imports = import_header(path.read_text())
        except ValueError as error:
            errors.append(f'{label}: {error}')
            continue
        for line, target in imports:
            if target.startswith('trb/'):
                if target == 'trb/std/test' and not test and group != 'testing':
                    errors.append(f'{label}:{line}: production code imports test support')
                if target == 'trb/std/test' and group == 'testing':
                    declaration = path.read_text().splitlines()[line - 1]
                    if re.search(r'\b(describe|test)\b', declaration.split(' from ')[0]):
                        errors.append(f'{label}:{line}: test helpers cannot register cases')
                continue
            edge_count += 1
            if target not in modules:
                errors.append(f'{label}:{line}: missing composed module {target}')
                continue
            _, target_group, target_test, target_area = modules[target]
            if area == 'src' and target_area == 'cli':
                errors.append(f'{label}:{line}: core cannot depend on CLI module {target}')
            elif target_test:
                errors.append(f'{label}:{line}: test modules cannot be imported ({target})')
            elif not test and group != 'testing' and target_group == 'testing':
                errors.append(f'{label}:{line}: production code imports testing helper {target}')
            elif test or group in ('entry', 'cli', 'testing'):
                continue
            elif target_group not in GROUPS.get(group, set()) and (name, target) not in EXCEPTIONS:
                errors.append(f'{label}:{line}: forbidden responsibility dependency {group} -> {target_group} ({target})')
    errors.extend(check_recovery(root))
    return errors, len(modules), edge_count


def check_recovery(root):
    """Recovery owns an independent import root and reads compiler input as data."""
    directory = root / 'recovery/src'
    modules = {}
    errors = []
    for path in sorted(directory.rglob('*.trb')):
        relative = path.relative_to(directory)
        name = relative.with_suffix('').as_posix()
        owner = relative.parts[0]
        if path.is_symlink() or not path.resolve().is_relative_to(directory.resolve()):
            errors.append(f'{path.relative_to(root)}: recovery source escapes its root')
        if owner not in ('snapshot', 'scalar', 'aggregate', 'managed', 'compiler', 'driver', 'support', 'testing', 'tests'):
            errors.append(f'{path.relative_to(root)}: missing recovery responsibility owner')
        if owner == 'tests' and not name.endswith('_test'):
            errors.append(f'{path.relative_to(root)}: recovery helpers belong in testing')
        modules[name] = (path, owner, name.endswith('_test'))
    for name, (path, owner, test) in modules.items():
        label = path.relative_to(root).as_posix()
        if test and owner not in ('tests', 'testing') and not unit_test_owner(path):
            errors.append(f'{label}: recovery unit test must name a colocated production owner')
        try:
            imports = import_header(path.read_text())
        except ValueError as error:
            errors.append(f'{label}: {error}')
            continue
        for line, target in imports:
            if target.startswith('trb/'):
                if target == 'trb/std/test' and not test and owner != 'testing':
                    errors.append(f'{label}:{line}: recovery production imports test support')
                if target == 'trb/std/test' and owner == 'testing':
                    declaration = path.read_text().splitlines()[line - 1]
                    if re.search(r'\b(describe|test)\b', declaration.split(' from ')[0]):
                        errors.append(f'{label}:{line}: recovery helpers cannot register cases')
                continue
            if target not in modules:
                errors.append(f'{label}:{line}: recovery import outside its source root: {target}')
                continue
            _, target_owner, target_test = modules[target]
            if target_test:
                errors.append(f'{label}:{line}: recovery test modules cannot be imported')
            elif not test and owner != 'testing' and target_owner == 'testing':
                errors.append(f'{label}:{line}: recovery production imports testing helper')
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args()
    errors, modules, edges = check(args.root)
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'Compiler ownership: {modules} modules, {edges} explicit imports; directory cycles allowed')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
