#!/usr/bin/env python3
"""Keep checked union alternatives across retained REPL sessions and replay."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix='native union ') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def repl(source):
        result = subprocess.run([str(binary), 'repl'], input=source + '\n:quit\n',
                                text=True, capture_output=True, cwd=root, env=env, timeout=30)
        assert result.returncode == 0, result
        return result

    declarations = '''alias Choice = Integer | String
record Box<T>
value: T
end
alias Nominal = Box<Choice> | String
def text(value: Choice): String
case value
when Integer(number)
return number.to_s()
when String(word)
return word
end
end
'''
    source = declarations + '''mut captured: Choice := "held" + " value"
read := fn(): String; return text(captured); end
puts(read())
captured = 3
puts(read())
holder := Box<Choice>.new(value: "boxed" + " value")
nominal: Nominal := holder
values := [1, nil]
puts(values.size())
optional: Integer? := 1
mixed := [optional, "word"]
puts(mixed.size())
record Earlier
field: Boolean
end
puts(text(holder.value))
nominal
:reload
puts(read())
puts(text(holder.value))
puts(values.size())
puts(mixed.size())
nominal
'''
    retained = repl(source)
    assert not retained.stderr, retained
    assert retained.stdout.count('boxed value\n') == 3, retained.stdout
    assert retained.stdout.count('Box(value: "boxed value") : Box<Integer | String> | String\n') == 3, retained.stdout
    assert retained.stdout.endswith('3\nboxed value\n2\n2\nBox(value: "boxed value") : Box<Integer | String> | String\n'), retained.stdout
    assert 'reloaded\n' in retained.stdout, retained.stdout

    rejected = repl(declarations + '''mut value: Choice := "unchanged"
value = true
puts(text(value))
''')
    assert rejected.stdout.endswith('unchanged\n'), rejected
    assert len(rejected.stderr.splitlines()) == 1 and 'expected Integer | String' in rejected.stderr, rejected

    # Inferred Nil remains a flow type. Retained binding projections do not
    # authorize the user to spell it in a later input or a new declaration.
    internal = repl('''values := [1, nil]
bad: Array<Nil> := []
def invalid(value: Nil)
end
puts(values.size())
:reload
puts(values.size())
''')
    assert internal.stdout.endswith('reloaded\n2\n'), internal
    assert internal.stderr.count('Nil is an internal flow type') == 2, internal

print('PASS union payloads, captured assignment, inferred arrays, nominal remapping and replay')
