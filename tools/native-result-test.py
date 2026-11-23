#!/usr/bin/env python3
"""Exercise Result ownership, retained values and lexical REPL transfers."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()

with tempfile.TemporaryDirectory(prefix='native result ') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def repl(source, expected, errors=()):
        result = subprocess.run([str(binary), 'repl'], cwd=root, env=env,
                                input=source + '\n:quit\n', text=True,
                                capture_output=True, timeout=30)
        assert result.returncode == 0, result
        assert result.stdout == expected, (source, result.stdout, result.stderr)
        assert len(result.stderr.splitlines()) == len(errors), result.stderr
        for error in errors:
            assert error in result.stderr, (error, result.stderr)

    repl('Result<Integer, String>::Ok(1)\nUnit.new()',
         'Result::Ok(value: 1) : Result<Integer, String>\nUnit() : Unit\n')
    held = 'Result::Err(error: "held") : Outcome<Integer, String>\n'
    repl('import { Result as Outcome } from trb/std/result\n'
         'def make(): Outcome<Integer, String>\n'
         'return Outcome<Integer, String>::Err("held")\nend\n'
         'value := make()\nvalue\n:type value\n'
         'enum Earlier\nOne\nend\nvalue\n:reload\nvalue',
         held + held + 'Outcome<Integer, String>\n' + held + 'reloaded\n' + held)
    repl('def empty()\nmut value := Result<Integer, String>::Ok(1)\n'
         'value = Result<Integer, String>::Ok(2)\nend\nempty()', '')
    repl('value := Result<Integer, String>::Ok(1)\n'
         'try value\nvalue',
         'Result::Ok(value: 1) : Result<Integer, String>\n' * 2,
         ('try requires the enclosing function to return Result',))
    repl('enum Result<T, E>\nOk(value: T)\nErr(error: E)\nend\n'
         'def ordinary()\nResult<Integer, String>::Ok(1)\n'
         '_unused := Result<Integer, String>::Ok(2)\nend\nordinary()', '')

    (root / 'helper.trb').write_text(
        'import trb/std/result\nimport trb/std/unit\n'
        'def work(): Result<Unit, String>\nreturn Result<Unit, String>::Ok(Unit.new())\nend\n')
    held = 'Result::Ok(value: Unit()) : Result<Unit, String>\n'
    repl('import { work } from helper\nvalue := work()\nvalue\n:reload\nvalue',
         held + held + 'reloaded\n' + held)

    # Project declaration roots retain their identity despite standard prelude names.
    (root / 'helper.trb').write_text('record Unit\nvalue: Integer\nend\n')
    (root / 'trbconfig.jsonc').write_text(json.dumps({'name': 'result-project', 'sourceDir': '.'}))
    repl('Unit.new(value: 7)', 'Unit(value: 7) : Unit\n')

print('PASS Result roots, aliases, retained values, reload and Void calls')
