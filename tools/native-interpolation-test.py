#!/usr/bin/env python3
"""Reference differential for shared compiled and REPL interpolation."""
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


def invoke(tool, root, command, source=None):
    return subprocess.run([str(tool), *map(str, command)], cwd=root, input=source,
                          text=True, capture_output=True, timeout=45,
                          env=dict(os.environ, NO_COLOR='1', TRBN_HISTORY=str(root / 'history')))


with tempfile.TemporaryDirectory(prefix='native-interpolation-') as directory:
    root = Path(directory).resolve()
    path = root / 'main.trb'
    declarations = '''def piece(mut calls: Array<Integer>, text: String): String
  calls[0] += 1
  puts(text)
  return text
end
'''
    body = '''s := "abc"
puts("hello #{s}")
puts("#{s}#{s}")
puts("#{"inner"}")
puts("#{"#{"nested"}"}")
puts("#{"}"}")
puts("#{1.to_s()} #{2.to_s()}")
puts("a\\nb #{s}\\tend")
puts("#{}")
mut calls := [0]
puts("#{piece(calls, "left")} #{piece(calls, "right")}")
puts(calls[0])
puts("#{s}".size())
'''
    expected = 'hello abc\nabcabc\ninner\nnested\n}\n1 2\na\nb abc\tend\n#{}\nleft\nright\nleft right\n2\n3\n'
    path.write_text(declarations + 'def main()\n' + body + 'end\n')
    for tool in [binary] + ([reference] if reference else []):
        result = invoke(tool, root, ('run', path))
        assert result.returncode == 0 and result.stderr == '' and result.stdout == expected, result
        # REPL declarations and mutable state are evaluated by its own evaluator.
        result = invoke(tool, root, ('repl',), declarations + body + ':quit\n')
        assert result.returncode == 0 and result.stderr == '', result
        assert 'hello abc\n' in result.stdout and 'left\nright\nleft right\n' in result.stdout, result
        assert '#{s}' not in result.stdout and '2\n3\n' in result.stdout, result

    for expression in ['123', 'true', '[1]', 'missing']:
        source = 'puts("hello #{' + expression + '}")'
        path.write_text('def main()\n' + source + '\nend\n')
        for tool in [binary] + ([reference] if reference else []):
            result = invoke(tool, root, ('check', path))
            assert result.returncode != 0 and result.stdout == '' and result.stderr, result
            result = invoke(tool, root, ('repl',), source + '\nputs("recovered")\n:quit\n')
            assert result.stderr and result.stdout == 'recovered\n', result

    escaped = r'''s := "abc"
puts("aaa\#{missing}")
puts("aaa\\#{s}")
puts("aaa\\\#{missing}")
puts("aaa\#{s} #{s}")
puts("#{"\#{missing}"}")
puts("\\n #{s}\nend")
'''
    escaped_output = 'aaa#{missing}\naaa\\abc\naaa\\#{missing}\naaa#{s} abc\n#{missing}\n\\n abc\nend\n'
    path.write_text('def main()\n' + escaped + 'end\n')
    for tool in [binary] + ([reference] if reference else []):
        result = invoke(tool, root, ('run', path))
        assert result.returncode == 0 and result.stderr == '' and result.stdout == escaped_output, result
        result = invoke(tool, root, ('repl',), escaped + ':quit\n')
        assert result.returncode == 0 and result.stderr == '' and result.stdout == '"abc" : String\n' + escaped_output, result

    for literal in [r'"aaa#\{s}"', r'"\q"', r'"\q #{"ok"}"', r'"#{"ok"}\q"', r'"#{"\q"}"']:
        statement = 'puts(' + literal + ')'
        path.write_text('def main()\n' + statement + '\nend\n')
        for tool in [binary] + ([reference] if reference else []):
            result = invoke(tool, root, ('check', path))
            assert result.returncode != 0 and result.stdout == '' and 'escape' in result.stderr, result
            result = invoke(tool, root, ('repl',), statement + '\nputs("recovered")\n:quit\n')
            assert 'escape' in result.stderr and result.stdout == 'recovered\n', result

    # Token expansion must fit source-sized REPL probes and hidden input.
    dense = 'puts("' + '#{s}' * 300 + '")'
    result = invoke(binary, root, ('repl',), 's := "z"\n' + dense + '\n:quit\n')
    assert result.returncode == 0 and result.stderr == '' and 'z' * 300 + '\n' in result.stdout, result

    path.write_text('def main()\n s := "abc"\n puts("hello #{\n s\n }")\nend\n')
    for tool in [binary] + ([reference] if reference else []):
        result = invoke(tool, root, ('run', path))
        assert result.returncode == 0 and result.stderr == '' and result.stdout == 'hello abc\n', result
    path.write_text('def main()\n puts("#{\n missing\n }")\nend\n')
    result = invoke(binary, root, ('check', path))
    assert result.returncode != 0 and ':3: error[TRBN4003]:' in result.stderr, result

    # Line origins survive lowering and nested lexical failures.
    path.write_text('def main()\n\n  puts("hello #{missing}")\nend\n')
    result = invoke(binary, root, ('check', path))
    assert ':3: error[' in result.stderr, result
    path.write_text('def main()\n  puts("hello #{"bad\\q"}")\nend\n')
    result = invoke(binary, root, ('check', path))
    assert result.returncode != 0 and ':2: error[' in result.stderr, result
    # Unclosed interpolation is diagnosed, never emitted literally.
    path.write_text('def main()\n  puts("hello #{missing")\nend\n')
    result = invoke(binary, root, ('check', path))
    assert result.returncode != 0 and 'unterminated' in result.stderr, result
print('Native String interpolation, evaluation order, type checks and reference comparison passed')
