#!/usr/bin/env python3
"""Exercise browser middleware through file routes and forced collection."""
import argparse
from native_network_link import static_network_flags
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
parser.add_argument('--reference-only', action='store_true')
args = parser.parse_args()
if args.reference_only and not args.reference:
    parser.error('--reference-only requires --reference')
binary = args.binary.resolve()
files = {
    'routes/_middleware.trb': '''import { Context, Next, Response } from trb/web
import { compose } from trb/web/middleware
import { CORS, CORSOptions, PreflightMaxAge } from trb/web/middleware/cors
import { RequestID } from trb/web/middleware/request_id
import { SecureHeaders } from trb/web/middleware/secure_headers
def call(context: Context, next_handler: Next): Response
options := CORSOptions.new(allow_origins: ["https://client.example.test"], allow_methods: ["GET", "POST"], allow_headers: [], expose_headers: ["X-Request-ID"], credentials: true, max_age: PreflightMaxAge::Seconds(600))
return compose(context, next_handler, [RequestID.middleware(), SecureHeaders.middleware(), CORS.middleware(options)])
end
''',
    'routes/browser.trb': '''import { Context, Response } from trb/web
def get(_context: Context): Response
puts("handler")
return Response.text("browser", 201).with_header("vary", "Accept").with_header("x-frame-options", "original").with_header("etag", "sample")
end
''',
    'main.trb': '''import { Body, Header, Headers, HttpMethod } from trb/http
import { Request, Response } from trb/web
import { dispatch } from trb/web/testing
def request(method: String, path: String, origin: String, preflight: Boolean): Response
mut headers := [Header.new(name: "Origin", value: origin), Header.new(name: "X-Request-ID", value: "browser-flow")]
if preflight
headers.push(Header.new(name: "Access-Control-Request-Method", value: "POST"))
headers.push(Header.new(name: "Access-Control-Request-Headers", value: "X-One"))
headers.push(Header.new(name: "Access-Control-Request-Headers", value: "X-Two"))
end
return dispatch(Request.new(method: HttpMethod.new(method), path: path, query_string: "", headers: Headers.new(headers), body: Body.empty()))
end
def header(response: Response, name: String): String
return response.header_values(name).join("|")
end
def probe()
normal := request("GET", "/browser", "https://client.example.test", false)
puts(normal.status == 201)
puts(normal.body.to_s() == "browser")
puts(header(normal, "access-control-allow-origin") == "https://client.example.test")
puts(header(normal, "access-control-allow-credentials") == "true")
puts(header(normal, "access-control-expose-headers") == "X-Request-ID")
puts(header(normal, "vary") == "Accept|Origin")
puts(header(normal, "x-frame-options") == "SAMEORIGIN")
puts(header(normal, "x-content-type-options") == "nosniff")
puts(header(normal, "referrer-policy") == "no-referrer")
puts(header(normal, "x-xss-protection") == "0")
puts(header(normal, "x-request-id") == "browser-flow")
puts(header(normal, "etag") == "sample")
head := request("HEAD", "/browser", "https://client.example.test", false)
puts(head.status == 201 && head.body.size() == 0)
puts(header(head, "access-control-allow-origin") == "https://client.example.test")
puts(header(head, "x-frame-options") == "SAMEORIGIN")
puts(header(head, "etag") == "sample")
denied := request("GET", "/browser", "https://denied.example.test", false)
puts(denied.status == 201 && denied.body.to_s() == "browser")
puts(header(denied, "access-control-allow-origin") == "")
puts(header(denied, "access-control-allow-credentials") == "")
puts(header(denied, "vary") == "Accept|Origin")
puts(header(denied, "x-frame-options") == "SAMEORIGIN")
# Preflight terminates before route lookup, even for a path without a handler.
preflight := request("OPTIONS", "/not-found", "https://client.example.test", true)
puts(preflight.status == 204 && preflight.body.size() == 0)
puts(header(preflight, "access-control-allow-origin") == "https://client.example.test")
puts(header(preflight, "access-control-allow-credentials") == "true")
puts(header(preflight, "access-control-allow-methods") == "GET, POST")
puts(header(preflight, "access-control-allow-headers") == "X-One, X-Two")
puts(header(preflight, "access-control-max-age") == "600")
puts(header(preflight, "vary") == "Origin|Access-Control-Request-Headers")
puts(header(preflight, "x-frame-options") == "SAMEORIGIN")
puts(header(preflight, "x-request-id") == "browser-flow")
options := request("OPTIONS", "/browser", "https://client.example.test", false)
puts(options.status == 204 && options.body.size() == 0)
puts(header(options, "allow") == "GET, HEAD, OPTIONS")
puts(header(options, "access-control-allow-methods") == "")
puts(header(options, "x-frame-options") == "SAMEORIGIN")
missing := request("GET", "/not-found", "https://client.example.test", false)
puts(missing.status == 404)
puts(header(missing, "access-control-allow-origin") == "https://client.example.test")
puts(header(missing, "x-frame-options") == "SAMEORIGIN")
invalid := request("GET", "/%zz", "https://client.example.test", false)
puts(invalid.status == 400)
puts(header(invalid, "access-control-allow-origin") == "https://client.example.test")
puts(header(invalid, "x-frame-options") == "SAMEORIGIN")
# Retain the first response across every allocating dispatch above.
puts(normal.body.to_s() == "browser")
puts(header(normal, "vary") == "Accept|Origin")
end
def main()
(0...8).each do |_index|
probe()
end
end
''',
}
# Exactly three handler invocations per iteration; preflight and protocol
# responses must never reach it. Expectations precede reference observation.
expected = ('handler\n' + 'true\n' * 12 + 'handler\n' + 'true\n' * 4
            + 'handler\n' + 'true\n' * 26) * 8
