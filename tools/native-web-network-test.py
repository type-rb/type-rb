#!/usr/bin/env python3
"""Real loopback HTTP acceptance for the Native Web project provider."""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time

CLIENT_HOST = '127.0.0.1'
QBE_TARGETS = {
    'darwin-arm64-v0': 'arm64_apple',
    'linux-arm64-v0': 'arm64',
    'linux-amd64-v0': 'amd64_sysv',
}

FILES = {
    'main.trb': '''import trb/web
import trb/std/process
def main()
mut host := "127.0.0.1"
mut port := PORT
mut limit := 32
mut timeout := 250
arguments := Process.argv()
if arguments.size() > 0
case arguments[0]
when "host"
host = ""
when "port"
port = 0
when "limit"
limit = 0
when "timeout"
timeout = -1
end
end
Web.serve(Web::ServerConfig.new(host: host, port: port, body_limit_bytes: limit, shutdown_timeout_milliseconds: timeout))
return
end
''',
    'routes/index.trb': '''import { Context, Response } from trb/web
def get(_context: Context): Response
return Response.text("hello")
end
''',
    'routes/echo.trb': '''import { Context, Response } from trb/web
def post(context: Context): Response
return Response.new(status: 200, headers: context.request.headers, body: context.request.body)
end
''',
    'routes/bad.trb': '''import { Context, Response } from trb/web
def get(context: Context): Response
zero := context.request.body.size()
puts(1 / zero)
return Response.text("unreachable")
end
''',
    'routes/informational.trb': '''import { Context, Response } from trb/web
def get(_context: Context): Response
return Response.text("final body", 103)
end
''',
    'routes/no_content.trb': '''import { Context, Response } from trb/web
def get(_context: Context): Response
return Response.text("must not be sent", 204)
end
''',
    'routes/items/_middleware.trb': '''import { Context, Next, Response } from trb/web
def call(context: Context, next_handler: Next): Response
return next_handler.call(context).with_header("x-directory", "items")
end
''',
    'routes/meta.trb': """import { Context, Response } from trb/web
def post(context: Context): Response
request := context.request
return Response.text(request.method.to_s() + "|" + request.path + "|" + request.query_string + "|" + request.headers.key?("host").to_s() + "|" + request.headers.key?("transfer-encoding").to_s() + "|" + request.headers.key?("x-trailer").to_s() + "|" + request.headers.key?("content-length").to_s())
end
""",
    'routes/large.trb': """import { Context, Response } from trb/web
def get(_context: Context): Response
mut value := "x"
while value.to_bytes().size() < 8388608
value += value
end
return Response.text(value)
end
""",
    'routes/invalid_response.trb': 'import { Context, Response } from trb/web\ndef get(_context: Context): Response\nreturn Response.text("bad").with_header("x-bad", "bad\\r\\nheader")\nend\n',
    'routes/_middleware.trb': '''import { Context, Next, Response } from trb/web
def call(context: Context, next_handler: Next): Response
return next_handler.call(context).with_header("x-root", "resumed").with_header("x-body-size", context.request.body.size().to_s())
end
''',
    'routes/items/[id].trb': '''import { Context, Response } from trb/web
import trb/std/result
record Params
id: Integer
end
record Output
id: Integer
end
def get(context: Context): Response
case context.params<Params>()
when Result::Ok(params)
return Response.json<Output>(Output.new(id: params.id))
when Result::Err(_error)
return Response.text("bad id", 400)
end
end
''',
}


def free_port():
    family, kind, protocol, _, address = socket.getaddrinfo(CLIENT_HOST, 0, type=socket.SOCK_STREAM)[0]
    with socket.socket(family, kind, protocol) as s:
        s.bind(address)
        return s.getsockname()[1]


class Wire:
    def __init__(self, port):
        self.socket = socket.create_connection((CLIENT_HOST, port), timeout=3)
        self.file = self.socket.makefile('rb')

    def send(self, data):
        self.socket.sendall(data)

    def response(self, head=False):
        status = self.file.readline(4096)
        assert status.startswith(b'HTTP/1.'), status
        code = int(status.split()[1])
        headers = {}
        while True:
            line = self.file.readline(65536)
            assert line, 'unexpected EOF in headers'
            if line == b'\r\n':
                break
            name, value = line.rstrip(b'\r\n').split(b':', 1)
            headers.setdefault(name.lower(), []).append(value.strip())
        length = int(headers.get(b'content-length', [b'0'])[0])
        body = b'' if head or code < 200 or code in (204, 304) else self.file.read(length)
        if not head and code >= 200 and code not in (204, 304):
            assert len(body) == length, (code, headers, body)
        return code, headers, body

    def close(self):
        self.file.close()
        self.socket.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()


