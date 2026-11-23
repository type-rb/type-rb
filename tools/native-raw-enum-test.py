#!/usr/bin/env python3
"""Preserve raw enum conversions and bound methods across REPL submissions."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix='native raw enum ') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def repl(source):
        result = subprocess.run([str(binary), 'repl'], input=source + '\n:quit\n',
                                text=True, capture_output=True, cwd=root, env=env, timeout=30)
        assert result.returncode == 0, result
        return result

    declaration = '''enum Status
Ready = "ready"
Done = "done"
def reader(prefix: String): () -> String
return fn(): String; return prefix + self.raw_value(); end
end
end
'''
    (root / 'status.trb').write_text(declaration)
    retained = repl('''import { Status as State } from status
def show(value: Result<State, EnumValueError>)
case value
when Result::Ok(status)
puts(status.raw_value())
when Result::Err(error)
puts(error.message)
end
end
good := State.from_raw("ready")
bad := State.from_raw("missing" + " value")
read := State::Done.reader("held:")
show(good)
show(bad)
puts(read())
enum Earlier
Only
end
record Holder
number: Integer
end
show(good)
show(bad)
puts(read())
:reload
show(good)
show(bad)
puts(read())
''')
    assert not retained.stderr, retained
    assert retained.stdout.endswith('reloaded\nready\nunknown raw value for Status\nheld:done\n'), retained
    assert retained.stdout.count('held:done\n') == 5, retained

    shadowed = repl('''record EnumValueError
flag: Boolean
end
''' + declaration + '''value := EnumValueError.new(flag: true)
case Status.from_raw("unknown")
when Result::Ok(_status)
puts("unexpected")
when Result::Err(error)
puts(error.message)
end
puts(value.flag)
''')
    assert not shadowed.stderr, shadowed
    assert shadowed.stdout.endswith('unknown raw value for Status\ntrue\n'), shadowed

    rejected = repl(declaration + '''value := Status::Ready
enum Invalid
One = "same"
Two = "same"
end
puts(value.raw_value())
''')
    assert rejected.stdout.endswith('ready\n'), rejected
    assert len(rejected.stderr.splitlines()) == 1 and 'raw enum value duplicates One' in rejected.stderr, rejected

print('PASS raw enum imports, conversion retention, bound closures, replay and error identity')
