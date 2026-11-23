#!/usr/bin/env python3
"""Exercise combined Web inputs across modules, retained REPL values and forced GC."""
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

models = '''record PathValues
id: Integer
end
record QueryValues
tags: Array<String>
limit: Integer?
end
record Payload
title: String
counts: Hash<String, Array<Integer>>
end
record Input
body: Payload
query: QueryValues
params: PathValues
end
'''
producer = r'''import { Input, PathValues, QueryValues, Payload } from models
import { Context, Request, Response, EndpointInputError } from trb/web
import { Body, Headers, HttpMethod } from trb/http
def error_name(error: EndpointInputError): String
case error
when EndpointInputError::Params(_error)
return "path-error"
when EndpointInputError::Query(_error)
return "query-error"
when EndpointInputError::Body(_error)
return "body-error"
end
end
def context(id: String = "7"): Context
puts("receiver")
return Context.new(request: Request.new(method: HttpMethod.post(), path: "/items/7", query_string: "tags=one&tags=two", headers: Headers.new().with("content-type", "application/json"), body: Body.new("{\"title\":\"kept\",\"counts\":{\"a\":[2,3]}}".to_bytes())), path_parameters: {"id": id})
end
def load_input(): Input
input := context().bind<Input>() catch |_error|
return Input.new(body: Payload.new(title: "error", counts: {}), query: QueryValues.new(tags: [], limit: nil), params: PathValues.new(id: 0))
end
return input
end
def handle(context: Context): Response
input := context.bind<Input>() catch |error|
return Response.text(error_name(error), 400)
end
return Response.text(input.params.id.to_s() + ":" + input.query.tags.join(",") + ":" + input.body.title + ":" + input.body.counts["a"][1].to_s())
end
def probe()
original := context()
response := handle(original)
puts(response.body.to_s())
puts(original.request.query_string)
puts(original.path_value("id"))
failed := handle(context("bad"))
puts(failed.status)
puts(failed.body.to_s())
end
'''
expected = 'receiver\n7:one,two:kept:3\ntags=one&tags=two\n7\nreceiver\n400\npath-error\n'
retained_input = '''import { load_input } from producer
stored := load_input()
record Input
other: Integer
end
puts(stored.body.title)
puts(stored.query.tags.join(","))
puts(stored.params.id)
record Later
end
puts(stored.body.counts["a"][1])
puts(stored.query.limit == nil)
:quit
'''

with tempfile.TemporaryDirectory(prefix='native-web-input-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=90)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    (root / 'models.trb').write_text(models)
    (root / 'producer.trb').write_text(producer)
    main = root / 'main.trb'
    main.write_text('import { probe } from producer\ndef main()\nprobe()\nend\n')
    program = root / 'program'
    if args.reference:
        reference = args.reference.resolve()
        reference_config = root / 'reference.trbconfig.jsonc'
        reference_config.write_text(json.dumps({'name': 'web-input', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/web-input'}}))
        run([reference, 'check', '--config', reference_config])
        run([reference, 'build', '--config', reference_config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected

    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'web-input',
        'mode': 'go' if args.reference_only else 'trb', 'sourceDir': '.',
        'go': {'module': 'example.com/web-input'}}))
    repl_binary = reference if args.reference_only else binary
    retained = run([repl_binary, 'repl', '--config', config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.count('receiver\n') == 1, retained
    assert retained.stdout.endswith('kept\none,two\n7\n3\ntrue\n'), retained

    if not args.reference_only:
        # Stress composed input providers and their callers at ordinary managed
        # allocation sites, including nested containers and failing decodes.
        main.write_text('import { probe } from producer\ndef main()\n'
                        '(0...25).each do |_index|\nprobe()\nend\nend\n')
        emitted = run([binary, '--internal-driver', 'emit-qbe', main])
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
            env=dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=90)
        assert collected.returncode == 0 and collected.stdout == expected * 25, collected
        statistics = {}
        for line in collected.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['collections'] >= 25, statistics
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics

print('Web endpoint input composition, canonical retention and managed lifetime tests passed')
