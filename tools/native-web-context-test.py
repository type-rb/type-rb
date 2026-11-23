#!/usr/bin/env python3
"""Exercise typed context propagation across modules, retained sessions and forced GC."""
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

producer = r'''import { Context, ContextKey, Request } from trb/web
import { Body, Headers, HttpMethod } from trb/http
import trb/std/result
record User
id: Integer
name: String
roles: Array<String>
end
USER := ContextKey<User>.new("user")
COUNT := ContextKey<Integer>.new("count")
OPTIONAL := ContextKey<String?>.new("optional")
def request_for(path: String): Request
return Request.new(method: HttpMethod.get(), path: path, query_string: "", headers: Headers.new(), body: Body.empty())
end
def make_context(): Context
mut roles := ["reader"]
user := User.new(id: 42, name: "Ada", roles: roles)
none: String? := nil
context := Context.new(request: request_for("/original"), path_parameters: {"id" => "42"}).with(USER, user).with(COUNT, 1).with(OPTIONAL, none)
roles.push("writer")
return context
end
def describe_context(context: Context)
puts(context.request.path)
puts(context.path_value("id"))
case context.fetch(USER)
when Result::Ok(user)
puts(user.id)
puts(user.name)
puts(user.roles.join(","))
when Result::Err(error)
puts(error.key)
end
case context.fetch(COUNT)
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.key)
end
case context.fetch(OPTIONAL)
when Result::Ok(value)
puts(value == nil)
when Result::Err(error)
puts(error.key)
end
end
def probe()
original := make_context()
updated := original.with(COUNT, 2).with_request(request_for("/updated"))
describe_context(updated)
describe_context(original)
end
'''
original = '/original\n42\n42\nAda\nreader,writer\n1\ntrue\n'
expected = '/updated\n42\n42\nAda\nreader,writer\n2\ntrue\n' + original
retained_input = r'''import { make_context, describe_context } from context_values
stored := make_context()
record User
other: Integer
end
record ContextKey
name: String
end
describe_context(stored)
record Later
end
describe_context(stored)
:quit
'''

with tempfile.TemporaryDirectory(prefix='native-web-context-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=90)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    (root / 'context_values.trb').write_text(producer)
    main = root / 'main.trb'
    main.write_text('import { probe } from context_values\ndef main()\nprobe()\nend\n')
    program = root / 'program'
    if args.reference:
        reference = args.reference.resolve()
        config = root / 'reference.trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'context-values', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/context-values'}}))
        run([reference, 'check', '--config', config])
        run([reference, 'build', '--config', config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected

    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'context-values',
        'mode': 'go' if args.reference_only else 'trb', 'sourceDir': '.',
        'go': {'module': 'example.com/context-values'}}))
    repl_binary = reference if args.reference_only else binary
    retained = run([repl_binary, 'repl', '--config', config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.endswith(original * 2), retained

    if not args.reference_only:
        main.write_text('import { probe } from context_values\ndef main()\n'
                        '(0...8).each do |_index|\nprobe()\nend\nend\n')
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
        assert collected.returncode == 0 and collected.stdout == expected * 8, collected
        statistics = {}
        for line in collected.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['collections'] >= 8, statistics
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
        print('Context GC hooks:', hooks, 'statistics:', json.dumps(statistics, sort_keys=True))
print('Web context module, retained-session and storage checks passed.')
