#!/usr/bin/env python3
"""Verify pinned URL semantics through ordinary compilation and retained REPL."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
reference = args.reference.resolve() if args.reference else None
source = '''import { Result } from trb/std/result
import trb/std/url

def decoded(value: String)
case URL.decode_component(value)
when Result::Ok(text)
puts("ok:" + text + ":" + text.size().to_s())
when Result::Err(error)
case error.kind
when URL::DecodeErrorKind::InvalidEscape
puts("escape:" + error.input + ":" + error.message)
when URL::DecodeErrorKind::InvalidUtf8
puts("utf8:" + error.input + ":" + error.message)
end
end
end

def query(value: String)
case URL.parse_query(value)
when Result::Ok(parameters)
parameters.each do |parameter|
puts(parameter.name + ":" + parameter.value)
end
when Result::Err(error)
puts(error.input + ":" + error.message)
end
end

def probe()
puts(URL.encode_component("a b/😀+~"))
puts(URL.encode_component("AZaz09-._~!*'()\\x00\\xff"))
decoded("")
decoded("a%20b%2f%f0%9f%98%80%2B~")
decoded("a+b")
decoded("日本語%00")
decoded("%C2%A2")
decoded("%E0%A0%80")
decoded("%ED%9F%BF")
decoded("%F0%90%80%80")
decoded("%F4%8F%BF%BF")
decoded("%")
decoded("%0")
decoded("%GG")
decoded("%FF%")
decoded("%FF")
decoded("%C0%AF")
decoded("%E1%80")
decoded("%ED%A0%80")
decoded("%F4%90%80%80")
decoded("\\xff")
puts(URL.build_query([
URL::QueryParameter.new(name: "tag", value: "type rb"),
URL::QueryParameter.new(name: "tag", value: "go"),
URL::QueryParameter.new(name: "symbol", value: "+&="),
URL::QueryParameter.new(name: "tilde", value: "~"),
URL::QueryParameter.new(name: "star", value: "*"),
]))
query("tag=go&&tag=type+rb&empty&symbol=%2B&text=%E6%97%A5%E6%9C%AC%E8%AA%9E&")
query("name=%")
query("%FF=value")
query("plus+name=plus+value=tail")
query("")
query("&&")
puts([OCTET_LITERALS].map { |value| URL.encode_component(value) }.join("|"))
[PERCENT_LITERALS].each do |value|
case URL.decode_component(value)
when Result::Ok(text)
puts(text.to_bytes().at(0))
when Result::Err(error)
puts(error.kind == URL::DecodeErrorKind::InvalidUtf8)
end
end
end

def main()
probe()
end
'''
source = source.replace('OCTET_LITERALS', ', '.join(f'"\\x{value:02x}"' for value in range(256)))
source = source.replace('PERCENT_LITERALS', ', '.join(f'"%{value:02X}"' for value in range(256)))

expected = ('a%20b%2F%F0%9F%98%80%2B~\nAZaz09-._~%21%2A%27%28%29%00%FF\n'
            'ok::0\nok:a b/😀+~:7\nok:a+b:3\nok:日本語\0:4\n'
            'ok:¢:1\nok:ࠀ:1\nok:퟿:1\nok:𐀀:1\nok:\U0010ffff:1\n')
for value in ('%', '%0', '%GG', '%FF%'):
    expected += 'escape:' + value + ':invalid percent escape in URL component\n'
for value in ('%FF', '%C0%AF', '%E1%80', '%ED%A0%80', '%F4%90%80%80'):
    expected += 'utf8:' + value + ':decoded URL component is not valid UTF-8\n'
expected += ('ok:�:1\ntag=type+rb&tag=go&symbol=%2B%26%3D&tilde=%7E&star=*\n'
             'tag:go\ntag:type rb\nempty:\nsymbol:+\ntext:日本語\n'
             '%:invalid percent escape in URL query component\n'
             '%FF:decoded URL query component is not valid UTF-8\n'
             'plus name:plus value=tail\n')

unreserved = b'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~'
expected += '|'.join(chr(value) if value in unreserved else f'%{value:02X}' for value in range(256)) + '\n'
expected += ''.join(str(value) + '\n' if value < 128 else 'true\n' for value in range(256))

with tempfile.TemporaryDirectory(prefix='native-url-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
    env.pop('TYPE_RB_NATIVE_RUNTIME_TRACE', None)
    env.pop('TYPE_RB_NATIVE_RUNTIME_STATS', None)

    def run(command, *, data=None, success=True, environment=env):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=120)
        assert (result.returncode == 0) == success, (command, result.stdout, result.stderr)
        return result

    file_root = root / 'file root'
    file_root.mkdir()
    main = file_root / 'main.trb'
    main.write_text(source)
    shadow = file_root / 'trb/std/url/index.trb'
    shadow.parent.mkdir(parents=True)
    shadow.write_text('module URL\ndef invalid()\nunknown()\nend\nend\n')
    assert run([binary, 'check', main]).stdout == 'ok\n'
    program = root / 'program'
    run([binary, 'build', '--compile', main, '--outfile', program])
    result = run([program], environment=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
    assert result.stdout == expected, result
    statistics = {}
    for line in result.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics
    assert run([binary, 'run', main]).stdout == expected

    # Force collection at every percent-byte normalization, retaining alias roots.
    emitted = run([binary, '--internal-driver', 'emit-qbe', main]).stdout
    match = re.search(r'^function l \$trbn_bytes_to_string\([^\n]*\) \{.*?^\}', emitted, re.M | re.S)
    assert match
    hook = ' %result =l call $trbn_string_alloc(l %size)'
    assert match[0].count(hook) == 1
    forced = emitted.replace(match[0], match[0].replace(hook, ' call $trbn_gc_collect(w 0)\n' + hook))
    ssa, assembly, forced_program = root / 'forced.ssa', root / 'forced.s', root / 'forced'
    ssa.write_text(forced)
    run([binary.with_name('qbe'), '-o', assembly, ssa])
    run(['/usr/bin/cc', assembly, '-lm', '-o', forced_program])
    assert run([forced_program]).stdout == expected

    project = root / 'project'
    (project / 'src').mkdir(parents=True)
    config = project / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'url-import', 'mode': 'trb', 'sourceDir': 'src'}))
    (project / 'src/main.trb').write_text(source)
    assert run([binary, 'check', '--config', config]).stdout == 'ok\n'
    run([binary, 'build', '--compile', '--config', config, '--outfile', program])
    assert run([program]).stdout == expected

    declarations = source.split('def main()')[0]
    retained = run([binary, 'repl'], data=declarations + 'probe()\nclass Later\nend\nprobe()\n:quit\n')
    assert not retained.stderr and retained.stdout == expected * 2, retained

    # Alias and bare importers must share query/error nominal identities.
    (file_root / 'helper.trb').write_text('import trb/std/url as OtherURL\n'
        'def identity(value: OtherURL::QueryParameter): OtherURL::QueryParameter\nreturn value\nend\n')
    main.write_text('import trb/std/url\nimport { identity } from helper\n'
        'def main()\nputs(URL.build_query([identity(URL::QueryParameter.new(name: "n", value: "v"))]))\nend\n')
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == 'n=v\n'

    # The pinned package exports its root; functions and helpers are members.
    main.write_text('import { URL as Components } from trb/std/url\n'
                    'def main()\nputs(Components.encode_component("a b"))\nend\n')
    assert run([binary, 'run', main]).stdout == 'a%20b\n'
    for declaration in ('encode_component', 'decode_component', 'parse_query', 'build_query',
                        '_decode_query_component', '_encode_query_component', '_parse_query_parameter',
                        'DecodeError', 'DecodeErrorKind', 'QueryParameter'):
        main.write_text('import { ' + declaration + ' as Imported } from trb/std/url\n'
                        'def main()\nreturn\nend\n')
        assert 'URL root declaration' in run([binary, 'check', main], success=False).stderr
        if reference:
            run([reference, 'check', main], success=False)
    (file_root / 'helper.trb').write_text('module URL\nend\n'
        'def encode_component(value: String): String\nreturn value\nend\n')
    main.write_text('import { encode_component } from helper\n'
                    'def main()\nputs(encode_component("authored"))\nend\n')
    assert run([binary, 'run', main]).stdout == 'authored\n'

    for imports in ('import trb/internal/url', 'import trb/internal/./url as hidden',
                    'import { encode_component } from trb/internal/url'):
        main.write_text(imports + '\ndef main()\nreturn\nend\n')
        assert 'internal package imports' in run([binary, 'check', main], success=False).stderr
    # The owned adapter is already cached when the later authored helper imports it.
    (file_root / 'helper.trb').write_text('import trb/internal/url as hidden\ndef identity()\nreturn\nend\n')
    main.write_text('import trb/std/url\nimport { identity } from helper\n'
        'def main()\nputs(URL.encode_component("value"))\nidentity()\nend\n')
    assert 'internal package imports' in run([binary, 'check', main], success=False).stderr
    rejected = run([binary, 'repl'], data='import trb/internal/url\n:quit\n')
    assert rejected.stderr, rejected
    main.write_text('module URL\ndef self.encode_component(value: String): String\nreturn value\nend\nend\n'
                    'def main()\nputs(URL.encode_component("authored"))\nend\n')
    assert run([binary, 'run', main]).stdout == 'authored\n'

    if reference:
        main.write_text(source)
        run([reference, 'check', main])
        run([reference, 'build', '--compile', '--outfile', program, main])
        assert run([program]).stdout == expected
        observed = run([reference, 'repl'], data=declarations + 'probe()\nprobe()\n:quit\n')
        assert not observed.stderr and observed.stdout == expected * 2, observed

print('PASS pinned URL components/query, errors, canonical identity, file/project/run/REPL and GC lifetime')
