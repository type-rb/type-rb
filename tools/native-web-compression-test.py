#!/usr/bin/env python3
"""Pinned Web gzip semantics, installed links, managed lifetimes and HTTP output."""
import argparse
import base64
import gzip
import hashlib
import http.client
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time

from network_artifact_smoke import imports
from network_artifacts import artifact_spec, inspect, install, pack, target_name

BASE = '''import { Body, Header, Headers, HttpMethod } from trb/http
import { Context, Next, Request, Response } from trb/web
import { Compression, CompressionOptions } from trb/web/middleware/compression
import trb/std/encoding/base64
class Reply implements Next
readonly @response: Response
def initialize(response: Response)
@response = response
end
def call(_context: Context): Response
return @response
end
end
def probe(id: String, method: String, accepted: String, response: Response, minimum: Integer)
request := Request.new(method: HttpMethod.new(method), path: "/", query_string: "", headers: Headers.new([Header.new(name: "Accept-Encoding", value: accepted)]), body: Body.empty())
context := Context.new(request: request, path_parameters: {})
result := Compression.call(context, Reply.new(response), CompressionOptions.new(minimum_size_bytes: minimum))
puts(id + "|" + result.status.to_s() + "|" + result.header_values("content-encoding").join(",") + "|" + result.header_values("vary").join(",") + "|" + result.header_values("content-length").join(",") + "|" + result.header_values("etag").join(",") + "|" + Base64.encode(result.body.bytes()))
end
'''

ALLOCATIONS = r'''
#include <assert.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern void *libdeflate_alloc_compressor(int);
extern void libdeflate_free_compressor(void *);
extern size_t libdeflate_gzip_compress_bound(void *, size_t);
extern size_t libdeflate_gzip_compress(void *, const void *, size_t, void *, size_t);
static long states, buffers, calls;
static int fault(const char *name) { const char *s = getenv("GZIP_TEST_FAULT"); return s && strcmp(s, name) == 0; }
void *gzip_test_state(int level) { if (fault("state")) return NULL; void *p = libdeflate_alloc_compressor(level); if (p) states++; return p; }
void gzip_test_state_free(void *p) { if (p) { assert(states > 0); states--; } libdeflate_free_compressor(p); }
void *gzip_test_buffer(size_t size) { if (fault("buffer")) return NULL; void *p = malloc(size); if (p) buffers++; return p; }
void gzip_test_buffer_free(void *p) { if (p) { assert(buffers > 0); buffers--; } free(p); }
size_t gzip_test_bound(void *state, size_t size) { return fault("bound") ? (size_t)-1 : libdeflate_gzip_compress_bound(state, size); }
size_t gzip_test_compress(void *state, const void *in, size_t size, void *out, size_t capacity) {
  calls++;
  return fault("compress") ? 0 : libdeflate_gzip_compress(state, in, size, out, capacity);
}
static void report(void) { fprintf(stderr, "gzip-test-stats={\"states\":%ld,\"buffers\":%ld,\"calls\":%ld}\n", states, buffers, calls); }
__attribute__((constructor)) static void initialize(void) { atexit(report); }
'''


def source_for(manifest):
    sample = manifest['sample_unit'] * manifest['sample_repetitions']
    helper = 'def sample(): Response\nreturn Response.text(' + json.dumps(sample, ensure_ascii=False) + ').with_header("content-length", "1680").with_header("etag", "strong")\nend\n'
    invocations = ['probe(' + ', '.join([json.dumps(row['id']), json.dumps(row['method']),
                   json.dumps(row['accept_encoding']), row['response'], str(row['minimum'])]) + ')\n'
                   for row in manifest['cases']]
    return BASE + helper + 'def main()\n' + ''.join(invocations) + 'end\n'


def check_output(manifest, stdout):
    lines = stdout.splitlines()
    assert len(lines) == len(manifest['cases']), stdout
    for line, case in zip(lines, manifest['cases']):
        fields = line.split('|')
        assert len(fields) == 7 and fields[0] == case['id'], line
        data = base64.b64decode(fields[-1], validate=True)
        if fields[2] == 'gzip':
            data = gzip.decompress(data)
        observed = dict(zip(['status', 'encoding', 'vary', 'length', 'etag'], fields[1:6]))
        observed.update(body_sha256=hashlib.sha256(data).hexdigest(), body_bytes=len(data))
        assert observed == case['expected'], (case['id'], observed, case['expected'])


