#!/usr/bin/env python3
"""Exercise typed JSON across modules, retained REPL values and forced GC."""
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

models = '''newtype Count = Integer
enum Mode
Ready = "ready"
end
record Input
name: String @json("display_name")
counts: Hash<String, Array<Count>>
mode: Mode
note: String?
end
'''
producer = r'''import { Input as Payload, Count, Mode } from models
import { Request, Response } from trb/web
import { Body, Headers, HttpMethod } from trb/http
import trb/std/json as Codec
import trb/std/result
def request(): Request
puts("receiver")
return Request.new(method: HttpMethod.post(), path: "/items", query_string: "", headers: Headers.new().with("content-type", "application/json"), body: Body.new("{\"display_name\":\"kept\",\"counts\":{\"a/~\":[2,3]},\"mode\":\"ready\"}".to_bytes()))
end
def load_input(): Payload
case request().json<Payload>()
when Result::Ok(value)
return value
when Result::Err(_error)
return Payload.new(name: "error", counts: {}, mode: Mode::Ready, note: nil)
end
end
def authored_value(): Payload
puts("value")
return Payload.new(name: "kept", counts: {"a/~": [Count.new(2), Count.new(3)]}, mode: Mode::Ready, note: nil)
end
def status(): Integer
puts("status")
return 201
end
def encode_probe(value: Payload)
case Codec.encode(value)
when Result::Ok(encoded)
puts(encoded)
when Result::Err(error)
puts(error.message)
end
end
def error_probe()
case Codec.decode<Payload>("{\"display_name\":\"kept\",\"counts\":{\"a/~\":[2,\"invalid\"]},\"mode\":\"ready\"}")
when Result::Ok(_value)
puts("unexpected")
when Result::Err(error)
puts(error.path)
puts(error.message)
end
end
def probe()
input := load_input()
puts(input.counts["a/~"][1].value())
encode_probe(input)
response := Response.json(authored_value(), status())
puts(response.status)
puts(response.body.to_s())
error_probe()
end
'''
encoded = '{"counts":{"a/~":[2,3]},"display_name":"kept","mode":"ready","note":null}\n'
expected = 'receiver\n3\n' + encoded + 'value\nstatus\n201\n' + encoded + '/counts/a~1~0/1\nexpected Integer\n'
retained_input = '''import { load_input, encode_probe } from producer
stored := load_input()
record Input
other: Integer
end
puts(stored.name)
puts(stored.counts["a/~"][0].value())
encode_probe(stored)
record Later
end
puts(stored.name)
:quit
'''

with tempfile.TemporaryDirectory(prefix='native-json-codec-') as temporary:
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
        reference_config.write_text(json.dumps({'name': 'json-codec', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/json-codec'}}))
        run([reference, 'check', '--config', reference_config])
        run([reference, 'build', '--config', reference_config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected

    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'json-codec',
        'mode': 'go' if args.reference_only else 'trb', 'sourceDir': '.',
        'go': {'module': 'example.com/json-codec'}}))
    repl_binary = reference if args.reference_only else binary
    retained = run([repl_binary, 'repl', '--config', config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.count('receiver\n') == 1, retained
    assert retained.stdout.endswith('kept\n2\n' + encoded + 'kept\n'), retained

    if not args.reference_only:
        # Stress generated codecs and their callers at ordinary managed
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
        il, assembly = root / 'codec.ssa', root / 'codec.s'
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

print('Typed JSON module composition, canonical retention and managed lifetime tests passed')
