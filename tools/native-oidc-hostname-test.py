#!/usr/bin/env python3
"""Compare real OIDC host, proxy and TLS behavior with the accepted reference."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
import os
from pathlib import Path
import select
import socket
import ssl
import subprocess
import tempfile
import threading
import urllib.parse

from network_artifacts import target_name

spec = importlib.util.spec_from_file_location('oidc_fixture', Path(__file__).with_name('native-oidc-test.py'))
fixture = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture)
ISSUER = 'https://ＢＵＣＨＥＲ.example.invalid'
OK = 'ok:user:Example:user@example.invalid:user,editor'
VALID = [
    ('ＢＵＣＨＥＲ.example.invalid', 'bucher.example.invalid'),
    ('BÜCHER.example.invalid', 'xn--bcher-kva.example.invalid'),
    ('bu\u0308cher.example.invalid', 'xn--bcher-kva.example.invalid'),
    ('XN--BCHER-KVA.example.invalid', 'xn--bcher-kva.example.invalid'),
    ('faß.example.invalid', 'xn--fa-hia.example.invalid'),
    ('ς.example.invalid', 'xn--3xa.example.invalid'),
    ('例え。example.invalid', 'xn--r8jz45g.example.invalid'),
    ('Ａ.example.invalid.', 'a.example.invalid.'),
    ('bücher.example.invalid:8080', 'xn--bcher-kva.example.invalid:8080'),
    ('%EF%BC%A2%EF%BC%B5%EF%BC%A3%EF%BC%A8%EF%BC%A5%EF%BC%B2.example.invalid', 'bucher.example.invalid'),
    ('Ａ.example.invalid。', 'a.example.invalid.'),
    ('[2001:0DB8:0:0:0:0:0:1]:8080', '[2001:db8::1]:8080'),
    ('[2001:0:0:1:0:0:1:1]', '[2001::1:0:0:1:1]'),
    ('[::FFFF:192.0.2.1]', '[::ffff:192.0.2.1]'),
]
INVALID = ['bad_name.example.invalid', '-bad.example.invalid', 'ab--cd.example.invalid',
           'bad..example.invalid', 'xn--.example.invalid', 'a\u200cb.example.invalid',
           'אa.example.invalid', '\u0301a.example.invalid', 'a' * 64 + '.example.invalid',
           '127.1', '0x7f000001', '0177.0.0.1', 'example.123', '[127.0.0.1]',
           '[fe80::1%25lo0]', '%62ucher.example.invalid', '%FF.example.invalid',
           '[2001:db8::1]\t:8080', '[2001:db8::1]\n:8080', '[2001:db8::1]\\']


class HostProvider:
    """A loopback-only proxy records authorities and tunnels to its own TLS peer."""
    def __init__(self, root, signing):
        self.records = []
        self.lock = threading.Lock()
        self.root = root
        self.certificate = root / 'hostname-ca.pem'
        subprocess.run(['openssl', 'req', '-new', '-x509', '-key', str(signing.key), '-days', '1',
                        '-subj', '/CN=bucher.example.invalid', '-addext',
                        'subjectAltName=DNS:bucher.example.invalid,DNS:xn--bcher-kva.example.invalid',
                        '-out', str(self.certificate)], check=True, capture_output=True, timeout=30)
        provider = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                parsed = urllib.parse.urlsplit(self.path)
                path = parsed.path + ('?' + parsed.query if parsed.query else '')
                provider.record('request', self.headers['Host'] + ' ' + path)
                body = fixture.encoded({'keys': [signing.jwk]})
                redirects = {
                    '/redirect': 'http://%EF%BC%A2%EF%BC%B5%EF%BC%A3%EF%BC%A8%EF%BC%A5%EF%BC%B2.example.invalid/keys?after=%2f',
                    '/invalid-redirect': 'http://bad_name.example.invalid/keys',
                    '/invalid-utf8-redirect': 'http://ÿ.example.invalid/keys',
                    '/numeric-redirect': 'http://127.1:' + str(provider.proxy.server_port) + '/keys',
                    '/authority-redirect': '//%EF%BC%A2%EF%BC%B5%EF%BC%A3%EF%BC%A8%EF%BC%A5%EF%BC%B2.example.invalid/keys',
                    '/encoded-ascii-redirect': 'http://%62ucher.example.invalid/keys',
                    '/loop': '/loop',
                    '/relative': '../keys',
                    '/numeric-authority-redirect': '//127.1:' + str(provider.proxy.server_port) + '/keys',
                }
                status = 200
                if parsed.path in redirects:
                    status, body = 302, b''
                elif parsed.path == '/.well-known/openid-configuration':
                    body = fixture.encoded({'issuer': ISSUER, 'jwks_uri': 'https://bücher.example.invalid:8443/keys'})
                self.send_response(status)
                if status == 302:
                    self.send_header('Location', redirects[parsed.path])
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_CONNECT(self):
                provider.record('connect', self.path)
                # This is a synthetic endpoint: never connect to the requested host.
                with socket.create_connection(('127.0.0.1', provider.tls.server_port), timeout=5) as peer:
                    self.send_response(200, 'Connection established')
                    self.end_headers()
                    self.wfile.flush()
                    try:
                        while True:
                            ready, _, _ = select.select([self.connection, peer], [], [], 5)
                            if not ready:
                                return
                            for source in ready:
                                data = source.recv(65536)
                                if not data:
                                    return
                                (peer if source is self.connection else self.connection).sendall(data)
                    except OSError:
                        return

        self.tls = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.load_cert_chain(self.certificate, signing.key)
        context.set_servername_callback(lambda _socket, name, _context: provider.record('sni', name))
        self.tls.socket = context.wrap_socket(self.tls.socket, server_side=True)
        self.proxy = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.servers = [self.tls, self.proxy]
        self.threads = [threading.Thread(target=server.serve_forever, daemon=True) for server in self.servers]
        self.proxy_url = 'http://127.0.0.1:' + str(self.proxy.server_port)

    def record(self, kind, value):
        with self.lock:
            self.records.append({'kind': kind, 'value': value})

    def __enter__(self):
        for thread in self.threads:
            thread.start()
        return self

    def __exit__(self, *_args):
        for server, thread in zip(self.servers, self.threads):
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)
            assert not thread.is_alive()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--stress', action='store_true')
    parser.add_argument('--receipt', type=Path, required=True)
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
        with tempfile.TemporaryDirectory(prefix='oidc-hostname-contracts-') as temporary:
            root = Path(temporary)
            (root / 'src').mkdir()
            source = root / 'src/main.trb'
            source.write_text(fixture.SOURCE)
            config = root / 'trbconfig.jsonc'
            config.write_text(json.dumps({'name': 'oidc-hostnames', 'mode': 'go' if args.reference else 'trb',
                                         'sourceDir': 'src', 'go': {'module': 'example.com/oidc-hostnames'}}))
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
            if args.stress:
                assert not args.reference
                import oidc_runtime_probe
                application = oidc_runtime_probe.build(binary, source, args.core, root, compiler_environment)
            assert not (root / 'forbidden-tools').exists(), 'application compilation invoked a reference tool'
            with fixture.Provider(root) as signing, HostProvider(root, signing) as provider:
                runtime = dict(environment, PATH='', HTTP_PROXY=provider.proxy_url, HTTPS_PROXY=provider.proxy_url,
                               SSL_CERT_FILE=str(provider.certificate), SSL_CERT_DIR=str(root / 'absent-roots'))
                for key in list(runtime):
                    if key.startswith('TRBN_') and key.endswith('_LIB_DIR'):
                        runtime.pop(key)
                token = 'Bearer ' + signing.token({'iss': ISSUER})

                def run(name, uri, expected=OK, observations=None, issuer=ISSUER, bearer=token, extra=None):
                    with provider.lock:
                        provider.records.clear()
                    result = subprocess.run([str(application), issuer, 'api', uri, 'roles', '0', bearer],
                                            cwd=root, env=dict(runtime, **(extra or {})), capture_output=True,
                                            text=True, timeout=12)
                    with provider.lock:
                        records = provider.records.copy()
                    row = {'id': name, 'uri': uri, 'exit': result.returncode, 'stdout': result.stdout.strip(),
                           'stderr': result.stderr, 'observations': records}
                    receipt['cases'].append(row)
                    assert result.returncode == 0, row
                    if expected == 'provider:':
                        assert row['stdout'].startswith(expected), row
                    else:
                        assert row['stdout'] == expected, row
                    if observations is not None:
                        assert records == [{'kind': k, 'value': v} for k, v in observations], row
                    if args.stress:
                        row['ownership'] = oidc_runtime_probe.statistics(result)
                    print(name, 'passed', flush=True)
                    return row

                for index, (host, ascii) in enumerate(VALID):
                    run('valid-' + str(index), 'http://' + host + '/keys%2fpart?q=%2f',
                        observations=[('request', ascii + ' /keys%2fpart?q=%2f')])
                for index, host in enumerate(INVALID):
                    run('invalid-' + str(index), 'http://' + host + '/keys', 'provider:', [])
                run('redirect', 'http://example.invalid/redirect', observations=[
                    ('request', 'example.invalid /redirect'), ('request', 'bucher.example.invalid /keys?after=%2f')])
                for name in ('invalid-redirect', 'numeric-redirect', 'encoded-ascii-redirect', 'numeric-authority-redirect', 'invalid-utf8-redirect'):
                    run(name, 'http://example.invalid/' + name, 'provider:', [('request', 'example.invalid /' + name)])
                run('authority-redirect', 'http://example.invalid/authority-redirect', observations=[
                    ('request', 'example.invalid /authority-redirect'), ('request', 'bucher.example.invalid /keys')])
                run('relative', 'http://example.invalid/relative', observations=[
                    ('request', 'example.invalid /relative'), ('request', 'example.invalid /keys')])
                run('redirect-limit', 'http://example.invalid/loop', 'provider:', [('request', 'example.invalid /loop')] * 10)
                run('exact-token-issuer', 'http://bücher.example.invalid/keys', 'invalid:JWT issuer does not match',
                    bearer='Bearer ' + signing.token({'iss': 'https://bucher.example.invalid'}))
                run('ipv4-mapping', signing.url.replace('127.0.0.1', '１２７。０。０。１') + '/keys', observations=[])
                run('tls-host', 'https://ＢＵＣＨＥＲ.example.invalid:8443/keys', observations=[
                    ('connect', 'bucher.example.invalid:8443'), ('sni', 'bucher.example.invalid'),
                    ('request', 'bucher.example.invalid:8443 /keys')])
                run('certificate-host', 'https://wrong.example.invalid:8443/keys', 'provider:', [
                    ('connect', 'wrong.example.invalid:8443'), ('sni', 'wrong.example.invalid')])
                discovery = [('connect', 'bucher.example.invalid:443'), ('sni', 'bucher.example.invalid'),
                             ('request', 'bucher.example.invalid /.well-known/openid-configuration')]
                run('discovery', '', observations=discovery + [
                    ('connect', 'xn--bcher-kva.example.invalid:8443'), ('sni', 'xn--bcher-kva.example.invalid'),
                    ('request', 'xn--bcher-kva.example.invalid:8443 /keys')])
                run('exact-discovery-issuer', '', 'provider:OIDC discovery response is invalid', discovery,
                    issuer='https://bucher.example.invalid')
                # Reserved .invalid names cannot resolve; a bypass must cause no proxy request.
                for index, pattern in enumerate(['bücher.example.invalid', '*.BÜCHER.example.invalid', '.bücher.example.invalid']):
                    host = 'bücher.example.invalid' if index == 0 else 'sub.bücher.example.invalid'
                    run('unicode-no-proxy-' + str(index), 'http://' + host + '/keys', 'provider:', [], extra={'NO_PROXY': pattern})
    finally:
        args.receipt.write_text(json.dumps(receipt, indent=2, ensure_ascii=False) + '\n')


if __name__ == '__main__':
    main()
