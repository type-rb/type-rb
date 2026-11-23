#!/usr/bin/env python3
"""Exercise persistent cells shared by named functions and anonymous captures."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-repl-globals-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb',
                       TRBN_HISTORY=str(root / 'history'))

    def repl(source):
        result = subprocess.run([binary, 'repl'], input=source, text=True, cwd=root,
                                env=environment, capture_output=True, timeout=60)
        assert result.returncode == 0, result
        return result

    shared = repl('''mut count := 1
def bump(): Integer
count += 1
return count
end
saved := fn(): Integer; return count; end
puts(bump())
puts(saved())
count = 7
puts(bump())
puts(saved())
:reload
puts(bump())
puts(saved())
:quit
''')
    assert not shared.stderr, shared
    assert shared.stdout == (
        '1 : Integer [mut]\n#<fn> : () -> Integer\n2\n2\n'
        '7 : Integer [mut]\n8\n8\n2\n2\n8\n8\nreloaded\n9\n9\n'), shared

    shadowed = repl('''def read(): Integer
return 1
end
def before(): Integer
return read()
end
saved := read
read := fn(): Integer; return 2; end
def after(): Integer
return read()
end
puts(saved())
puts(before())
puts(read())
puts(after())
:reload
puts(saved())
puts(before())
puts(after())
:quit
''')
    assert not shadowed.stderr, shadowed
    assert shadowed.stdout == (
        '#<callable> : () -> Integer\n#<fn> : () -> Integer\n1\n1\n2\n2\n'
        '1\n1\n2\n2\nreloaded\n1\n1\n2\n'), shadowed

    once = repl('''def mark(): String
puts("initialize")
return "held" + " value"
end
mut text := mark()
def read(): String
return text
end
puts(read())
text = "replaced"
record Later
value: String
end
puts(read())
:reload
puts(read())
:quit
''')
    assert not once.stderr, once
    assert once.stdout.count('initialize\n') == 2, once
    assert once.stdout.endswith('reloaded\nreplaced\n'), once

    (root / 'loaded.trb').write_text('''mut value := 3
def read(): Integer
return value
end
''')
    loaded = repl('mut before := 1\n:load loaded.trb\nputs(read())\n:reload\nputs(read())\n:quit\n')
    assert loaded.stdout == '1 : Integer [mut]\n3\n3\nreloaded\n3\n', loaded
    assert not loaded.stderr, loaded

    (root / 'empty.trb').write_text('mut table := {}\ntable["x"] = 1\nputs(table["x"])\n')
    empty = repl(':load empty.trb\nputs(table["x"])\n:reload\nputs(table["x"])\n:quit\n')
    assert not empty.stderr, empty
    assert empty.stdout == '1\n1\n1\n1\nreloaded\n1\n', empty

    ordered = repl('''mut count := 0
def mark(): Integer
count += 1
puts(count)
return count
end
value := mark()
VALUE := mark()
puts(value)
puts(VALUE)
:reload
puts(count)
:quit
''')
    assert not ordered.stderr, ordered
    assert ordered.stdout == (
        '0 : Integer [mut]\n1\n1 : Integer\n2\n2 : Integer\n1\n2\n'
        '1\n2\n1\n2\nreloaded\n2\n'), ordered

    failed = repl('''mut value: Integer? := 1
def clear(): Integer
value = nil
return 1 / 0
end
value = 3
BAD := clear()
puts(value + 1)
puts(value == nil)
:quit
''')
    assert failed.stdout == '1 : Integer? [mut]\n3 : Integer? [mut]\ntrue\n', failed
    assert len(failed.stderr.splitlines()) == 2, failed
    assert 'zero' in failed.stderr and 'operator + does not support Integer? and Integer' in failed.stderr, failed

    (root / 'invalid.trb').write_text('''mut value := 3
def read(): Integer
return "invalid"
end
''')
    rejected = repl('puts("execute once")\n:load invalid.trb\nputs("retained")\n:quit\n')
    assert rejected.stdout == 'execute once\nretained\n', rejected
    assert rejected.stderr, rejected

print('Shared session globals, single initialization, load/replay order and failure facts passed')