with tempfile.TemporaryDirectory(prefix='native-web-middleware-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
    for name, source in files.items():
        path = root / 'src' / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    config = root / 'trbconfig.jsonc'

    def configure(mode):
        config.write_text(json.dumps({'name': 'web-middleware-integration', 'mode': mode,
            'sourceDir': 'src', 'outDir': 'out', 'go': {'module': 'example.com/web-middleware-integration'}}))

    def run(command):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
            capture_output=True, text=True, timeout=120)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    program = root / 'program'
    if args.reference:
        configure('go')
        run([args.reference.resolve(), 'check', '--config', config])
        run([args.reference.resolve(), 'build', '--config', config, '--compile', '--outfile', program])
        actual = run([program]).stdout
        assert actual == expected, (actual, expected)
    if not args.reference_only:
        configure('trb')
        run([binary, 'check', '--config', config])
        run([binary, 'build', '--config', config, '--compile', '--outfile', program])
        actual = run([program]).stdout
        assert actual == expected, (actual, expected)
        emitted = run([binary, 'build', '--config', config, '--stdout'])
        forced, ordinary, hooks = [], False, 0
        for line in emitted.stdout.splitlines():
            if line.startswith('function '):
                ordinary = '$trbnf' in line
            if line == '}':
                ordinary = False
            if ordinary and any(marker in line for marker in (
                    'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                    'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                    'call $trbn_gc_alloc(', 'call $trbn_string_concat(')):
                forced.append('\tcall $trbn_gc_collect(w 0)')
                hooks += 1
            forced.append(line)
        assert hooks >= 8, hooks
        il, assembly = root / 'input.ssa', root / 'input.s'
        il.write_text('\n'.join(forced) + '\n')
        run([binary.with_name('qbe'), '-o', assembly, il])
        run(['/usr/bin/cc', assembly, *static_network_flags(binary.with_name('qbe')), '-lm', '-o', program])
        collected = subprocess.run([str(program)], cwd=root, text=True, capture_output=True,
            env=dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=120)
        assert collected.returncode == 0 and collected.stdout == expected, collected
        statistics = {}
        for line in collected.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['collections'] >= 8 and statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        print('Web middleware GC hooks:', hooks, 'statistics:', json.dumps(statistics, sort_keys=True))
print('Browser middleware routes, headers, preflight, HEAD and managed lifetimes passed.')
