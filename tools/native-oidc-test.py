#!/usr/bin/env python3
"""Differential OIDC contracts using an ephemeral local provider and signing key."""
import argparse
import base64
import hashlib
import gzip
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import select
import socket
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.parse
import zlib


SOURCE = '''import { OidcAuthErrorKind, OidcBearerOptions } from trb/auth/oidc
import { Body, Header, Headers, HttpMethod } from trb/http
import { Context, Next, Request, Response } from trb/web
import trb/web/auth/bearer
import trb/std/process
import trb/std/result

class ProtectedReply implements Next
    def call(context: Context): Response
        case Bearer.principal(context)
        when Result::Ok(value)
            return Response.text(value.subject + ":" + value.roles.join(","))
        when Result::Err(error)
            return Response.text(error.message, 500)
        end
    end
end

def report(args: Array<String>, middleware: Boolean = false)
    mut uri: String? := nil
    if args[2] != ""
        uri = args[2]
    end
    options := OidcBearerOptions.new(issuer: args[0], audience: args[1], jwks_uri: uri,
        roles_claim: args[3], clock_skew_seconds: args[4].to_i())
    mut headers: Array<Header> := []
    args.slice(5...args.size()).each do |value|
        headers.push(Header.new(name: "Authorization", value: value))
    end
    request := Request.new(method: HttpMethod.get(), path: "/", query_string: "",
        headers: Headers.new(headers), body: Body.new("".to_bytes()))
    if middleware
        context := Context.new(request: request, path_parameters: {})
        response := Bearer.authenticate(context, ProtectedReply.new(), options)
        puts(response.status.to_s() + ":" + response.body.bytes().to_s() + ":" +
            response.header_values("www-authenticate").join(",") + ":" +
            response.header_values("content-type").join(","))
        case Bearer.principal(context)
        when Result::Ok(_value)
            puts("principal leaked to original context")
        when Result::Err(error)
            puts(error.message)
        end
        return
    end
    case Bearer.verify(request, options)
    when Result::Ok(value)
        mut name := ""
        mut email := ""
        if value.name != nil
            name = value.name
        end
        if value.email != nil
            email = value.email
        end
        puts("ok:" + value.subject + ":" + name + ":" + email + ":" + value.roles.join(","))
    when Result::Err(error)
        mut kind := "invalid"
        case error.kind
        when OidcAuthErrorKind::MissingCredentials
            kind = "missing"
        when OidcAuthErrorKind::Provider
            kind = "provider"
        when OidcAuthErrorKind::Configuration
            kind = "configuration"
        else
            nil
        end
        puts(kind + ":" + error.message)
    end
end

def main()
    args := Process.argv()
    if args[0] == "matrix"
        mut index := 1
        while index < args.size()
            report(args.slice(index...index + 6))
            index += 6
        end
    elsif args[0] == "middleware"
        report(args.slice(1...args.size()), true)
    elsif args[0] == "sequence"
        base := args.slice(1...6)
        args.slice(6...args.size()).each do |token|
            mut request := base.slice(0...base.size())
            request.push(token)
            report(request)
        end
    else
        report(args)
    end
end
'''


def b64(value):
    return base64.urlsafe_b64encode(value).decode().rstrip('=')


def encoded(value):
    return json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode()


