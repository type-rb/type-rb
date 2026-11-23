#!/usr/bin/env python3
"""Exercise canonical typed Web binding, retained values and managed lifetimes."""
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
args = parser.parse_args()
binary = args.binary.resolve()

models = '''record Input
name: String
tags: Array<String>
page: Integer?
end
record PathInput
id: Integer
end
record Numbers
ids: Array<Integer>
end
'''
producer = '''import { Input as Query, PathInput, Numbers } from models
import { Request, Context, Response } from trb/web
import { Body, Headers, HttpMethod } from trb/http
import trb/std/result
def request(): Request
puts("receiver")
return Request.new(method: HttpMethod.get(), path: "/items/7", query_string: "name=kept&tags=one&tags=two", headers: Headers.new(), body: Body.empty())
end
def load_input(): Query
case request().query<Query>()
when Result::Ok(value)
return value
when Result::Err(_error)
return Query.new(name: "error", tags: [], page: nil)
end
end
def handle(context: Context): Response
case context.params<PathInput>()
when Result::Ok(path)
case context.request.query<Query>()
when Result::Ok(value)
return Response.text(value.name + ":" + value.tags.join(",") + ":" + path.id.to_s())
when Result::Err(_error)
return Response.text("query error", 400)
end
when Result::Err(_error)
return Response.text("path error", 400)
end
end
def error_probe()
input := Request.new(method: HttpMethod.get(), path: "/items", query_string: "ids=1&ids=invalid", headers: Headers.new(), body: Body.empty())
case input.query<Numbers>()
when Result::Ok(_value)
puts("unexpected")
when Result::Err(_error)
puts("invalid")
end
end
def probe()
input := request()
context := Context.new(request: input, path_parameters: {"id": "7"})
response := handle(context)
puts(response.body.to_s())
puts(input.query_string)
error_probe()
end
'''
expected = 'receiver\nkept:one,two:7\nname=kept&tags=one&tags=two\ninvalid\n'

with tempfile.TemporaryDirectory(prefix='native-web-binding-') as temporary:
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
    assert run([binary, 'check', main]).stdout == 'ok\n'
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == expected
    if args.reference:
        reference = args.reference.resolve()
        reference_config = root / 'reference.trbconfig.jsonc'
        reference_config.write_text(json.dumps({'name': 'web-binding', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/web-binding'}}))
        run([reference, 'check', '--config', reference_config])
        run([reference, 'build', '--config', reference_config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected

    # A later homonym and checked submission cannot change the retained record
    # identity or replay the receiver used by its generated binding function.
    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'web-binding', 'mode': 'trb', 'sourceDir': '.'}))
    retained = run([binary, 'repl', '--config', config], data='''import { load_input } from producer
stored := load_input()
record Input
other: Integer
end
puts(stored.name)
puts(stored.tags.join(","))
puts(stored.page == nil)
record Later
end
puts(stored.name)
:quit
''')
    assert not retained.stderr, retained
    assert retained.stdout.count('receiver\n') == 1, retained
    assert retained.stdout.endswith('kept\none,two\ntrue\nkept\n'), retained

    # Force collection before allocation sites in application and generated
    # provider functions, including error-capable record/Array/Hash operations.
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
    il, assembly = root / 'binding.ssa', root / 'binding.s'
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

print('Native typed Web binding, canonical retention and forced GC tests passed')
