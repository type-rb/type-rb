#!/usr/bin/env python3
"""Qualify Native/reference OIDC HTTP/2 against an independent local server."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

from network_artifacts import target_name
from oidc_http2_fixture import Http2Provider

spec = importlib.util.spec_from_file_location('oidc_fixture', Path(__file__).with_name('native-oidc-test.py'))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--stress', action='store_true')
    parser.add_argument('--receipt', required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    receipt = {'compiler_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
               'reference_revision': (Path(__file__).resolve().parent.parent / 'TYPE_RB_REVISION').read_text().strip(),
               'mode': 'reference' if args.reference else 'native', 'stress': args.stress, 'cases': []}
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb')
    for key in list(environment):
        if key.lower().endswith('_proxy') or key in ('REQUEST_METHOD', 'SSL_CERT_FILE', 'SSL_CERT_DIR'):
            environment.pop(key)
    try:
        with tempfile.TemporaryDirectory(prefix='oidc-http2-contracts-') as temporary:
            root = Path(temporary)
            (root / 'src').mkdir()
            source = root / 'src/main.trb'
            source.write_text(fixture.SOURCE)
            config = root / 'trbconfig.jsonc'
            config.write_text(json.dumps({'name': 'oidc-http2-contracts', 'mode': 'go' if args.reference else 'trb',
                                         'sourceDir': 'src', 'go': {'module': 'example.com/oidc-http2-contracts'}}))
            application = root / 'application'
            compiler_environment = environment
            if not args.reference:
                blocked = root / 'blocked-tools'
                blocked.mkdir()
                for name in ('go', 'trb'):
                    command = blocked / name
                    command.write_text('#!/bin/sh\nprintf "%s\\n" "$0" >> "$OIDC_FORBIDDEN_TOOLS"\nexit 91\n')
                    command.chmod(0o755)
                compiler_environment = dict(environment, PATH=str(blocked) + os.pathsep + environment.get('PATH', ''),
                                            OIDC_FORBIDDEN_TOOLS=str(root / 'forbidden-tools'))
            command = [str(binary), 'build', '--compile', '--config', str(config), '--outfile', str(application)]
            if args.core:
                command = [str(binary), 'build', str(source), '--output', str(application),
                           '--qbe', os.environ['TRBN_QBE'], '--cc', os.environ.get('CC', '/usr/bin/cc'),
                           '--target', target_name() + '-v0']
            built = subprocess.run(command, cwd=root, env=compiler_environment, capture_output=True, text=True, timeout=180)
            receipt['build'] = {'exit': built.returncode, 'stderr': built.stderr}
            assert built.returncode == 0, built.stderr
            receipt['application_bytes'] = application.stat().st_size
            if args.stress:
                assert not args.reference
                import oidc_runtime_probe
                application = oidc_runtime_probe.build(binary, source, args.core, root, compiler_environment)
            assert not (root / 'forbidden-tools').exists(), 'application compilation invoked a reference tool'
            with fixture.Provider(root) as signing:
                with Http2Provider(root / 'http2', {'keys': [signing.jwk]}) as server:
                    receipt['provider'] = server.identity
                    runtime = dict(environment, PATH='', SSL_CERT_FILE=server.endpoints['certificate'],
                                   SSL_CERT_DIR=str(root / 'absent-roots'))
                    # Runtime uses only its linked dependencies and explicit CA.
                    for key in list(runtime):
                        if key.startswith('TRBN_') and key.endswith('_LIB_DIR'):
                            runtime.pop(key)
                    token = 'Bearer ' + signing.token()
                    cases = [('http1-fallback', 'h1', '/keys', True),
                             ('http2-required', 'h2', '/required/keys', True),
                             ('http2-gzip', 'h2', '/gzip/keys', True),
                             ('http2-redirect', 'h2', '/redirect/keys', True),
                             ('http2-body-limit', 'h2', '/oversized/keys', False),
                             ('http2-stream-reset', 'h2', '/reset/keys', False),
                             ('http2-deadline', 'h2', '/stall/keys', False)]
                    for name, protocol, path, success in cases:
                        before = len(server.requests())
                        start = time.monotonic()
                        result = subprocess.run([str(application), signing.url, 'api', server.endpoints[protocol] + path,
                                                 'roles', '0', token], cwd=root, env=runtime,
                                                capture_output=True, text=True, timeout=10)
                        row = {'id': name, 'exit': result.returncode, 'stdout': result.stdout.strip(),
                               'stderr': result.stderr, 'seconds': time.monotonic() - start,
                               'requests': server.requests()[before:]}
                        receipt['cases'].append(row)
                        assert result.returncode == 0, row
                        if success:
                            assert row['stdout'] == 'ok:user:Example:user@example.invalid:user,editor', row
                        elif name == 'http2-body-limit':
                            assert row['stdout'] == 'provider:load OIDC JWKS response is too large', row
                        else:
                            assert row['stdout'].startswith('provider:load OIDC JWKS: '), row
                        requests = [r for r in row['requests'] if r['event'] == 'request']
                        expected = 'HTTP/2.0' if protocol == 'h2' else 'HTTP/1.1'
                        assert requests and all(r['protocol'] == expected for r in requests), row
                        assert len(requests) == (2 if name == 'http2-redirect' else 1), row
                        if name == 'http2-deadline':
                            assert 4.0 <= row['seconds'] < 7.0, row
                            until = time.monotonic() + 1
                            while not any(r['event'] == 'cancelled' for r in row['requests']) and time.monotonic() < until:
                                time.sleep(0.01)
                                row['requests'] = server.requests()[before:]
                            assert any(r['event'] == 'cancelled' for r in row['requests']), row
                        if args.stress:
                            row['ownership'] = oidc_runtime_probe.statistics(result)
                        print(name, 'passed', flush=True)
                    receipt['requests'] = server.requests()
    finally:
        args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')


if __name__ == '__main__':
    main()
