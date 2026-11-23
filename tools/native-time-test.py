#!/usr/bin/env python3
"""Exercise portable time services and codecs across modules, retained REPL values and forced GC."""
import argparse
from native_network_link import static_network_flags
import json
import os
from pathlib import Path
import subprocess
import struct
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
parser.add_argument('--reference-only', action='store_true')
args = parser.parse_args()
if args.reference_only and not args.reference:
    parser.error('--reference-only requires --reference')
binary = args.binary.resolve()

models = r'''import { Date, TimeOfDay, DateTime, Instant, Duration, TimeZone } from trb/std/time
record PathValues
date: Date
end
record QueryValues
clock: TimeOfDay
zone: TimeZone
end
record Payload
civil: DateTime
instant: Instant
duration: Duration
end
record Input
params: PathValues
query: QueryValues
body: Payload
end
'''
producer = r'''import { Input, Payload, PathValues, QueryValues } from models
import { Date, TimeOfDay, DateTime, Instant, Duration, TimeZone } from trb/std/time
import { Context, Request } from trb/web
import { Body, Headers, HttpMethod } from trb/http
import trb/std/json
import trb/std/result
def load_input(): Input
puts("receiver")
context := Context.new(request: Request.new(method: HttpMethod.post(), path: "/events", query_string: "clock=12%3A34%3A56.123&zone=America%2FNew_York", headers: Headers.new().with("content-type", "application/json"), body: Body.new("{\"civil\":\"2024-07-01T12:34:56\",\"instant\":\"2024-07-01T16:34:56Z\",\"duration\":\"-PT0.5S\"}".to_bytes())), path_parameters: {"date": "2024-07-01"})
return context.bind<Input>() catch |_error|
return Input.new(params: PathValues.new(date: Date.new(2000, 1, 1)), query: QueryValues.new(clock: TimeOfDay.new(0, 0), zone: TimeZone.utc()), body: Payload.new(civil: DateTime.new(2000, 1, 1, 0, 0), instant: Instant.new(0), duration: Duration.new(0)))
end
end
def encode_input(value: Input)
case JSON.encode(value)
when Result::Ok(text)
puts(text)
when Result::Err(error)
puts(error.message)
end
end
def probe()
input := load_input()
puts(input.params.date.add_days(1).to_s())
puts(input.query.clock.to_s())
puts(input.body.civil.to_instant(input.query.zone).to_s())
puts(input.body.instant.to_datetime(input.query.zone).to_s())
puts(input.body.instant.add(input.body.duration).to_s())
encode_input(input)
case JSON.decode<Payload>("{\"civil\":\"2024-07-01T12:34\",\"instant\":\"bad\",\"duration\":\"PT0S\"}")
when Result::Ok(_value)
puts("unexpected")
when Result::Err(error)
puts(error.path)
puts(error.message)
end
end
'''
encoded = '{"body":{"civil":"2024-07-01T12:34:56","duration":"-PT0.5S","instant":"2024-07-01T16:34:56Z"},"params":{"date":"2024-07-01"},"query":{"clock":"12:34:56.123","zone":"America/New_York"}}\n'
expected = 'receiver\n2024-07-02\n12:34:56.123\n2024-07-01T16:34:56Z\n2024-07-01T12:34:56\n2024-07-01T16:34:55.5Z\n{"body":{"civil":"2024-07-01T12:34:56","duration":"-PT0.5S","instant":"2024-07-01T16:34:56Z"},"params":{"date":"2024-07-01"},"query":{"clock":"12:34:56.123","zone":"America/New_York"}}\n/instant\ninvalid Instant\n'
retained_input = r'''import { load_input, encode_input } from producer
stored := load_input()
record Input
other: Integer
end
puts(stored.params.date.to_s())
puts(stored.query.zone.identifier())
puts(stored.body.duration.to_s())
record Later
end
encode_input(stored)
:quit
'''