def force_collection(qbe):
    rows, gzip_body, ordinary, hooks = [], False, False, 0
    replacements = {'libdeflate_alloc_compressor': 'gzip_test_state', 'libdeflate_free_compressor': 'gzip_test_state_free',
                    'libdeflate_gzip_compress_bound': 'gzip_test_bound', 'libdeflate_gzip_compress': 'gzip_test_compress',
                    'malloc': 'gzip_test_buffer', 'free': 'gzip_test_buffer_free'}
    for line in qbe.splitlines():
        if line.startswith('function '):
            gzip_body = '$trbn_gzip(' in line
            ordinary = '$trbnf' in line
        if line == '}':
            gzip_body = ordinary = False
        if (gzip_body and 'call $trbn_string_alloc(' in line) or (ordinary and any(marker in line for marker in (
                'call $trbn_array_new(', 'call $trbn_array_push_', 'call $trbn_gc_alloc(', 'call $trbn_string_concat('))):
            rows.append(' call $trbn_gc_collect(w 0)')
            hooks += 1
        if gzip_body:
            for old, new in replacements.items():
                line = line.replace('call $' + old + '(', 'call $' + new + '(')
        rows.append(line)
    assert hooks >= 3, hooks
    return '\n'.join(rows) + '\n'


def allocation_statistics(observed):
    rows = [line.split('=', 1)[1] for line in observed.stderr.splitlines() if line.startswith('gzip-test-stats=')]
    assert len(rows) == 1, (observed.returncode, observed.stdout, observed.stderr)
    return json.loads(rows[0])