class Provider:
    def __init__(self, root, tls=False):
        self.root = root
        self.requests = []
        self.disconnects = 0
        self.connects = []
        self.lock = threading.Lock()
        self.key = root / 'ephemeral-key.pem'
        subprocess.run(['openssl', 'genrsa', '-out', str(self.key), '2048'], check=True,
                       capture_output=True, timeout=30)
        modulus = subprocess.check_output(['openssl', 'rsa', '-in', str(self.key), '-modulus', '-noout'],
                                          text=True, timeout=10).strip().split('=')[1]
        self.jwk = {'kid': 'first', 'kty': 'RSA', 'use': 'sig', 'alg': 'RS256',
                    'n': b64(bytes.fromhex(modulus)), 'e': 'AQAB'}
        self.rotated_key = root / 'rotated-key.pem'
        subprocess.run(['openssl', 'genrsa', '-out', str(self.rotated_key), '2048'], check=True,
                       capture_output=True, timeout=30)
        rotated_modulus = subprocess.check_output(['openssl', 'rsa', '-in', str(self.rotated_key), '-modulus', '-noout'],
                                                  text=True, timeout=10).strip().split('=')[1]
        self.rotated_jwk = dict(self.jwk, kid='second', n=b64(bytes.fromhex(rotated_modulus)))
        provider = self

        class Handler(BaseHTTPRequestHandler):
            def handle(self):
                try:
                    super().handle()
                except (ConnectionResetError, BrokenPipeError):
                    # A failed TLS verification or the body limit can abort the
                    # connection. Keep this observation without thread tracebacks.
                    with provider.lock:
                        provider.disconnects += 1

            def log_message(self, *_args):
                pass

            def do_GET(self):
                with provider.lock:
                    provider.requests.append(self.path)
                # The same local endpoint can act as a synthetic HTTP proxy.
                path = urllib.parse.urlsplit(self.path).path
                if path == '/slow':
                    time.sleep(6)
                elif path.startswith('/slow-redirect/'):
                    time.sleep(1.8)
                status, body = provider.response(path)
                encoding = None
                if path.startswith('/gzip'):
                    encoding, body = 'gzip', gzip.compress(body)
                    if path == '/gzip-uppercase':
                        encoding = 'GZIP'
                    elif path == '/gzip-truncated':
                        body = body[:-4]
                    elif path == '/gzip-bad-checksum':
                        body = body[:-1] + bytes([body[-1] ^ 1])
                    elif path == '/gzip-long-header':
                        body = body[:3] + b'\x08' + body[4:10] + b'x' * 100000 + b'\0' + body[10:]
                    elif path == '/gzip-concatenated':
                        decoded = gzip.decompress(body)
                        midpoint = len(decoded) // 2
                        body = gzip.compress(decoded[:midpoint]) + gzip.compress(decoded[midpoint:])
                elif path == '/deflate':
                    encoding, body = 'deflate', zlib.compress(body)
                elif path == '/deflate-plaintext':
                    encoding = 'deflate'
                elif path == '/encoding-unknown':
                    encoding = 'br'
                self.send_response(status)
                if path.startswith('/redirect/'):
                    remaining = int(path.removeprefix('/redirect/'))
                    self.send_header('Location', '/keys' if remaining == 1 else '/redirect/' + str(remaining - 1))
                elif path.startswith('/slow-redirect/'):
                    remaining = int(path.removeprefix('/slow-redirect/'))
                    self.send_header('Location', '/keys' if remaining == 1 else '/slow-redirect/' + str(remaining - 1))
                if encoding:
                    self.send_header('Content-Encoding', encoding)
                    if path == '/gzip-duplicate':
                        self.send_header('Content-Encoding', 'deflate')
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                if path == '/status-with-stalled-body':
                    time.sleep(6)
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.server.daemon_threads = True
        self.certificate = root / 'ephemeral-ca.pem'
        if tls:
            subprocess.run(['openssl', 'req', '-new', '-x509', '-key', str(self.key), '-days', '1',
                            '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1,DNS:provider.example.invalid',
                            '-out', str(self.certificate)], check=True, capture_output=True, timeout=30)
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain(self.certificate, self.key)
            self.server.socket = context.wrap_socket(self.server.socket, server_side=True)
        self.url = ('https' if tls else 'http') + '://127.0.0.1:' + str(self.server.server_port)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        class Proxy(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_CONNECT(self):
                # This fixture can only tunnel to its own ephemeral provider.
                expected = 'provider.example.invalid:' + str(provider.server.server_port)
                if self.path != expected:
                    self.send_error(403)
                    return
                with provider.lock:
                    provider.connects.append(self.path)
                with socket.create_connection(('127.0.0.1', provider.server.server_port), timeout=5) as upstream:
                    self.send_response(200, 'Connection established')
                    self.end_headers()
                    self.wfile.flush()
                    try:
                        while True:
                            ready, _, _ = select.select([self.connection, upstream], [], [], 8)
                            if not ready:
                                break
                            for source in ready:
                                data = source.recv(65536)
                                if not data:
                                    return
                                (upstream if source is self.connection else self.connection).sendall(data)
                    except (OSError, TimeoutError):
                        return

        self.proxy = ThreadingHTTPServer(('127.0.0.1', 0), Proxy)
        self.proxy.daemon_threads = True
        self.proxy_url = 'http://127.0.0.1:' + str(self.proxy.server_port)
        self.proxy_thread = threading.Thread(target=self.proxy.serve_forever, daemon=True)

    def response(self, path):
        if path in ['/issuer-one/.well-known/openid-configuration', '/issuer-two/.well-known/openid-configuration']:
            issuer = self.url + path.removesuffix('/.well-known/openid-configuration')
            return 200, encoded({'issuer': issuer, 'jwks_uri': self.url + '/keys'})
        if path == '/.well-known/openid-configuration':
            return 200, encoded({'issuer': self.url, 'jwks_uri': self.url + '/keys'})
        if path == '/discovery-invalid/.well-known/openid-configuration':
            return 200, encoded({'issuer': 'wrong', 'jwks_uri': self.url + '/keys'})
        if path == '/refresh-fails':
            count = sum(urllib.parse.urlsplit(item).path == path for item in self.requests)
            return (200, encoded({'keys': [self.jwk]})) if count == 1 else (503, b'{}')
        if path == '/rotate':
            count = sum(urllib.parse.urlsplit(item).path == path for item in self.requests)
            key = self.rotated_jwk if count > 1 else self.jwk
            return 200, encoded({'keys': [key]})
        if path in ('/keys', '/gzip', '/deflate'):
            return 200, encoded({'keys': [self.jwk]})
        if path.startswith('/key/'):
            key = dict(self.jwk)
            case = path.removeprefix('/key/')
            changes = {'weak': {'n': 'AQAB'}, 'even-exponent': {'e': 'BA'},
                       'bad-modulus': {'n': '!'}, 'bad-exponent': {'e': 'AAAAAAE'},
                       'wrong-use': {'use': 'enc'}, 'wrong-alg': {'alg': 'PS256'},
                       'wrong-type': {'kty': 'EC'}, 'optional': {'use': '', 'alg': ''}}
            key.update(changes[case])
            return 200, encoded({'keys': [key]})
        if path == '/null':
            return 200, b'null'
        if path == '/empty':
            return 200, b'{"keys":[]}'
        if path == '/invalid':
            return 200, b'not json'
        if path == '/large':
            return 200, b' ' * (1024 * 1024 + 1)
        if path == '/gzip-large':
            return 200, b' ' * (1024 * 1024 + 1)
        if path.startswith('/gzip') or path in ('/deflate-plaintext', '/encoding-unknown', '/slow'):
            return 200, encoded({'keys': [self.jwk]})
        if path.startswith('/redirect/') or path.startswith('/slow-redirect/'):
            return 302, b''
        if path == '/case-fold':
            return 200, encoded({'KEYS': [{k.upper(): v for k, v in self.jwk.items()}]})
        return 503, b'{}'

    def token(self, claims=None, header=None, raw_header=None, raw_claims=None):
        payload = {'iss': self.url, 'aud': 'api', 'exp': int(time.time()) + 3600,
                   'sub': 'user', 'name': 'Example', 'email': 'user@example.invalid', 'roles': ['user', 'editor']}
        payload.update(claims or {})
        head = {'alg': 'RS256', 'kid': 'first'}
        head.update(header or {})
        message = (b64(raw_header if raw_header is not None else encoded(head)) + '.' +
                   b64(raw_claims if raw_claims is not None else encoded(payload))).encode()
        key = self.rotated_key if head.get('kid') == 'second' else self.key
        signature = subprocess.check_output(['openssl', 'dgst', '-sha256', '-sign', str(key)],
                                            input=message, timeout=10)
        return message.decode() + '.' + b64(signature)

    def __enter__(self):
        self.thread.start()
        self.proxy_thread.start()
        return self

    def __exit__(self, *_args):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        self.proxy.shutdown()
        self.proxy.server_close()
        self.proxy_thread.join(timeout=5)


def cases(provider):
    valid = provider.token()
    ok = 'ok:user:Example:user@example.invalid:user,editor'
    yield {'id': 'discovery', 'headers': ['Bearer ' + valid], 'expected': ok, 'discovery': True}
    yield {'id': 'explicit-jwks', 'headers': ['Bearer ' + valid], 'expected': ok}
    # The shared JSON scanner may retain exact integers without a raw token.
    # OIDC NumericDate must still accept both integer and fractional claims.
    now = int(time.time())
    for label, claims in [('integer', {'exp': now + 3600, 'nbf': now - 3600}),
                          ('fractional', {'exp': now + 3600.5, 'nbf': now - 3600.5})]:
        yield {'id': 'numeric-date-' + label, 'headers': ['Bearer ' + provider.token(claims)],
               'expected': ok}
    yield {'id': 'missing', 'headers': [], 'expected': 'missing:Authorization header is required'}
    yield {'id': 'duplicate', 'headers': ['Bearer ' + valid] * 2,
           'expected': 'invalid:exactly one Authorization header is required'}
    for value in ['', 'Basic token', 'Bearer', 'Bearer one two']:
        yield {'id': 'header-' + repr(value), 'headers': [value], 'expected': 'invalid:Authorization header must use Bearer'}
    yield {'id': 'header-case-space', 'headers': ['  bEaReR\t' + valid + '  '], 'expected': ok}
    for field, value in [('issuer', ''), ('audience', ''), ('roles', ''), ('skew', '-1')]:
        yield {'id': 'configuration-' + field, field: value, 'headers': [],
               'expected': 'configuration:OIDC bearer configuration is invalid'}
    for token, message in [('a.b', 'JWT must contain three segments'), ('!.e30.AA', 'JWT header is not base64url'),
                           ('e30.!.AA', 'JWT payload is not base64url'), ('e30.e30.!', 'JWT signature is not base64url')]:
        yield {'id': 'decode-' + message, 'headers': ['Bearer ' + token], 'expected': 'invalid:' + message}
    for header in [{'alg': 'none'}, {'alg': 'PS256'}, {'kid': ''}, {'kid': 1}]:
        yield {'id': 'header-json-' + str(header), 'headers': ['Bearer ' + provider.token(header=header)],
               'expected': 'invalid:JWT must use RS256 with a key id'}
    for header in [b'{"ALG":"RS256","KID":"first"}', b'{"alg":"RS256","alg":null,"kid":"first"}']:
        yield {'id': 'header-compat-' + header.decode(), 'headers': ['Bearer ' + provider.token(raw_header=header)], 'expected': ok}
    changes = [({'iss': 'wrong'}, 'JWT issuer does not match'), ({'aud': 'wrong'}, 'JWT audience does not match'),
               ({'exp': 0}, 'JWT is expired'), ({'exp': 'later'}, 'JWT is expired'),
               ({'nbf': int(time.time()) + 7200}, 'JWT is not active'), ({'sub': ''}, 'JWT subject is missing'),
               ({'roles': 'admin'}, 'JWT roles claim is invalid'), ({'roles': [1]}, 'JWT roles claim is invalid')]
    for claims, message in changes:
        yield {'id': 'claims-' + str(claims), 'headers': ['Bearer ' + provider.token(claims)], 'expected': 'invalid:' + message}
    for claims in [{'aud': [1, 'api']}, {'nbf': 'ignored'}, {'unknown': 1e30}]:
        yield {'id': 'claims-compat-' + str(claims), 'headers': ['Bearer ' + provider.token(claims)], 'expected': ok}
    yield {'id': 'signature', 'headers': ['Bearer ' + valid[:-4] + 'AAAA'], 'expected': 'invalid:JWT signature is invalid'}
    yield {'id': 'unknown-kid', 'headers': ['Bearer ' + provider.token(header={'kid': 'unknown'})],
           'expected': 'invalid:JWT signing key was not found', 'requests': 2}
    for key in ['weak', 'even-exponent', 'bad-modulus', 'bad-exponent']:
        yield {'id': 'key-' + key, 'jwks': '/key/' + key, 'headers': ['Bearer ' + valid],
               'expected': 'invalid:OIDC signing key is invalid'}
    for key in ['wrong-use', 'wrong-alg', 'wrong-type']:
        yield {'id': 'key-' + key, 'jwks': '/key/' + key, 'headers': ['Bearer ' + valid],
               'expected': 'invalid:JWT signing key was not found', 'requests': 2}
    for path in ['/key/optional', '/case-fold']:
        yield {'id': path, 'jwks': path, 'headers': ['Bearer ' + valid], 'expected': ok}
    for path, message in [('/null', 'OIDC JWKS response is invalid'), ('/empty', 'OIDC JWKS response is invalid'), ('/invalid', 'load OIDC JWKS response is invalid'),
                           ('/large', 'load OIDC JWKS response is too large'), ('/unavailable', 'load OIDC JWKS: HTTP 503')]:
        yield {'id': path, 'jwks': path, 'headers': ['Bearer ' + valid], 'expected': 'provider:' + message}


def transport_cases(provider):
    valid = provider.token()
    headers = ['Bearer ' + valid]
    ok = 'ok:user:Example:user@example.invalid:user,editor'
    for path in ['/gzip', '/redirect/1', '/redirect/9']:
        yield {'id': path, 'jwks': path, 'headers': headers, 'expected': ok}
    yield {'id': '/gzip-large', 'jwks': '/gzip-large', 'headers': headers,
           'expected': 'provider:load OIDC JWKS response is too large'}
    yield {'id': '/deflate', 'jwks': '/deflate', 'headers': headers,
           'expected': 'provider:load OIDC JWKS response is invalid'}
    yield {'id': '/redirect/10', 'jwks': '/redirect/10', 'headers': headers,
           'expected_prefix': 'provider:load OIDC JWKS: ', 'requests': 10}
    for path in ['/gzip-concatenated', '/gzip-uppercase', '/gzip-duplicate',
                 '/deflate-plaintext', '/encoding-unknown']:
        yield {'id': path, 'jwks': path, 'headers': headers, 'expected': ok}
    for path in ['/gzip-long-header', '/gzip-truncated', '/gzip-bad-checksum', '/slow', '/slow-redirect/4']:
        yield {'id': path, 'jwks': path, 'headers': headers,
               'expected_prefix': 'provider:load OIDC JWKS: ',
               **({'seconds': [4.5, 7.5]} if path.startswith('/slow') else {})}
    yield {'id': '/status-with-stalled-body', 'jwks': '/status-with-stalled-body', 'headers': headers,
           'expected': 'provider:load OIDC JWKS: HTTP 503', 'seconds': [0, 3]}
    if provider.url.startswith('http:'):
        uri = 'http://provider.example.invalid/keys'
        for name, env in [
                ('uppercase', {'HTTP_PROXY': provider.url}),
                ('lowercase', {'http_proxy': provider.url}),
                ('uppercase-priority', {'HTTP_PROXY': provider.url, 'http_proxy': 'http://127.0.0.1:1'}),
                ('irrelevant-no-proxy', {'HTTP_PROXY': provider.url, 'NO_PROXY': 'unrelated.invalid'}),
                ('different-no-proxy-port', {'HTTP_PROXY': provider.url, 'NO_PROXY': 'provider.example.invalid:81'})]:
            yield {'id': 'proxy-' + name, 'uri': uri, 'environment': env,
                   'headers': headers, 'expected': ok, 'requests': 1}
        for name, rule in [('domain', 'provider.example.invalid'), ('port', 'provider.example.invalid:80'),
                           ('suffix', '.example.invalid'), ('wildcard', '*')]:
            yield {'id': 'no-proxy-' + name, 'uri': uri,
                   'environment': {'HTTP_PROXY': provider.url, 'NO_PROXY': rule}, 'headers': headers,
                   'expected_prefix': 'provider:load OIDC JWKS: ', 'requests': 0}
        yield {'id': 'proxy-loopback-bypass', 'headers': headers, 'expected': ok,
               'environment': {'HTTP_PROXY': 'http://127.0.0.1:1'}, 'requests': 1}
        yield {'id': 'proxy-all-ignored', 'headers': headers, 'expected': ok,
               'environment': {'ALL_PROXY': 'http://127.0.0.1:1'}, 'requests': 1}
        yield {'id': 'proxy-cgi', 'headers': headers, 'expected_prefix': 'provider:load OIDC JWKS: ',
               'environment': {'HTTP_PROXY': provider.url, 'REQUEST_METHOD': 'GET'}, 'requests': 0}

    else:
        uri = provider.url.replace('127.0.0.1', 'provider.example.invalid') + '/keys'
        for name, env in [('uppercase', {'HTTPS_PROXY': provider.proxy_url}),
                          ('lowercase', {'https_proxy': provider.proxy_url}),
                          ('uppercase-priority', {'HTTPS_PROXY': provider.proxy_url,
                                                 'https_proxy': 'http://127.0.0.1:1'}),
                          ('irrelevant-no-proxy', {'HTTPS_PROXY': provider.proxy_url,
                                                  'NO_PROXY': 'unrelated.invalid'})]:
            yield {'id': 'https-connect-' + name, 'uri': uri, 'environment': env,
                   'headers': headers, 'expected': ok, 'requests': 1, 'connects': 1}
        for name, env in [('http-only-ignored', {'HTTP_PROXY': provider.proxy_url}),
                          ('no-proxy', {'HTTPS_PROXY': provider.proxy_url, 'NO_PROXY': 'provider.example.invalid'})]:
            yield {'id': 'https-connect-' + name, 'uri': uri, 'environment': env,
                   'headers': headers, 'expected_prefix': 'provider:load OIDC JWKS: ',
                   'requests': 0, 'connects': 0}



def middleware_cases(provider):
    suffix = "\nOIDC principal is not available in this request context"
    yield {'id': 'middleware-principal', 'middleware': True, 'headers': ['Bearer ' + provider.token()],
           'expected': '200:user:user,editor::text/plain; charset=utf-8' + suffix}
    for name, headers, path in [('missing', [], '/keys'), ('invalid', ['Bearer a.b'], '/keys'),
                                 ('provider', ['Bearer ' + provider.token()], '/unavailable')]:
        yield {'id': 'middleware-' + name, 'middleware': True, 'headers': headers, 'jwks': path,
               'expected': '401:{"error":"unauthorized"}:Bearer:application/json; charset=utf-8' + suffix}


def cache_cases(provider):
    valid = 'Bearer ' + provider.token()
    unknown = 'Bearer ' + provider.token(header={'kid': 'unknown'})
    second = 'Bearer ' + provider.token(header={'kid': 'second'})
    ok = 'ok:user:Example:user@example.invalid:user,editor'
    missing = 'invalid:JWT signing key was not found'
    yield {'id': 'cache-jwks-hit', 'sequence': True, 'headers': [valid] * 3,
           'expected': '\n'.join([ok] * 3), 'requests': 1}
    yield {'id': 'cache-discovery-hit', 'sequence': True, 'discovery': True, 'headers': [valid] * 3,
           'expected': '\n'.join([ok] * 3), 'requests': 2}
    yield {'id': 'cache-refresh-throttle', 'sequence': True, 'headers': [valid, unknown, unknown],
           'expected': '\n'.join([ok, missing, missing]), 'requests': 2}
    yield {'id': 'cache-refresh-failure', 'sequence': True, 'jwks': '/refresh-fails',
           'headers': [valid, unknown, valid, unknown],
           'expected': '\n'.join([ok, 'provider:load OIDC JWKS: HTTP 503', ok, missing]), 'requests': 2}
    yield {'id': 'cache-key-rotation', 'sequence': True, 'jwks': '/rotate',
           'headers': [valid, second, second], 'expected': '\n'.join([ok] * 3), 'requests': 2}
    def request(issuer, audience, uri, token):
        return [issuer, audience, uri, 'roles', '0', token]
    yield {'id': 'cache-uri-separation', 'headers': [], 'matrix': [
        request(provider.url, 'api', provider.url + '/keys', valid),
        request(provider.url, 'api', provider.url + '/empty', valid),
        request(provider.url, 'api', provider.url + '/keys', valid)],
        'expected': '\n'.join([ok, 'provider:OIDC JWKS response is invalid', ok]), 'requests': 2}
    yield {'id': 'cache-audience-rechecked', 'headers': [], 'matrix': [
        request(provider.url, 'api', provider.url + '/keys', valid),
        request(provider.url, 'another', provider.url + '/keys', valid)],
        'expected': ok + '\ninvalid:JWT audience does not match', 'requests': 1}
    first_issuer, second_issuer = provider.url + '/issuer-one', provider.url + '/issuer-two'
    first_token = 'Bearer ' + provider.token({'iss': first_issuer})
    second_token = 'Bearer ' + provider.token({'iss': second_issuer})
    yield {'id': 'cache-issuer-separation', 'headers': [], 'matrix': [
        request(first_issuer, 'api', '', first_token), request(second_issuer, 'api', '', second_token),
        request(first_issuer, 'api', '', first_token)], 'expected': '\n'.join([ok] * 3), 'requests': 3}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--core', action='store_true', help='development check with an ordinary Native core')
    parser.add_argument('--tls', action='store_true', help='use an ephemeral HTTPS provider and explicit CA file')
    parser.add_argument('--trust', choices=['file', 'directory', 'untrusted'], default='file')
    parser.add_argument('--cli-paths', action='store_true', help='check project/run and retained REPL behavior')
    parser.add_argument('--stress', action='store_true', help='instrument GC and foreign-resource failure cleanup')
    parser.add_argument('--transport', action='store_true', help='exercise redirects, encoding and proxy policy')
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve()
    repository = Path(__file__).resolve().parent.parent
    receipt = {'reference_revision': (repository / 'TYPE_RB_REVISION').read_text().strip(),
               'mode': 'reference' if args.reference else ('native-core' if args.core else 'native'),
               'transport': 'https-' + args.trust if args.tls else 'http',
               'compiler_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
               'source_sha256': hashlib.sha256(SOURCE.encode()).hexdigest(), 'cases': []}
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb')
    for name in ('http_proxy', 'https_proxy', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'all_proxy',
                 'NO_PROXY', 'no_proxy', 'REQUEST_METHOD'):
        environment.pop(name, None)
    try:
        with tempfile.TemporaryDirectory(prefix='oidc-contracts-') as directory:
            root = Path(directory)
            (root / 'src').mkdir()
            (root / 'src/main.trb').write_text(SOURCE)
            config = root / 'trbconfig.jsonc'
            config.write_text(json.dumps({'name': 'oidc-contracts', 'mode': 'go' if args.reference else 'trb',
                                         'sourceDir': 'src', 'go': {'module': 'example.com/oidc-contracts'}}))
            application = root / 'application'
            command = [str(binary), 'build', '--compile', '--config', str(config), '--outfile', str(application)]
            if args.core:
                from network_artifacts import target_name
                command = [str(binary), 'build', str(root / 'src/main.trb'), '--output', str(application),
                           '--qbe', os.environ['TRBN_QBE'], '--cc', os.environ.get('CC', '/usr/bin/cc'),
                           '--target', target_name() + '-v0']
            built = subprocess.run(command,
                                   cwd=root, env=environment, capture_output=True, text=True, timeout=180)
            receipt['build'] = {'exit': built.returncode, 'stdout': built.stdout, 'stderr': built.stderr}
            assert built.returncode == 0, built.stderr
            receipt['application_bytes'] = application.stat().st_size
            receipt['godebug'] = environment.get('GODEBUG', '')
            if args.reference:
                receipt['go_toolchain'] = subprocess.check_output(['go', 'version'], text=True).strip()
                receipt['generated_go_versions'] = sorted({line for module in root.rglob('go.mod')
                                                          for line in module.read_text().splitlines()
                                                          if line.startswith('go ')})
                assert receipt['generated_go_versions'], 'generated module version was not recorded'

            if args.stress:
                assert not args.reference, 'foreign instrumentation is Native-only'
                import oidc_runtime_probe
                application = oidc_runtime_probe.build(binary, root / 'src/main.trb', args.core, root, environment)
                environment['TYPE_RB_NATIVE_RUNTIME_STATS'] = '1'
                blocked_config = root / 'blocked-openssl-config'
                os.mkfifo(blocked_config)
                environment.update(OPENSSL_CONF=str(blocked_config), OPENSSL_MODULES=str(root / 'absent-modules'))
            with Provider(root, tls=args.tls) as provider:
                environment.pop('SSL_CERT_FILE', None)
                environment.pop('SSL_CERT_DIR', None)
                if args.tls:
                    if args.trust == 'file':
                        environment['SSL_CERT_FILE'] = str(provider.certificate)
                    elif args.trust == 'directory':
                        certificates = root / 'certificates'
                        certificates.mkdir()
                        (certificates / 'ordinary-name.pem').write_bytes(provider.certificate.read_bytes())
                        (certificates / 'not-a-certificate').write_text('ignored data')
                        environment['SSL_CERT_DIR'] = str(root / 'missing') + ':' + str(certificates)
                selected = list(cases(provider)) + list(cache_cases(provider)) + list(middleware_cases(provider))
                if args.transport:
                    selected.extend(transport_cases(provider))
                if args.tls and args.trust == 'untrusted':
                    selected = [{'id': 'untrusted-ca', 'headers': ['Bearer ' + provider.token()],
                                 'expected_prefix': 'provider:load OIDC JWKS: ', 'requests': 0}]
                elif args.tls:
                    selected.append({'id': 'wrong-certificate-hostname', 'headers': ['Bearer ' + provider.token()],
                                     'uri': provider.url.replace('127.0.0.1', 'localhost') + '/keys',
                                     'expected_prefix': 'provider:load OIDC JWKS: ', 'requests': 0})
                if args.stress:
                    selected = [case for case in selected if case['id'] in {
                        'explicit-jwks', 'middleware-principal', 'middleware-missing', 'cache-refresh-failure',
                        'cache-key-rotation', 'cache-uri-separation', 'cache-issuer-separation', 'cache-audience-rechecked', '/gzip', '/gzip-truncated', '/large'}]
                    valid = 'Bearer ' + provider.token()
                    unknown = 'Bearer ' + provider.token(header={'kid': 'unknown'})
                    ok = 'ok:user:Example:user@example.invalid:user,editor'
                    for name, discovery, after, seconds, tokens, expected, requests in [
                            ('keys-expiry', False, 2, 301, [valid] * 3, [ok] * 3, 2),
                            ('discovery-expiry', True, 3, 301, [valid] * 3, [ok] * 3, 4),
                            ('refresh-window', False, 4, 31, [valid, unknown, unknown],
                             [ok, 'invalid:JWT signing key was not found', 'invalid:JWT signing key was not found'], 3)]:
                        selected.append({'id': 'clock-' + name, 'sequence': True, 'discovery': discovery,
                                         'headers': tokens, 'expected': '\n'.join(expected), 'requests': requests,
                                         'environment': {'OIDC_TEST_CLOCK_AFTER': str(after),
                                                         'OIDC_TEST_CLOCK_SECONDS': str(seconds)}})
                    for fault in ['uidna_openUTS46_78', 'malloc', 'curl_url', 'url-get', 'curl_easy_init', 'ssl-init', 'perform',
                                  'BN_bin2bn', 'OSSL_PARAM_BLD_new', 'OSSL_PARAM_BLD_to_param',
                                  'EVP_PKEY_CTX_new_from_name', 'EVP_MD_CTX_new', 'fromdata', 'verify', 'crypto-init',
                                  'inflate-init']:
                        rsa = fault in ['BN_bin2bn', 'OSSL_PARAM_BLD_new', 'OSSL_PARAM_BLD_to_param',
                                        'EVP_PKEY_CTX_new_from_name', 'EVP_MD_CTX_new', 'fromdata', 'verify', 'crypto-init']
                        selected.append({'id': 'failure-' + fault, 'headers': ['Bearer ' + provider.token()],
                                         'jwks': '/gzip' if fault == 'inflate-init' else '/keys',
                                         'environment': {'OIDC_TEST_FAULT': fault},
                                         'expected_prefix': 'invalid:JWT signature is invalid' if rsa else 'provider:load OIDC JWKS: '})
                for case in selected:
                    argv = [case.get('issuer', provider.url), case.get('audience', 'api'),
                            '' if case.get('discovery') else case.get('uri', provider.url + case.get('jwks', '/keys')),
                            case.get('roles', 'roles'), case.get('skew', '0'), *case['headers']]
                    if case.get('matrix'):
                        argv = ['matrix', *(argument for request in case['matrix'] for argument in request)]
                    elif case.get('sequence'):
                        argv.insert(0, 'sequence')
                    elif case.get('middleware'):
                        argv.insert(0, 'middleware')
                    before = len(provider.requests)
                    connects_before = len(provider.connects)
                    case_environment = dict(environment, **case.get('environment', {}))
                    started = time.monotonic()
                    result = subprocess.run([str(application), *argv], capture_output=True, text=True,
                                            cwd=root, env=case_environment, timeout=10)
                    elapsed = time.monotonic() - started
                    actual = result.stdout.strip()
                    requests = len(provider.requests) - before
                    matches = (actual.startswith(case['expected_prefix']) if 'expected_prefix' in case
                               else actual == case['expected'])
                    row = {'id': case['id'], 'expected': case['expected_prefix'] + '*' if 'expected_prefix' in case else case['expected'], 'actual': actual,
                           'exit': result.returncode, 'stderr': result.stderr, 'requests': requests, 'connects': len(provider.connects) - connects_before, 'seconds': elapsed}
                    row['passed'] = result.returncode == 0 and matches and (
                        'requests' not in case or requests == case['requests']) and (
                        'connects' not in case or row['connects'] == case['connects']) and (
                        'seconds' not in case or case['seconds'][0] <= elapsed <= case['seconds'][1])
                    if args.stress:
                        row['resources'] = oidc_runtime_probe.statistics(result)
                        gc = {line.split(',')[1]: int(line.split(',')[2]) for line in result.stderr.splitlines()
                              if line.startswith('type-rb-native-gc-stat-v1,')}
                        row['gc'] = gc
                        row['passed'] = row['passed'] and gc['collections'] > 0 and gc['live-bytes'] == 0
                    receipt['cases'].append(row)
                if args.cli_paths:
                    assert not args.core and not args.stress
                    paths = []
                    for target in [[str(root / 'src/main.trb')], ['--config', str(config)]]:
                        checked = subprocess.run([str(binary), 'check', *target], cwd=root, env=environment,
                                                 capture_output=True, text=True, timeout=180)
                        assert checked.returncode == 0, checked.stderr
                        paths.append('source check' if len(target) == 1 else 'project check')
                    argv = [provider.url, 'api', provider.url + '/keys', 'roles', '0', 'Bearer ' + provider.token()]
                    executed = subprocess.run([str(binary), 'run', '--config', str(config), '--', *argv],
                                              cwd=root, env=environment, capture_output=True, text=True, timeout=180)
                    assert executed.returncode == 0 and executed.stdout.strip() == 'ok:user:Example:user@example.invalid:user,editor', executed
                    paths.append('run')
                    for arguments in [argv, ['', '', '', '', '-1']]:
                        before = len(provider.requests)
                        repl_input = 'import { report } from main\nreport(' + json.dumps(arguments) + ')\nputs(7)\n:quit\n'
                        repl = subprocess.run([str(binary), 'repl', '--config', str(config)], input=repl_input,
                                              cwd=root, env=dict(environment, TRBN_HISTORY=str(root / 'history')),
                                              capture_output=True, text=True, timeout=30)
                        assert 'trb.internal.auth.oidc.verify_bearer' in repl.stderr and 'no REPL runtime adapter' in repl.stderr, repl
                        assert repl.stdout.strip() == '7' and len(provider.requests) == before, repl
                    receipt['paths'] = paths + ['retained REPL rejection before validation/network']
                receipt['provider_disconnects'] = provider.disconnects
            failures = [case for case in receipt['cases'] if not case['passed']]
            assert not failures, failures
    finally:
        args.receipt.parent.mkdir(parents=True, exist_ok=True)
        args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
    print(str(len(receipt['cases'])) + ' OIDC contracts passed')


if __name__ == '__main__':
    main()
