#!/usr/bin/env python3
"""Check ordinary constant roots, retained REPL state and explicit replay."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()

with tempfile.TemporaryDirectory(prefix='native-constants-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, data=None):
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=env, capture_output=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    # More managed globals than the runtime's fixed external-root slots.
    source = 'def make(value: Integer): String\nreturn "kept-" + value.to_s()\nend\n'
    source += ''.join(f'VALUE_{i} := make({i})\n' for i in range(20))
    source += 'def main()\n' + ''.join(f'puts(VALUE_{i})\n' for i in range(20)) + 'end\n'
    (root / 'main.trb').write_text(source)
    emitted = run([binary, '--internal-driver', 'emit-qbe', root / 'main.trb'])
    lines = emitted.stdout.decode().splitlines(keepends=True)
    forced = []
    caller = False
    hooks = 0
    for line in lines:
        if line.startswith(('function ', 'export function ')):
            caller = '$trbn_initialize_globals(' in line or '$main(' in line
        if caller and 'call $trbnf' in line:
            forced.append('\tcall $trbn_gc_collect(w 0)\n')
            hooks += 1
        forced.append(line)
    assert hooks == 21, hooks
    (root / 'program.ssa').write_text(''.join(forced))
    run([binary.with_name('qbe'), '-o', root / 'program.s', root / 'program.ssa'])
    run(['/usr/bin/cc', root / 'program.s', '-lm', '-o', root / 'program'])
    executed = run([root / 'program'])
    assert executed.stdout == ''.join(f'kept-{i}\n' for i in range(20)).encode(), executed
    assert not executed.stderr, executed
    (root / 'main.trb').unlink()

    retained = run([binary, 'repl'], b'''def mark(): String
puts("initialize")
return "saved" + " value"
end
TEXT := mark()
CALL := fn(): String; return TEXT; end
saved := CALL
:type TEXT
record Later
number: Integer
end
puts(saved())
puts(CALL())
:reload
puts(saved())
puts(CALL())
:quit
''')
    assert not retained.stderr, retained
    assert retained.stdout.count(b'initialize\n') == 2, retained
    assert retained.stdout.count(b'saved value\n') == 6, retained
    assert b'reloaded\n' in retained.stdout, retained

    recovery = run([binary, 'repl'], b'''TEXT := "saved" + " value"
FAILED := 1 / 0
GOOD := 7
puts(TEXT)
puts(GOOD)
:quit
''')
    assert recovery.stdout.endswith(b'saved value\n7\n'), recovery
    assert len(recovery.stderr.splitlines()) == 1 and b'zero' in recovery.stderr, recovery

    # Replaying a declaration-only session must initialize imported values too.
    (root / 'trbconfig.jsonc').write_text('{"name":"constants","sourceDir":"src"}')
    (root / 'src').mkdir()
    (root / 'src/values.trb').write_text('''def mark(): Integer
puts("dependency")
return 7
end
COUNT := mark()
''')
    (root / 'src/consumer.trb').write_text('''import { COUNT } from values
def mark(value: Integer): Integer
puts("consumer")
return value + 1
end
TOTAL := mark(COUNT)
def read(): Integer
return TOTAL
end
''')
    replayed = run([binary, 'repl'], b':reload\nputs(read())\nputs(read())\n:quit\n')
    assert not replayed.stderr, replayed
    assert replayed.stdout == b'dependency\nconsumer\ndependency\nconsumer\nreloaded\n8\n8\n', replayed

    # Configured projects type independent roots too; file-root loading keeps
    # its explicit import closure and must not initialize an unrelated sibling.
    project = root / 'independent'
    (project / 'src').mkdir(parents=True)
    config = project / 'trbconfig.jsonc'
    config.write_text('{"name":"independent","sourceDir":"src"}')
    entry = project / 'src/main.trb'
    entry.write_text('def main()\nputs("entry")\nend\n')
    (project / 'src/values.trb').write_text('''def mark(): String
puts("independent")
return "kept" + " value"
end
VALUE := mark()
''')
    assert run([binary, 'check', '--config', config]).stdout == b'ok\n'
    assert run([binary, 'run', '--config', config]).stdout == b'entry\n'
    emitted = run([binary, '--internal-driver', 'emit-qbe', entry])
    (root / 'file.ssa').write_bytes(emitted.stdout)
    run([binary.with_name('qbe'), '-o', root / 'file.s', root / 'file.ssa'])
    run(['/usr/bin/cc', root / 'file.s', '-lm', '-o', root / 'file'])
    assert run([root / 'file']).stdout == b'entry\n'

    # Source imports retain their authored DFS order across directory boundaries.
    files = {
        'shared.trb': 'def mark(value: String): Integer\nputs(value)\nreturn 1\nend\nBASE := mark("shared")\n',
        'a/z.trb': 'import { mark, BASE } from shared\nZ := mark("z") + BASE\n',
        'b/y.trb': 'import { mark } from shared\nimport { Z } from a/z\nY := mark("y") + Z\n',
        'a/x.trb': 'import { mark } from shared\nimport { Y } from b/y\nX := mark("x") + Y\n',
        'main.trb': 'import { X } from a/x\nimport { Z } from a/z\ndef main()\nputs(X)\nputs(Z)\nend\n',
    }
    for name, source in files.items():
        path = project / 'src' / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
    assert run([binary, 'run', '--config', config]).stdout == b'shared\nz\ny\nx\n4\n2\n'

    # Suppressing execution must not suppress static diagnostics in unused files.
    (project / 'src/invalid.trb').write_text('INVALID: Integer := "wrong"\n')
    invalid = subprocess.run([binary, 'check', '--config', config], cwd=root,
                             env=env, capture_output=True, timeout=60)
    assert invalid.returncode != 0 and b'Integer' in invalid.stderr, invalid

print('PASS Native constant runtime roots, import order, GC roots, retention and replay')
