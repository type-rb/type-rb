#!/usr/bin/env python3
"""Keep concrete generic identities across constructors, imports and REPL replay."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
binary = parser.parse_args().binary.resolve()

with tempfile.TemporaryDirectory(prefix='native generic ') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def repl(source, expected, errors=()):
        result = subprocess.run([str(binary), 'repl'], input=source + '\n:quit\n',
                                text=True, capture_output=True, cwd=root, env=env, timeout=30)
        assert result.returncode == 0, result
        assert result.stdout == expected, (source, result.stdout, result.stderr)
        assert len(result.stderr.splitlines()) == len(errors), result.stderr
        for message in errors:
            assert message in result.stderr, (message, result.stderr)

    box = 'record Box<T>\nvalue: T\nend\n'
    item = 'enum Item<T>\nValue(value: T)\nOther(text: String)\nend\n'
    held = 'Box(value: "held") : Box<String>\n'
    repl(box + 'value := Box<String>.new(value: "held")\n'
         'number := Box<Integer>.new(value: 7)\nvalue\n:type value\n'
         'record Earlier\nfield: Integer\nend\nvalue\n:reload\nvalue',
         held + 'Box(value: 7) : Box<Integer>\n' + held + 'Box<String>\n' + held
         + 'reloaded\n' + held)
    enum_held = 'Item::Value(value: "held") : Item<String>\n'
    repl(item + 'value := Item<String>::Value("held")\n'
         'number := Item<Integer>::Value(7)\nvalue\n:type value\n'
         'enum Earlier\nOne\nend\nvalue\n:reload\nvalue\n'
         'case number\nwhen Item::Value(n)\nputs(n)\nwhen Item::Other(text)\nputs(text)\nend',
         enum_held + 'Item::Value(value: 7) : Item<Integer>\n' + enum_held
         + 'Item<String>\n' + enum_held + 'reloaded\n' + enum_held + '7\n')

    (root / 'box.trb').write_text(box)
    (root / 'item.trb').write_text(item)
    (root / 'entry.trb').write_text('record Entry\ntext: String\nend\n')
    nested = 'Box(value: Entry(text: "kept")) : Holder<Entry>\n'
    repl('import { Box as Holder } from box\nimport entry\n'
         'value := Holder<Entry>.new(value: Entry.new(text: "kept"))\n'
         'record Earlier\nvalue: String\nend\nvalue\n:type value\n:reload\nvalue\nvalue.value.text',
         nested + nested + 'Holder<Entry>\nreloaded\n' + nested + '"kept" : String\n')
    alias = 'Item::Value(value: Box(value: "kept")) : Choice<Holder<String>>\n'
    repl('import { Box as Holder } from box\nimport { Item as Choice } from item\n'
         'value := Choice<Holder<String>>::Value(Holder<String>.new(value: "kept"))\n'
         'record Earlier\nfield: Integer\nend\nvalue\n:type value\n:reload\nvalue',
         alias + alias + 'Choice<Holder<String>>\nreloaded\n' + alias)

    # A failed application cannot corrupt an already retained instance.
    repl(box + 'value := Box<Integer>.new(value: 3)\n'
         'bad: Box<String> := value\nvalue.value',
         'Box(value: 3) : Box<Integer>\n3 : Integer\n', ('expected Box<String>',))

    # Different modules declaring the same template name retain different identity.
    (root / 'other.trb').write_text(box)
    (root / 'main.trb').write_text(
        'import { Box as Left } from box\nimport { Box as Right } from other\n'
        'def main()\nleft := Left<Integer>.new(value: 1)\n'
        'right := Right<String>.new(value: "right")\nputs(left.value)\nputs(right.value)\nend\n')
    result = subprocess.run([str(binary), 'run', str(root / 'main.trb')],
                            capture_output=True, text=True, cwd=root, env=env, timeout=30)
    assert (result.returncode, result.stdout, result.stderr) == (0, '1\nright\n', ''), result
    (root / 'main.trb').write_text(
        'import { Box as Left } from box\nimport { Box as Right } from other\n'
        'def main()\nvalue: Left<Integer> := Right<Integer>.new(value: 1)\nend\n')
    result = subprocess.run([str(binary), 'check', str(root / 'main.trb')],
                            capture_output=True, text=True, cwd=root, env=env, timeout=30)
    assert result.returncode == 1 and 'TRBN4004' in result.stderr, result

    methods = """enum Choice<T>
Value(value: T)
Other(value: T)
def _get(fallback: T): T
case self
when Choice::Value(value)
return value
when Choice::Other(_value)
return fallback
end
end
def reader(fallback: T): () -> T
return fn(): T; return _get(fallback); end
end
def select(value: T, *, other: T = value): T
return other
end
end
"""
    (root / 'choice.trb').write_text(methods)
    retained = subprocess.run([str(binary), 'repl'], input="""import { Choice as Pick } from choice
read := Pick<String>::Value("held").reader("unused")
puts(read())
enum Before
Only
end
record Earlier
number: Integer
end
puts(Pick<Integer>::Other(0).select(7))
puts(read())
puts(Pick<Integer>::Other(0).select("bad"))
puts(read())
:reload
puts(read())
:quit
""", capture_output=True, text=True, cwd=root, env=env, timeout=30)
    assert retained.returncode == 0, retained
    assert retained.stdout == '#<fn> : () -> String\nheld\n7\nheld\nheld\nheld\n7\nheld\nheld\nreloaded\nheld\n', retained
    assert len(retained.stderr.splitlines()) == 1 and 'expected Integer, found String' in retained.stderr, retained

print('PASS generic nominal identity, retained arguments, aliases, replay and invariance')
