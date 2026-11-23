#!/usr/bin/env python3
"""Check retained REPL rendering for quoted Strings and compact Floats."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-repl-display-') as temporary:
    env = dict(os.environ, NO_COLOR='1', TERM='dumb',
               TRBN_HISTORY=str(Path(temporary) / 'history'))

    def evaluate(source):
        result = subprocess.run([str(binary), 'repl'], input=source, text=True,
                                capture_output=True, cwd=temporary, env=env, timeout=30)
        assert result.returncode == 0 and not result.stderr, (result.returncode, result.stderr)
        return result.stdout

    quoted = evaluate(r'["a\x00b", "\a\b\f\n\r\t\v", "\x1b", "\x7f", "😀"]' + '\n')
    assert quoted == (r'["a\x00b", "\a\b\f\n\r\t\v", "\x1b", "\x7f", "😀"]'
                      ' : Array<String>\n'), repr(quoted)

    malformed = evaluate(r'["\xff", "\xc2", "\xff\xfe"]' + '\n')
    assert malformed == '["\\xff", "\\xc2", "\\xff\\xfe"] : Array<String>\n', repr(malformed)

    floats = evaluate('''[0.0 / 0.0, 1.0 / 0.0, - 1.0 / 0.0, 0.0, - 0.0,
  1.2345678901234567, 1000000000000000.0, 0.000001]
[100000.0, 1000000.0, 0.0001, 0.00001, 123456.0, 1234567.0]
''')
    assert floats == ('[NaN, +Inf, -Inf, 0, -0, 1.2345678901234567, 1e+15, 1e-06] : Array<Float>\n'
                      '[100000, 1e+06, 0.0001, 1e-05, 123456, 1.234567e+06] : Array<Float>\n'), floats

print('PASS REPL display: quoted control characters and compact Float notation')
