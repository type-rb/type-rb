#!/usr/bin/env python3
"""Execute the frozen Web deadline scenarios and inspect live owned resources."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import selectors
import signal
import socket
import subprocess
import sys
import tempfile
import time


ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / 'fixtures/projects/native-web-deadlines.json'
MANIFEST_SHA256 = '93e8b689bf8600688064f8e40081a23a7374df1b374a30ea2561fd579d063aee'


def fixture_module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'tools' / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def stop(process):
    if process.poll() is None:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    process.wait(timeout=5)


def command(row, arguments, cwd, environment, timeout):
    started = time.monotonic()
    entry = {'command': list(map(str, arguments)), 'complete': False}
    row.setdefault('commands', []).append(entry)
    process = subprocess.Popen(entry['command'], cwd=cwd, env=environment,
                               text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               start_new_session=True)
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except BaseException as error:
        stop(process)
        stdout, stderr = process.communicate()
        entry.update(stdout=stdout, stderr=stderr, error=type(error).__name__)
        raise
    finally:
        entry['elapsedSeconds'] = time.monotonic() - started
    entry.update(exit=process.returncode, stdout=stdout, stderr=stderr,
                 complete=process.returncode == 0)
    return entry


def inspect_owned(process, *, file=None, port=None):
    assert process.poll() is None, 'inspection requires a live application'
    if sys.platform == 'darwin':
        result = subprocess.run(['lsof', '-nP', '-a', '-p', str(process.pid), '-Fn'],
                                text=True, capture_output=True, timeout=3)
        assert result.returncode == 0, (result.stdout, result.stderr)
        names = [line[1:] for line in result.stdout.splitlines() if line.startswith('n')]
        if file is not None:
            count = sum(name.removesuffix(' (deleted)') == str(file.resolve()) for name in names)
        else:
            count = sum('->' in name and name.rsplit('->', 1)[1].endswith(':' + str(port))
                        for name in names)
        evidence = {'inspector': 'lsof', 'names': names, 'ownedCount': count}
    elif sys.platform.startswith('linux'):
        proc = Path('/proc') / str(process.pid)
        names = []
        for fd in (proc / 'fd').iterdir():
            try:
                names.append(os.readlink(fd))
            except FileNotFoundError:
                pass
        if file is not None:
            count = sum(name.removesuffix(' (deleted)') == str(file.resolve()) for name in names)
        else:
            sockets = {name[8:-1] for name in names if name.startswith('socket:[')}
            count = 0
            for table in ('tcp', 'tcp6'):
                for line in (proc / 'net' / table).read_text().splitlines()[1:]:
                    columns = line.split()
                    if columns[9] in sockets and int(columns[2].split(':')[1], 16) == port:
                        count += 1
        evidence = {'inspector': 'procfs', 'names': names, 'ownedCount': count}
    else:
        raise RuntimeError('live descriptor inspection requires Darwin or Linux')
    assert process.poll() is None, 'application exited during resource inspection'
    return evidence


def observed_application(row, program, cwd, environment, expected, *, file=None,
                         provider=None, reference=False):
    arguments = [str(program)]
    if provider:
        arguments += [provider.url, provider.token()]
    started = time.monotonic()
    row['events'] = []
    output = bytearray()
    pending = bytearray()
    foreign_started = None
    with (cwd / 'application.stderr').open('w+') as errors:
        process = subprocess.Popen(arguments, cwd=cwd, env=environment, stdout=subprocess.PIPE,
                                   stderr=errors, start_new_session=True)
        selector = selectors.DefaultSelector()
        selector.register(process.stdout, selectors.EVENT_READ)
        try:
            while selector.get_map():
                elapsed = time.monotonic() - started
                assert elapsed < 15, 'application exceeded the 15s controller watchdog'
                for key, _ in selector.select(timeout=min(.1, 15 - elapsed)):
                    chunk = os.read(key.fd, 65536)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        break
                    output.extend(chunk)
                    pending.extend(chunk)
                    while b'\n' in pending:
                        line, _, rest = pending.partition(b'\n')
                        pending = bytearray(rest)
                        text = line.decode()
                        event = {'line': text, 'elapsedSeconds': time.monotonic() - started}
                        row['events'].append(event)
                        if text == 'foreign-start':
                            foreign_started = time.monotonic()
                        if text == 'inspect':
                            event['resources'] = inspect_owned(process, file=file)
                            assert event['resources']['ownedCount'] == 0, event
                        if text in ('after-response', 'after-wait'):
                            assert provider is not None
                            event['resources'] = inspect_owned(process, port=provider.server.server_port)
                            if text == 'after-response':
                                assert foreign_started is not None
                                event['responseSeconds'] = time.monotonic() - foreign_started
                                assert event['responseSeconds'] < expected['handlerResponseWatchdogSeconds'], event
                            if not reference or text == 'after-wait':
                                assert event['resources']['ownedCount'] == 0, event
            process.wait(timeout=3)
            errors.seek(0)
            row['execution'] = {'exit': process.returncode, 'stdout': output.decode(),
                                'stderr': errors.read(), 'elapsedSeconds': time.monotonic() - started}
            assert process.returncode == 0, row['execution']
            assert output.decode() == expected['stdout'], row['execution']
            assert row['execution']['stderr'] == '', row['execution']
            if file:
                assert any(event['line'] == 'inspect' for event in row['events'])
            if provider:
                row['providerRequests'] = list(provider.requests)
                assert '/slow' in provider.requests, 'deadline did not reach the foreign provider'
        finally:
            selector.close()
            stop(process)
            process.stdout.close()


def application_result(row, result, expected):
    if expected['exit'] == 'zero':
        assert result['exit'] == 0, result
        assert result['stderr'] == '', result
    else:
        assert result['exit'] != 0, result
    if 'stdout' in expected:
        assert result['stdout'] == expected['stdout'], result
    if 'stderrContains' in expected:
        assert expected['stderrContains'] in result['stderr'], result
    if expected.get('mustNotProduceTimeoutResponse'):
        assert 'gateway_timeout' not in result['stdout'], result
        assert 'division by zero' in result['stderr'], result
    row['execution'] = result


def server_result(row, program, cwd, environment, port, expected):
    network = fixture_module('deadline_network', 'native-web-network-test.py')
    process = subprocess.Popen([str(program)], cwd=cwd, env=environment,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    try:
        until = time.monotonic() + 10
        while True:
            assert process.poll() is None, 'server exited before listening'
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=.1):
                    break
            except OSError:
                assert time.monotonic() < until, 'server did not start'
                time.sleep(.025)
        row['requests'] = []
        # A raw Wire never reconnects, so every response uses this exact socket.
        with network.Wire(port) as wire:
            for path, status in zip(['/fast', '/slow', '/fast', '/bad', '/fast', '/slow', '/fast'],
                                    expected['statuses']):
                wire.send(f'GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\n\r\n'.encode())
                actual, headers, body = wire.response()
                row['requests'].append({'path': path, 'status': actual, 'body': body.decode()})
                assert actual == status, row['requests'][-1]
                if status == 200:
                    assert body == b'complete', row['requests'][-1]
                if status == 504:
                    assert body == b'{"error":"gateway_timeout"}', row['requests'][-1]
        process.send_signal(signal.SIGTERM)
        stdout, stderr = process.communicate(timeout=5)
        row['shutdown'] = {'exit': process.returncode, 'stdout': stdout.decode(), 'stderr': stderr.decode()}
        assert process.returncode == 0, row['shutdown']
        with socket.socket() as probe:
            probe.settimeout(.2)
            assert probe.connect_ex(('127.0.0.1', port)) != 0, 'listener survived process shutdown'
    finally:
        stop(process)
        process.stdout.close()
        process.stderr.close()


def scenario(args, case, row, environment):
    kind = case['kind']
    if kind == 'runtime':
        assert args.runtime_reference and args.qbe, 'runtime controls require --runtime-reference and --qbe'
        env = dict(environment, TYPE_RB_NATIVE_QBE=str(args.qbe.resolve()))
        invocation = [str(args.runtime_reference.resolve()), *case['command'][1:]]
        result = command(row, invocation, ROOT, env, 240)
        assert result['exit'] == 0, result
        return
    if kind == 'existing-network':
        invocation = [sys.executable, str(ROOT / 'tools/native-web-network-test.py'), str(args.binary.resolve())]
        if args.reference:
            invocation += ['--reference']
        else:
            assert args.libraries, 'network controls require --libraries'
            invocation += ['--libraries', str(args.libraries.resolve())]
        result = command(row, invocation, ROOT, environment, 300)
        assert result['exit'] == 0, result
        return
    with tempfile.TemporaryDirectory(prefix='native-deadline-') as temporary:
        root = Path(temporary)
        port = None
        if kind == 'server':
            with socket.socket() as reservation:
                reservation.bind(('127.0.0.1', 0))
                port = reservation.getsockname()[1]
        for name, content in case['files'].items():
            assert hashlib.sha256(content.encode()).hexdigest() == case['sourceHashes'][name]
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content.replace('PORT', str(port)) if port else content)
        binary = args.binary.resolve()
        check = [binary, 'check', 'main.trb']
        build = [binary, 'build', '--compile', '--outfile', 'program', 'main.trb']
        if kind == 'server':
            config = root / 'trbconfig.jsonc'
            config.write_text(json.dumps({'name': 'deadline-acceptance', 'mode': 'go' if args.reference else 'trb',
                                         'sourceDir': '.', 'go': {'module': 'example.com/deadline-acceptance'}}))
            check = [binary, 'check', '--config', config]
            build = [binary, 'build', '--compile', '--config', config, '--outfile', 'program']
        for invocation in (check, build):
            result = command(row, invocation, root, environment, 180)
            assert result['exit'] == 0, result
        program = root / 'program'
        row['programSha256'] = hashlib.sha256(program.read_bytes()).hexdigest()
        expected = case['expected']
        if kind == 'server':
            server_result(row, program, root, environment, port, expected)
        elif kind == 'file-lifetime':
            observed_application(row, program, root, environment, expected, file=root / 'resource.txt')
        elif kind == 'oidc-lifetime':
            oidc = fixture_module('deadline_oidc', 'native-oidc-test.py')
            with oidc.Provider(root, tls=case['tls']) as provider:
                env = dict(environment)
                for key in ('http_proxy', 'https_proxy', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY',
                            'all_proxy', 'NO_PROXY', 'no_proxy', 'REQUEST_METHOD'):
                    env.pop(key, None)
                if case['tls']:
                    empty = root / 'empty-roots'
                    empty.mkdir()
                    env.update(SSL_CERT_FILE=str(provider.certificate), SSL_CERT_DIR=str(empty))
                observed_application(row, program, root, env, expected, provider=provider, reference=args.reference)
        else:
            result = command(row, [program], root, environment, 15)
            application_result(row, result, expected)
            if expected.get('newTimerOrNetworkCryptoDependency') is False and not args.reference:
                imports = command(row, ['nm', '-u', program], root, environment, 10)
                assert imports['exit'] == 0, imports
                for name in ('clock_gettime', 'curl_', 'uv_', 'OPENSSL_', 'EVP_'):
                    assert name not in imports['stdout'], imports


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--libraries', type=Path)
    parser.add_argument('--runtime-reference', type=Path, default=os.environ.get('TYPE_RB_NATIVE_REFERENCE_TRB'))
    parser.add_argument('--qbe', type=Path, default=os.environ.get('TYPE_RB_NATIVE_QBE'))
    parser.add_argument('--case', action='append', default=[])
    parser.add_argument('--scope', choices=('complete', 'applications'), default='complete',
                        help='applications excludes only the separate Native runtime-unit authority')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.reference and not args.runtime_reference:
        args.runtime_reference = args.binary
    raw = MANIFEST.read_bytes()
    assert hashlib.sha256(raw).hexdigest() == MANIFEST_SHA256, 'the initial manifest is immutable'
    manifest = json.loads(raw)
    known = {case['id'] for case in manifest['cases']}
    assert len(known) == 20 and not set(args.case) - known, 'invalid scenario selection'
    selected = [case for case in manifest['cases']
                if (not args.case or case['id'] in args.case)
                and (args.scope == 'complete' or case['kind'] != 'runtime')]
    assert selected, 'empty scenario selection'
    args.output.mkdir(parents=True, exist_ok=False)
    receipt = {'schemaVersion': 1, 'manifestSha256': MANIFEST_SHA256,
               'binarySha256': hashlib.sha256(args.binary.resolve().read_bytes()).hexdigest(),
               'controllerSha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
               'reference': args.reference, 'scope': args.scope,
               'selectedCaseIds': [case['id'] for case in selected],
               'omittedCaseIds': [case['id'] for case in manifest['cases'] if case not in selected],
               'complete': False, 'cases': []}
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb')
    if args.libraries:
        environment['TRBN_NETWORK_LIB_DIR'] = str(args.libraries.resolve())
    try:
        for case in selected:
            authority = 'native-runtime' if case['kind'] == 'runtime' else (
                'reference-application' if args.reference else 'native-application')
            row = {'id': case['id'], 'authority': authority, 'complete': False}
            receipt['cases'].append(row)
            started = time.monotonic()
            try:
                scenario(args, case, row, environment)
                row['complete'] = True
            except (AssertionError, OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
                row['error'] = f'{type(error).__name__}: {error}'
            row['elapsedSeconds'] = time.monotonic() - started
            (args.output / (case['id'] + '.json')).write_text(json.dumps(row, indent=2) + '\n')
            print(('PASS ' if row['complete'] else 'FAIL ') + case['id'], flush=True)
        receipt['complete'] = all(row['complete'] for row in receipt['cases'])
    finally:
        (args.output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    raise SystemExit(0 if receipt['complete'] else 1)


if __name__ == '__main__':
    main()