with tempfile.TemporaryDirectory(prefix='native-time-') as temporary:
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
        reference_config.write_text(json.dumps({'name': 'time-services', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/time-services'}}))
        run([reference, 'check', '--config', reference_config])
        run([reference, 'build', '--config', reference_config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected

    main.unlink()
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'time-services',
        'mode': 'go' if args.reference_only else 'trb', 'sourceDir': '.',
        'go': {'module': 'example.com/time-services'}}))
    repl_binary = reference if args.reference_only else binary
    retained = run([repl_binary, 'repl', '--config', config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.count('receiver\n') == 1, retained
    assert retained.stdout.endswith('2024-07-01\nAmerica/New_York\n-PT0.5S\n' + encoded), retained

    if not args.reference_only:
        # Stress composed input providers and their callers at ordinary managed
        # allocation sites, including nested containers and failing decodes.
        main.write_text('import { probe } from producer\ndef main()\n'
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


# Stage the actual TypeRB helper bodies as ordinary modules. Only import paths
# change; no helper implementation is duplicated in this test. This lets the
# exact reference and Native check malformed binary input without exposing a
# public import of loader-owned internals.
with tempfile.TemporaryDirectory(prefix='native-time-zones-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb')
    repository = Path(__file__).resolve().parent.parent
    for name in ('time_calendar', 'time_parse', 'time_zones'):
        source = (repository / 'compiler/packages/internal' / (name + '.trb')).read_text()
        source = source.replace('trb/internal/time_calendar', 'time_calendar')
        source = source.replace('trb/internal/time_parse', 'time_parse')
        source = source.replace('trb/internal/time_host', 'time_host')
        (root / (name + '.trb')).write_text(source)
    (root / 'time_host.trb').write_text('module TimeHost\ndef self.zone_data(_path: String, _start: Integer, _length: Integer, mut _parts: Array<String>): Integer\nreturn 2\nend\ndef self.zone_environment(mut _parts: Array<String>): Boolean\nreturn false\nend\nend\n')

    def header(version=b'\0', *, times=0, types=1, chars=4):
        return b'TZif' + version + bytes(15) + struct.pack('>6I', 0, 0, 0, times, types, chars)

    block = struct.pack('>iBB', 3600, 0, 0) + b'FIX\0'
    valid = header() + block
    fixtures = [b'', b'TZif', b'bad!' + valid[4:], header(b'4') + block,
                header(types=0) + block, header(chars=0) + block, valid[:-1],
                header(times=0xFFFFFFFF) + block,
                header() + struct.pack('>iBB', 3600, 2, 0) + b'FIX\0',
                header() + struct.pack('>iBB', 3600, 0, 4) + b'FIX\0',
                header(times=1) + struct.pack('>iB', 0, 1) + block,
                header(b'2') + block + b'TZif']
    lines = ['puts(decode("' + ''.join('\\x%02x' % byte for byte in data)
             + '".to_bytes()).error != "")' for data in fixtures]
    expected_helpers = 'true\n' * len(fixtures)
    for fixture_index, data in enumerate((valid, header(b'2') + block + header(b'2') + block + b'\nFIX-1\n',
                 header(b'3') + block + header(b'3') + block + b'\nFIX-1\n')):
        literal = ''.join('\\x%02x' % byte for byte in data)
        zone = 'zone' + str(fixture_index)
        lines.extend([zone + ' := decode("' + literal + '".to_bytes())',
                      'puts(' + zone + '.error == "")', 'puts(offset(' + zone + ', 0))',
                      'puts(offset(' + zone + ', 253402300799))'])
        expected_helpers += 'true\n3600\n3600\n'
    main = root / 'main.trb'
    main.write_text('import { decode, offset } from time_zones\ndef main()\n' + '\n'.join(lines) + '\nend\n')
    program = root / 'program'
    if args.reference:
        config = root / 'reference.trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'time-zone-parser', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/time-zone-parser'}}))
        run([reference, 'check', '--config', config])
        run([reference, 'build', '--config', config, '--compile', '--outfile', program])
        assert run([program]).stdout == expected_helpers
    if not args.reference_only:
        assert run([binary, 'check', main]).stdout == 'ok\n'
        run([binary, 'build', '--compile', main, '--outfile', program])
        assert run([program]).stdout == expected_helpers

print('Time value, zone, codec, retained REPL and managed lifetime tests passed')
