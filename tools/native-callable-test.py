#!/usr/bin/env python3
"""Exercise retained checked code, capture cells, and cross-submission calls."""
import argparse
import os
import select
import signal
import time
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()

with tempfile.TemporaryDirectory(prefix='native-callable-') as temporary:
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

    # Named declarations retain their checked program across later submissions.
    run('def add(value: Integer): Integer; return value + 2; end\n'
        'saved := add\n:type saved\nsaved(3)\n'
        'record Later\nvalue: String\nend\nsaved(4)\n:reload\nsaved(5)',
        '#<callable> : (Integer) -> Integer\n(Integer) -> Integer\n'
        '5 : Integer\n6 : Integer\nreloaded\n7 : Integer\n')
    run('def invoke(callback: (Integer) -> Integer): Integer; return callback(3); end\n'
        'older := invoke\ndef add(value: Integer): Integer; return value + 2; end\n'
        'newer := add\nolder(newer)\nolder(add)',
        '#<callable> : ((Integer) -> Integer) -> Integer\n'
        '#<callable> : (Integer) -> Integer\n5 : Integer\n5 : Integer\n')
    run('def fail(value: Integer): Integer; return 10 / value; end\n'
        'saved := fail\nsaved(0)\nsaved(2)\nputs("after failure")',
        '#<callable> : (Integer) -> Integer\n5 : Integer\nafter failure\n',
        ('division by zero',))
    run('alias Callback = (Integer) -> Integer\n'
        'def add(value: Integer): Integer; return value + 2; end\n'
        'mut optional: Callback? := nil\noptional = add\noptional(7)\n'
        'optional = nil\noptional(7)\nputs("after nil")',
        'nil : ((Integer) -> Integer)? [mut]\n'
        '#<callable> : ((Integer) -> Integer)? [mut]\n9 : Integer\n'
        'nil : Nil [mut]\nafter nil\n', ('value is not directly callable',))

    # Bodies are collected atomically and never executed by :type or witnesses.
    run('f := fn(value: Integer): Integer\nputs("called")\nreturn value + 1\nend\n'
        ':type f\nf(2)\nf(3)\nputs("done")',
        '#<fn> : (Integer) -> Integer\n(Integer) -> Integer\n'
        'called\n3 : Integer\ncalled\n4 : Integer\ndone\n')
    run('f := fn(): Integer; return "wrong"; end\nputs("after invalid")',
        'after invalid\n', ('expected Integer, found String',))

    # A factory is evaluated once; two closures share the same escaping cell.
    run('def make(): () -> Integer\nputs("make")\nmut value := 10\n'
        'return fn(): Integer; value += 1; return value; end\nend\n'
        'f := make()\ng := f\nf()\ng()\n'
        'record Extra\nnumber: Integer\nend\nf()\n'
        ':type g\n:reload\nf()',
        'make\n#<fn> : () -> Integer\n#<fn> : () -> Integer\n'
        '11 : Integer\n12 : Integer\n13 : Integer\n() -> Integer\n'
        'make\nreloaded\n14 : Integer\n')

    # Old code invokes newer code, which calls back into another retained program.
    run('old := fn(callback: () -> Integer): Integer; return callback() + 1; end\n'
        'mut value := 40\nnewer := fn(): Integer; value += 1; return value; end\n'
        'old(newer)\nvalue\nold(newer)',
        '#<fn> : (() -> Integer) -> Integer\n40 : Integer [mut]\n'
        '#<fn> : () -> Integer\n42 : Integer\n41 : Integer [mut]\n43 : Integer\n')

    # Captured nullable proofs cannot survive unknown calls in later submissions.
    run('mut value: String? := nil\nclear := fn(); value = nil; end\n'
        'value = "kept"\nclear()\nvalue.size()\nvalue == nil\n'
        'value = "again"\nvalue.size()',
        'nil : String? [mut]\n#<fn> : () -> Void\n"kept" : String? [mut]\n'
        'true : Boolean\n"again" : String? [mut]\n5 : Integer\n',
        ('size is unavailable on String?',))

    # Failures restore the current program and preserve mutations already made.
    run('mut count := 0\nf := fn(): Integer\ncount += 1\nreturn 1 / 0\nend\n'
        'f()\ncount\nputs("recovered")\ncount += 1',
        '0 : Integer [mut]\n#<fn> : () -> Integer\n1 : Integer [mut]\n'
        'recovered\n2 : Integer [mut]\n', ('division by zero',))

    run('mut callbacks: Array<() -> Integer> := []\n'
        'callback := fn(): Integer; return callbacks.size(); end\n'
        'callbacks.push(callback)\ncallbacks[0]()\ncallback()',
        '[] : Array<() -> Integer> [mut]\n#<fn> : () -> Integer\n'
        '1 : Integer\n1 : Integer\n')
    run('mut recur: (Integer) -> Integer := fn(n: Integer): Integer; return n; end\n'
        'recur = fn(n: Integer): Integer\nif n == 0; return 0; end\n'
        'return recur(n - 1) + 1\nend\nrecur(8)\nrecur(3)',
        '#<fn> : (Integer) -> Integer [mut]\n#<fn> : (Integer) -> Integer [mut]\n'
        '8 : Integer\n3 : Integer\n')

    run('alias F = () -> Integer\nmut f: F? := nil\n:type f\n'
        'f = fn(): Integer; return 42; end\nf()\nf = nil\n:type f',
        'nil : (() -> Integer)? [mut]\n(() -> Integer)?\n'
        '#<fn> : (() -> Integer)? [mut]\n42 : Integer\nnil : Nil [mut]\nNil\n')

    # A function result need not have a finite eager constructor witness.
    run('record Link\nnext: () -> Link\nend\n'
        'def make(): Link\nreturn Link.new(next: fn(): Link; return make(); end)\nend\n'
        'link := make()\nlink.next().next()\n:type link.next',
        'Link(next: #<fn>) : Link\nLink(next: #<fn>) : Link\n() -> Link\n')

    (root / 'data.trb').write_text('record Box<T>\nvalue: T\nend\n'
                                 'enum Choice<T>\nOther(text: String)\nValue(value: T)\nend\n')
    # Imported nominal IDs shift after adding local records/enums. Values move in
    # both directions across catalogs, including containers and generic instances.
    run('import { Box, Choice } from data\n'
        'read := fn(box: Box<String>): String; return box.value; end\n'
        'choose := fn(choice: Choice<String>): String\ncase choice\n'
        'when Choice::Other(other)\nreturn other\nwhen Choice::Value(text)\nreturn text\nend\nend\n'
        'wrap := fn(text: String): Box<String>; return Box<String>.new(value: text); end\n'
        'record Local\nnumber: Integer\nend\nenum LocalChoice\nOnly\nend\n'
        'read(Box<String>.new(value: "new"))\nread(wrap("old"))\n'
        'choose(Choice<String>::Value("variant"))\nchoose(Choice<String>::Other("empty"))',
        '#<fn> : (Box<String>) -> String\n#<fn> : (Choice<String>) -> String\n'
        '#<fn> : (String) -> Box<String>\n"new" : String\n"old" : String\n'
        '"variant" : String\n"empty" : String\n')

    child = subprocess.Popen([str(binary), 'repl'], cwd=root, env=env,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             start_new_session=True, bufsize=0)
    try:
        child.stdin.write(b'mut count := 0\nrun := fn(): Integer\ncount += 1\n'
                          b'puts("ready")\nwhile true\nend\nreturn count\nend\nrun()\n')
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
        output, error = child.communicate(b'count\ncount += 1\n:quit\n', timeout=10)
        assert child.returncode == 0, error
        assert prefix + output == (b'0 : Integer [mut]\n#<fn> : () -> Integer\nready\n'
                                   b'1 : Integer [mut]\n2 : Integer [mut]\n'), (prefix, output, error)
        assert b'Interrupted' in error and len(error.splitlines()) == 1, error
    finally:
        if child.poll() is None:
            child.kill()
        child.communicate(timeout=10)

print('PASS retained REPL closures, captures, callbacks, nominal identities and recovery')
