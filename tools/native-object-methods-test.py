#!/usr/bin/env python3
"""Exercise concrete object methods through Native code and retained REPL values."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()
repository = Path(__file__).resolve().parent.parent

with tempfile.TemporaryDirectory(prefix='native-object-methods-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb',
                       TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, stats=False):
        env = dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1') if stats else environment
        result = subprocess.run(list(map(str, command)), input=data, cwd=root,
                                env=env, capture_output=True, text=True, timeout=60)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    for name in (
        'object-class-array-field',
        'object-class-branch-initialization',
        'object-class-captured-value',
        'object-class-constant',
        'object-class-constructor-named-default',
        'object-class-field-assignment',
        'object-class-field-default',
        'object-class-float-boolean-fields',
        'object-class-generic-field',
        'object-class-generic-instance-method',
        'object-class-generic-interface',
        'object-class-inherited-interface',
        'object-class-inherited-method',
        'object-class-interface-array',
        'object-class-interface-dispatch',
        'object-class-interface-two-implementations',
        'object-class-in-hash',
        'object-class-in-record',
        'object-class-initializer',
        'object-class-managed-fields',
        'object-class-mutable-fields',
        'object-class-named-constructor',
        'object-class-named-method-default',
        'object-class-nullable',
        'object-class-private-method',
        'object-class-readonly-initialization',
        'object-class-recursive-field',
        'object-class-reference-alias',
        'object-class-self-method-call',
        'object-class-self-return',
        'object-class-static-method',
        'nominal-record-default-scope',
        'nominal-record-class-shadow',
        'nominal-class-inherited-capture',
        'nominal-interface-separate',
        'nominal-interface-result-scope',
        'nominal-interface-callable',
        'class-union-readonly-discriminants',
        'class-union-overlap-else',
        'class-union-captured-discriminants',
        'class-union-collections',
        'class-union-nullable-container',
        'class-union-result-payload',
        'class-union-generic-field',
        'class-union-common-managed-field',
        'class-union-common-numeric-field',
        'class-union-common-callable',
    ):
        fixture = repository / 'compiler/conformance/valid' / name
        source = root / 'main.trb'
        source.write_text(fixture.with_suffix('.trb').read_text())
        checked = run([binary, '--internal-driver', 'check', source])
        assert not checked.stderr, checked
        emitted = run([binary, '--internal-driver', 'emit-qbe', source])
        assert not emitted.stderr, emitted
        forced, ordinary, hooks = [], False, 0
        for line in emitted.stdout.splitlines():
            if line.startswith('function '):
                ordinary = '$trbnf' in line
            if line == '}':
                ordinary = False
            # A literal-return static call can fold away while its allocating
            # Integer-to-String conversion remains a collection boundary.
            if ordinary and any(marker in line for marker in (
                    'call $trbn_gc_alloc(', 'call $trbnf', 'call $trbn_hash_',
                    'call $trbn_integer_to_string(')):
                forced.append('\tcall $trbn_gc_collect(w 0)')
                hooks += 1
            forced.append(line)
        assert hooks > 0, name
        il, assembly, program = [root / item for item in ('program.ssa', 'program.s', 'program')]
        il.write_text('\n'.join(forced) + '\n')
        run([binary.with_name('qbe'), '-o', assembly, il])
        run(['/usr/bin/cc', assembly, '-lm', '-o', program])
        result = run([program], stats=True)
        assert result.stdout == fixture.with_suffix('.out').read_text(), result
        statistics = {}
        for line in result.stderr.splitlines():
            prefix, key, value = line.split(',')
            assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
            statistics[key] = int(value)
        assert statistics['live-bytes'] == 0, statistics
        assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics

    retained = run([binary, 'repl'], data='''interface Source<T>
def get(): T
end
class Box<T>
def identity<U>(value: U): U
return value
end
def value(input: T): T
return input
end
def read(): Integer
return 42
end
end
mut saved := Box<Integer>.new()
read := fn(): Integer; return saved.read(); end
puts(saved.identity<String>("held"))
class Earlier
end
:type saved
puts(saved.value(7))
puts(read())
:reload
puts(saved.value(9))
puts(read())
:quit
''')
    assert not retained.stderr, retained
    assert retained.stdout == (
        '#<Box > : Box<Integer> [mut]\n#<fn> : () -> Integer\n'
        'held\nBox<Integer>\n7\n42\nheld\n7\n42\nreloaded\n9\n42\n'), retained

    initialized = run([binary, 'repl'], data='''class Answer
def initialize(*, marker: Integer = 7)
puts(marker)
end
def self.make(): Answer
return Answer.new(marker: 9)
end
def value(*, extra: Integer = 2): Integer
return 40 + extra
end
end
item := Answer.new()
class Later
end
puts(item.value())
puts(Answer.make().value(extra: 3))
:quit
''')
    assert not initialized.stderr, initialized
    assert initialized.stdout == '7\n#<Answer > : Answer\n42\n9\n43\n', initialized

    fields = run([binary, 'repl'], data='''class Box<T>
readonly @label: String := "kept"
@value: T
def initialize(value: T)
@value = value
end
def replace(value: T)
@value = value
end
end
mut saved := Box<String>.new("a" + "b")
read := fn(): String; return saved.value; end
saved.replace("c" + "d")
class Earlier
end
puts(saved.value)
puts(read())
saved.value = "end"
puts(read())
:reload
puts(saved.value)
puts(read())
:quit
''')
    assert not fields.stderr, fields
    assert fields.stdout == (
        '#<Box label: "kept", value: "ab"> : Box<String> [mut]\n'
        '#<fn> : () -> String\ncd\ncd\n"end" : String\nend\n'
        'cd\ncd\nend\nreloaded\nend\nend\n'), fields

    defaults = run([binary, 'repl'], data='''class Defaults
@first: String := "a" + "b"
readonly @second: String := @first + "c"
end
item := Defaults.new()
puts(item.second)
class Later
end
puts(item.second)
:quit
''')
    assert not defaults.stderr, defaults
    assert defaults.stdout == '#<Defaults first: "ab", second: "abc"> : Defaults\nabc\nabc\n', defaults

    interfaces = run([binary, 'repl'], data='''interface Named
name(): String
end
class Label implements Named
@text: String
def initialize(text: String)
@text = text
end
def name(): String
return @text
end
def replace(text: String)
@text = text
end
end
class Fixed implements Named
def name(): String
return "fixed"
end
end
mut source := Label.new("a" + "b")
value: Named := source
read := fn(): String; return value.name(); end
source.replace("changed")
class Later
end
puts(value.name())
puts(read())
items: Array<Named> := [source, Fixed.new()]
items.each { |item| puts(item.name()) }
:reload
puts(value.name())
puts(read())
:quit
''')
    assert not interfaces.stderr, interfaces
    assert interfaces.stdout == (
        '#<Label text: "ab"> : Label [mut]\n'
        '#<Label text: "ab"> : Named\n#<fn> : () -> String\n'
        'changed\nchanged\n'
        '[#<Label text: "changed">, #<Fixed >] : Array<Named>\n'
        'changed\nfixed\nchanged\nchanged\nchanged\nfixed\n'
        'reloaded\nchanged\nchanged\n'), interfaces

    namespaced = run([binary, 'repl'], data='''module First
record Entry
value: String
end
interface Source
read(): Entry
end
class Holder implements Source
@entry: Entry
def initialize(text: String)
@entry = Entry.new(value: text)
end
def read(): Entry
return @entry
end
end
end
source: First::Source := First::Holder.new("a" + "b")
read := fn(): String; return source.read().value; end
module Second
record Entry
value: Integer
end
interface Source
read(): Entry
end
end
second := Second::Entry.new(value: 7)
:type source
:type second
puts(read())
puts(second.value)
:reload
:type source
:type second
puts(read())
puts(second.value)
:quit
''')
    assert not namespaced.stderr, namespaced
    assert namespaced.stdout == (
        '#<First::Holder entry: First::Entry(value: "ab")> : First::Source\n'
        '#<fn> : () -> String\nSecond::Entry(value: 7) : Second::Entry\n'
        'First::Source\nSecond::Entry\nab\n7\nab\n7\nreloaded\n'
        'First::Source\nSecond::Entry\nab\n7\n'), namespaced

    union_values = run([binary, 'repl'], data='''class Loaded
readonly @kind: "loaded" := "loaded"
@value: String
def initialize(value: String)
@value = value + "!"
end
end
class Missing
readonly @kind: "missing" := "missing"
@code: Integer := 404
end
alias Response = Loaded | Missing
def read(value: Response): String
return case value.kind
when "loaded"
value.value
when "missing"
value.code.to_s()
end
end
def retain(value: Response): () -> String
return fn(): String
return read(value)
end
end
mut saved: Response := Loaded.new("held")
reader := fn(): String; return read(saved); end
class Later
end
puts(reader())
saved = Missing.new()
puts(reader())
:reload
puts(reader())
:quit
''')
    assert not union_values.stderr, union_values
    assert union_values.stdout == (
        '#<Loaded kind: "loaded", value: "held!"> : Loaded | Missing [mut]\n'
        '#<fn> : () -> String\n'
        'held!\n'
        '#<Missing code: 404, kind: "missing"> : Loaded | Missing [mut]\n'
        '404\n'
        'held!\n'
        '404\n'
        'reloaded\n'
        '404\n'
    ), union_values

    (root / 'src').mkdir()
    (root / 'trbconfig.jsonc').write_text('{"name":"objects","sourceDir":"src"}')
    (root / 'src/helper.trb').write_text(
        'class Helper\ndef value(): Integer\nreturn 17\nend\nend\n')
    imported = run([binary, 'repl'], data='value := Helper.new()\nputs(value.value())\n:reload\nputs(value.value())\n:quit\n')
    assert not imported.stderr, imported
    assert imported.stdout == '#<Helper > : Helper\n17\n17\nreloaded\n17\n', imported

print('Native object methods, fields, initialization, inheritance, interface dispatch, forced collection, retained identities and replay passed')