def run_server(program, port, environment):
    process = subprocess.Popen([str(program)], env=environment, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE)
    for _ in range(200):
        if process.poll() is not None:
            raise AssertionError(('server exited before listening', process.communicate()))
        try:
            with socket.create_connection((CLIENT_HOST, port), timeout=.05):
                return process
        except OSError:
            time.sleep(.025)
    process.kill()
    raise AssertionError(('server did not listen', process.communicate()))


def exercise(program, port, environment, reference=False, dynamic=False):
    if dynamic:
        missing = dict(environment, TRBN_NETWORK_LIB_DIR=str(program.parent / 'missing-libraries'))
        result = subprocess.run([str(program)], env=missing, capture_output=True, timeout=5)
        assert result.returncode != 0 and b'TRBN_NETWORK_LIB_DIR' in result.stderr, result
        if environment.get('TRBN_NETWORK_TEST_ALLOCATIONS') == '1':
            assert b'network-test-live-allocations=0\n' in result.stderr, result.stderr
    for argument, diagnostic in [('host', b'ServerConfig.host'), ('port', b'ServerConfig.port'),
                                 ('limit', b'ServerConfig.body_limit_bytes'), ('timeout', b'ServerConfig.shutdown_timeout_milliseconds')]:
        result = subprocess.run([str(program), argument], env=environment, capture_output=True, timeout=5)
        assert result.returncode != 0 and diagnostic in result.stderr, result
        if environment.get('TRBN_NETWORK_TEST_ALLOCATIONS') == '1':
            assert b'network-test-live-allocations=0\n' in result.stderr, result.stderr
    process = run_server(program, port, environment)
    passed = ['configuration errors']
    try:
        with Wire(port) as wire:
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            code, headers, body = wire.response()
            assert (code, body) == (200, b'hello'), (code, body)
            assert headers[b'x-root'] == [b'resumed'], headers
            passed += ['static route', 'root middleware']
            wire.send(b'GET /items/42 HTTP/1.1\r\nHost: localhost\r\n\r\n')
            code, headers, body = wire.response()
            assert code == 200 and json.loads(body) == {'id': 42}, (code, body)
            assert headers[b'x-directory'] == [b'items']
            passed += ['typed input and JSON', 'keepalive']
            wire.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\n\r\n\x00\xff\xc0\x80')
            assert wire.response()[2] == b'\x00\xff\xc0\x80'
            passed.append('binary body')
            wire.send(b'POST /meta?q=a%2Bb&x=2 HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\nTrailer: X-Trailer\r\n\r\n0\r\nX-Trailer: hidden\r\n\r\n')
            assert wire.response()[2] == b'POST|/meta|q=a%2Bb&x=2|false|false|false|false'
            passed.append('request metadata and trailer boundary')
            wire.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 3\r\nExpect: 100-continue\r\n\r\n')
            assert wire.response()[0] == 100
            wire.send(b'abc')
            assert wire.response()[2] == b'abc'
            wire.send(b'CUSTOM / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 405
            wire.send(b'get / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[2] == b'hello'
            wire.send(b'GET http://localhost/ HTTP/1.1\r\nHost: localhost\r\nConnection: upgrade\r\nUpgrade: example\r\n\r\n')
            assert wire.response()[2] == b'hello'
            passed.append('Expect, extension methods and ordinary upgrade headers')
            wire.send(b'GET /invalid_response HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 500

            for fragment in [b'POST /ec', b'ho HTTP/1.1\r\nHo', b'st: localhost\r\nTransfer-Encoding: chunked\r\n\r\n', b'2\r\na', b'b\r\n1\r\nc\r\n0\r\n\r\n']:
                wire.send(fragment)
                time.sleep(.005)
            assert wire.response()[2] == b'abc'
            passed.append('fragmented chunked request')
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n' * 3)
            for _ in range(3):
                assert wire.response()[2] == b'hello'
            passed.append('pipeline')
            wire.send(b'HEAD / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response(head=True)[0] == 200
            wire.send(b'OPTIONS / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 204
            wire.send(b'GET /no_content HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 204
            wire.send(b'GET /informational HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 103
            code, _, body = wire.response()
            assert (code, body) == (200, b'final body'), (code, body)
            passed.append('HEAD and OPTIONS')
            wire.send(b'GET /bad HTTP/1.1\r\nHost: localhost\r\n\r\n')
            code, headers, _ = wire.response()
            assert code == 500 and headers[b'x-root'] == [b'resumed']
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 200
            passed.append('handler recovery and continuation')
        with Wire(port) as wire:
            wire.send(b'GET / HTTP/1.0\r\n\r\n')
            assert wire.response()[2] == b'hello'
        with Wire(port) as wire:
            wire.send(b'GET / HTTP/1.0\r\nConnection: keep-alive\r\n\r\n')
            code, headers, body = wire.response()
            assert code == 200 and body == b'hello' and headers[b'connection'] == [b'keep-alive']
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
            code, headers, body = wire.response()
            assert code == 200 and headers[b'connection'] == [b'close']
            assert wire.file.read(1) == b''
        with Wire(port) as wire:
            wire.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 33\r\n\r\n' + b'a' * 33)
            code, headers, _ = wire.response()
            assert code == 413 and headers[b'x-root'] == [b'resumed'], code
            assert headers[b'x-body-size'] == [b'32'], headers
            passed.append('custom body limit')
        # Match the pinned server's chunked precedence; discard Content-Length
        # before application middleware and preserve the next request boundary.
        with Wire(port) as wire:
            wire.send(b'POST /meta HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n')
            code, headers, body = wire.response()
            assert code == 200 and headers[b'x-root'] == [b'resumed'], headers
            assert body == b'POST|/meta||false|false|false|false', body
            assert headers[b'x-body-size'] == [b'3'], headers
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[2] == b'hello'
        for bad, dispatched in [
            (b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1\r\nContent-Length: 2\r\n\r\nab', False),
            (b'GET / HTTP/1.1\r\nHost: localhost\r\nBad Header: no\r\n\r\n', False),
            (b'GET /a%2fb HTTP/1.1\r\nHost: localhost\r\n\r\n', True),
        ]:
            with Wire(port) as wire:
                wire.send(bad)
                code, headers, _ = wire.response()
                assert code == 400, bad
                assert (b'x-root' in headers) == dispatched, headers
        passed += ['conflicting framing', 'invalid header and path']
        with Wire(port) as partial, Wire(port) as complete:
            partial.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\n\r\na')
            complete.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert complete.response()[2] == b'hello'
            partial.send(b'bcd')
            assert partial.response()[2] == b'abcd'
        passed.append('concurrent partial connections')
        with Wire(port) as wire:
            wire.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\n\r\nab')
            wire.socket.shutdown(socket.SHUT_WR)
            # The pinned server closes an incomplete request on peer EOF.
            assert wire.file.read(1) == b''
        for _ in range(20):
            with Wire(port) as wire:
                wire.send(b'GET / HTTP/1.1\r\nHost:')
        with Wire(port) as wire:
            wire.send(b'GET / HTTP/1.1\r\nHost: localhost\r\n\r\n')
            assert wire.response()[0] == 200
        passed.append('peer disconnect')
        duplicate = subprocess.run([str(program)], env=environment, capture_output=True, timeout=5)
        assert duplicate.returncode != 0 and b'bind' in duplicate.stderr, duplicate
        if environment.get('TRBN_NETWORK_TEST_ALLOCATIONS') == '1':
            assert b'network-test-live-allocations=0\n' in duplicate.stderr, duplicate.stderr
        passed.append('bind failure')
        # A hostname may resolve to both families while the listener binds one.
        # Try the addresses like create_connection, setting the small receive
        # buffer before the TCP handshake so the stalled-write probe is bounded.
        stalled = None
        for family, kind, protocol, _, address in socket.getaddrinfo(CLIENT_HOST, port, type=socket.SOCK_STREAM):
            candidate = socket.socket(family, kind, protocol)
            candidate.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 1024)
            candidate.settimeout(5)
            try:
                candidate.connect(address)
            except OSError:
                candidate.close()
                continue
            stalled = candidate
            break
        assert stalled is not None, 'no resolved address accepted a connection'
        stalled.sendall(b'GET /large HTTP/1.1\r\nHost: localhost\r\n\r\n')
        assert stalled.recv(1) == b'H'
        before = time.monotonic()
        process.send_signal(signal.SIGTERM)
        out, err = process.communicate(timeout=5)
        stalled.close()
        assert time.monotonic() - before < 3, 'write drain exceeded shutdown deadline'
        assert process.returncode == 0, (out, err, process.returncode)
        verify_allocations(err, environment)
        passed.append('SIGTERM, stalled-write deadline and cleanup')
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
    restarted = run_server(program, port, environment)
    try:
        restarted.send_signal(signal.SIGINT)
        out, err = restarted.communicate(timeout=5)
        assert restarted.returncode == 0, (out, err)
        verify_allocations(err, environment)
        passed.append('SIGINT and restart')
    finally:
        if restarted.poll() is None:
            restarted.kill()
            restarted.wait()
    draining = run_server(program, port, environment)
    try:
        with Wire(port) as wire:
            wire.send(b'POST /echo HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\nExpect: 100-continue\r\n\r\n')
            assert wire.response()[0] == 100
            wire.send(b'a')
            draining.send_signal(signal.SIGTERM)
            time.sleep(.05)
            wire.send(b'bcd')
            assert wire.response()[2] == b'abcd'
        out, err = draining.communicate(timeout=5)
        assert draining.returncode == 0, (out, err)
        verify_allocations(err, environment)
    finally:
        if draining.poll() is None:
            draining.kill()
            draining.wait()
    print(json.dumps({'passed': passed}, indent=2))


def verify_allocations(stderr, environment):
    if environment.get('TRBN_NETWORK_TEST_ALLOCATIONS') != '1':
        return
    assert b'network-test-live-allocations=0\n' in stderr, stderr.decode()
    stats = {}
    for line in stderr.decode().splitlines():
        if line.startswith('type-rb-native-gc-stat-v1,'):
            _, name, value = line.split(',')
            stats[name] = int(value)
    assert stats.get('live-bytes') == 0, stats
    assert stats['allocated-bytes'] == stats['reclaimed-bytes'], stats


# Test-only allocation accounting. Production transport remains TypeRB/QBE;
# the probe replaces only raw allocations in network runtime functions.
ALLOCATOR_CHECK = r"""
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
typedef union { struct { size_t size; uint64_t magic; } info; long double align; } Header;
static long live;
static const uint64_t magic = UINT64_C(0x7472626e6e657477);
static void report(void) { fprintf(stderr, "network-test-live-allocations=%ld\n", live); if (live) abort(); }
__attribute__((constructor)) static void initialize(void) { atexit(report); }
void *network_test_malloc(size_t size) {
    if (size > SIZE_MAX - sizeof(Header)) return NULL;
    Header *header = malloc(sizeof(Header) + size);
    if (!header) return NULL;
    header->info.size = size; header->info.magic = magic; ++live;
    return header + 1;
}
void network_test_free(void *pointer) {
    if (!pointer) return;
    Header *header = (Header *)pointer - 1;
    if (header->info.magic != magic) abort();
    memset(pointer, 0xa5, header->info.size);
    header->info.magic = 0; --live; free(header);
}
void *network_test_calloc(size_t count, size_t size) {
    if (size && count > SIZE_MAX / size) return NULL;
    void *pointer = network_test_malloc(count * size);
    if (pointer) memset(pointer, 0, count * size);
    return pointer;
}
void *network_test_realloc(void *old, size_t size) {
    if (!old) return network_test_malloc(size);
    Header *header = (Header *)old - 1;
    if (header->info.magic != magic) abort();
    void *pointer = network_test_malloc(size);
    if (!pointer) return NULL;
    memcpy(pointer, old, size < header->info.size ? size : header->info.size);
    network_test_free(old);
    return pointer;
}
"""


def main():
    global CLIENT_HOST
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--libraries', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--dynamic', action='store_true', help='Exercise the retained dynamic baseline')
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--target', choices=QBE_TARGETS)
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--gc-stress', action='store_true')
    parser.add_argument('--host', default='127.0.0.1')
    args = parser.parse_args()
    if args.reference and (args.core or args.gc_stress):
        parser.error('--reference cannot be combined with --core or --gc-stress')
    if not args.reference and not args.libraries:
        parser.error('Native execution requires --libraries')
    if args.core and (not args.qbe or not args.target):
        parser.error('--core requires --qbe and --target')
    if args.target and not args.core:
        parser.error('--target requires --core')
    CLIENT_HOST = args.host
    with tempfile.TemporaryDirectory(prefix='native-http-') as temporary:
        root = Path(temporary)
        port = free_port()
        for name, source in FILES.items():
            path = root / 'src' / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(source.replace('PORT', str(port)).replace('127.0.0.1', args.host))
        config = root / 'trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'network-acceptance', 'mode': 'go' if args.reference else 'trb',
                                     'sourceDir': 'src', 'outDir': 'out',
                                     'go': {'module': 'example.com/network-acceptance'}}))
        program = root / 'server'
        environment = dict(os.environ)
        if args.libraries:
            environment['TRBN_NETWORK_LIB_DIR'] = str(args.libraries.resolve())
        if args.core:
            command = [args.binary.resolve(), 'build', config, '--output', program, '--qbe', args.qbe.resolve(), '--cc', '/usr/bin/cc', '--target', args.target]
        else:
            command = [args.binary.resolve(), 'build', '--config', config, '--compile', '--outfile', program]
        built = subprocess.run(list(map(str, command)), capture_output=True, env=environment, timeout=180)
        assert built.returncode == 0, (built.stdout.decode(), built.stderr.decode())
        if args.gc_stress:
            qbe = args.qbe.resolve() if args.qbe else args.binary.resolve().with_name('qbe')
            emit_command = [args.binary.resolve(), 'emit-qbe', config] if args.core else [args.binary.resolve(), 'build', '--config', config, '--stdout']
            emitted = subprocess.run(list(map(str, emit_command)), capture_output=True, env=environment, text=True, timeout=180)
            assert emitted.returncode == 0, emitted.stderr
            lines, managed, raw, hooks = [], False, False, 0
            for line in emitted.stdout.splitlines():
                if line.startswith('function ') or line.startswith('export function '):
                    managed = '$trbnf' in line
                    raw = '$net_' in line or '$trbn_network_' in line
                if line == '}':
                    managed = raw = False
                if managed and any(marker in line for marker in ('call $trbn_array_new(', 'call $trbn_array_new_reserved(', 'call $trbn_array_push_', 'call $trbn_array_grow_paced(', 'call $trbn_gc_alloc(', 'call $trbn_string_concat(')):
                    lines.append(' call $trbn_gc_collect(w 0)')
                    hooks += 1
                if raw:
                    for name in ('malloc', 'calloc', 'realloc', 'free'):
                        line = line.replace('call $' + name + '(', 'call $network_test_' + name + '(')
                lines.append(line)
            assert hooks >= 8, hooks
            il, assembly, checker = root / 'stress.ssa', root / 'stress.s', root / 'allocator-check.c'
            il.write_text('\n'.join(lines) + '\n')
            checker.write_text(ALLOCATOR_CHECK)
            target_options = ['-t', QBE_TARGETS[args.target]] if args.core else []
            subprocess.run([str(qbe), *target_options, '-o', str(assembly), str(il)], check=True)
            linker_options = ['-fuse-ld=lld'] if args.target and args.target.startswith('linux-') else []
            network_flags = []
            if not args.dynamic:
                network_flags = [str(args.libraries.resolve() / name) for name in ('libuv.a', 'libllhttp.a', 'network-identity.o')]
                if os.uname().sysname == 'Linux':
                    network_flags += ['-pthread', '-ldl', '-lrt']
            subprocess.run(['/usr/bin/cc', *linker_options, str(assembly), str(checker), *network_flags, '-lm', '-o', str(program)], check=True)
            environment['TYPE_RB_NATIVE_RUNTIME_STATS'] = '1'
            environment['TRBN_NETWORK_TEST_ALLOCATIONS'] = '1'
            print('forced managed GC hooks:', hooks)
        if not args.reference and not args.dynamic:
            from network_artifacts import inspect
            assert inspect(program)['linkage'] == 'static'
            # An application runs independently of its build-time installation.
            environment.pop('TRBN_NETWORK_LIB_DIR', None)
            environment.pop('LD_LIBRARY_PATH', None)
            environment.pop('DYLD_LIBRARY_PATH', None)
            environment['PATH'] = ''
            relocated = root / 'isolated runtime' / 'server'
            relocated.parent.mkdir()
            program.rename(relocated)
            program = relocated
        exercise(program, port, environment, args.reference, args.dynamic)


if __name__ == '__main__':
    main()
