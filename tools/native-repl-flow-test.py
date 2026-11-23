#!/usr/bin/env python3
"""Check retained semantic facts without replaying interactive side effects."""
import argparse
import os
from pathlib import Path
import select
import signal
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()

with tempfile.TemporaryDirectory(prefix='native-repl-flow-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(source, expected, errors=()):
        result = subprocess.run([str(binary), 'repl'], input=source + '\n:quit\n',
                                text=True, capture_output=True, cwd=root, env=env, timeout=30)
        assert result.returncode == 0, result
        assert result.stdout == expected, (source, result.stdout, result.stderr)
        assert len(result.stderr.splitlines()) == len(errors), result.stderr
        for message in errors:
            assert message in result.stderr, (message, result.stderr)

    run('mut values := []\n:type values\nvalues.empty?()\nvalues.size()\n'
        'if true\nvalues.push(1)\nelse\nvalues.push(2.5)\nend\nvalues',
        '[] : Array<Any> [mut]\nArray<Any>\ntrue : Boolean\n0 : Integer\n'
        '[1] : Array<Float> [mut]\n')
    run('mut values := []\nvalues.unshift(3)\nvalues\nmut fields := {}\n'
        'fields["count"] = 1\nfields',
        '[] : Array<Any> [mut]\n[3] : Array<Integer> [mut]\n'
        '{} : Hash [mut]\n1 : Integer\n'
        '{"count": 1} : Hash<String, Integer> [mut]\n')
    run('mut values := [\n]\n:reload\nvalues.push(1)\nvalues',
        '[] : Array<Any> [mut]\nreloaded\n[1] : Array<Integer> [mut]\n')
    run('mut values := []\nvalues.push(1)\nvalues.push("wrong")',
        '[] : Array<Any> [mut]\n',
        ('expected Integer, found String',))

    run('mut value: String? := nil\nvalue = "kept"\n:type value\nvalue.size()\n'
        'value = nil\n:type value\nvalue == nil',
        'nil : String? [mut]\n"kept" : String? [mut]\nString\n4 : Integer\n'
        'nil : Nil [mut]\nNil\ntrue : Boolean\n')
    run('mut value: String? := "hello"\nif true\nvalue = nil\nend\n'
        'value = "again"\nwhile value != nil\nvalue = nil\nend\n'
        'h := {1 => {"a" => 2}}',
        '"hello" : String? [mut]\nnil : Nil [mut]\n'
        '"again" : String? [mut]\nnil : Nil [mut]\n'
        '{1: {"a": 2}} : Hash<Integer, Hash<String, Integer>>\n')
    run('mut visits := 0\nif true\nvisits += 1\nend\nvisits\n'
        'mut h := {"a" => [1]}\nif true\nh = {"b" => [2]}\nend\nh',
        '0 : Integer [mut]\n1 : Integer [mut]\n1 : Integer [mut]\n'
        '{"a": [1]} : Hash<String, Array<Integer>> [mut]\n'
        '{"b": [2]} : Hash<String, Array<Integer>> [mut]\n'
        '{"b": [2]} : Hash<String, Array<Integer>> [mut]\n')
    run('mut number := 1\ncase number\nwhen 1\nnumber += 2\nelse\nnumber += 10\n'
        'end\nif false\nnumber += 100\nend\nnumber',
        '1 : Integer [mut]\n3 : Integer [mut]\n3 : Integer [mut]\n')
    run('mut events := [1]\nmut value: String? := nil\nevents.push(2)\n'
        'value = "kept"\n# a leading comment\n\nvalue.size()\nputs(events.size())',
        '[1] : Array<Integer> [mut]\nnil : String? [mut]\n"kept" : String? [mut]\n4 : Integer\n2\n')
    run('mut value: String? := nil\nvalue = "kept"\nvalue = 2\n:type value\nvalue.size()',
        'nil : String? [mut]\n"kept" : String? [mut]\nString\n4 : Integer\n',
        ('expected String?, found Integer',))

    run('mut values := [1]\nvalues[0] = 2\nvalues[0] += 1\nvalues[0]\n(values[0])\n'
        'mut count := 1\ncount += 1\n(count)\ncount + 1',
        '[1] : Array<Integer> [mut]\n2 : Integer\n3 : Integer\n3 : Integer\n3 : Integer\n'
        '1 : Integer [mut]\n2 : Integer [mut]\n2 : Integer [mut]\n3 : Integer\n')

    for body in ('if true\nvalue = nil\nputs(1 / 0)\nend',
                 'value = if true\nvalue = nil\nputs(1 / 0)\n"other"\nelse\n"unused"\nend'):
        run('mut value: String? := nil\nvalue = "kept"\n' + body +
            '\n:type value\nvalue == nil\nvalue.size()\nvalue = "again"\nvalue.size()',
            'nil : String? [mut]\n"kept" : String? [mut]\nString?\ntrue : Boolean\n'
            '"again" : String? [mut]\n5 : Integer\n',
            ('division by zero', 'size is unavailable on String?'))

    run('mut value: String? := nil\nvalue = "kept"\n'
        'while true\nputs(value.size())\nvalue = nil\nend\nvalue.size()',
        'nil : String? [mut]\n"kept" : String? [mut]\n4 : Integer\n',
        ('size is unavailable on String?',))
    run('mut value: String? := nil\nvalue = "kept"\n'
        'record Extra\nnumber: Integer\nend\n:type value\nvalue.size()\n'
        ':reload\n:type value\nvalue.size()',
        'nil : String? [mut]\n"kept" : String? [mut]\nString\n4 : Integer\n'
        'reloaded\nString\n4 : Integer\n')

    run('mut events := [1]\nevents.push(2)\nmut value: String? := nil\nvalue = "kept"\n'
        'if true\nevents.push(3)\nvalue = nil\nputs(1 / 0)\nend\n'
        'value == nil\nrecord Extra\nnumber: Integer\nend\n:reload\n:type value\n'
        'if value != nil\nputs(value.size())\nend\nputs(events.size())',
        '[1] : Array<Integer> [mut]\nnil : String? [mut]\n"kept" : String? [mut]\n'
        'true : Boolean\nreloaded\nString?\n4\n2\n', ('division by zero',))

    run('mut value: String? := nil\nvalue = "kept"\n'
        'if true\nvalue = nil\nputs(1 / 0)\nend\n:reload\n:type value\nvalue == nil',
        'nil : String? [mut]\n"kept" : String? [mut]\nreloaded\nString?\nfalse : Boolean\n',
        ('division by zero',))

    run('mut value: String? := nil\nreturn if value == nil\nvalue = "kept"\nvalue.size()',
        'nil : String? [mut]\n"kept" : String? [mut]\n4 : Integer\n',
        ('return is only valid inside a function or method',))

    (root / 'more.trb').write_text('record Entry\nvalue: String\nend\n'
                                   'def label(): String\nreturn "imported"\nend\n')
    run('mut value: String? := nil\nvalue = "kept"\n'
        'import { label } from more\n:type value\nvalue.size()\nlabel()',
        'nil : String? [mut]\n"kept" : String? [mut]\nString\n4 : Integer\n"imported" : String\n')

    run('import { Entry as Visible } from more\nmut value: Visible? := nil\n'
        'value = Visible.new(value: "kept")\nrecord Extra\nnumber: Integer\nend\n'
        ':type value\nvalue.value.size()\nvalue.value = "invalid"\n:reload\n'
        ':type value\nvalue.value.size()',
        'nil : Visible? [mut]\nEntry(value: "kept") : Visible? [mut]\nVisible\n'
        '4 : Integer\nreloaded\nVisible\n4 : Integer\n', ('field is readonly',))

    (root / 'loaded.input').write_text('if value != nil\nputs(value.size())\nend\n')
    run('mut value: String? := nil\nvalue = "kept"\n'
        'if true\nvalue = nil\nputs(1 / 0)\nend\nvalue == nil\n'
        ':load loaded.input\n:type value\nvalue == nil',
        'nil : String? [mut]\n"kept" : String? [mut]\ntrue : Boolean\n4\n'
        'String?\nfalse : Boolean\n', ('division by zero',))

    run('mut number: Float? := nil\nnumber = 3\n:type number\nnumber.to_i()\n'
        'def assign(): String\nmut inner: String? := nil\ninner = "inner"\nreturn inner\nend\nassign()',
        'nil : Float? [mut]\n3 : Float? [mut]\nFloat\n3 : Integer\n"inner" : String\n')

    child = subprocess.Popen([str(binary), 'repl'], cwd=root, env=env,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             start_new_session=True, bufsize=0)
    try:
        child.stdin.write(b'mut value: String? := nil\nvalue = "kept"\n'
                          b'if true\nvalue = nil\nputs("ready")\nwhile true\nend\nend\n')
        prefix = b''
        deadline = time.monotonic() + 10
        while b'ready\n' not in prefix:
            remaining = deadline - time.monotonic()
            assert remaining > 0 and child.poll() is None, prefix
            readable, _, _ = select.select([child.stdout], [], [], remaining)
            assert readable, prefix
            chunk = os.read(child.stdout.fileno(), 65536)
            assert chunk, prefix
            prefix += chunk
        child.send_signal(signal.SIGINT)
        output, error = child.communicate(b':type value\nvalue == nil\n:quit\n', timeout=10)
        assert child.returncode == 0, error
        assert prefix + output == b'nil : String? [mut]\n"kept" : String? [mut]\nready\nString?\ntrue : Boolean\n', (prefix, output, error)
        assert b'Interrupted' in error and len(error.splitlines()) == 1, error
    finally:
        if child.poll() is None:
            child.kill()
        child.communicate(timeout=10)

print('PASS retained REPL flow, failures, interruption, declarations, imports and replay')
