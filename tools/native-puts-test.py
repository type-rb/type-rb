#!/usr/bin/env python3
"""Check builtin scalar output independently of REPL value inspection."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
reference = args.reference.resolve() if args.reference else None

with tempfile.TemporaryDirectory(prefix='native puts ') as temporary:
    root = Path(temporary)
    source = root / 'main.trb'
    source.write_text('''def count_once(mut calls: Array<Integer>): Integer
    calls[0] += 1
    return 123
end

def main()
    puts(123)
    puts(0)
    puts(-9007199254740991)
    puts(9007199254740991)
    puts(1.5)
    puts(-0.0)
    puts(true)
    puts(false)
    puts(1 < 2)
    puts("text")
    puts("")
    mut calls := [0]
    puts(count_once(calls))
    puts(calls[0])
end
''')
    expected = '123\n0\n-9007199254740991\n9007199254740991\n1.5\n0.0\ntrue\nfalse\ntrue\ntext\n\n123\n1\n'

    def run(tool, *arguments, text=None, success=True):
        result = subprocess.run([str(tool), *map(str, arguments)], cwd=root,
                                input=text, capture_output=True, text=True,
                                env=dict(os.environ, NO_COLOR='1',
                                         TRBN_HISTORY=str(root / 'history')),
                                timeout=60)
        assert (result.returncode == 0) == success, result
        return result

    for tool in [binary] + ([reference] if reference else []):
        result = run(tool, 'run', source)
        assert result.stdout == expected and not result.stderr, result
        result = run(tool, 'repl', text='puts(123)\nputs(true)\nputs(false)\nputs(1.5)\nputs(-0.0)\n:quit\n')
        assert result.stdout == '123\ntrue\nfalse\n1.5\n0.0\n' and not result.stderr, result

    for expression in ['puts()', 'puts(1, 2)', 'puts(1 + true)', 'takes_string(123)']:
        source.write_text('def takes_string(value: String)\nend\ndef main()\n' + expression + '\nend\n')
        assert 'TRBN4004' in run(binary, 'check', source, success=False).stderr

    # Wider output formatting remains explicit unsupported coverage, not Any.
    for expression, typ in [('[1, 2]', 'Array<Integer>')]:
        source.write_text('def main()\nputs(' + expression + ')\nend\n')
        assert 'puts does not yet support ' + typ in run(binary, 'check', source, success=False).stderr

print('Native scalar puts, reference output and strict argument checks passed')
