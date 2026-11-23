#!/usr/bin/env python3
"""Exercise file-route dispatch, nested middleware, JSON and managed lifetimes."""
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
'shared.trb': '''import { ContextKey } from trb/web
KEY := ContextKey<String>.new("scope")
record Params
id: Integer
end
record Output
id: Integer
tag: String
end
''',
'routes/_middleware.trb': '''import { Context, Next, Response } from trb/web
import { RequestID } from trb/web/middleware/request_id
def call(context: Context, next_handler: Next): Response
if context.request.path == "/root-before"
zero := context.request.body.size()
puts(1 / zero)
end
response := RequestID.call(context, next_handler)
if context.request.path == "/root-after"
zero := context.request.body.size()
puts(1 / zero)
end
return response.with_header("x-root", "resumed")
end
''',
'routes/items/_middleware.trb': '''import { Context, Next, Response } from trb/web
import { KEY } from shared
def call(context: Context, next_handler: Next): Response
return next_handler.call(context.with(KEY, "scoped")).with_header("x-scope", "yes")
end
''',
'routes/items/[id]/index.trb': '''import { Context, Response } from trb/web
import { KEY, Params, Output } from shared
import trb/std/result
def get(context: Context): Response
case context.params<Params>()
when Result::Ok(params)
case context.fetch(KEY)
when Result::Ok(tag)
return Response.json<Output>(Output.new(id: params.id, tag: tag))
when Result::Err(_error)
return Response.text("lost scope", 500)
end
when Result::Err(_error)
return Response.text("bad params", 400)
end
end
''',
'routes/bad.trb': '''import { Context, Response } from trb/web
def get(context: Context): Response
zero := context.request.body.size()
puts(1 / zero)
puts("abandoned handler continued")
return Response.text("unreachable")
end
''',
'main.trb': '''import { Body, Header, Headers, HttpMethod } from trb/http
import { Request, Response } from trb/web
import { dispatch } from trb/web/testing
import trb/std/result
def probe()
request := Request.new(method: HttpMethod.get(), path: "/items/42", query_string: "", headers: Headers.new([Header.new(name: "X-Request-ID", value: "flow-42")]), body: Body.empty())
response := dispatch(request)
puts(response.status == 200)
puts(response.body.to_s() == "{\\"id\\":42,\\"tag\\":\\"scoped\\"}")
case response.header_value("x-request-id")
when Result::Ok(value)
puts(value == "flow-42")
when Result::Err(_error)
puts(false)
end
case response.header_value("x-scope")
when Result::Ok(value)
puts(value == "yes")
when Result::Err(_error)
puts(false)
end
end
def failing(method: String, path: String): Response
return dispatch(Request.new(method: HttpMethod.new(method), path: path, query_string: "", headers: Headers.new(), body: Body.empty()))
end
def recovery_probe()
response := failing("GET", "/bad")
puts(response.status == 500)
puts(response.header_values("x-root").join("") == "resumed")
# Keep the failed response alive while another dispatch allocates and collects.
healthy := failing("GET", "/items/42")
puts(healthy.status == 200)
puts(response.body.to_s() == "{\\"error\\":\\"internal_server_error\\"}")
head := failing("HEAD", "/bad")
puts(head.status == 500 && head.body.size() == 0)
before := failing("GET", "/root-before")
puts(before.status == 500 && before.header_values("x-root").size() == 0)
after := failing("GET", "/root-after")
puts(after.status == 500 && after.header_values("x-root").size() == 0)
end
def main()
(0...8).each do |_index|
probe()
recovery_probe()
end
end
'''
}
expected = 'true\n' * 88
with tempfile.TemporaryDirectory(prefix='native-web-route-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root/'history'))
    for name, source in files.items():
        path = root/'src'/name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    config = root/'trbconfig.jsonc'
    def configure(mode):
        config.write_text(json.dumps({'name': 'web-route-integration', 'mode': mode,
            'sourceDir': 'src', 'outDir': 'out', 'go': {'module': 'example.com/web-route-integration'}}))
    def run(command, *, data=None):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
            input=data, capture_output=True, text=True, timeout=120)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result
    program = root/'program'
    if args.reference:
        configure('go')
        run([args.reference.resolve(), 'check', '--config', config])
        run([args.reference.resolve(), 'build', '--config', config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        configure('trb')
        run([binary, 'check', '--config', config])
        run([binary, 'build', '--config', config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
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
        il, assembly = root/'input.ssa', root/'input.s'
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
        print('Web route GC hooks:', hooks, 'statistics:', json.dumps(statistics, sort_keys=True))
    # Project validation also runs before REPL input; no executable entry is
    # required, and duplicate handler names must not hide invalid route files.
    (root/'src/main.trb').unlink()
    route = root/'src/routes/items/[id]/index.trb'
    original = route.read_text()
    readers = [('go', args.reference.resolve())] if args.reference else []
    if not args.reference_only:
        readers.append(('trb', binary))
    for mode, reader in readers:
        configure(mode)
        accepted = run([reader, 'repl', '--config', config], data=':quit\n')
        assert not accepted.stderr, accepted
        route.write_text('import { Response } from trb/web\ndef get(): Response\nreturn Response.text("invalid handler")\nend\n')
        rejected = subprocess.run([str(reader), 'repl', '--config', str(config)],
            cwd=root, env=environment, input=':quit\n', text=True,
            capture_output=True, timeout=120)
        assert rejected.returncode == 1 and rejected.stderr, rejected
        route.write_text(original)
print('Web route project, middleware, JSON and managed lifetime checks passed.')
