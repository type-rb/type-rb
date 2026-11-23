#!/usr/bin/env python3
"""Qualify metadata-only Web generation without a target toolchain."""
from __future__ import annotations

import argparse
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile


def execute(binary: Path, arguments: list[str], root: Path, environment: dict[str, str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run([str(binary), *arguments], cwd=root, env=environment, text=True, capture_output=True, timeout=120)


def write_project(root: Path, files: dict[str, str], mode: str) -> Path:
    for name, source in files.items():
        path = root / 'src' / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'sample-api', 'version': '1.2.3', 'mode': mode, 'sourceDir': 'src', 'outDir': 'out', 'go': {'module': 'example.com/endpoint-contract', 'version': '1.27'}}))
    return config


def require(result: subprocess.CompletedProcess[str], success: bool, label: str) -> None:
    assert (result.returncode == 0) == success, (label, result.returncode, result.stdout, result.stderr)
    if success:
        assert result.stderr == '', (label, result.stderr)
    else:
        assert result.stdout == '' and result.stderr, (label, result.stdout, result.stderr)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('compiler', type=Path)
    parser.add_argument('--reference', action='store_true')
    options = parser.parse_args()
    binary = options.compiler.resolve()
    fixtures = json.loads(Path(__file__).with_name('native-web-openapi-cases.json').read_text())
    checks = 0
    with tempfile.TemporaryDirectory(prefix='native-web-openapi-') as directory:
        base = Path(directory)
        forbidden = base / 'forbidden tools'
        forbidden.mkdir()
        marker = base / 'external-tool-invoked'
        for name in ['go', 'ruby', 'node', 'qbe', 'cc', 'clang']:
            tool = forbidden / name
            tool.write_text('#!/bin/sh\n: > "$OPENAPI_TOOL_MARKER"\nexit 99\n')
            tool.chmod(0o755)
        environment = dict(os.environ, PATH=str(forbidden), TRBN_QBE=str(forbidden / 'qbe'), OPENAPI_TOOL_MARKER=str(marker), NO_COLOR='1', TERM='dumb')
        for index, case in enumerate(fixtures):
            root = base / f'case {index}'
            root.mkdir()
            config = write_project(root, case['files'], 'go' if options.reference else 'trb')
            result = execute(binary, ['web', 'openapi', '--config', str(config)], root, environment)
            require(result, case['success'], case['id'])
            if case['success']:
                expected = json.dumps(case['document'], ensure_ascii=False, indent=2) + '\n'
                assert result.stdout == expected, (case['id'], result.stdout, expected)
            checks += 1
        sample = next(case for case in fixtures if case['id'] == 'web-endpoint-record')
        root = base / 'output project'
        root.mkdir()
        files = copy.deepcopy(sample['files'])
        files['main.trb'] = 'def main()\nputs("main must never execute during generation")\nend\n'
        config = write_project(root, files, 'go' if options.reference else 'trb')
        expected = json.dumps(sample['document'], ensure_ascii=False, indent=2) + '\n'
        for mode in ['go', 'ruby', 'typescript', 'trb']:
            config = write_project(root, files, mode)
            result = execute(binary, ['web', 'openapi', '--config', str(config)], root, environment)
            require(result, True, 'mode ' + mode)
            assert result.stdout == expected
            checks += 1
        destination = root / 'docs' / 'api schema.json'
        arguments = ['web', 'openapi', '--config', str(config), '--output', 'docs/api schema.json']
        first = execute(binary, arguments, root, environment)
        require(first, True, 'file output')
        assert first.stdout == str(destination) + '\n' and destination.read_text() == expected
        destination.write_text('old output\n')
        repeated = execute(binary, arguments, root, environment)
        require(repeated, True, 'atomic replacement')
        assert destination.read_text() == expected
        checks += 2
        custom = copy.deepcopy(sample['document'])
        custom['info'] = {'title': '改訂 <API>&', 'version': '2.0'}
        metadata = execute(binary, ['web', 'openapi', '--config', str(config), '--title=  改訂 <API>&  ', '--api-version', '  2.0  '], root, environment)
        require(metadata, True, 'metadata override')
        assert metadata.stdout == json.dumps(custom, ensure_ascii=False, indent=2) + '\n'
        defaults = execute(binary, ['web', 'openapi', '--config', str(config), '--title', ' \t ', '--api-version= ', '--output='], root, environment)
        require(defaults, True, 'empty metadata defaults')
        assert defaults.stdout == expected
        checks += 2
        nested = root / 'nested' / 'working directory'
        nested.mkdir(parents=True)
        found = execute(binary, ['web', 'openapi'], nested, environment)
        require(found, True, 'configuration discovery')
        assert found.stdout == expected
        checks += 1
        for output in [str(base / 'absolute.json'), '../escape.json', 'nested/../../escape.json']:
            result = execute(binary, ['web', 'openapi', '--config', str(config), '--output', output], root, environment)
            require(result, False, 'unsafe output ' + output)
            checks += 1
        for tail in [['--compile'], ['extra.trb'], ['--unknown'], ['--title']]:
            result = execute(binary, ['web', 'openapi', '--config', str(config), *tail], root, environment)
            require(result, False, 'invalid arguments ' + repr(tail))
            checks += 1
        destination.write_text('preserve on failure\n')
        route = root / 'src' / 'routes' / 'items.trb'
        route.write_text(route.read_text().replace('handles(get)', ''))
        failed = execute(binary, arguments, root, environment)
        require(failed, False, 'generation failure')
        assert destination.read_text() == 'preserve on failure\n'
        checks += 1
        assert not marker.exists(), 'generation launched an external target tool'
        assert not (base / 'escape.json').exists() and not (base / 'absolute.json').exists()
    print(f'Web OpenAPI: {checks} checks passed; no external target tools invoked')


if __name__ == '__main__':
    main()
