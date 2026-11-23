#!/usr/bin/env python3
"""Exercise namespace storage across retained submissions and explicit replay."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-namespace-bindings-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb',
                       TRBN_HISTORY=str(root / 'history'))

    def repl(source):
        result = subprocess.run([binary, 'repl'], input=source, text=True, cwd=root,
                                env=environment, capture_output=True, timeout=60)
        assert result.returncode == 0 and not result.stderr, result
        return result.stdout

    counter = '''module Counter
mut value := 1
def self.bump(): Integer
value += 1
return value
end
end
'''
    shared = repl(counter + '''puts(Counter.bump())
module Counter
def self.read(): Integer
return value
end
end
puts(Counter.bump())
:reload
puts(Counter.read())
:quit
''')
    assert shared == '2\n3\n2\n3\nreloaded\n3\n', shared

    once = repl('''def mark(): String
puts("initialize")
return "held" + " value"
end
module State
mut text := mark()
def self.read(): String
return text
end
def self.replace()
text = "new" + " value"
end
end
puts(State.read())
State.replace()
record Later
value: String
end
module State
def self.copy(): String
return text
end
end
puts(State.copy())
:reload
puts(State.read())
:quit
''')
    assert once == ('initialize\nheld value\nnew value\n'
                    'initialize\nheld value\nnew value\nreloaded\nnew value\n'), once

    (root / 'counter.trb').write_text(counter)
    loaded = repl(':load counter.trb\nputs(Counter.bump())\n:reload\n'
                  'puts(Counter.bump())\n:quit\n')
    assert loaded == '2\n2\nreloaded\n3\n', loaded

    outer = repl('''value := 1
module State
def self.before(): Integer
return value
end
value := 2
def self.after(): Integer
return value
end
end
puts(State.before())
puts(State.after())
:reload
puts(State.before())
puts(State.after())
:quit
''')
    assert outer == '1 : Integer\n1\n2\n1\n2\nreloaded\n1\n2\n', outer

print('Namespace shared storage, lexical identity, initialization and load/replay passed')
