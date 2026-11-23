#!/usr/bin/env python3
"""Exercise literal Symbol bytes, parser boundaries and retained REPL values."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-symbols-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, code=0):
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=env, capture_output=True, timeout=60)
        assert result.returncode == code, (command, result.stdout, result.stderr)
        return result

    body = (repository / 'compiler/conformance/valid/symbol-managed-mir.trb').read_text()
    body = body.replace(':before', ':"日本\\x00😀"')
    source = root / 'main.trb'
    source.write_text(body)
    emitted = run([binary, '--internal-driver', 'emit-qbe', source])
    marker = b'\t%result =l call $trbn_gc_alloc(l $trbn_desc_string, l %size, l 0)'
    assert emitted.stdout.count(marker) == 1
    (root / 'program.ssa').write_bytes(emitted.stdout.replace(marker, b'\tcall $trbn_gc_collect(w 0)\n' + marker))
    run([binary.with_name('qbe'), '-o', root / 'program.s', root / 'program.ssa'])
    run(['/usr/bin/cc', root / 'program.s', '-lm', '-o', root / 'program'])
    result = run([root / 'program'])
    expected = '[日本\0😀/after/#{literal}]\nchanged\n日本\0😀/after/#{literal}\n[日本\0😀/after/#{literal}]\n'
    assert result.stdout == expected.encode() and result.stderr == b'', result

    # Literal bytes are not interpreted as interpolation, even in the REPL.
    retained = run([binary, 'repl'], data=r'''mut labels := {:ready => :"#{missing}"}
saved := labels[:ready]
def later(): String
return :later
end
labels[:ready] = later()
puts(saved)
puts(labels[:ready])
puts(:"#{@}")
puts(:"\q")
puts("#{:normal}")
(:ready)
:quit
'''.encode())
    assert b'#{missing}\nlater\n#{@}\nnormal\n' in retained.stdout, retained
    assert retained.stdout.endswith(b'"ready" : String\n'), retained
    assert len(retained.stderr.splitlines()) == 1 and b'unsupported String escape' in retained.stderr, retained

    # Expanding an earlier String must preserve the line of a later failure.
    source.write_text('''def main()
value := "#{:first}"
puts(value)
puts("#{:second}")
puts(:name + 1)
end
''')
    failed = run([binary, 'check', source], code=1)
    assert b':5:12: error[TRBN4004]: operator + does not support String and Integer' in failed.stderr, failed
    assert len(failed.stderr.splitlines()) == 1, failed

    # The ordinary parser owns colon disambiguation for submission framing too.
    keywords = run([binary, 'repl'], data=b'''def names(): String
return :end + :fn
end
puts(names())
def apply(*, callback: () -> String): String
return callback()
end
puts(apply(callback: fn(): String
return :if
end))
puts(:return)
:quit
''')
    assert keywords.stdout == b'endfn\nif\nreturn\n' and not keywords.stderr, keywords

    malformed = run([binary, 'repl'], data=r'''def invalid()
text := "#{"ok"}\q"
puts(text)
end
puts(:end)
:quit
'''.encode())
    assert malformed.stdout == b'end\n', malformed
    assert len(malformed.stderr.splitlines()) == 1, malformed
    assert b'unsupported String escape' in malformed.stderr, malformed

    suffixed = run([binary, 'repl'], data=r'''def label!(): String
return "日\x00本"
end
read: () -> String := label!
def label?(): String
return "other"
end
puts(read())
__trb_saved := 7
def invalid(alias: Integer): Integer
return alias
end
puts(label?())
def operators!(): String
values := {alias: :>>, newtype: :**}
return values[:alias] + values[:newtype]
end
puts(operators!())
:type read
:reload
puts(read())
:quit
'''.encode())
    assert suffixed.stdout == ('#<callable> : () -> String\n日\x00本\nother\n'
                               '>>**\n() -> String\n日\x00本\nother\n>>**\nreloaded\n日\x00本\n').encode(), suffixed
    assert len(suffixed.stderr.splitlines()) == 2, suffixed
    assert b'compiler-reserved __trb prefix' in suffixed.stderr, suffixed
    assert b'reserved keyword cannot be an identifier' in suffixed.stderr, suffixed

print('Symbol bytes, callable suffixes, reserved names, GC, origins and retained REPL checks passed')
