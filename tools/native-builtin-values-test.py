#!/usr/bin/env python3
"""Keep builtin Result values and Unicode names across REPL catalog changes."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-builtin-values-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def session(source, expected, errors=()):
        result = subprocess.run([str(binary), 'repl'], cwd=root, env=env,
                                input=source + '\n:quit\n', text=True,
                                capture_output=True, timeout=60)
        assert result.returncode == 0, result
        assert result.stdout == expected, (source, result.stdout, result.stderr)
        assert len(result.stderr.splitlines()) == len(errors), result.stderr
        for error in errors:
            assert error in result.stderr, (error, result.stderr)

    changes = ''.join(f'record Later{i}\nvalue: String\nend\n'
                      f'enum Choice{i}\nOne\nend\n' for i in range(16))
    read = '''case kept
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.message)
end
case missing
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.index)
puts(error.size)
end
case invalid
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.input)
puts(error.message)
end
'''
    initial = ('Result::Ok(value: "日\\x00本") : Result<String, IndexLookupError>\n'
               'Result::Err(error: IndexLookupError(index: 3, size: 1, message: "String index is out of bounds")) : Result<String, IndexLookupError>\n'
               'Result::Err(error: NumberParseError(kind: NumberParseErrorKind::InvalidFormat, input: "bad", message: "invalid Float")) : Result<Float, NumberParseError>\n')
    output = '日\x00本\n3\n1\nbad\ninvalid Float\n'
    session('kept := ["日\\x00本"].try_fetch(-1)\n'
            'missing := "日".try_fetch(3)\ninvalid := "bad".try_to_f()\n'
            + changes + read + ':reload\n' + read,
            initial + output * 2 + 'reloaded\n' + output)

    session('import { Result as Outcome } from trb/std/result\n'
            'import { SliceRangeError as BadSlice } from trb/std/errors\n'
            'def 読む(values: Array<String>): Outcome<Array<String>, BadSlice>\n'
            'return values.try_slice(0..1)\nend\n'
            'saved := 読む(["日"])\n' + changes +
            'case saved\nwhen Outcome::Ok(value)\nputs(value.size())\n'
            'when Outcome::Err(error)\nputs(error.message)\nend',
            'Result::Err(error: SliceRangeError(start: 0, finish: 1, exclusive: false, size: 1, message: "Array slice range is out of bounds")) : Outcome<Array<String>, BadSlice>\n'
            'Array slice range is out of bounds\n')

    # A rejected submission leaves earlier values available and the next
    # Unicode declaration intact, including names with different UTF-8 bytes.
    session('Å := 1\nÅ := 2\nå := 3\n😀 := 9\n'
            'puts(Å + Å + å)\n"1e309".to_f()\nputs("2.5".to_f())',
            '1 : Integer\n2 : Integer\n3 : Integer\n6\n2.5\n',
            ('unsupported source character 😀', 'Float is outside the portable range'))

    # The REPL's convenience imports cannot replace an authored project root.
    (root / 'types.trb').write_text('record IndexLookupError\nvalue: Integer\nend\n')
    (root / 'trbconfig.jsonc').write_text(json.dumps({'name': 'builtin-project', 'sourceDir': '.'}))
    session('IndexLookupError.new(value: 7)', 'IndexLookupError(value: 7) : IndexLookupError\n')

print('PASS builtin Result retention, aliases, Unicode names, replay and failed submissions')