def exercise_http(program, port, sample, environment, gzip_enabled=True):
    started = time.monotonic()
    child = subprocess.Popen([str(program)], env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        for _ in range(200):
            assert child.poll() is None, child.communicate()
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=.05):
                    break
            except OSError:
                time.sleep(.025)
        else:
            raise AssertionError('gzip HTTP server did not listen')
        startup = time.monotonic() - started
        for method, accepted, compressed in [('GET', 'gzip', True), ('GET', 'gzip;q=0', False), ('HEAD', 'gzip', False)]:
            compressed = compressed and gzip_enabled
            connection = http.client.HTTPConnection('127.0.0.1', port, timeout=5)
            try:
                connection.request(method, '/', headers={'Accept-Encoding': accepted})
                response = connection.getresponse()
                body = response.read()
                assert response.status == 200, (response.status, body)
                assert response.getheader('Content-Encoding') == ('gzip' if compressed else None)
                assert response.getheader('ETag') == (None if compressed else 'strong')
                if method == 'HEAD':
                    assert body == b''
                else:
                    if gzip_enabled:
                        assert 'accept-encoding' in response.getheader('Vary', '').lower()
                    else:
                        assert response.getheader('Vary') is None
                    assert int(response.getheader('Content-Length')) == len(body)
                    assert (gzip.decompress(body) if compressed else body) == sample
            finally:
                connection.close()
        return startup
    finally:
        child.terminate()
        try:
            stdout, stderr = child.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            child.kill()
            child.communicate()
            raise
        assert child.returncode == 0, (child.returncode, stdout, stderr)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--libraries', type=Path)
    parser.add_argument('--compression', type=Path)
    parser.add_argument('--receipt', type=Path, required=True)
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--target')
    args = parser.parse_args()
    if not args.reference and (not args.libraries or not args.compression):
        parser.error('Native requires both static dependency installations')
    if args.core and (not args.qbe or not args.target):
        parser.error('--core requires --qbe and --target')
    repository = Path(__file__).resolve().parent.parent
    manifest = json.loads(Path(__file__).with_name('web-compression-cases.json').read_text())
    assert manifest['reference_revision'] == (repository / 'TYPE_RB_REVISION').read_text().strip()
    assert len(manifest['cases']) <= 20
    binary = args.binary.resolve()
    receipt = {'reference_revision': manifest['reference_revision'], 'mode': 'reference' if args.reference else 'native',
               'target': target_name(), 'cases': [row['id'] for row in manifest['cases']], 'build_seconds': [], 'execution_seconds': []}
    with tempfile.TemporaryDirectory(prefix='web gzip ') as temporary:
        root = Path(temporary)
        source = root / 'src/main.trb'
        config = root / 'trbconfig.jsonc'

        def setup():
            source.parent.mkdir(exist_ok=True)
            source.write_text(source_for(manifest))
            config.write_text(json.dumps({'name': 'gzip-acceptance', 'mode': 'go' if args.reference else 'trb',
                                         'sourceDir': 'src', 'go': {'module': 'example.com/gzip-acceptance'}}))

        if args.reference:
            setup()
        environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
        for key in ('TRBN_NETWORK_LIB_DIR', 'TRBN_COMPRESSION_LIB_DIR', 'TRBN_QBE', 'TRBN_CC', 'LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH'):
            environment.pop(key, None)
        if not args.reference:
            package = root / 'installed compiler'
            package.mkdir()
            qbe = args.qbe.resolve() if args.qbe else binary.with_name('qbe')
            shutil.copy2(binary, package / 'trbn')
            shutil.copy2(qbe, package / 'qbe')
            binary = package / 'trbn'
            archive = root / 'network.tar.gz'
            install(archive, package / 'network', pack(args.libraries, archive), target_name())

        def run(arguments, *, input=None, success=True, env=environment):
            result = subprocess.run(list(map(str, arguments)), input=input, capture_output=True, text=True,
                                    cwd=root, env=env, timeout=180)
            if (result.returncode == 0) != success:
                # Retain the installed boundary before the temporary tree is
                # removed. A late link failure must distinguish vanished files
                # from compiler path resolution without retrying the build.
                dependencies = {}
                if not args.reference:
                    for name in ('trbn', 'qbe', 'network/libuv.a', 'network/libllhttp.a',
                                 'network/network-identity.o', 'compression/libdeflate.a',
                                 'compression/compression-identity.o'):
                        path = package / name
                        try:
                            dependencies[name] = {'exists': path.is_file(), 'readable': os.access(path, os.R_OK)}
                            if path.is_file():
                                dependencies[name].update(bytes=path.stat().st_size,
                                                          sha256=hashlib.sha256(path.read_bytes()).hexdigest())
                        except OSError as error:
                            dependencies[name] = {'inspection_error': str(error)}
                receipt['failure'] = {'command': list(map(str, arguments)),
                                      'exit_code': result.returncode,
                                      'stdout': result.stdout, 'stderr': result.stderr,
                                      'installed_dependencies': dependencies}
                args.receipt.parent.mkdir(parents=True, exist_ok=True)
                args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
                raise AssertionError((arguments, result.stdout, result.stderr,
                                      {'installed_dependencies': dependencies}))
            return result

        def build(input_path, output, *, success=True):
            if args.core:
                command = [binary, 'build', input_path, '--output', output, '--qbe', binary.with_name('qbe'),
                           '--cc', '/usr/bin/cc', '--target', args.target]
            else:
                command = [binary, 'build', '--compile', *(['--config'] if input_path == config else []), input_path, '--outfile', output]
            return run(command, success=success)

        program = root / 'program'
        if not args.reference:
            plain = root / 'plain.trb'
            plain.write_text('def compiler_set_compression_link(_needed: Boolean)\nend\ndef main()\ncompiler_set_compression_link(true)\nputs("plain")\nend\n')
            build(plain, program)
            assert artifact_spec('compression')[2] not in program.read_bytes()
            assert run([program], env={'PATH': ''}).stdout == 'plain\n'
            setup()
            sentinel = b'previous output\n'
            program.write_bytes(sentinel)
            failed = build(config, program, success=False)
            assert 'pinned static gzip library is missing' in failed.stderr, failed
            assert program.read_bytes() == sentinel and not list(root.glob('program.*'))
            archive = root / 'compression.tar.gz'
            install(archive, package / 'compression', pack(args.compression, archive, 'compression'), target_name(), 'compression')
            corrupt = root / 'corrupt compression'
            shutil.copytree(package / 'compression', corrupt)
            (corrupt / 'libdeflate.a').write_bytes(b'invalid archive\n')
            environment['TRBN_COMPRESSION_LIB_DIR'] = str(corrupt)
            failed = build(config, program, success=False)
            environment.pop('TRBN_COMPRESSION_LIB_DIR')
            assert 'compiler: cc failed' in failed.stderr, failed
            assert program.read_bytes() == sentinel and not list(root.glob('program.*'))
            shutil.rmtree(corrupt)
        run([binary, 'check', source])
        run([binary, 'check', *([] if args.core else ['--config']), config])
        if args.reference:
            rejected = build(source, program, success=False)
            assert '--compile builds the configured project and does not accept source paths' in rejected.stderr, rejected
            receipt['file_compile'] = 'reference CLI requires a project configuration'
        for input_path in ([config, config, config] if args.reference else [source, config, config]):
            started = time.monotonic()
            build(input_path, program)
            receipt['build_seconds'].append(time.monotonic() - started)
            started = time.monotonic()
            result = run([program], env={'PATH': ''})
            receipt['execution_seconds'].append(time.monotonic() - started)
            check_output(manifest, result.stdout)
        receipt['executable_bytes'] = program.stat().st_size
        if not args.reference:
            receipt['imports'] = imports(program)
            assert not any('libdeflate' in name or 'libuv' in name or 'llhttp' in name for name in receipt['imports'])
            receipt['identity'] = inspect(program, 'compression')
        if not args.core:
            check_output(manifest, run([binary, 'run', '--config', config]).stdout)
            repl = run([binary, 'repl', '--config', config], input='import { probe } from main\nimport { Response } from trb/web\nprobe("repl", "GET", "gzip", Response.text("body"), 0)\nputs(7)\n:quit\n')
            assert 'trb.web.middleware.compression.gzip' in repl.stderr and 'no REPL runtime adapter' in repl.stderr, repl
            assert repl.stdout == '7\n', repl
            receipt['repl_rejection'] = True
        if not args.reference:
            command = [binary, 'emit-qbe', config] if args.core else [binary, '--internal-driver', 'emit-qbe', config]
            qbe_source = force_collection(run(command).stdout)
            il, assembly, checker = root / 'stress.ssa', root / 'stress.s', root / 'allocator.c'
            il.write_text(qbe_source)
            checker.write_text(ALLOCATIONS)
            qbe_targets = {'linux-amd64': 'amd64_sysv', 'linux-arm64': 'arm64', 'darwin-arm64': 'arm64_apple', 'darwin-amd64': 'amd64_apple'}
            run([binary.with_name('qbe'), *(['-t', qbe_targets[args.target.removesuffix('-v0')]] if args.core else []), '-o', assembly, il])
            libraries = [package / 'network' / name for name in ('libuv.a', 'libllhttp.a', 'network-identity.o')]
            libraries += [package / 'compression' / name for name in ('libdeflate.a', 'compression-identity.o')]
            flags = ['-pthread', '-ldl', '-lrt'] if target_name().startswith('linux-') else []
            linker_options = ['-fuse-ld=lld'] if args.core and args.target.startswith('linux-') else []
            run(['/usr/bin/cc', *linker_options, assembly, checker, *libraries, *flags, '-lm', '-o', program])
            observed = run([program], env=dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
            check_output(manifest, observed.stdout)
            statistics = allocation_statistics(observed)
            assert statistics['states'] == statistics['buffers'] == 0 and statistics['calls'] >= 5, statistics
            gc = {line.split(',')[1]: int(line.split(',')[2]) for line in observed.stderr.splitlines() if line.startswith('type-rb-native-gc-stat-v1,')}
            assert gc['collections'] > 0 and gc['live-bytes'] == 0, gc
            receipt['forced_gc'] = gc
            receipt['foreign_allocations'] = statistics
            for fault in ('state', 'buffer', 'bound', 'compress'):
                observed = run([program], env=dict(environment, GZIP_TEST_FAULT=fault), success=False)
                assert '\x00' not in observed.stderr and observed.stderr.startswith('panic: allocation failed\n'), observed
                statistics = allocation_statistics(observed)
                assert statistics['states'] == statistics['buffers'] == 0, (fault, statistics)
            receipt['failure_cleanup'] = ['state', 'buffer', 'bound', 'compress']
            # This ablation records optional dependency cost, not a general speedup.
            baseline_source = source.read_text().replace('import { Compression, CompressionOptions } from trb/web/middleware/compression\n', '')
            baseline_source = baseline_source.replace('result := Compression.call(context, Reply.new(response), CompressionOptions.new(minimum_size_bytes: minimum))', 'result := response')
            baseline_source = baseline_source.replace('context := Context.new', '_context := Context.new')
            source.write_text(baseline_source)
            baseline = root / 'uncompressed'
            build(config, baseline)
            assert artifact_spec('compression')[2] not in baseline.read_bytes()
            receipt['uncompressed_executable_bytes'] = baseline.stat().st_size
            assert len(run([baseline], env={'PATH': ''}).stdout.splitlines()) == len(manifest['cases'])

        # Separate byte/error boundaries; the frozen negotiation corpus stays fixed.
        boundary = BASE + 'def main()\nprobe("binary", "GET", "gzip", Response.new(status: 200, headers: Headers.new([Header.new(name: "content-type", value: "application/json")]), body: Body.new("A\\x00\\xff".to_bytes())), 0)\nend\n'
        source.write_text(boundary)
        run([binary, 'check', source])
        build(config, program)
        fields = run([program], env={'PATH': ''}).stdout.strip().split('|')
        assert fields[:6] == ['binary', '200', 'gzip', 'Accept-Encoding', '', ''], fields
        assert gzip.decompress(base64.b64decode(fields[6], validate=True)) == b'A\x00\xff'
        source.write_text(boundary.replace(')), 0)', ')), -1)'))
        build(config, program)
        rejected = run([program], success=False)
        assert 'compression minimum_size_bytes must be non-negative' in rejected.stderr, rejected
        receipt['binary_and_invalid_options'] = True

        sample = (manifest['sample_unit'] * manifest['sample_repetitions']).encode()
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
        source.write_text('import trb/web\ndef main()\nWeb.serve(Web::ServerConfig.new(host: "127.0.0.1", port: ' + str(port) + ', shutdown_timeout_milliseconds: 250))\nend\n')
        routes = source.parent / 'routes'
        routes.mkdir()
        response = 'return Response.text(' + json.dumps(sample.decode(), ensure_ascii=False) + ').with_header("etag", "strong")\n'
        (routes / 'index.trb').write_text('import { Context, Response } from trb/web\ndef get(_context: Context): Response\n' + response + 'end\ndef head(_context: Context): Response\n' + response + 'end\n')
        (routes / '_middleware.trb').write_text('import { Context, Next, Response } from trb/web\nimport { Compression } from trb/web/middleware/compression\ndef call(context: Context, next_handler: Next): Response\nreturn Compression.call(context, next_handler)\nend\n')
        server = root / 'relocated application'
        build(config, server)
        if not args.reference:
            assert inspect(server, 'compression') == receipt['identity']
            middleware = routes / '_middleware.trb'
            compressed_source = middleware.read_text()
            uncompressed_source = compressed_source.replace('import { Compression } from trb/web/middleware/compression\n', '').replace('Compression.call(context, next_handler)', 'next_handler.call(context)')
            baseline = root / 'uncompressed server'
            comparison = {'compressed': {'build_seconds': [], 'startup_seconds': []},
                          'uncompressed': {'build_seconds': [], 'startup_seconds': []}}
            for repetition in range(3):
                roles = ['compressed', 'uncompressed'] if repetition % 2 == 0 else ['uncompressed', 'compressed']
                for role in roles:
                    enabled = role == 'compressed'
                    middleware.write_text(compressed_source if enabled else uncompressed_source)
                    executable = server if enabled else baseline
                    started = time.monotonic()
                    build(config, executable)
                    comparison[role]['build_seconds'].append(time.monotonic() - started)
                    comparison[role]['startup_seconds'].append(exercise_http(executable, port, sample, {'PATH': ''}, enabled))
                    comparison[role]['executable_bytes'] = executable.stat().st_size
            receipt['http_dependency_cost'] = comparison
            middleware.write_text(compressed_source)
            shutil.rmtree(package)
        receipt['http_startup_seconds'] = [exercise_http(server, port, sample, {'PATH': '', 'TRBN_COMPRESSION_LIB_DIR': str(root / 'absent')}) for _ in range(3)]
        receipt['http_contracts'] = ['default gzip middleware', 'q=0 identity', 'HEAD without compression', 'relocated executable without sidecars']
        args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
    print('20 gzip response contracts, installed static links and REPL boundary passed')


if __name__ == '__main__':
    main()
