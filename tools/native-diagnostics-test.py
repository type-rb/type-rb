#!/usr/bin/env python3
"""Compare public diagnostic origins with the pinned reference renderer."""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
reference = args.reference.resolve() if args.reference else None


def run(tool, root, command, source=None):
    result = subprocess.run([str(tool), *map(str, command)], cwd=root,
                            input=source, text=True, capture_output=True,
                            env=dict(os.environ, NO_COLOR='1',
                                     TRBN_HISTORY=str(root / 'history')))
    assert '\x1b' not in result.stderr, result
    return result


def origin(text):
    # Compare shared locations without erasing each diagnostic's own span.
    match = re.fullmatch(r'(.+?):(\d+)(?::\d+)?: (error)(?:\[[A-Z0-9]+\])?: .+\n', text)
    assert match, text
    return match.groups()


with tempfile.TemporaryDirectory(prefix='native-diagnostics-') as temporary:
    root = Path(temporary).resolve()
    entry = root / 'main.trb'
    entry.write_text('def main()\n  missing\nend\n')
    expected = (str(entry), '2', 'error')
    for command in [('check', entry), ('build', entry, '--stdout'),
                    ('build', entry), ('build', entry, '--compile'), ('run', entry)]:
        result = run(binary, root, command)
        assert result.returncode == 1 and result.stdout == '', result
        assert origin(result.stderr) == expected, result
    if reference:
        result = run(reference, root, ('check', entry))
        assert result.returncode == 1 and origin(result.stderr) == expected, result
    assert not list(root.rglob('*.ssa')), list(root.rglob('*'))
    assert not (root / 'bin/main').exists()

    library = root / 'helper.trb'
    library.write_text('def answer(): Integer\n  return missing\nend\n')
    entry.write_text('import { answer } from helper\ndef main()\n  puts(answer())\nend\n')
    result = run(binary, root, ('check', entry))
    assert origin(result.stderr) == (str(library), '2', 'error'), result
    if reference:
        assert origin(run(reference, root, ('check', entry)).stderr) == origin(result.stderr)

    # Project mode and imported REPL modules must keep the real file origin.
    (root / 'trbconfig.jsonc').write_text('{"name":"diagnostics","mode":"trb","sourceDir":"."}')
    result = run(binary, root, ('check',))
    assert origin(result.stderr) == (str(library), '2', 'error'), result
    (root / 'trbconfig.jsonc').unlink()
    entry.unlink()
    result = run(binary, root, ('repl',), 'import { answer } from helper\n:quit\n')
    assert origin(result.stderr) == (str(library), '2', 'error'), result
    library.unlink()

    # Binary and unary errors own the actual operator token. The reference
    # currently uses the expression start; do not invent matching columns.
    for expression, column, message in (
            ('s+"aaa"', 2, 'operator + does not support Integer and String'),
            ('s + "aaa"', 3, 'operator + does not support Integer and String'),
            ('\ts + "aaa"', 4, 'operator + does not support Integer and String'),
            ('!s', 1, 'operator ! does not support Integer')):
        entry.write_text('def main()\ns := 123\n' + expression + '\nend\n')
        for command in [('check', entry), ('build', entry, '--stdout')]:
            result = run(binary, root, command)
            assert (result.returncode, result.stdout, result.stderr) == (
                1, '', f'{entry}:3:{column}: error[TRBN4004]: {message}\n'), result
        result = run(binary, root, ('repl',), 's := 123\n' + expression + '\nputs(s + 1)\n:quit\n')
        assert result.stderr == f'(trb):2:{column}: error[TRBN4004]: {message}\n', (expression, result)
        assert result.returncode == 0 and result.stdout.endswith('124\n'), result
    entry.unlink()

    library.write_text('def answer(): Integer\n\t名前 := 1\n\treturn 名前 + "x"\nend\n')
    entry.write_text('import { answer } from helper\ndef main()\nputs(answer())\nend\n')
    imported = run(binary, root, ('check', entry))
    assert imported.stderr == (f'{library}:3:12: error[TRBN4004]: '
                               'operator + does not support Integer and String\n'), imported
    entry.unlink()
    library.unlink()

    cases = [
        ('missing\n', [1]),
        ('1 + true\n', [1]),
        ('s:= "hello"\ns*2\n', [2]),
        ('s := "hello"\ns * 2\n', [2]),
        ('mut x := 1\nmissing\nmissing\n', [2, 2]),
        ('1\nmissing\n', [2]),
        ('def answer(): Integer\n  return true\nend\n', [2]),
        ('def answer(): Integer\n  return 1\nend\nmut x := 2\nmissing\n', [5]),
        ('mut x := 1\n1 / 0\nmissing\n', [2, 2]),
    ]
    for source, lines in cases:
        result = run(binary, root, ('repl',), source + ':quit\n')
        assert result.returncode == 0, result
        actual = [origin(line + '\n') for line in result.stderr.splitlines()]
        assert actual == [('(trb)', str(line), 'error') for line in lines], (source, result)
        if reference:
            baseline = run(reference, root, ('repl',), source + ':quit\n')
            assert actual == [origin(line + '\n') for line in baseline.stderr.splitlines()], baseline
        if '1 / 0' in source:
            assert '(trb):2: error: division by zero\n' in result.stderr, result

    for expression, message in (
            ('2 ** -1', 'negative Integer exponent'),
            ('2 ** 53', 'Integer is outside the portable range')):
        entry.write_text('def main()\n puts(' + expression + ')\nend\n')
        executable = root / 'power-program'
        built = run(binary, root, ('build', '--compile', '--outfile', executable, entry))
        assert built.returncode == 0 and executable.is_file(), built
        failed = run(executable, root, ())
        assert (failed.returncode == 2 and failed.stdout == ''
                and failed.stderr == 'panic: ' + message + '\n'), failed
        interactive = run(binary, root, ('repl',),
                          'puts(' + expression + ')\nputs(5)\n:quit\n')
        assert (interactive.returncode == 0 and interactive.stdout == '5\n'
                and interactive.stderr == '(trb):1: error: ' + message + '\n'), interactive
        executable.unlink()
    entry.unlink()

    loaded = root / 'session.trb'
    loaded.write_text('mut x := 1\ndef answer(): Integer\n  return true\nend\n')
    result = run(binary, root, ('repl',), f':load {loaded}\nputs(123)\n:quit\n')
    assert origin(result.stderr) == ('(trb)', '3', 'error'), result
    assert result.stdout == '123\n', result
    loaded.write_text('mut x := 1\ndef answer(): Integer\n  return 1\nend\nmissing\n')
    result = run(binary, root, ('repl',), f':load {loaded}\n:quit\n')
    assert origin(result.stderr) == ('(trb)', '5', 'error'), result

    # Rejected input never enters the session; recovery remains usable.
    result = run(binary, root, ('repl',), 'missing\nputs(123)\n:quit\n')
    assert result.stdout == '123\n' and len(result.stderr.splitlines()) == 1, result
print('Native file/project/REPL diagnostic origins and reference presentation passed')
