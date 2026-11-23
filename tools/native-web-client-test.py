#!/usr/bin/env python3
"""Qualify checked browser-client generation without running target tools."""
from __future__ import annotations

import argparse
import difflib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def execute(binary, arguments, root, environment):
    return subprocess.run([str(binary), *arguments], cwd=root, env=environment,
                          text=True, capture_output=True, timeout=120)


def write_project(base, case, mode):
    root = base / 'app'
    root.mkdir(parents=True, exist_ok=True)
    for name, source in case['files'].items():
        path = root / 'src' / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    for name, source in case['packageFiles'].items():
        path = base / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'sample-api', 'version': '1.2.3', 'mode': mode,
        'sourceDir': 'src', 'outDir': 'out',
        'go': {'module': 'example.com/browser-client', 'version': '1.27'},
        'packages': case['packages']}))
    return root, config


def require(result, success, label):
    assert (result.returncode == 0) == success, (label, result.returncode, result.stdout, result.stderr)
    assert result.stderr == '' if success else result.stdout == '' and result.stderr, (label, result.stdout, result.stderr)


def browser_projection(binary, case, base, environment):
    consumer = dict(case, files={name: source for name, source in case['files'].items()
                                if name != 'main.trb' and not name.startswith('routes/')})
    consumer['files']['generated/client.trb'] = case['source']
    root, config = write_project(base, consumer, 'typescript')
    settings = json.loads(config.read_text())
    settings['typescript'] = {'runtime': 'browser', 'moduleType': 'module', 'packageManager': 'npm'}
    config.write_text(json.dumps(settings))
    if case['packages']:
        require(execute(binary, ['install', '--config', str(config)], root, os.environ), True, 'browser package install')
    result = execute(binary, ['build', '--config', str(config)], root, environment)
    gap = case.get('browserCompileGap')
    require(result, gap is None, case['id'] + ' browser projection')
    if gap:
        assert gap in result.stderr, (case['id'], gap, result.stderr)
        return None
    assert (root/'out/generated/client.ts').is_file(), case['id']
    return root/'out'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('compiler', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--report', type=Path)
    parser.add_argument('--browser-check', action='store_true')
    parser.add_argument('--browser-runtime', type=Path, help='Node with TypeScript stripping support for the synthetic Fetch transport check')
    options = parser.parse_args()
    if (options.browser_check or options.browser_runtime) and not options.reference:
        parser.error('browser projection uses the pinned reference TypeScript backend')
    binary = options.compiler.resolve()
    fixtures = json.loads(Path(__file__).with_name('native-web-client-cases.json').read_text())
    checks = 0
    observations = []
    failures = []
    projections = []
    report = {'cases': observations, 'failures': failures,
              'browserProjections': projections, 'complete': False}

    def write_report():
        if options.report:
            options.report.write_text(json.dumps(report, indent=2) + '\n')

    with tempfile.TemporaryDirectory(prefix='native-web-client-') as directory:
        base = Path(directory).resolve()
        forbidden = base / 'forbidden tools'
        forbidden.mkdir()
        marker = base / 'external-tool-invoked'
        for name in ['go', 'ruby', 'node', 'qbe', 'cc', 'clang', 'trb']:
            tool = forbidden / name
            tool.write_text('#!/bin/sh\n: > "$CLIENT_TOOL_MARKER"\nexit 99\n')
            tool.chmod(0o755)
        environment = dict(os.environ, PATH=str(forbidden), TRBN_QBE=str(forbidden / 'qbe'),
                           CLIENT_TOOL_MARKER=str(marker), NO_COLOR='1', TERM='dumb')
        for index, case in enumerate(fixtures):
            root, config = write_project(base / f'case {index}', case, 'go' if options.reference else 'trb')
            if case['packages']:
                installed = execute(binary, ['install', '--config', str(config)], root, os.environ)
                require(installed, True, case['id'] + ' local install')
            result = execute(binary, ['web', 'client', '--config', str(config), *case['arguments']], root, environment)
            observation = {'id': case['id'], 'code': result.returncode,
                           'stdout': result.stdout.replace(str(base), '<case>'),
                           'stderr': result.stderr.replace(str(base), '<case>')}
            observations.append(observation)
            try:
                require(result, case['success'], case['id'])
                if case['success']:
                    assert result.stdout == case['source'], '\n'.join(difflib.unified_diff(
                        case['source'].splitlines(), result.stdout.splitlines(), fromfile='expected', tofile='actual'))
                else:
                    for fragment in case['fragments']:
                        assert fragment in result.stderr, (fragment, result.stderr)
            except AssertionError as error:
                failures.append(case['id'])
                print(case['id'] + ': ' + str(error))
            checks += 1
        write_report()
        assert not failures, failures
        projected = 0
        known_gaps = 0
        if options.browser_check or options.browser_runtime:
            for index, case in enumerate(fixtures):
                if not case['success']:
                    continue
                projection = {'id': case['id'], 'status': 'pending'}
                projections.append(projection)
                try:
                    output = browser_projection(binary, case, base / f'browser {index}', environment)
                    if output is None:
                        known_gaps += 1
                        projection.update(status='known-gap', diagnostic=case['browserCompileGap'])
                    else:
                        projected += 1
                        projection['status'] = 'accepted'
                    if options.browser_runtime and case['id'] in {'web-client-complete-input', 'web-client-payload-input'}:
                        assert output is not None
                        harness = Path(__file__).with_name('web-client-transport.mjs')
                        runtime = execute(options.browser_runtime.resolve(), ['--experimental-strip-types', str(harness), str(output)],
                                          output, dict(os.environ, NODE_NO_WARNINGS='1'))
                        require(runtime, True, 'synthetic browser Fetch transport ' + case['id'])
                        assert '12 transport checks passed' in runtime.stdout, runtime.stdout
                        projection['syntheticFetchChecks'] = 12
                except Exception as error:
                    projection.update(status='failed', error=str(error))
                    raise
                finally:
                    write_report()
            print(f'Browser projection: {projected} accepted; {known_gaps} explicit pinned-reference gaps')
        sample = next(case for case in fixtures if case['id'] == 'web-client-complete-input')
        root, config = write_project(base / 'output project', sample, 'go')
        expected = sample['source']
        for mode in ['go', 'ruby', 'typescript', 'trb']:
            root, config = write_project(base / 'output project', sample, mode)
            result = execute(binary, ['web', 'client', '--config', str(config)], root, environment)
            require(result, True, 'mode ' + mode)
            assert result.stdout == expected
            checks += 1
        destination = root / 'generated' / 'api client.trb'
        arguments = ['web', 'client', '--config', str(config), '--output', 'generated/api client.trb']
        result = execute(binary, arguments, root, environment)
        require(result, True, 'file output')
        assert result.stdout == str(destination)+'\n' and destination.read_text() == expected
        assert destination.stat().st_mode & 0o777 == 0o644
        destination.write_text('old output\n')
        require(execute(binary, arguments, root, environment), True, 'replacement')
        assert destination.read_text() == expected
        checks += 2
        for tail in [['--name=', '--output='], ['--name', ' \t ', '--output', '-']]:
            result = execute(binary, ['web', 'client', '--config', str(config), *tail], root, environment)
            require(result, True, 'default names')
            assert result.stdout == expected
            checks += 1
        nested = root / 'nested' / 'working directory'
        nested.mkdir(parents=True)
        result = execute(binary, ['web', 'client'], nested, environment)
        require(result, True, 'configuration discovery')
        assert result.stdout == expected
        checks += 1
        for output in [str(base/'absolute.trb'), '../escape.trb', 'nested/../../escape.trb']:
            require(execute(binary, ['web', 'client', '--config', str(config), '--output', output], root, environment), False, 'unsafe output')
            checks += 1
        for tail in [['--compile'], ['extra.trb'], ['--title', 'API'], ['--name']]:
            require(execute(binary, ['web', 'client', '--config', str(config), *tail], root, environment), False, 'invalid arguments')
            checks += 1
        destination.write_text('preserve on failure\n')
        require(execute(binary, [*arguments, '--name', 'invalid-name'], root, environment), False, 'generation failure')
        assert destination.read_text() == 'preserve on failure\n'
        checks += 1
        assert not marker.exists(), 'generation invoked an external target tool'
        assert not (base/'escape.trb').exists() and not (base/'absolute.trb').exists()
    report.update(complete=True, generationChecks=checks)
    write_report()
    print(f'Web browser client: {checks} checks passed; no external target tools invoked')


if __name__ == '__main__':
    main()
